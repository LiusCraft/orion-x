package tts

import (
	"io"

	"github.com/liuscraft/orion-x/internal/language"
)

// ── 文本输入 ──

// TextType 表示合成文本的类型。
type TextType string

const (
	TextTypePlain TextType = "plain"
	TextTypeSSML  TextType = "ssml"
)

// TextInput 是一次合成请求的文本输入。
type TextInput struct {
	Text     string   // 纯文本或 SSML 文档
	TextType TextType // plain 或 ssml
}

// ── 音色选择 ──

// VoiceSelection 选择合成音色。
type VoiceSelection struct {
	VoiceID  string // provider 原生 voice_id，如 "longanyang"、"zh_female_qingxin"
	Model    string // 模型名，如 "cosyvoice-v3-flash"
	Language string // BCP-47 语言标签，可选
}

// ── 音频输出配置 ──

// AudioFormat 是音频编码格式。
type AudioFormat string

const (
	FormatPCM  AudioFormat = "pcm"
	FormatMP3  AudioFormat = "mp3"
	FormatWAV  AudioFormat = "wav"
	FormatOpus AudioFormat = "opus"
	FormatFlac AudioFormat = "flac"
)

// AudioConfig 配置输出音频的格式参数。
type AudioConfig struct {
	Format     AudioFormat // pcm/mp3/wav/opus/flac
	SampleRate int         // Hz，0 = provider 默认
	Bitrate    int         // kbps，0 = provider 默认
	Channels   int         // 声道数，0 = provider 默认
}

// ── 语音参数（归一化范围，adapter 内部映射到 provider 实际尺度）──

// SpeechParams 是归一化的语音控制参数。
// 语速/音调倍率 0.5~2.0，1.0=正常。音量 0.0~1.0，1.0=最大。
// 零值表示使用 provider 默认值。
type SpeechParams struct {
	Speed   float64 // 语速倍率 0.5~2.0，1.0=正常
	Pitch   float64 // 音调倍率 0.5~2.0，1.0=正常
	Volume  float64 // 音量 0.0~1.0，1.0=最大
	Emotion string  // 情绪标签：happy/sad/angry/calm/excited/fearful/neutral
}

// ── 合成请求 ──

// SynthesizeRequest 是一次 TTS 合成的完整请求，包含四个正交维度。
type SynthesizeRequest struct {
	Input  TextInput
	Voice  VoiceSelection
	Audio  AudioConfig
	Speech SpeechParams
	Extra  map[string]any // provider 专属参数逃生舱
}

// ── 合成结果 ──

// SynthesizeResult 是一次同步合成的结果。
type SynthesizeResult struct {
	Audio      io.ReadCloser
	Format     AudioFormat
	SampleRate int
}

// ── 句边界（流式场景）──

// SentenceBoundary 标记流式音频中的一个句子分界点。
// Offset 为音频字节的累计偏移量，到达时该句音频已全部写入。
type SentenceBoundary struct {
	Offset  int    // 音频字节累计偏移量，-1 表示句开始
	Text    string // 句子原文
	IsBegin bool   // true=句开始（后续音频属于此句），false=句结束
}

// ── VoiceInfo（对齐 ModelVoice）──

// VoiceInfo 是 provider 音色的元数据。
type VoiceInfo struct {
	VoiceID     string          // provider 原生音色标识
	Name        string          // 显示名称
	Gender      string          // male / female / neutral
	Description string          // 描述文案
	Languages   []language.Code // 支持的语言代码
	Tags        []string        // 标签，如 "播报"、"对话"、"童声"
	SampleURL   string          // 试听链接
	Emotions    []string        // 该音色支持的情绪列表
}
