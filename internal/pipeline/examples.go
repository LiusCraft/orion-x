package pipeline

import (
	"context"
	"strings"
)

// TextFilterStage 文本过滤 Stage（示例）
type TextFilterStage struct {
	*BaseStage
}

// NewTextFilterStage 创建文本过滤 Stage
func NewTextFilterStage() Stage {
	return &TextFilterStage{
		BaseStage: NewBaseStage("text_filter"),
	}
}

// Process 处理文本消息，过滤特殊标记
func (s *TextFilterStage) Process(ctx context.Context, input <-chan Message) <-chan Message {
	output := make(chan Message)

	go func() {
		defer close(output)

		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-input:
				if !ok {
					return
				}

				// 只处理文本类型消息
				if msg.Type == MessageTypeData {
					if text, ok := msg.Payload.(string); ok {
						// 过滤 <metadata> 标签
						filtered := s.filterMetadataTags(text)
						msg.Payload = filtered
					}
				}

				select {
				case output <- msg:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return output
}

func (s *TextFilterStage) filterMetadataTags(text string) string {
	// 简单实现：移除 <metadata>...</metadata> 标签
	result := text
	for {
		start := strings.Index(result, "<metadata>")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "</metadata>")
		if end == -1 {
			break
		}
		result = result[:start] + result[start+end+len("</metadata>"):]
	}
	return result
}

// EmotionExtractorStage 情感提取 Stage（示例）
type EmotionExtractorStage struct {
	*BaseStage
}

// NewEmotionExtractorStage 创建情感提取 Stage
func NewEmotionExtractorStage() Stage {
	return &EmotionExtractorStage{
		BaseStage: NewBaseStage("emotion_extractor"),
	}
}

// Process 从文本中提取情感元数据
func (s *EmotionExtractorStage) Process(ctx context.Context, input <-chan Message) <-chan Message {
	output := make(chan Message)

	go func() {
		defer close(output)

		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-input:
				if !ok {
					return
				}

				// 只处理文本类型消息
				if msg.Type == MessageTypeData {
					if text, ok := msg.Payload.(string); ok {
						emotion := s.extractEmotion(text)
						if emotion != "" {
							msg.Metadata.Emotion = emotion
						}
					}
				}

				select {
				case output <- msg:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return output
}

func (s *EmotionExtractorStage) extractEmotion(text string) string {
	// 简单实现：从 <emotion>tag</emotion> 提取
	start := strings.Index(text, "<emotion>")
	if start == -1 {
		return ""
	}
	end := strings.Index(text[start:], "</emotion>")
	if end == -1 {
		return ""
	}
	return text[start+len("<emotion>") : start+end]
}
