package voicebot

// TTSSchedulerConfig controls how many sentences can be in-flight to TTS.
type TTSSchedulerConfig struct {
	MaxInFlightSentences int
	MaxCacheSentences    int
}

func defaultTTSSchedulerConfig() TTSSchedulerConfig {
	return TTSSchedulerConfig{
		MaxInFlightSentences: 2,
		MaxCacheSentences:    0,
	}
}

type SentenceStatus string

const (
	SentencePending  SentenceStatus = "pending"
	SentenceEnqueued SentenceStatus = "enqueued"
	SentenceDone     SentenceStatus = "done"
	SentenceAborted  SentenceStatus = "aborted"
	SentenceSkipped  SentenceStatus = "skipped"
)

type SentenceRecord struct {
	ID      int64
	TurnID  int64
	Text    string
	Emotion string
	Status  SentenceStatus
}
