package agent

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/liuscraft/orion-x/internal/llm/provider/openai"
	"github.com/liuscraft/orion-x/internal/session"
	"github.com/liuscraft/orion-x/internal/tools"
)

func requireAgentAPIKey(t *testing.T) {
	t.Helper()
	if os.Getenv("ZHIPU_API_KEY") == "" && os.Getenv("DASHSCOPE_API_KEY") == "" {
		t.Skip("requires ZHIPU_API_KEY or DASHSCOPE_API_KEY")
	}
}

func testAgentConfig() Config {
	key := os.Getenv("ZHIPU_API_KEY")
	if key == "" {
		key = os.Getenv("DASHSCOPE_API_KEY")
	}
	return Config{
		APIKey:  key,
		BaseURL: "https://open.bigmodel.cn/api/coding/paas/v4",
		Model:   "glm-4-flash",
	}
}

func newTestAgent(t *testing.T, ctx context.Context) *Agent {
	t.Helper()
	cfg := testAgentConfig()
	toolManager, err := tools.NewManager(ctx, tools.ManagerConfig{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	t.Cleanup(func() {
		if err := toolManager.Close(); err != nil {
			t.Fatalf("Close Manager error = %v", err)
		}
	})

	agent, err := New(ctx, cfg, toolManager, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if agent == nil {
		t.Fatal("New() returned nil")
	}
	return agent
}

func TestNewAgent(t *testing.T) {
	requireAgentAPIKey(t)

	ctx := context.Background()
	newTestAgent(t, ctx)
}

func TestAgentProcess(t *testing.T) {
	requireAgentAPIKey(t)

	ctx := context.Background()
	agent := newTestAgent(t, ctx)

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"简单问候", "你好", false},
		{"天气查询", "北京天气怎么样", false},
		{"时间查询", "现在几点了", false},
		{"音乐播放", "播放一首歌", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := session.New(session.SessionMeta{Model: "test"})
			sess.Add(session.Message{Role: session.RoleUser, Content: tt.input})
			eventChan, err := agent.Run(ctx, sess)
			if (err != nil) != tt.wantErr {
				t.Errorf("Process() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			var hasTextChunk, hasFinished bool
			timeout := time.After(30 * time.Second)
			finished := false

			for !finished {
				select {
				case event, ok := <-eventChan:
					if !ok {
						return
					}
					switch e := event.(type) {
					case *TextChunkEvent:
						hasTextChunk = true
						if e.Chunk == "" {
							t.Error("TextChunkEvent has empty chunk")
						}
						t.Logf("TextChunk: %s", e.Chunk)
					case *FinishedEvent:
						hasFinished = true
						finished = true
						if e.Error != nil {
							t.Logf("Finished with error: %v", e.Error)
						}
					}
				case <-timeout:
					t.Fatal("Test timed out after 30 seconds")
				}
			}

			if !hasTextChunk && !hasFinished {
				t.Error("No TextChunkEvent or FinishedEvent received")
			}
		})
	}
}

func TestNormalizeConfigRequiresLLMEndpointAndModel(t *testing.T) {
	_, err := normalizeConfig(Config{APIKey: "test-key", Model: "model"})
	if !errors.Is(err, errLLMBaseURLRequired) {
		t.Fatalf("expected base URL error, got %v", err)
	}

	_, err = normalizeConfig(Config{APIKey: "test-key", BaseURL: "https://example.com"})
	if !errors.Is(err, errLLMModelRequired) {
		t.Fatalf("expected model error, got %v", err)
	}
}
