package aliyun

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	asr "github.com/liuscraft/orion-x/internal/provider/asr"
)

func TestDashScopeRecognizerReusesConnectionAcrossTasks(t *testing.T) {
	var upgrader websocket.Upgrader
	var mu sync.Mutex
	connections := 0
	runTasks := 0
	finishTasks := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()

		mu.Lock()
		connections++
		mu.Unlock()

		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if msgType != websocket.TextMessage {
				continue
			}

			var msg struct {
				Header taskHeader `json:"header"`
			}
			if err := json.Unmarshal(data, &msg); err != nil {
				t.Errorf("unmarshal message failed: %v", err)
				return
			}

			switch msg.Header.Action {
			case "run-task":
				mu.Lock()
				runTasks++
				mu.Unlock()
				_ = conn.WriteJSON(eventMessage{Header: taskHeader{Event: "task-started"}})
			case "finish-task":
				mu.Lock()
				finishTasks++
				mu.Unlock()
				_ = conn.WriteJSON(eventMessage{
					Header: taskHeader{Event: "result-generated"},
					Payload: taskPayload{
						Output: &taskOutput{
							Sentence: &taskSentence{
								Text:        "hello",
								SentenceEnd: true,
							},
						},
					},
				})
				_ = conn.WriteJSON(eventMessage{Header: taskHeader{Event: "task-finished"}})
			}
		}
	}))
	defer server.Close()

	recognizer, err := NewDashScopeRecognizer(asr.Config{
		APIKey:   "test-key",
		Endpoint: "ws" + server.URL[len("http"):],
	})
	if err != nil {
		t.Fatalf("new recognizer failed: %v", err)
	}
	defer func() { _ = recognizer.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	for i := 0; i < 2; i++ {
		if err := recognizer.Start(ctx); err != nil {
			t.Fatalf("start task %d failed: %v", i+1, err)
		}
		if err := recognizer.SendAudio(ctx, []byte{0, 1, 2, 3}); err != nil {
			t.Fatalf("send audio task %d failed: %v", i+1, err)
		}
		if err := recognizer.Finish(ctx); err != nil {
			t.Fatalf("finish task %d failed: %v", i+1, err)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if connections != 1 {
		t.Fatalf("expected one websocket connection, got %d", connections)
	}
	if runTasks != 2 || finishTasks != 2 {
		t.Fatalf("expected two tasks, got run=%d finish=%d", runTasks, finishTasks)
	}
}

func TestDashScopeRecognizerRejectsOverlappingTasks(t *testing.T) {
	var upgrader websocket.Upgrader
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()

		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if msgType != websocket.TextMessage {
				continue
			}
			var msg struct {
				Header taskHeader `json:"header"`
			}
			if err := json.Unmarshal(data, &msg); err != nil {
				t.Errorf("unmarshal message failed: %v", err)
				return
			}
			if msg.Header.Action == "run-task" {
				_ = conn.WriteJSON(eventMessage{Header: taskHeader{Event: "task-started"}})
			}
		}
	}))
	defer server.Close()

	recognizer, err := NewDashScopeRecognizer(asr.Config{
		APIKey:   "test-key",
		Endpoint: "ws" + server.URL[len("http"):],
	})
	if err != nil {
		t.Fatalf("new recognizer failed: %v", err)
	}
	defer func() { _ = recognizer.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := recognizer.Start(ctx); err != nil {
		t.Fatalf("start task failed: %v", err)
	}
	if err := recognizer.Start(ctx); err == nil {
		t.Fatal("expected overlapping Start to fail")
	}
}
