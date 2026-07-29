# TTS Provider 统一 SDK 设计

> 状态：设计提案 | 日期：2026-07-26 | 范围：阿里云 DashScope、火山引擎、Azure、Google Cloud、MiniMax

## 1. 结论

TTS 层当前以阿里云 DashScope 的字段作为公共模型，导致对接新 provider 时参数无处安放。建议以**归一化语义 + provider Extra 兜底**为策略，将公共层精简为真实共性的集合：

- **`SynthesizeRequest`** 拆成四个正交维度：`TextInput`、`VoiceSelection`、`AudioConfig`、`SpeechParams`，每个 provider adapter 内部做尺度映射。
- **`Synthesizer` / `StreamingSynthesizer`** 两个接口覆盖同步和流式场景，流式保持当前的 `AudioReader + SentenceBoundaries` 低延迟模式。
- **`ProviderOptions map[string]any`** 作为 provider 特有字段的逃生舱，不试图把 `Workspace`、`cluster`、`sound_effects` 装进公共类型。
- **`Capabilities`** 让 Manager 可以动态查询 provider 的模型、音色、情绪、SSML 支持，替代当前仅靠 `ProviderMeta` 静态声明的局限。

推荐的核心调用接口是 `Synthesize + StartSynthesis`。TTSProcessor 自动检测 `StreamingSynthesizer` 走流式路径，不实现则回退 `Synthesize`。

首期支持边界：文本输入（纯文本 + SSML）、音色选择、音频格式/采样率、语速/音调/音量/情绪的归一化控制、流式合成、句边界标记、预热连接。声音克隆、声音设计、长文本异步合成暂不纳入公共能力。

## 2. 当前实现的问题

当前实现位于：

- `internal/provider/tts/factory.go` — `Config`、`SynthesisOptions`、`VoiceInfo`、接口定义、注册与工厂
- `internal/provider/tts/aliyun/dashscope.go` — DashScope WebSocket 适配器
- `internal/config/config.go` — `TTSConfig` 配置加载
- `internal/audio/tts.go` — `TTSProcessor` 调用方

主要问题如下。

### 2.1 Config 字段全部是阿里云专属

```go
type Config struct {
    APIKey               string
    Endpoint             string
    Workspace            string     // ← 阿里云专属：X-DashScope-WorkSpace
    Model                string
    Voice                string
    Format               string
    SampleRate           int
    Volume               int        // ← 范围 0-100，但 MiniMax 是 0.1-10.0
    Rate                 float64
    Pitch                float64    // ← 倍率 0.5-2.0，但 Google 是半音 -20~+20
    EnableSSML           bool
    TextType             string     // ← 阿里云专属：PlainText
    EnableDataInspection *bool      // ← 阿里云专属：X-DashScope-DataInspection
}
```

对接火山引擎时需要 `appid`、`cluster`、`token`；对接 MiniMax 时需要 `bitrate`、`channel`、`sound_effects`。这些字段无法塞进现有 Config。

### 2.2 SynthesisOptions 只支持情绪和语速

```go
type SynthesisOptions struct {
    Emotion string  // ← 只支持阿里云行内标签映射的情绪
    Rate    float64 // ← 无法控制音调、音量
}
```

MiniMax 支持 9 种情绪枚举 + 独立音量控制；Azure 有 30+ 种 SSML style + style degree；Google 通过 SSML prosody 控制 pitch/rate/volume。当前 opts 丢失了大部分能力。

### 2.3 公共层混入了 Provider 协议细节

`Config.TextType`（PlainText）、`Config.EnableDataInspection`（阿里云数据检查）、`Config.Workspace` 都是 DashScope 独有字段，却暴露在公共 `Config` 里。`SentenceBoundaries` 是阿里云 `sentence-begin/end` 事件的产物，其他 provider 可能没有等价机制，但当前接口把它当成了必须项。

### 2.4 参数尺度不统一

| 参数 | 阿里云 | 火山 | MiniMax | Google | Azure |
|------|:---:|:---:|:---:|:---:|:---:|
| 语速 | 0.5~2.0 | ~1.0 倍率 | 0.5~2.0 | 0.25~4.0 | 0.5~2.0 |
| 音调 | 0.5~2.0 | ~1.0 倍率 | -12~+12 semitones | -20~+20 semitones | SSML -50%~+150% |
| 音量 | 0~100 | ~1.0 倍率 | 0.1~10.0 | -96~+16 dB | 0~100 |
| 情绪 | 行内标签 `[happy]` | API 参数字符串 | API 枚举 9 种 | SSML | SSML style 30+ 种 |

调用方不应感知这些差异，SDK 层需要在归一化范围和 provider 实际范围之间做映射。

### 2.5 VoiceInfo 与 ModelVoice（DB 模型）不对齐

`VoiceInfo` 只有 7 个字段（VoiceID、Name、Gender、Description、Languages、Tags、SampleURL），而数据库 `ModelVoice` 有 15 个字段（多了 AvatarURL、PreviewURL、Emotions、IsSystem、IsCloned、SourceAudioURL）。`ListAllSystemVoices` 直接透传 `VoiceInfo`，Manager 同步时需要补字段。

## 3. 五家 Provider API 对比

### 3.1 协议与认证

| 维度 | 阿里云 DashScope | 火山引擎 | Azure | Google Cloud | MiniMax |
|------|:---:|:---:|:---:|:---:|:---:|
| 同步协议 | WebSocket（仅 WS） | REST POST | REST POST + SDK | REST POST + gRPC | REST POST + SSE |
| 流式协议 | WebSocket duplex | WebSocket | SDK WS | gRPC streaming | WS events |
| 认证 | Header `Authorization: Bearer <key>` | Bearer token + AppID | `Ocp-Apim-Subscription-Key` 或 Bearer | OAuth2 / API Key | Header `Authorization: Bearer <key>` |
| 特有 Header | `X-DashScope-WorkSpace`, `X-DashScope-DataInspection` | — | — | — | — |

关键事实：

- 阿里云 **不支持 REST**。必须 WebSocket，`run-task → continue-task → finish-task` 三步走，服务端主动发 `sentence-begin/end` 事件。
- 火山 **同时支持 REST 和 WebSocket**。REST 一次性返回完整音频，WebSocket 流式分片返回。
- MiniMax HTTP 模式支持 **SSE 流式**（`stream: true`），每帧返回 hex 编码音频 + `is_final` 标记。
- Azure 的 REST API 必须以 **SSML** 作为请求体；SDK 支持 WebSocket 流式但不直接暴露非 MS 语言。
- Google 的流式合成走 **gRPC `StreamingSynthesize`**，不是 WebSocket。

### 3.2 音色选择

| 维度 | 阿里云 | 火山 | Azure | Google | MiniMax |
|------|:---:|:---:|:---:|:---:|:---:|
| 选择方式 | `voice` 字符串 | `voice_type` 字符串 | `ShortName` 如 `zh-CN-YunxiNeural` | `name` 或 `languageCode + gender` | `voice_id` 字符串 |
| 多语言音色 | 部分音色支持 `language_hints` | 不同 voice_type 对应不同语言 | `SecondaryLocaleList` 标记 | `languageCode` 即语言 | `language_boost` 参数 |
| 动态列出声线 | ✅ `ListVoices` | ✅ | ✅ `GET /voices/list` | ✅ `ListVoices` | ✅ |

### 3.3 语音参数

| 参数 | 阿里云 | 火山 | Azure | Google | MiniMax |
|------|:---:|:---:|:---:|:---:|:---:|
| 语速 | `rate` 0.5~2.0 | `speed_ratio` | SSML `rate` 0.5~2.0 | `speakingRate` 0.25~4.0 | `speed` 0.5~2.0 |
| 音调 | `pitch` 0.5~2.0 | `pitch_ratio` | SSML `pitch` | `pitch` -20~+20 semitones | `pitch` -12~+12 |
| 音量 | `volume` 0~100 | `volume_ratio` | SSML `volume` 0~100 | `volumeGainDb` -96~+16 | `vol` 0.1~10.0 |
| 情绪 | 行内 `[happy]` | `emotion` 字符串 | SSML `mstts:express-as` style | SSML | `emotion` 枚举 9 种 |
| 风格 | `instruction` | — | 30+ styles + style degree + roles | — | `sound_effects` |

### 3.4 音频输出

| 维度 | 阿里云 | 火山 | Azure | Google | MiniMax |
|------|:---:|:---:|:---:|:---:|:---:|
| 格式 | pcm/wav/mp3/opus | mp3/wav/pcm | SSML output format | MP3/OGG/LINEAR16/... | mp3/pcm/flac/wav |
| 采样率 | 8k~48k，默认 22050 | 依赖 voice_type | 8k~48k | 任意（服务端转换） | 16000~48000，默认 32000 |
| 位深 | 16bit | — | 16bit | 16bit | — |
| 声道 | 单声道 | — | 单声道 | 单声道 | 单声道 |

### 3.5 服务端分句

| 能力 | 阿里云 | 火山 | Azure | Google | MiniMax |
|------|:---:|:---:|:---:|:---:|:---:|
| 自动分句 | ✅ `sentence-begin/end` 事件 | ❌ | ❌ | ❌ | ❌（仅有 `is_final`） |
| 句边界标记 | ✅ | ❌ | ⚠️ SSML mark 可见但非自动 | ⚠️ `TimepointType.SSML_MARK` | ❌ |

## 4. 设计原则

### 4.1 统一语义，不统一 wire shape

公共类型表达"用什么音色、生成什么格式的音频、带什么情绪"，每个 adapter 负责把公共类型转换为自己的请求格式（JSON body、SSML、WebSocket message、gRPC proto）。

不把阿里云 `run-task.header.task_id`、Azure `mstts:express-as` 或 Google `VoiceSelectionParams.language_code` 暴露给调用方。

### 4.2 不以最小公分母丢数据

可移植的参数进入公共字段（语速、音调、音量、情绪）；provider 特有能力（阿里云 `instruction`、MiniMax `sound_effects`、Azure `style_degree`）通过 `SynthesizeRequest.Extra map[string]any` 透传。

`Extra` 的 key 由各 adapter 文档定义，调用方按需使用。公共层不校验、不转换 Extra 内容。

### 4.3 公共参数必须有稳定语义

- **语速**归一化为倍率 `0.5~2.0`，`1.0` = 正常。Google 的 0.25~4.0 映射时截断到 0.5~2.0。
- **音调**归一化为倍率 `0.5~2.0`，`1.0` = 正常。Google 的 semitone 和 MiniMax 的 pitch 通过公式映射：`倍率 = 2^(semitones/12)`。
- **音量**归一化为 `0.0~1.0`，`1.0` = 最大。阿里云 volume 0~100、MiniMax vol 0.1~10.0、Google gainDb -96~+16 各自映射。
- **情绪**定义为核心枚举（`happy`/`sad`/`angry`/`calm`/`excited`/`fearful`/`neutral`）。各 adapter 做 best-effort 映射到 provider 支持的情绪集合，不支持的情绪静默忽略。

### 4.4 流式保持低延迟

`StreamingSynthesizer.StartSynthesis` 返回 `SynthesisStream`，`AudioReader()` 是流式 reader（音频帧一到即可读），`SentenceBoundaries()` 返回句边界 channel（不支持分句的 provider 返回 nil，调用方自然降级）。

`WarmableProvider` 预热机制保持不变，由 adapter 内部实现。

### 4.5 能力声明

每个 provider 注册时通过 `ProviderMeta` 声明静态能力（名称、BaseURL、Models、Features）。运行时通过 `CapabilityProvider` 接口动态查询模型、音色、支持的情绪列表。

`ProviderMeta.Features` 用于 Manager 前端展示该 provider 支持哪些能力（streaming、ssml、emotion、voice_clone）。`CapabilityProvider` 用于实际获取可用模型和音色列表。

## 5. 公共 API

以下代码用于约束接口形状，不是逐字实现要求。

```go
package tts

import (
    "context"
    "io"
    "github.com/liuscraft/orion-x/internal/language"
)

// ── 输入 ──

type TextType string

const (
    TextTypePlain TextType = "plain"
    TextTypeSSML  TextType = "ssml"
)

type TextInput struct {
    Text     string   // 纯文本或 SSML 文档
    TextType TextType
}

// ── 音色选择 ──

type VoiceSelection struct {
    VoiceID  string // provider 原生 voice_id，如 "longanyang"、"zh_female_qingxin"
    Model    string // 模型名，如 "cosyvoice-v3-flash"、"speech-2.8-hd"
    Language string // BCP-47 语言标签，可选。Provider 支持语言 fallback 时使用
}

// ── 音频输出配置 ──

type AudioFormat string

const (
    FormatPCM  AudioFormat = "pcm"
    FormatMP3  AudioFormat = "mp3"
    FormatWAV  AudioFormat = "wav"
    FormatOpus AudioFormat = "opus"
    FormatFlac AudioFormat = "flac"
)

type AudioConfig struct {
    Format     AudioFormat
    SampleRate int // Hz，如 16000、24000。0 = provider 默认
    Bitrate    int // kbps，仅 opus/mp3 等有损格式有意义。0 = provider 默认
    Channels   int // 1=单声道。0 = provider 默认
}

// ── 语音参数（归一化范围，adapter 内部映射到 provider 实际尺度）──

type SpeechParams struct {
    Speed   float64 // 语速倍率 0.5~2.0，1.0=正常。0=使用 provider 默认
    Pitch   float64 // 音调倍率 0.5~2.0，1.0=正常。0=使用 provider 默认
    Volume  float64 // 音量 0.0~1.0，1.0=最大。0=使用 provider 默认
    Emotion string  // 情绪标签：happy/sad/angry/calm/excited/fearful/neutral。空=无情绪
}
```

### 5.1 SynthesizeRequest

```go
type SynthesizeRequest struct {
    Input  TextInput
    Voice  VoiceSelection
    Audio  AudioConfig
    Speech SpeechParams
    Extra  map[string]any // provider 专属参数逃生舱
}
```

`Extra` 的典型 key：

| Provider | Extra Key | 类型 | 说明 |
|----------|-----------|------|------|
| aliyun | `workspace` | string | DashScope 工作空间 ID |
| aliyun | `data_inspection` | bool | 数据检查开关 |
| aliyun | `instruction` | string | 指令控制（方言、情绪） |
| aliyun | `language_hints` | []string | 目标语言列表 |
| aliyun | `seed` | int | 随机种子 |
| volcengine | `appid` | string | 应用 ID |
| volcengine | `cluster` | string | 服务集群 |
| minimax | `sound_effects` | string | 音效（spacious_echo 等） |
| minimax | `language_boost` | string | 语言增强（auto 等） |
| azure | `style_degree` | float64 | 风格强度 0.01~2.0 |
| azure | `role` | string | 角色扮演（Girl/Boy/SeniorMale 等） |

### 5.2 结果

```go
type SynthesizeResult struct {
    Audio      io.ReadCloser
    Format     AudioFormat
    SampleRate int
}
```

### 5.3 句边界（流式场景）

```go
// SentenceBoundary 标记流式音频中的一个句子分界点。
// Offset 为音频字节的累计偏移量，到达时该句音频已全部写入。
type SentenceBoundary struct {
    Offset int    // 音频字节累计偏移量，-1 表示句开始
    Text   string // 句子原文
    IsBegin bool  // true=句开始（后续音频属于此句），false=句结束
}
```

`IsBegin=true` 的 boundary 用于告知调用方"接下来的音频帧文本是什么"；
`IsBegin=false` 的 boundary 用于标记该句音频何时播放完毕，以便下游做字幕同步。

不支持服务端分句的 provider 返回 nil channel，调用方 select 永不命中。

### 5.4 核心接口

```go
// Synthesizer 是基本的 TTS 接口（同步、整段合成）。
type Synthesizer interface {
    Synthesize(ctx context.Context, req SynthesizeRequest) (*SynthesizeResult, error)
}

// StreamingSynthesizer 是流式 TTS 接口（低延迟、逐句合成）。
// TTSProcessor 通过类型断言检测，不实现则回退到 Synthesize。
type StreamingSynthesizer interface {
    Synthesizer
    // StartSynthesis 建立连接并返回可复用的流式会话。
    StartSynthesis(ctx context.Context, req SynthesizeRequest) (SynthesisStream, error)
}

// SynthesisStream 是一次 TTS 流式会话，支持多句话连续合成。
// 典型用法：WriteTextChunk → Finish → AudioReader 流式读取。
type SynthesisStream interface {
    // WriteTextChunk 发送一句文本。多句话可连续写入，不需等前句完成。
    WriteTextChunk(ctx context.Context, text string) error

    // Finish 通知服务端文本发送完毕，立即返回（不阻塞等 task-finished）。
    Finish(ctx context.Context) error

    // AudioReader 返回流式音频 reader，可在 Finish 前开始读。
    // task-finished 后返回 EOF。
    AudioReader() io.ReadCloser

    // SentenceBoundaries 返回句边界通知 channel。
    // 不支持服务端分句的 provider 返回 nil。
    SentenceBoundaries() <-chan SentenceBoundary

    // Abort 立即中止会话（打断场景）。
    Abort()
}

// WarmableProvider 是支持预连接预热的扩展接口。
// Warm 在 goroutine 里调用，阻塞直到连接就绪或 ctx 取消。
type WarmableProvider interface {
    Warm(ctx context.Context, req SynthesizeRequest) SynthesisStream
}
```

### 5.5 Capability 查询

```go
// CapabilityProvider 允许 Manager 动态查询 provider 的能力。
// Manager 通过类型断言使用；不实现则回退到 ProviderMeta 静态声明。
type CapabilityProvider interface {
    Synthesizer
    GetCapabilities(ctx context.Context) (*Capabilities, error)
}

type Capabilities struct {
    Models   []ModelCapability
    Features []Feature
}

type ModelCapability struct {
    Name               string
    SupportedLanguages []language.Code
    SupportedFormats   []AudioFormat
    SupportedSampleRates []int
    SupportedEmotions  []string
    Voices             []VoiceInfo
}

type Feature string

const (
    FeatureStreaming    Feature = "streaming"
    FeatureSSML         Feature = "ssml"
    FeatureEmotion      Feature = "emotion"
    FeatureVoiceCloning Feature = "voice_cloning"
    FeatureWarmup       Feature = "warmup"
)
```

## 6. Provider 配置与注册

### 6.1 Config 精简

```go
type Config struct {
    APIKey     string
    Endpoint   string
    Model      string // 默认模型
    Voice      string // 默认音色
    SampleRate int    // 默认采样率
    Extra      map[string]any // provider 专属连接参数
}
```

`Extra` 替代当前的 `Workspace`、`EnableDataInspection` 等专属字段。Provider 创建时从 Extra 读取，不做公共校验。

### 6.2 注册（保持现有模式）

```go
type ModelInfo struct {
    SupportedLanguages []language.Code
    SystemVoices       []VoiceInfo
}

type ProviderMeta struct {
    Name           string
    Description    string
    DefaultBaseURL string
    Models         map[string]ModelInfo
    Features       []Feature // streaming / ssml / emotion / voice_cloning / warmup
}

type Constructor func(cfg Config) (Synthesizer, error)

func Register(providerType string, constructor Constructor, meta ProviderMeta)
func NewProvider(cfg ProviderConfig) (Synthesizer, error)
func ListRegistered() map[string]ProviderMeta
```

`ProviderMeta.Description` 和 `ProviderMeta.Features` 供 Manager API 展示；`Models` 和 `SystemVoices` 供音色同步。

### 6.3 VoiceInfo 对齐

```go
type VoiceInfo struct {
    VoiceID     string          // provider 原生音色标识
    Name        string          // 显示名称
    Gender      string          // male / female / neutral
    Description string          // 描述文案
    Languages   []language.Code // 支持的语言代码
    Tags        []string        // 标签，如 "播报"、"对话"、"童声"
    SampleURL   string          // 试听链接
    Emotions    []string        // 该音色支持的情绪列表（新增）
}
```

`Emotions` 用于 Manager 前端展示该音色支持哪些情绪，与 `ModelVoice` 的 `Emotions` 字段对齐。

## 7. Adapter 映射规则

### 7.1 阿里云 DashScope

协议层：WebSocket `run-task → continue-task → finish-task`。

| 公共字段 | 适配器映射 |
|----------|-----------|
| `TextInput.Text` | `continue-task` payload 中的 `input` text |
| `TextInput.TextType` | `run-task` 的 `parameters.text_type`（PlainText 或 SSML） |
| `VoiceSelection.VoiceID` | `run-task` 的 `parameters.voice` |
| `VoiceSelection.Model` | `run-task` 的 `payload.model` |
| `AudioConfig.Format` | `run-task` 的 `parameters.format` |
| `AudioConfig.SampleRate` | `run-task` 的 `parameters.sample_rate` |
| `SpeechParams.Speed` | `run-task` 的 `parameters.rate`（直接使用，同为 0.5~2.0） |
| `SpeechParams.Pitch` | `run-task` 的 `parameters.pitch`（直接使用，同为 0.5~2.0） |
| `SpeechParams.Volume` | `run-task` 的 `parameters.volume`（volume × 100 → 0~100） |
| `SpeechParams.Emotion` | 映射为行内标签 `[happy]`，或通过 `instruction` 参数传递 |
| `Extra.workspace` | Header `X-DashScope-WorkSpace` |
| `Extra.data_inspection` | Header `X-DashScope-DataInspection` |
| `Extra.instruction` | `run-task` 的 `parameters.instruction` |
| `Extra.language_hints` | `run-task` 的 `parameters.language_hints` |
| `Extra.seed` | `run-task` 的 `parameters.seed` |

句边界：透传 `sentence-begin/end` 事件 → `SentenceBoundary` channel。
预热：复用 `StartSynthesis`，内部通过 `atomic.Bool` 防止并发预热。

### 7.2 火山引擎

协议层：REST（同步）+ WebSocket（流式）。

| 公共字段 | 适配器映射 |
|----------|-----------|
| `TextInput.Text` | `request.text` |
| `TextInput.TextType` | `request.text_type`（plain 或 ssml） |
| `VoiceSelection.VoiceID` | `audio.voice_type` 或 `speaker` |
| `AudioConfig.Format` | `audio.encoding`（mp3/wav/pcm） |
| `SpeechParams.Speed` | `audio.speed_ratio`（直接使用） |
| `SpeechParams.Pitch` | `audio.pitch_ratio`（直接使用） |
| `SpeechParams.Volume` | `audio.volume_ratio`（直接使用） |
| `SpeechParams.Emotion` | `request.emotion` |
| `Extra.appid` | `app.appid` |
| `Extra.cluster` | `app.cluster` |

句边界：不支持（返回 nil channel）。

### 7.3 Azure

协议层：REST（SSML body）+ SDK WebSocket。

| 公共字段 | 适配器映射 |
|----------|-----------|
| `TextInput.Text` + `TextType` | SSML `<speak>` body；TextType=ssml 时原样放入 |
| `VoiceSelection.VoiceID` | SSML `<voice name="...">` |
| `VoiceSelection.Language` | SSML `xml:lang` |
| `AudioConfig.Format` | Header `X-Microsoft-OutputFormat` |
| `SpeechParams.Speed` | SSML `<prosody rate="...">` |
| `SpeechParams.Pitch` | SSML `<prosody pitch="...">` |
| `SpeechParams.Volume` | SSML `<prosody volume="...">` |
| `SpeechParams.Emotion` + `Extra.style_degree` | SSML `<mstts:express-as style="..." styledegree="...">` |
| `Extra.role` | SSML `<mstts:express-as role="...">` |

句边界：不支持（返回 nil channel）。

### 7.4 Google Cloud

协议层：REST + gRPC streaming。

| 公共字段 | 适配器映射 |
|----------|-----------|
| `TextInput.Text` | `SynthesisInput.text` 或 `SynthesisInput.ssml` |
| `VoiceSelection.VoiceID` | `VoiceSelectionParams.name` |
| `VoiceSelection.Language` | `VoiceSelectionParams.language_code` |
| `AudioConfig.Format` | `AudioConfig.audio_encoding` |
| `AudioConfig.SampleRate` | `AudioConfig.sample_rate_hertz` |
| `SpeechParams.Speed` | `AudioConfig.speaking_rate`（需截断 0.5~2.0） |
| `SpeechParams.Pitch` | `AudioConfig.pitch`（倍率 → semitone：`12*log2(pitch)`） |
| `SpeechParams.Volume` | `AudioConfig.volume_gain_db`（0.0~1.0 → -96~+16 dB） |

句边界：不支持（返回 nil channel）。

### 7.5 MiniMax

协议层：REST（同步 + SSE 流式）+ WebSocket。

| 公共字段 | 适配器映射 |
|----------|-----------|
| `TextInput.Text` | `text` |
| `VoiceSelection.VoiceID` | `voice_setting.voice_id` |
| `VoiceSelection.Model` | `model` |
| `AudioConfig.Format` | `audio_setting.format` |
| `AudioConfig.SampleRate` | `audio_setting.sample_rate` |
| `AudioConfig.Bitrate` | `audio_setting.bitrate` |
| `SpeechParams.Speed` | `voice_setting.speed`（直接使用） |
| `SpeechParams.Pitch` | `voice_setting.pitch`（倍率 → semitone 映射：`12*log2(pitch)`，截断到 -12~+12） |
| `SpeechParams.Volume` | `voice_setting.vol`（0.0~1.0 → 0.1~10.0 线性映射） |
| `SpeechParams.Emotion` | `voice_setting.emotion` |
| `Extra.sound_effects` | `voice_modify.sound_effects` |
| `Extra.language_boost` | `language_boost` |

句边界：HTTP SSE 模式有 `is_final` 标记但粗粒度；WebSocket 模式返回 nil channel。

## 8. 参数归一化公式

### 8.1 音调映射

各 provider 的 pitch 参数统一映射到倍率 `0.5~2.0`：

| Provider | 原始范围 | → 倍率公式 |
|----------|---------|-----------|
| Google | semitone -20~+20 | `pitch_ratio = 2^(semitones/12)` |
| MiniMax | -12~+12 | `pitch_ratio = 2^(semitones/12)` |
| 阿里云 | 0.5~2.0 | 直接使用 |
| 火山 | ~1.0 倍率 | 直接使用 |
| Azure | SSML -50%~+150% | `pitch_ratio = 1 + percent/100` |

### 8.2 音量映射

统一映射到 `0.0~1.0`：

| Provider | 原始范围 | → 0.0~1.0 公式 |
|----------|---------|----------------|
| 阿里云 | 0~100 | `vol_norm = volume / 100` |
| MiniMax | 0.1~10.0 | `vol_norm = (vol - 0.1) / 9.9` |
| Google | -96~+16 dB | `vol_norm = (gainDb + 96) / 112` |

### 8.3 情绪映射（Best Effort）

公共情绪 → 各 provider 的情绪值映射表：

| 公共情绪 | 阿里云 | 火山 | MiniMax | Azure | Google |
|----------|:---:|:---:|:---:|:---:|:---:|
| `happy` | `[happy]` | `happy` | `happy` | SSML `cheerful` | SSML |
| `sad` | `[sad]` | `sad` | `sad` | SSML `sad` | SSML |
| `angry` | `[angry]` | `angry` | `angry` | SSML `angry` | SSML |
| `calm` | `[calm]` | `calm` | `calm` | SSML `calm` | SSML |
| `excited` | `[excited]` | `excited` | `surprised` | SSML `excited` | SSML |
| `fearful` | — | `fearful` | `fearful` | SSML `fearful` | SSML |
| `neutral` | — | — | `fluent` | 默认 | 默认 |

不支持的组合静默忽略，不报错。

## 9. 错误模型

```go
var (
    ErrTransient  = errors.New("tts transient error")   // 网络或服务端临时故障，可重试
    ErrAuth       = errors.New("tts auth error")        // 认证失败
    ErrBadRequest = errors.New("tts bad request")       // 参数错误（不支持的格式、超长文本等）
)

type APIError struct {
    Provider   string // 如 "aliyun"、"volcengine"
    StatusCode int
    Code       string // provider 原生错误码
    Message    string
    RequestID  string
    Retryable  bool
    Cause      error
}

func (e *APIError) Error() string { ... }
func (e *APIError) Unwrap() error { ... }
```

## 10. 与 TTSProcessor 的集成

TTSProcessor（`internal/audio/tts.go`）当前用 `tts.Config` 创建 Provider，用 `tts.SynthesisOptions` 传动态参数。改为 SDK 后：

```go
// 旧：
proc.Write(text, tts.SynthesisOptions{Emotion: "happy", Rate: 1.2})

// 新：
proc.Write(text, tts.SynthesizeRequest{
    Speech: tts.SpeechParams{Emotion: "happy", Speed: 1.2},
})
```

`TTSProcessor` 在 `Start()` 时做能力检测：

```go
func (p *ttsProcessor) Start(ctx context.Context) error {
    p.streamingProv, _ = p.cfg.Synthesizer.(tts.StreamingSynthesizer)
    p.warmableProv, _  = p.cfg.Synthesizer.(tts.WarmableProvider)
    // ...
}
```

`Write()` 构建 `SynthesizeRequest` 传给 dispatcher；dispatcher 内部 `StartSynthesis` 时传入完整 request。

## 11. 实施顺序

### Phase 1：公共类型定义

- 新增 `internal/provider/tts/sdk/request.go`：`TextInput`、`VoiceSelection`、`AudioConfig`、`SpeechParams`、`SynthesizeRequest`
- 新增 `internal/provider/tts/sdk/result.go`：`SynthesizeResult`
- 新增 `internal/provider/tts/sdk/audio.go`：`AudioFormat` 类型 + 常量
- 新增 `internal/provider/tts/sdk/interface.go`：`Synthesizer`、`StreamingSynthesizer`、`SynthesisStream`
- 新增 `internal/provider/tts/sdk/capabilities.go`：`Capabilities`、`ModelCapability`、`Feature`
- 新增 `internal/provider/tts/sdk/normalize.go`：参数归一化工具函数

### Phase 2：重构 Config 与注册

- 修改 `factory.go`：`Config` 精简、新增 `ProviderMeta.Features`、`ProviderMeta.Description`、`VoiceInfo.Emotions`
- 保留 `SentenceBoundary` 和 `SentenceBoundaries()` 接口方法

### Phase 3：阿里云适配器迁移

- `aliyun/dashscope.go`：`DashScopeProvider` 改实现 `Synthesizer` + `StreamingSynthesizer`
- 内部新增 `dashscopeTranslator`：`SynthesizeRequest` → run-task/continue-task/finish-task 消息
- 现有 `Config.Workspace`、`Config.EnableDataInspection` 改为从 `Config.Extra` 读取
- 情绪映射：`SpeechParams.Emotion` → 阿里云行内标签

### Phase 4：TTSProcessor 适配

- `audio/tts.go`：`Write` 签名改为接收 `SynthesizeRequest`，内部透传
- `audio/tts_stage.go`：构建 `SynthesizeRequest` 填入 `Write`
- `config/config.go`：`TTSConfig` 对齐新 `Config` 结构

### Phase 5：新 provider 接入

- `volcengine/adapter.go`：火山引擎适配器
- `minimax/adapter.go`：MiniMax 适配器
- 每家需提供 `Register` 调用 + `ProviderMeta` + `Capabilities` 实现

## 12. 兼容策略

**保留现有的接口签名 3 个 release**（通过 type alias 过渡）：

```go
// factory.go - 过渡期兼容
type Provider = Synthesizer

// aliyun/dashscope.go - DashScopeProvider 同时实现旧 Synthesize(ctx, text, opts)
// 和新 Synthesize(ctx, req)，旧方法内部构建 SynthesizeRequest 调用新方法
func (p *DashScopeProvider) Synthesize(ctx context.Context, text string, opts SynthesisOptions) (io.ReadCloser, error) {
    return p.synthesize(ctx, SynthesizeRequest{
        Input:  TextInput{Text: text},
        Speech: SpeechParams{Emotion: opts.Emotion, Speed: opts.Rate},
    })
}
```

## 13. 明确不做的事情

- **不新增 SentenceBoundaries 的替代机制**。当前 channel 模式足够低延迟，保持不动。
- **不把 SSML 生成放进 SDK**。调用方自行决定是否使用 SSML，SDK 只负责透传 `TextType=ssml`。
- **不支持长文本异步合成**（阿里云 ISI、MiniMax Async T2A）。场景差异大，后续独立设计。
- **不支持声音克隆/设计的统一抽象**。各家 API 差异过大，建议每家单独封装为 tool 级别能力。
- **不引入第三方 SDK 依赖**。当前阿里云 WebSocket 用 `gorilla/websocket`，保持不变。Google 的 gRPC client 按需引入官方 SDK。
- **不在公共层做文本预处理**（分句、emotion 标签解析）。这些都是 TTSProcessor 的职责。

## 14. 参考

- [阿里云 CosyVoice WebSocket API](https://help.aliyun.com/en/model-studio/cosyvoice-websocket-api)
- [火山引擎 TTS WebSocket API](https://www.volcengine.com/docs/6561/1329505)
- [Azure Text to Speech REST API](https://learn.microsoft.com/en-us/azure/ai-services/speech-service/rest-text-to-speech)
- [Google Cloud Text-to-Speech API](https://cloud.google.com/text-to-speech/docs/reference/rest)
- [MiniMax T2A API](https://platform.minimax.io/docs/api-reference/speech-t2a-http)
