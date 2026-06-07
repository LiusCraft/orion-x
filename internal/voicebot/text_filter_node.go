package voicebot

import textutil "github.com/liuscraft/orion-x/internal/text"

type TextFilterNode interface {
	Process(text string) string
}

type speechTextFilterNode struct{}

func NewTextFilterNode() TextFilterNode {
	return &speechTextFilterNode{}
}

func (n *speechTextFilterNode) Process(text string) string {
	return textutil.RemoveEmotionTags(textutil.FilterMarkdown(text))
}
