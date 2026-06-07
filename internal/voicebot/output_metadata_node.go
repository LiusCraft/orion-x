package voicebot

import textutil "github.com/liuscraft/orion-x/internal/text"

type OutputMetadata struct {
	Emotion string
}

type OutputMetadataNode interface {
	Process(text string) OutputMetadata
}

type emotionMetadataNode struct{}

func NewOutputMetadataNode() OutputMetadataNode {
	return &emotionMetadataNode{}
}

func (n *emotionMetadataNode) Process(text string) OutputMetadata {
	emotion := textutil.ExtractEmotionTag(text)
	if emotion == "" {
		return OutputMetadata{}
	}
	return OutputMetadata{Emotion: emotion}
}
