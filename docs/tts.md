# tts（DashScope WebSocket 实现）

## 功能
通过 WebSocket 调用 DashScope CosyVoice 进行流式语音合成，输出音频流供播放或存储。支持多厂商扩展（仅当前实现 DashScope）。

## 接口

```go
type Config struct {
    APIKey               string
    Endpoint             string
    Workspace            string
    Model                string   // 默认 cosyvoice-v3-flash
    Voice                string   // 默认 longanyang
    Format               string   // mp3/wav/pcm/opus
    SampleRate           int      // 默认 22050
    Volume               int      // 0-100
    Rate                 float64  // 语速
    Pitch                float64  // 音高
    EnableSSML           bool
    TextType             string   // 默认 PlainText
    EnableDataInspection *bool
}

// Provider 是基础接口（同步合成）。
type Provider interface {
    Synthesize(ctx context.Context, text string, opts SynthesisOptions) (io.ReadCloser, error)
}

// StreamingProvider 是流式接口，DashScopeProvider 实现此接口。
// TTSProcessor 通过类型断言检测并优先走此路径，不实现则回退 Synthesize。
type StreamingProvider interface {
    Provider
    StartSynthesis(ctx context.Context, opts SynthesisOptions) (SynthesisStream, error)
}

// SynthesisStream 是一次 TTS 会话，支持流式音频输出和非阻塞完成。
type SynthesisStream interface {
    WriteTextChunk(ctx context.Context, text string) error
    // Finish 发送 finish-task，立即返回，不等 task-finished。
    // task-finished 到达后 AudioReader() 返回 EOF。
    Finish(ctx context.Context) error
    // AudioReader 返回流式音频 reader，可在 Finish 前开始读。
    AudioReader() io.ReadCloser
    // Abort 立即中止 stream（打断场景）。
    Abort()
}
```

## DashScopeProvider 使用流程（StreamingProvider 路径）

1. 调用 `provider.StartSynthesis(ctx, opts)` 建立连接并返回 SynthesisStream
2. 调用 `stream.WriteTextChunk(ctx, text)` 发送文本片段（建议已分句）
3. 调用 `stream.Finish(ctx)` 触发 finish-task，**立即返回**
4. 通过 `stream.AudioReader()` 流式读取音频（EOF 表示 task-finished，全部音频到达）
5. 打断时调用 `stream.Abort()` 关闭连接和 audioBuf

## 错误类型

```go
var (
    ErrTransient  = errors.New("tts transient error")
    ErrAuth       = errors.New("tts auth error")
    ErrBadRequest = errors.New("tts bad request")
)
```

## 使用示例（TTSProcessor 内部流程，StreamingProvider 路径）

```go
import tts "github.com/liuscraft/orion-x/internal/provider/tts"

ttsProvider, _ := tts.NewProvider(tts.ProviderConfig{Type: tts.TypeAliyun})
sp := ttsProvider.(tts.StreamingProvider)

stream, _ := sp.StartSynthesis(ctx, tts.SynthesisOptions{})
_ = stream.WriteTextChunk(ctx, "你好！")
_ = stream.Finish(ctx)              // 立即返回，不等 task-finished

reader := stream.AudioReader()
// 流式读取：task-finished 后 reader 返回 EOF
io.Copy(audioSink, reader)
```

## 注意事项

- `TTSProcessor` 自动检测 `StreamingProvider` 并走流式路径，调用方无需手动选择接口。
- 分句在 `TTSProcessor` 内部（`sentenceSplitter`）完成，每句独立建立 stream。
- `Finish` 非阻塞，`AudioReader` 的 EOF 才是"该句音频全部到达"的信号。
- 打断时调用 `Abort`，会立即关闭 audioBuf（reader EOF）和 WebSocket 连接。

## 延迟优化（已实现）

### 优化 1：流式音频播放（减少 ~210ms）

**问题**：原路径 `Synthesize → io.ReadAll → OnChunk` 等 `task-finished` 后才播放。首帧 ~499ms，`task-finished` ~709ms，白等 210ms。

**实现**：`StreamingProvider.StartSynthesis` 返回 `SynthesisStream`，`Finish` 发完 finish-task 立即返回（不阻塞），`AudioReader()` 是流式 reader，`task-finished` 时自动 EOF。`TTSProcessor.playbackLoop` 收到 reader 后即开始流式读取（4096 字节/帧），每帧调用 `OnChunk`，PCM 格式天然支持按帧写入 PortAudio sink。

**预期收益**：首帧音频到达（~499ms）即开始播放，而不是等 task-finished（~709ms）。

### 优化 2：预连接 / task-start 预热（减少 ~130-180ms）

**问题**：每句话都新建 WebSocket 连接并等 `task-started`，约 130-180ms。

**实现**：`TTSProcessor` 内置容量为 1 的预热池（`warmCh`）。`dispatcher` 在把一个句子交给 worker 的同时，异步启动 `doWarm`（调用 `StartSynthesis` 预热下一个 stream）。下一个 worker 启动时先 `tryConsumeWarm`，opts key（`{emotion, rate}`）匹配则直接使用预热 stream，跳过 connect+task-started 阶段。打断时 `warmCtx` 取消，warmCh 中的 stream 被 `Abort`。

**预期收益**：相邻句子 opts 相同时，第 2 句及之后无需等待连接和 task-started。

### 实现细节

- `DashScopeProvider` 同时实现 `Provider`（`Synthesize`，向后兼容）和 `StreamingProvider`（`StartSynthesis`）。
- `markDone` 统一关闭 `audioBuf` 和 `conn`，避免多处 close。
- 打断检测：`streamChunk` 每帧间检查 `synthCtx.Done()`，快速退出并关闭 reader。
- drain 函数（`drainResultCh`/`drainPlaybackCh`）在打断时正确关闭 reader，避免 goroutine 泄漏。
