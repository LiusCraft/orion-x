package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/liuscraft/orion-x/internal/llm"
	"github.com/liuscraft/orion-x/internal/logging"
)

type Options struct {
	SystemPrompt string
	LLM          LLMConfig
	Store        Store
	Now          func() time.Time
}

type serviceImpl struct {
	cfg          Config
	systemPrompt string
	store        Store
	session      *SessionBuffer
	extractor    *llmExtractor
	now          func() time.Time
}

func NewService(cfg Config, opts Options) (Service, error) {
	cfg = normalizeConfig(cfg)
	if opts.Now == nil {
		opts.Now = time.Now
	}

	svc := &serviceImpl{
		cfg:          cfg,
		systemPrompt: strings.TrimSpace(opts.SystemPrompt),
		store:        opts.Store,
		now:          opts.Now,
	}

	if cfg.Mode == ModeSession || cfg.Mode == ModeLongTerm {
		var summarizer Summarizer
		model, err := newLLMModel(context.Background(), opts.LLM)
		if err != nil {
			return nil, err
		}
		if model != nil && cfg.SessionSummaryEveryN > 0 {
			summarizer = &llmSummarizer{model: model}
		}
		svc.session = NewSessionBuffer(cfg.SessionMaxTurns, cfg.SessionSummaryEveryN, summarizer)

		if cfg.Mode == ModeLongTerm && model != nil {
			svc.extractor = &llmExtractor{
				model:             model,
				now:               opts.Now,
				retentionDays:     cfg.RetentionDays,
				defaultType:       "fact",
				defaultImportance: 3,
			}
		}
	}

	if cfg.Mode == ModeLongTerm {
		if svc.store == nil {
			if strings.TrimSpace(cfg.LongTermDBPath) == "" {
				return nil, errors.New("memory.long_term_db_path is required")
			}
			store, err := NewSQLiteStore(cfg.LongTermDBPath)
			if err != nil {
				return nil, err
			}
			svc.store = store
		}
		if cfg.RetentionDays > 0 {
			if err := svc.store.Purge(opts.Now(), cfg.RetentionDays); err != nil {
				logging.Warnf("Memory: purge failed: %v", err)
			}
		}
	}

	return svc, nil
}

func (s *serviceImpl) BuildContextMessages(ctx context.Context, userText string) ([]*llm.Message, error) {
	messages := make([]*llm.Message, 0, 8)
	if s.systemPrompt != "" {
		messages = append(messages, &llm.Message{Role: "system", Content: s.systemPrompt})
	}

	var queryErr error
	if s.cfg.Mode == ModeLongTerm && s.store != nil {
		memCtx, ok := FromContext(ctx)
		if ok && strings.TrimSpace(memCtx.UserID) != "" && strings.TrimSpace(userText) != "" {
			items, err := s.store.Query(memCtx.UserID, userText, s.cfg.LongTermMaxResults, s.cfg.FTSMinScore)
			if err != nil {
				queryErr = err
			} else if len(items) > 0 {
				messages = append(messages, &llm.Message{Role: "system", Content: formatLongTermMemory(items)})
			}
		}
	}

	if s.session != nil {
		messages = append(messages, s.session.Messages()...)
	}

	messages = append(messages, &llm.Message{Role: "user", Content: userText})
	return messages, queryErr
}

func (s *serviceImpl) RecordTurn(ctx context.Context, turn Turn) error {
	if s.cfg.Mode == ModeNone {
		return nil
	}
	if turn.Aborted {
		return nil
	}
	if s.session != nil {
		s.session.Add(ctx, turn)
	}
	if s.cfg.Mode != ModeLongTerm || s.store == nil {
		return nil
	}

	memCtx, ok := FromContext(ctx)
	if ok {
		turn.UserID = memCtx.UserID
		turn.SessionID = memCtx.SessionID
		turn.DeviceID = memCtx.DeviceID
	}
	if strings.TrimSpace(turn.UserID) == "" {
		return nil
	}

	if err := s.store.SaveTurn(turn); err != nil {
		return err
	}

	if s.extractor == nil {
		return nil
	}
	items, err := s.extractor.Extract(ctx, turn)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	for i := range items {
		items[i].UserID = turn.UserID
		if items[i].CreatedAt.IsZero() {
			items[i].CreatedAt = s.now()
		}
	}
	return s.store.SaveItems(items)
}

func (s *serviceImpl) Close() error {
	if s.store != nil {
		return s.store.Close()
	}
	return nil
}

func normalizeConfig(cfg Config) Config {
	if cfg.Mode == "" {
		cfg.Mode = ModeSession
	}
	if cfg.SessionMaxTurns <= 0 {
		cfg.SessionMaxTurns = 10
	}
	if cfg.SessionSummaryEveryN < 0 {
		cfg.SessionSummaryEveryN = 0
	}
	if cfg.LongTermMaxResults <= 0 {
		cfg.LongTermMaxResults = 6
	}
	if cfg.RetentionDays < 0 {
		cfg.RetentionDays = 0
	}
	return cfg
}

func formatLongTermMemory(items []MemoryItem) string {
	var b strings.Builder
	b.WriteString("以下是用户长期记忆（仅供参考，不确定时请向用户确认）：")
	for _, item := range items {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		typeLabel := strings.TrimSpace(item.Type)
		if typeLabel != "" {
			typeLabel = fmt.Sprintf("[%s] ", typeLabel)
		}
		b.WriteString("\n- ")
		b.WriteString(typeLabel)
		b.WriteString(content)
	}
	return b.String()
}
