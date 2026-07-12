package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/liuscraft/orion-x/internal/llm"
	"github.com/liuscraft/orion-x/internal/logging"
)

type Options struct {
	LLM          llm.Client
	ManagerURL   string
	DeviceID     string
	ReviewConfig ReviewConfig
	CompressCfg  CompressionConfig
	Now          func() time.Time
}

type Service struct {
	store         *CuratedStore
	review        *BackgroundReview
	compressor    *Compressor
	sessionBuffer []Turn // local buffer for recent turns
	managerURL    string
	deviceID      string
	httpClient    *http.Client
	now           func() time.Time
}

func NewService(cfg Config, opts Options) (*Service, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}

	store := NewCuratedStore(opts.ManagerURL, opts.DeviceID, cfg.MemoryCharLimit, cfg.UserCharLimit)
	review := NewBackgroundReview(store, opts.ReviewConfig, opts.LLM)

	var compressor *Compressor
	if opts.LLM != nil {
		compressor = NewCompressor(opts.LLM, opts.CompressCfg)
	}

	svc := &Service{
		store:      store,
		review:     review,
		compressor: compressor,
		managerURL: strings.TrimRight(opts.ManagerURL, "/"),
		deviceID:   opts.DeviceID,
		httpClient: &http.Client{},
		now:        opts.Now,
	}

	// Load memory from Manager
	if err := store.Load(context.Background()); err != nil {
		logging.Warnf("Memory: failed to load curated store: %v", err)
	}

	return svc, nil
}

// BuildContextMessages assembles messages for the LLM using the same three-tier
// order as Hermes: stable (soul → rules) → volatile (memory+user history).
func (s *Service) BuildContextMessages(ctx context.Context, history []*llm.Message, soulPrompt, rulesPrompt string) []*llm.Message {
	messages := make([]*llm.Message, 0, 16)

	// T1: Stable — soul identity + behavioral rules
	if soulPrompt != "" {
		messages = append(messages, &llm.Message{
			Role:    "system",
			Content: "═══════════════════ 身份设定 (SOUL) ═══════════════════\n" + soulPrompt,
		})
	}
	if rulesPrompt != "" {
		messages = append(messages, &llm.Message{
			Role:    "system",
			Content: rulesPrompt,
		})
	}

	// T2: Volatile — curated memory snapshot + user profile snapshot
	memoryBlock := s.store.FormatForSystemPrompt("memory")
	userBlock := s.store.FormatForSystemPrompt("user")
	if memoryBlock != "" || userBlock != "" {
		var b strings.Builder
		if memoryBlock != "" {
			b.WriteString(memoryBlock)
		}
		if userBlock != "" {
			if b.Len() > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString(userBlock)
		}
		messages = append(messages, &llm.Message{Role: "system", Content: b.String()})
	}

	messages = append(messages, history...)
	return messages
}

// RecordTurn saves a turn and triggers background review.
func (s *Service) RecordTurn(ctx context.Context, turn Turn) error {
	if turn.Aborted {
		return nil
	}
	s.sessionBuffer = append(s.sessionBuffer, turn)
	if len(s.sessionBuffer) > 50 {
		s.sessionBuffer = s.sessionBuffer[len(s.sessionBuffer)-50:]
	}

	// Save turn to Manager via HTTP (async, best-effort)
	s.saveTurnAsync(ctx, turn)

	// Trigger background review
	snapshot := s.store.FormatForSystemPrompt("memory")
	s.review.Spawn(ctx, s.sessionBuffer, snapshot)

	// Check compression (best effort)
	if s.compressor != nil {
		if s.compressor.ShouldCompress(s.sessionBuffer, 500, 8192) {
			result, err := s.compressor.Compress(ctx, s.sessionBuffer)
			if err != nil {
				logging.Warnf("Memory: compression failed: %v", err)
			} else {
				compressedTurns := []Turn{{
					TurnID:        -1,
					UserText:      "[摘要]",
					AssistantText: result.Summary,
					StartedAt:     turn.StartedAt,
					EndedAt:       turn.EndedAt,
				}}
				compressedTurns = append(compressedTurns, result.TailTurns...)
				s.sessionBuffer = compressedTurns
				logging.Infof("Memory: compression done, %d turns → %d turns + summary",
					len(s.sessionBuffer), len(result.TailTurns))
			}
		}
	}

	return nil
}

// saveTurnAsync persists a turn to the Manager via HTTP. Best-effort, logs errors.
// Uses context.Background with a timeout — the caller's context may be canceled
// before the async goroutine runs.
func (s *Service) saveTurnAsync(_ context.Context, turn Turn) {
	if s.deviceID == "" || s.managerURL == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		body, err := json.Marshal(map[string]interface{}{
			"session_id":     turn.SessionID,
			"turn_id":        turn.TurnID,
			"user_text":      turn.UserText,
			"assistant_text": turn.AssistantText,
			"started_at":     turn.StartedAt,
			"ended_at":       turn.EndedAt,
			"aborted":        turn.Aborted,
		})
		if err != nil {
			logging.Errorf("RecordTurn[%s]: marshal: %v", s.deviceID, err)
			return
		}
		url := fmt.Sprintf("%s/internal/devices/%s/turns", s.managerURL, s.deviceID)
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
		if err != nil {
			logging.Errorf("RecordTurn[%s]: create request: %v", s.deviceID, err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := s.httpClient.Do(req)
		if err != nil {
			logging.Errorf("RecordTurn[%s]: HTTP POST: %v", s.deviceID, err)
			return
		}
		_ = resp.Body.Close()
	}()
}

func (s *Service) Close() error {
	if s.store != nil {
		s.store.WaitForSync()
	}
	return nil
}

// CuratedStore exposes the underlying store for tool handlers.
func (s *Service) CuratedStore() *CuratedStore {
	return s.store
}
