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
	"time"

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
		text:      r.FormValue("text"),
	})
	if f.failures > 0 {
		f.failures--
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":500,"description":"internal error"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if method == "getMe" {
		_, _ = w.Write([]byte(`{"ok":true,"result":{"id":42,"is_bot":true,"first_name":"fake","username":"fake_bot"}}`))
		return
	}
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

// newStreamReplyFixture 起一个假 Telegram 服务端，返回指向它的流式回复器。
func newStreamReplyFixture(t *testing.T, chatType string) (*fakeTelegram, *streamReply) {
	t.Helper()
	fake := &fakeTelegram{}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	bot, err := tgbotapi.NewBotAPIWithAPIEndpoint("42:TEST", server.URL+"/bot%s/%s")
	if err != nil {
		t.Fatalf("init bot against fake server: %v", err)
	}
	return fake, newStreamReply(bot, "dev-1", &tgbotapi.Chat{ID: 100, Type: chatType})
}

// 首块立即建消息；节流窗口内的增量不刷；finish 把全文定稿到同一条消息。
func TestStreamReplySendsFirstChunkThenEditsOnFinish(t *testing.T) {
	fake, stream := newStreamReplyFixture(t, "private")

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
func TestStreamReplyRotatesWhenMessageIsFull(t *testing.T) {
	fake, stream := newStreamReplyFixture(t, "private")
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
func TestStreamReplyRetriesAfterFailedSend(t *testing.T) {
	fake, stream := newStreamReplyFixture(t, "private")
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
func TestStreamReplyEmptyOutputSendsPlaceholder(t *testing.T) {
	fake, stream := newStreamReplyFixture(t, "private")

	stream.finish()

	calls := fake.messageCalls()
	if len(calls) != 1 {
		t.Fatalf("calls = %+v, want 1 sendMessage", calls)
	}
	if calls[0].method != "sendMessage" || calls[0].text != replyEmptyText {
		t.Errorf("call = %+v, want sendMessage with %q", calls[0], replyEmptyText)
	}
}

// 群聊的发送配额比单聊小，节流间隔必须更保守。
func TestStreamReplyThrottleByChatType(t *testing.T) {
	tests := []struct {
		chatType string
		want     time.Duration
	}{
		{"private", streamThrottlePrivate},
		{"group", streamThrottleGroup},
		{"supergroup", streamThrottleGroup},
	}
	for _, tt := range tests {
		t.Run(tt.chatType, func(t *testing.T) {
			stream := newStreamReply(nil, "dev-1", &tgbotapi.Chat{ID: 1, Type: tt.chatType})
			if stream.throttle != tt.want {
				t.Errorf("throttle for %s = %v, want %v", tt.chatType, stream.throttle, tt.want)
			}
		})
	}
}
