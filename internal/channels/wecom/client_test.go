package wecom

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// ackFrame 是假服务端的回执帧。
type ackFrame struct {
	Headers frameHeaders `json:"headers"`
	ErrCode int          `json:"errcode"`
	ErrMsg  string       `json:"errmsg"`
}

// fakeWecom 假装企业微信开放平台：接受连接、回执订阅/心跳/回复，并允许测试
// 在每个连接订阅成功后塞入回调帧。
type fakeWecom struct {
	url string

	mu       sync.Mutex
	conns    int
	received []frame

	// onSubscribe 在每个连接订阅成功后调用（可选），此时可向该连接推回调帧。
	onSubscribe func(idx int, ws *websocket.Conn)
}

func newFakeWecom(t *testing.T) *fakeWecom {
	t.Helper()
	f := &fakeWecom{}
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = ws.Close() }()

		f.mu.Lock()
		f.conns++
		idx := f.conns
		f.mu.Unlock()

		for {
			var fr frame
			if err := ws.ReadJSON(&fr); err != nil {
				return
			}
			f.mu.Lock()
			f.received = append(f.received, fr)
			f.mu.Unlock()

			if err := ws.WriteJSON(ackFrame{Headers: frameHeaders{ReqID: fr.Headers.ReqID}, ErrMsg: "ok"}); err != nil {
				return
			}
			if fr.Cmd == cmdSubscribe && f.onSubscribe != nil {
				f.onSubscribe(idx, ws)
			}
		}
	}))
	t.Cleanup(server.Close)

	f.url = "ws" + strings.TrimPrefix(server.URL, "http")
	return f
}

func (f *fakeWecom) connectionCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.conns
}

func (f *fakeWecom) countFrames(match func(frame) bool) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, fr := range f.received {
		if match(fr) {
			n++
		}
	}
	return n
}

func (f *fakeWecom) waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func (f *fakeWecom) waitForFrame(t *testing.T, what string, match func(frame) bool) frame {
	t.Helper()
	var found frame
	f.waitFor(t, what, func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, fr := range f.received {
			if match(fr) {
				found = fr
				return true
			}
		}
		return false
	})
	return found
}

func TestClientSubscribesRepliesAndHeartbeats(t *testing.T) {
	server := newFakeWecom(t)
	server.onSubscribe = func(idx int, ws *websocket.Conn) {
		if idx != 1 {
			return
		}
		payload := `{"cmd":"aibot_msg_callback","headers":{"req_id":"req-1"},"body":{` +
			`"msgid":"m1","aibotid":"AIBOT1","chattype":"single","from":{"userid":"u1"},` +
			`"msgtype":"text","text":{"content":"你好"}}}`
		_ = ws.WriteMessage(websocket.TextMessage, []byte(payload))
	}

	callbacks := make(chan *frame, 1)
	client := NewClient(ClientOptions{
		URL: server.url, BotID: "AIBOT1", Secret: "s3cret", Label: "dev-1",
		OnMessage: func(f *frame) { callbacks <- f },
	})
	client.heartbeatEvery = 20 * time.Millisecond
	client.heartbeatTimeout = 2 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go client.Run(ctx)

	subscribe := server.waitForFrame(t, "subscribe frame", func(f frame) bool { return f.Cmd == cmdSubscribe })
	var credentials subscribeBody
	if err := json.Unmarshal(subscribe.Body, &credentials); err != nil {
		t.Fatalf("decode subscribe body: %v", err)
	}
	if credentials.BotID != "AIBOT1" || credentials.Secret != "s3cret" {
		t.Errorf("subscribe credentials = %+v, want bot_id=AIBOT1 secret=s3cret", credentials)
	}

	select {
	case f := <-callbacks:
		if f.Headers.ReqID != "req-1" {
			t.Errorf("callback req_id = %q, want %q", f.Headers.ReqID, "req-1")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message callback")
	}

	reply := streamBody{
		MsgType: msgTypeStream,
		Stream:  streamContent{ID: "stream:1", Finish: true, Content: "收到了"},
	}
	if err := client.Reply(ctx, "req-1", cmdRespondMsg, reply, 2*time.Second); err != nil {
		t.Fatalf("Reply() error = %v", err)
	}
	sent := server.waitForFrame(t, "reply frame", func(f frame) bool { return f.Cmd == cmdRespondMsg })
	if sent.Headers.ReqID != "req-1" {
		t.Errorf("reply req_id = %q, want callback's %q", sent.Headers.ReqID, "req-1")
	}
	var got streamBody
	if err := json.Unmarshal(sent.Body, &got); err != nil {
		t.Fatalf("decode reply body: %v", err)
	}
	if got.Stream.Content != "收到了" || !got.Stream.Finish {
		t.Errorf("reply stream = %+v, want content=收到了 finish=true", got.Stream)
	}

	server.waitForFrame(t, "heartbeat frame", func(f frame) bool { return f.Cmd == cmdPing })
}

func TestClientReconnectsAndResubscribes(t *testing.T) {
	server := newFakeWecom(t)
	server.onSubscribe = func(idx int, ws *websocket.Conn) {
		if idx == 1 {
			_ = ws.Close() // 第一轮订阅成功后立刻断链
		}
	}

	client := NewClient(ClientOptions{URL: server.url, BotID: "AIBOT1", Secret: "s3cret", Label: "dev-1"})
	client.minBackoff = 10 * time.Millisecond
	client.maxBackoff = 20 * time.Millisecond
	client.heartbeatEvery = time.Hour

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go client.Run(ctx)

	server.waitFor(t, "second connection", func() bool { return server.connectionCount() >= 2 })
	server.waitFor(t, "second subscribe", func() bool {
		return server.countFrames(func(f frame) bool { return f.Cmd == cmdSubscribe }) >= 2
	})
}

func TestReplyWithoutConnectionFails(t *testing.T) {
	client := NewClient(ClientOptions{URL: "ws://127.0.0.1:1/ws", BotID: "b", Secret: "s"})
	err := client.Reply(context.Background(), "req-1", cmdRespondMsg, nil, time.Second)
	if err == nil {
		t.Fatal("Reply() error = nil, want not-connected error")
	}
}
