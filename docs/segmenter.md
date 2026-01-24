# text.Segmenter

## 功能
将流式文本按规则切分为适合语音的句子片段，用于在发送给 TTS 前控制断句与延迟。

## 接口

```go
type Segmenter struct {
    MaxRunes int   // 单句最大字数兜底
    buffer   []rune
}

func NewSegmenter(maxRunes int) *Segmenter
func (s *Segmenter) Feed(text string) []string
func (s *Segmenter) Flush() string
```

## 行为规则

- **`Feed(text)`**：输入一段文本，返回“已切分的完整句子列表”。没有完整句子时返回空。
- **`Flush()`**：将剩余缓存强制吐出，用于会话结束时发完最后一段。
- **断句触发条件**：
  1. 遇到结束标点：`. ! ? ; \n 。！？；…`
  2. 缓存长度 ≥ `MaxRunes`
- 每次断句都会 `TrimSpace`。

## 使用示例

```go
seg := text.NewSegmenter(120) // 最大120字

for _, chunk := range llmChunks {
    for _, sentence := range seg.Feed(chunk) {
        tts.WriteTextChunk(ctx, sentence) // 完整句子立即发给 TTS
    }
}
if last := seg.Flush(); last != "" {
    tts.WriteTextChunk(ctx, last)
}
tts.Close(ctx)
```

## CLI 使用

```bash
go run ./cmd/tts -segmenter -segmenter-max 120 -text "你好。这里是第二句！"
```
