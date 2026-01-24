# 实时语音识别 (ASR)

基于阿里云百炼 DashScope 实时 ASR 的 Go 实现，支持从麦克风采集音频并实时转换为文字。

## 功能概述

- 通过 WebSocket 协议接入阿里云百炼实时语音识别服务
- 支持从麦克风实时采集 16kHz/单声道/16-bit PCM 音频
- 实时输出 partial/final 识别结果
- 支持参数化配置（模型、端点、语种提示、标点策略等）
- 提供可扩展的 `Recognizer` 接口，便于后续接入其他厂商

## 架构设计

```
┌─────────────┐
│   麦克风     │  16kHz 单声道 PCM
└──────┬──────┘
       │
       ▼
┌──────────────────┐
│  音频采集层      │  PortAudio (gordonklaus/portaudio)
└──────┬───────────┘
       │
       ▼
┌──────────────────┐
│  音频处理层      │  帧切分 (3200 samples/帧)
└──────┬───────────┘
       │
       ▼
┌────────────────────┐
│  ASR 流式调用层    │  DashScope WebSocket (gorilla/websocket)
└──────┬─────────────┘
       │
       ▼
┌──────────────────┐
│  文本输出层      │  partial/final 结果回调
└──────────────────┘
```

## 项目结构

```
internal/asr/
├── recognizer.go       # 通用接口定义 (Recognizer / Config / Result)
└── dashscope.go        # DashScope WebSocket 实现

cmd/asr/
└── main.go            # 麦克风实时转写 CLI
```

## 接口设计

### Recognizer 接口

```go
type Recognizer interface {
    Start(ctx context.Context) error
    SendAudio(ctx context.Context, data []byte) error
    Finish(ctx context.Context) error
    Close() error
    OnResult(handler func(Result))
}
```

### Config 配置

```go
type Config struct {
    APIKey                     string
    Endpoint                   string // 默认: wss://dashscope.aliyuncs.com/api-ws/v1/inference
    Model                      string // 默认: fun-asr-realtime
    Format                     string // 默认: pcm
    SampleRate                 int    // 默认: 16000
    VocabularyID               string
    SemanticPunctuationEnabled *bool // 语义断句 vs VAD 断句
    MaxSentenceSilence         int    // VAD 静音阈值 (ms)
    MultiThresholdModeEnabled  *bool
    Heartbeat                  *bool
    LanguageHints              []string // 语言提示 (zh/en/ja)
}
```

### Result 结果

```go
type Result struct {
    Text          string  // 识别文本
    IsFinal       bool    // 是否为最终结果 (sentence_end)
    BeginTimeMs   int64   // 开始时间
    EndTimeMs     *int64  // 结束时间 (中间结果为 nil)
    UsageDuration *int    // 计费时长 (秒)
}
```

## 使用说明

### CLI 运行

```bash
export DASHSCOPE_API_KEY=sk-xxx
go run ./cmd/asr
```

可选参数

```bash
go run ./cmd/asr \
    -model fun-asr-realtime \
    -endpoint wss://dashscope-intl.aliyuncs.com/api-ws/v1/inference \
    -sample-rate 16000 \
    -frames 3200 \
    -semantic-punctuation \
    -language-hints zh,en
```

参数说明

- `-model`: ASR 模型名称 (默认: fun-asr-realtime)
- `-endpoint`: WebSocket 端点 (可选)
- `-sample-rate`: 采样率 (Hz)
- `-frames`: 每次读取的帧数 (samples)
- `-semantic-punctuation`: 开启语义断句 (默认: false，使用 VAD 断句)
- `-language-hints`: 语言提示，逗号分隔 (例如: zh,en)

### 代码示例

```go
import "github.com/liuscraft/orion-x/internal/asr"

cfg := asr.Config{
    APIKey:    os.Getenv("DASHSCOPE_API_KEY"),
    Model:     "fun-asr-realtime",
    Format:    "pcm",
    SampleRate: 16000,
}

rec, err := asr.NewDashScopeRecognizer(cfg)
if err != nil {
    log.Fatal(err)
}

rec.OnResult(func(result asr.Result) {
    if result.IsFinal {
        log.Printf("final: %s", result.Text)
    } else {
        log.Printf("partial: %s", result.Text)
    }
})

ctx := context.Background()
if err := rec.Start(ctx); err != nil {
    log.Fatal(err)
}
defer rec.Close()

// 发送音频数据
rec.SendAudio(ctx, pcmData)

// 结束识别
rec.Finish(ctx)
```

## 实现细节

### 音频流规范

- 声道: 单声道
- 格式: 16-bit PCM (little-endian)
- 采样率: 16000 Hz
- 帧长: 3200 samples (约 200ms)

### WebSocket 交互时序

```
客户端                服务端
  |                      |
  |--- 连接 ------------->|
  |                      |
  |-- run-task --------->|
  |<-- task-started ----|
  |                      |
  |--[音频帧]---------->|
  |<-- result-generated-| (partial)
  |                      |
  |--[音频帧]---------->|
  |<-- result-generated-| (final)
  |                      |
  |-- finish-task ------>|
  |<-- task-finished ---|
  |                      |
  |--- 关连 ----------->|
```

### 断句策略

- **VAD 断句** (默认): `semantic_punctuation_enabled=false`
  - 响应快，适合交互场景
  - 通过静音阈值 (`max_sentence_silence`) 判定句子结束

- **语义断句**: `semantic_punctuation_enabled=true`
  - 准确度高，适合会议转写
  - 关闭 VAD，使用语义信息判定句子边界

## 依赖

- `github.com/gorilla/websocket`: WebSocket 客户端
- `github.com/gordonklaus/portaudio`: 麦克风采集

## macOS 权限

首次运行时需要在“系统设置 → 隐私与安全性 → 麦克风”中授权 Terminal 或 VS Code。

## 安全说明

- `DASHSCOPE_API_KEY` 敏感信息请勿提交到代码仓库
- 建议通过环境变量或配置管理工具管理密钥
- 如密钥泄露，请及时在阿里云控制台撤销并重新生成

## 后续扩展

- VAD 断句优化
- 自动标点增强
- 语种识别/切换
- 定制热词支持
- 多通道并发识别
- 其他厂商接入（通过 `Recognizer` 接口）
