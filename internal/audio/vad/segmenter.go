package vad

import (
	"sync"
)

// Segment 语音段，是一段连续语音的音频帧集合
type Segment struct {
	Frames [][]byte
	Bytes  int
}

// Segmenter VAD 驱动的语音切段器
// 职责：检测人声 → 缓存预语音帧 → 切出完整语音段
type Segmenter interface {
	// Process 输入一帧音频，检测并维护切段状态
	// 返回切好的语音段（nil 表示还在攒帧或静音），以及是否刚刚开始新语音段
	Process(audio []byte) (seg *Segment, started bool)
	// Flush 强制结束当前语音段并返回（用于停止前清理）
	Flush() *Segment
	// Close 释放底层 VAD 检测器资源
	Close() error
}

// segmenter 基于 Detector 的切段实现
type segmenter struct {
	detector Detector

	preSpeechFrames [][]byte
	preSpeechBytes  int
	preSpeechMax    int

	active        bool
	segmentFrames [][]byte
	segmentBytes  int

	mu sync.Mutex
}

// NewSegmenter 创建切段器
// sampleRate: 音频采样率
// speechPadMs: 语音段前保留多少毫秒静音作为 padding（0 表示不保留）
func NewSegmenter(detector Detector, sampleRate int, speechPadMs int) Segmenter {
	return &segmenter{
		detector:     detector,
		preSpeechMax: preSpeechMaxBytes(sampleRate, speechPadMs),
	}
}

// SegmenterConfig Segmenter 创建配置
type SegmenterConfig struct {
	SampleRate   int
	Threshold    float64
	MinSilenceMs int
	SpeechPadMs  int
	ModelPath    string
}

// NewSegmenterWithConfig 根据配置创建 Segmenter（内部创建 Silero VAD）
func NewSegmenterWithConfig(cfg SegmenterConfig) (Segmenter, error) {
	det, err := NewSilero(cfg.ModelPath, cfg.SampleRate, cfg.Threshold, cfg.MinSilenceMs, cfg.SpeechPadMs)
	if err != nil {
		return nil, err
	}
	return NewSegmenter(det, cfg.SampleRate, cfg.SpeechPadMs), nil
}

func (s *segmenter) Process(audio []byte) (*Segment, bool) {
	isSpeech, _ := s.detector.Detect(audio)

	s.mu.Lock()
	defer s.mu.Unlock()

	if isSpeech {
		if !s.active {
			s.active = true
			// 开启新段时先塞入预语音 padding
			if s.preSpeechBytes > 0 {
				s.segmentFrames = append(s.segmentFrames, s.preSpeechFrames...)
				s.segmentBytes += s.preSpeechBytes
			}
			s.preSpeechFrames = nil
			s.preSpeechBytes = 0
			s.appendFrameLocked(audio)
			return nil, true
		}
		s.appendFrameLocked(audio)
		return nil, false
	}

	if s.active {
		seg := s.takeSegmentLocked()
		return &seg, false
	}

	s.rememberPreSpeechLocked(audio)
	return nil, false
}

func (s *segmenter) Flush() *Segment {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.active {
		return nil
	}
	seg := s.takeSegmentLocked()
	return &seg
}

func (s *segmenter) Close() error {
	if s.detector != nil {
		return s.detector.Close()
	}
	return nil
}

func (s *segmenter) appendFrameLocked(audio []byte) {
	if len(audio) == 0 {
		return
	}
	frame := make([]byte, len(audio))
	copy(frame, audio)
	s.segmentFrames = append(s.segmentFrames, frame)
	s.segmentBytes += len(frame)
}

func (s *segmenter) takeSegmentLocked() Segment {
	seg := Segment{
		Frames: s.segmentFrames,
		Bytes:  s.segmentBytes,
	}
	s.active = false
	s.segmentFrames = nil
	s.segmentBytes = 0
	return seg
}

func (s *segmenter) rememberPreSpeechLocked(audio []byte) {
	if len(audio) == 0 || s.preSpeechMax <= 0 {
		return
	}

	frame := make([]byte, len(audio))
	copy(frame, audio)
	s.preSpeechFrames = append(s.preSpeechFrames, frame)
	s.preSpeechBytes += len(frame)
	for s.preSpeechBytes > s.preSpeechMax && len(s.preSpeechFrames) > 0 {
		s.preSpeechBytes -= len(s.preSpeechFrames[0])
		s.preSpeechFrames[0] = nil
		s.preSpeechFrames = s.preSpeechFrames[1:]
	}
}

func preSpeechMaxBytes(sampleRate int, speechPadMs int) int {
	if sampleRate <= 0 || speechPadMs <= 0 {
		return 0
	}
	// PCM16 单声道 = sampleRate * 2 bytes/s
	return sampleRate * 2 * speechPadMs / 1000
}
