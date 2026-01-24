package text

// MarkdownFilter Markdown过滤器接口
type MarkdownFilter interface {
	Filter(text string) string
}

// MarkdownFilterConfig 配置
type MarkdownFilterConfig struct {
	// 是否移除加粗标记
	RemoveBold bool
	// 是否移除代码块
	RemoveCodeBlock bool
	// 是否移除链接
	RemoveLink bool
	// 是否移除标题
	RemoveHeading bool
}

// DefaultMarkdownFilterConfig 默认配置
func DefaultMarkdownFilterConfig() *MarkdownFilterConfig {
	return &MarkdownFilterConfig{
		RemoveBold:      true,
		RemoveCodeBlock: true,
		RemoveLink:      true,
		RemoveHeading:   true,
	}
}
