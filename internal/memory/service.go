package memory

import (
	"context"
	"strings"
	"time"

	"github.com/liuscraft/orion-x/internal/llm"
	"github.com/liuscraft/orion-x/internal/logging"
)

type Options struct {
	SystemPrompt string
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
	systemPrompt  string
	now           func() time.Time
}

func NewService(cfg Config, opts Options) (*Service, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}

	store := NewCuratedStore(opts.ManagerURL, opts.DeviceID, cfg.MemoryCharLimit, cfg.UserCharLimit)
	review := NewBackgroundReview(store, opts.ReviewConfig)

	var compressor *Compressor
	if opts.LLM != nil {
		compressor = NewCompressor(opts.LLM, opts.CompressCfg)
	}

	svc := &Service{
		store:        store,
		review:       review,
		compressor:   compressor,
		systemPrompt: strings.TrimSpace(opts.SystemPrompt),
		now:          opts.Now,
	}

	// Load memory from Manager
	if err := store.Load(context.Background()); err != nil {
		logging.Warnf("Memory: failed to load curated store: %v", err)
	}

	return svc, nil
}

// BuildContextMessages assembles messages for the LLM: system prompt + frozen snapshot + history.
func (s *Service) BuildContextMessages(ctx context.Context, history []*llm.Message) []*llm.Message {
	messages := make([]*llm.Message, 0, 16)
	if s.systemPrompt != "" {
		messages = append(messages, &llm.Message{Role: "system", Content: s.systemPrompt})
	}

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

	// Async: save turn to Manager via HTTP (TODO: wire up)
	// POST /internal/devices/{device_id}/turns

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

func (s *Service) Close() error {
	return nil
}

// CuratedStore exposes the underlying store for tool handlers.
func (s *Service) CuratedStore() *CuratedStore {
	return s.store
}
