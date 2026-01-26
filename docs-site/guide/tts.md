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

type Provider interface {
    Start(ctx context.Context, cfg Config) (Stream, error)
}

type Stream interface {
    WriteTextChunk(ctx context.Context, text string) error
    Close(ctx context.Context) error
    AudioReader() io.ReadCloser
}
```

## DashScopeProvider 使用流程

1. 调用 `provider.Start(ctx, cfg)` 建立连接并返回 Stream
2. 调用 `stream.WriteTextChunk(ctx, text)` 发送文本片段（建议已分句）
3. 通过 `stream.AudioReader()` 读取音频流并播放或保存
4. 调用 `stream.Close(ctx)` 触发 finish-task 并等待任务完成

## 错误类型

```go
var (
    ErrTransient  = errors.New("tts transient error")
    ErrAuth       = errors.New("tts auth error")
    ErrBadRequest = errors.New("tts bad request")
)
```

## 使用示例（调用方使用 segmenter 分句）

```go
provider := tts.NewDashScopeProvider()
stream, _ := provider.Start(ctx, cfg)

seg := text.NewSegmenter(120)

for _, chunk := range llmChunks {
    for _, sentence := range seg.Feed(chunk) {
        stream.WriteTextChunk(ctx, sentence)
    }
}
if last := seg.Flush(); last != "" {
    stream.WriteTextChunk(ctx, last)
}

go playAudio(stream.AudioReader())
stream.Close(ctx)
```

## CLI 示例

```bash
DASHSCOPE_API_KEY=... go run ./cmd/tts -text "你好。" -player ffplay
DASHSCOPE_API_KEY=... go run ./cmd/tts -text "..." -output out.mp3
```

## 注意事项

- 调用方负责分句（建议使用 `text.Segmenter`），TTS 仅做流式转发。
- `WriteTextChunk` 必须等 `task-started` 事件返回后才能发送。
- 音频流通过 `AudioReader()` 以二进制形式逐帧输出。
- 务必调用 `Close`，否则可能收不到最后一段语音。
