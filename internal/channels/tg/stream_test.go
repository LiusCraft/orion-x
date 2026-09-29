package tg

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path"
	"strconv"
	"strings"
	"sync"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// fakeTelegram 是一个最小可用的 Telegram Bot API 服务端：记录收到的调用并按方法
// 返回假响应；failures > 0 时接下来的调用返回 500。
type fakeTelegram struct {
	mu       sync.Mutex
	calls    []apiCall
	failures int
	nextID   int
}

// apiCall 是一次 Bot API 调用里测试关心的参数。
type apiCall struct {
	method    string
	messageID string
	draftID   string
	text      string
}

func (f *fakeTelegram) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	method := path.Base(r.URL.Path)

	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, apiCall{
		method:    method,
		messageID: r.FormValue("message_id"),
		draftID:   r.FormValue("draft_id"),
		text:      r.FormValue("text"),
	})
	if f.failures > 0 {
		f.failures--
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":500,"description":"internal error"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	switch method {
	case "getMe":
		_, _ = w.Write([]byte(`{"ok":true,"result":{"id":42,"is_bot":true,"first_name":"fake","username":"fake_bot"}}`))
	case "sendMessageDraft":
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	default:
		msgID, _ := strconv.Atoi(r.FormValue("message_id"))
		if msgID == 0 {
			f.nextID++
			msgID = 100 + f.nextID
		}
		chatID, _ := strconv.ParseInt(r.FormValue("chat_id"), 10, 64)
		body, _ := json.Marshal(map[string]any{
			"ok": true,
			"result": map[string]any{
				"message_id": msgID,
				"chat":       map[string]any{"id": chatID, "type": "private"},
				"date":       0,
				"text":       r.FormValue("text"),
			},
		})
		_, _ = w.Write(body)
	}
}

// messageCalls 返回除 getMe 之外的调用（getMe 是构造 bot 时的握手）。
func (f *fakeTelegram) messageCalls() []apiCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]apiCall, 0, len(f.calls))
	for _, c := range f.calls {
		if c.method != "getMe" {
			out = append(out, c)
		}
	}
	return out
}

func newFakeBot(t *testing.T, fake *fakeTelegram) *tgbotapi.BotAPI {
	t.Helper()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	bot, err := tgbotapi.NewBotAPIWithAPIEndpoint("42:TEST", server.URL+"/bot%s/%s")
	if err != nil {
		t.Fatalf("init bot against fake server: %v", err)
	}
	return bot
}

// newEditFixture 起一个假服务端，返回指向群聊（原地编辑路径）的流。
func newEditFixture(t *testing.T) (*fakeTelegram, *editReply) {
	t.Helper()
	fake := &fakeTelegram{}
	return fake, newEditReply(newFakeBot(t, fake), "dev-1", &tgbotapi.Chat{ID: 100, Type: "group"})
}

// newDraftFixture 起一个假服务端，返回指向私聊（草稿路径）的流。
func newDraftFixture(t *testing.T) (*fakeTelegram, *draftReply) {
	t.Helper()
	fake := &fakeTelegram{}
	return fake, newDraftReply(newFakeBot(t, fake), "dev-1", &tgbotapi.Chat{ID: 100, Type: "private"})
}

// 私聊用草稿预览：首块立即出草稿，节流窗口内的增量不刷，收尾用 sendMessage 落地全文。
func TestDraftReplyStreamsDraftThenSendsFinalMessage(t *testing.T) {
	fake, stream := newDraftFixture(t)

	stream.push("你")
	stream.push("好")
	stream.finish()

	calls := fake.messageCalls()
	if len(calls) != 2 {
		t.Fatalf("calls = %+v, want draft + final sendMessage", calls)
	}
	if calls[0].method != "sendMessageDraft" || calls[0].text != "你" {
		t.Errorf("first call = %+v, want sendMessageDraft with 你", calls[0])
	}
	if calls[1].method != "sendMessage" || calls[1].text != "你好" {
		t.Errorf("second call = %+v, want sendMessage with 你好", calls[1])
	}
	if id, _ := strconv.ParseInt(calls[0].draftID, 10, 64); id <= 0 {
		t.Errorf("draft_id = %q, want a non-zero identifier", calls[0].draftID)
	}
}

// 节流放开时每个增量都刷新草稿，同一轮共用同一个 draft_id；最终消息只发一次。
func TestDraftReplyUpdatesDraftWhileGenerating(t *testing.T) {
	fake, stream := newDraftFixture(t)
	stream.throttle = 0

	stream.push("你")
	stream.push("好")
	stream.finish()

	calls := fake.messageCalls()
	if len(calls) != 3 {
		t.Fatalf("calls = %+v, want two drafts + one sendMessage", calls)
	}
	if calls[0].method != "sendMessageDraft" || calls[0].text != "你" {
		t.Errorf("first call = %+v, want sendMessageDraft with 你", calls[0])
	}
	if calls[1].method != "sendMessageDraft" || calls[1].text != "你好" {
		t.Errorf("second call = %+v, want sendMessageDraft with 你好", calls[1])
	}
	if calls[0].draftID == "" || calls[0].draftID != calls[1].draftID {
		t.Errorf("draft ids = %q / %q, want the same non-empty id", calls[0].draftID, calls[1].draftID)
	}
	if calls[2].method != "sendMessage" || calls[2].text != "你好" {
		t.Errorf("third call = %+v, want sendMessage with 你好", calls[2])
	}
}

// 草稿只展示头部（上限 4096 码元），超出后不再刷新；收尾把全文拆成多条落地。
func TestDraftReplySplitsLongFinalMessage(t *testing.T) {
	fake, stream := newDraftFixture(t)
	stream.throttle = 0

	head := strings.Repeat("a", maxMessageUnits)
	stream.push(head)
	stream.push("b")
	stream.finish()

	calls := fake.messageCalls()
	if len(calls) != 3 {
		t.Fatalf("calls = %+v, want one draft + two sendMessage calls", calls)
	}
	if calls[0].method != "sendMessageDraft" || calls[0].text != head {
		t.Errorf("first call = method %q, runes %d, want sendMessageDraft with %d a's", calls[0].method, len([]rune(calls[0].text)), maxMessageUnits)
	}
	if calls[1].method != "sendMessage" || calls[1].text != head {
		t.Errorf("second call = method %q, runes %d, want sendMessage with %d a's", calls[1].method, len([]rune(calls[1].text)), maxMessageUnits)
	}
	if calls[2].method != "sendMessage" || calls[2].text != "b" {
		t.Errorf("third call = %+v, want sendMessage with b", calls[2])
	}
}

// 草稿更新失败不影响交付：收尾的 sendMessage 仍把全文发出。
func TestDraftReplyDeliversFinalMessageAfterFailedDraft(t *testing.T) {
	fake, stream := newDraftFixture(t)
	fake.failures = 1

	stream.push("你好")
	stream.finish()

	calls := fake.messageCalls()
	if len(calls) != 2 {
		t.Fatalf("calls = %+v, want failed draft + final sendMessage", calls)
	}
	if calls[0].method != "sendMessageDraft" {
		t.Errorf("first call = %+v, want the failed sendMessageDraft attempt", calls[0])
	}
	if calls[1].method != "sendMessage" || calls[1].text != "你好" {
		t.Errorf("second call = %+v, want sendMessage with 你好", calls[1])
	}
}

// 草稿路径只用于私聊；群聊没有该接口，选择原地编辑。
func TestNewReplyStreamSelectsByChatType(t *testing.T) {
	tests := []struct {
		name      string
		chatType  string
		wantDraft bool
	}{
		{"private uses drafts", "private", true},
		{"group edits in place", "group", false},
		{"supergroup edits in place", "supergroup", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := newReplyStream(nil, "dev-1", &tgbotapi.Chat{ID: 1, Type: tt.chatType})
			switch s := stream.(type) {
			case *draftReply:
				if !tt.wantDraft {
					t.Errorf("chat type %s = *draftReply, want *editReply", tt.chatType)
				}
				if s.throttle != draftThrottle {
					t.Errorf("draft throttle = %v, want %v", s.throttle, draftThrottle)
				}
			case *editReply:
				if tt.wantDraft {
					t.Errorf("chat type %s = *editReply, want *draftReply", tt.chatType)
				}
				if s.throttle != editThrottleGroup {
					t.Errorf("edit throttle = %v, want %v", s.throttle, editThrottleGroup)
				}
			default:
				t.Errorf("chat type %s = %T, want a replyStream", tt.chatType, stream)
			}
		})
	}
}

// 首块立即建消息；节流窗口内的增量不刷；finish 把全文定稿到同一条消息。
func TestEditReplySendsFirstChunkThenEditsOnFinish(t *testing.T) {
	fake, stream := newEditFixture(t)

	stream.push("你")
	stream.push("好")
	stream.finish()

	calls := fake.messageCalls()
	if len(calls) != 2 {
		t.Fatalf("calls = %+v, want 2 calls (send + final edit)", calls)
	}
	if calls[0].method != "sendMessage" || calls[0].text != "你" {
		t.Errorf("first call = %+v, want sendMessage with 你", calls[0])
	}
	if calls[1].method != "editMessageText" || calls[1].text != "你好" {
		t.Errorf("second call = %+v, want editMessageText with 你好", calls[1])
	}
	if want := strconv.Itoa(stream.messageID); calls[1].messageID != want {
		t.Errorf("edit message_id = %q, want %q", calls[1].messageID, want)
	}
}

// 超过单条上限时：当前消息按上限定稿，剩余内容另起一条，不丢字也不做无谓编辑。
func TestEditReplyRotatesWhenMessageIsFull(t *testing.T) {
	fake, stream := newEditFixture(t)
	stream.throttle = 0 // 每次 push 都刷新，不依赖真实等待

	head := strings.Repeat("a", maxMessageUnits)
	stream.push(head)
	stream.push("b")
	stream.finish()

	calls := fake.messageCalls()
	if len(calls) != 2 {
		t.Fatalf("calls = %+v, want 2 sendMessage calls (full message + overflow)", calls)
	}
	if calls[0].method != "sendMessage" || calls[0].text != head {
		t.Errorf("first call = method %q, runes %d, want sendMessage with %d a's", calls[0].method, len([]rune(calls[0].text)), maxMessageUnits)
	}
	if calls[1].method != "sendMessage" || calls[1].text != "b" {
		t.Errorf("second call = %+v, want sendMessage with b", calls[1])
	}
}

// 发送失败不能丢内容：下一次刷新把累计全文发出，且不重复发送。
func TestEditReplyRetriesAfterFailedSend(t *testing.T) {
	fake, stream := newEditFixture(t)
	stream.throttle = 0
	fake.failures = 1

	stream.push("你好")
	stream.push("世界")
	stream.finish()

	calls := fake.messageCalls()
	if len(calls) != 2 {
		t.Fatalf("calls = %+v, want 2 sendMessage attempts", calls)
	}
	if calls[0].method != "sendMessage" {
		t.Errorf("first call = %+v, want the failed sendMessage attempt", calls[0])
	}
	if calls[1].method != "sendMessage" || calls[1].text != "你好世界" {
		t.Errorf("second call = %+v, want sendMessage with 你好世界", calls[1])
	}
}

// 没有任何增量时，finish 发占位文案（保持原「（无响应）」行为）。
func TestEditReplyEmptyOutputSendsPlaceholder(t *testing.T) {
	fake, stream := newEditFixture(t)

	stream.finish()

	calls := fake.messageCalls()
	if len(calls) != 1 {
		t.Fatalf("calls = %+v, want 1 sendMessage", calls)
	}
	if calls[0].method != "sendMessage" || calls[0].text != replyEmptyText {
		t.Errorf("call = %+v, want sendMessage with %q", calls[0], replyEmptyText)
	}
}

// 私聊没有任何增量时同样要落地占位文案。
func TestDraftReplyEmptyOutputSendsPlaceholder(t *testing.T) {
	fake, stream := newDraftFixture(t)

	stream.finish()

	calls := fake.messageCalls()
	if len(calls) != 1 {
		t.Fatalf("calls = %+v, want 1 sendMessage", calls)
	}
	if calls[0].method != "sendMessage" || calls[0].text != replyEmptyText {
		t.Errorf("call = %+v, want sendMessage with %q", calls[0], replyEmptyText)
	}
}
