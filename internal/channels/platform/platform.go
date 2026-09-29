// Package platform 是通道平台的注册表。每个平台在这里声明自己的标识、能力与
// 配置字段 Schema；manager 的配置接口（GET /api/channels、.../channels/:platform）
// 与数据面的通道实现共用这一份声明，新增平台只需要加一条 descriptor。
package platform

import (
	"fmt"
	"sort"
	"strings"

	"github.com/liuscraft/orion-x/internal/channels"
)

// 平台标识。写入 device_channels.platform、出现在管理面与内部下发端点的路径里，
// 也是数据面通道拉取配置时用的键；改名等于数据迁移。
const (
	Telegram = "telegram"
	WeCom    = "wecom"
)

// maxFieldValueBytes 是单个配置值的长度上限（凭证类字段都是短字符串）。
const maxFieldValueBytes = 256

// Field 是平台的一个配置项。
type Field struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
	// Secret 为真时，公网响应只回掩码，明文只出现在内部下发端点。
	Secret bool   `json:"secret"`
	Hint   string `json:"hint,omitempty"`
}

// Descriptor 描述一个通道平台：它是什么、能做什么、要配哪些字段。
type Descriptor struct {
	Name         string                `json:"name"`
	DisplayName  string                `json:"display_name"`
	Type         channels.ChannelType  `json:"type"`
	Capabilities []channels.Capability `json:"capabilities"`
	Fields       []Field               `json:"fields"`
	// QRBinding 为真表示平台支持「扫码开通」：控制台在手动表单之外显示扫码入口。
	// 运行时是否可用还要看 manager 是否装配了 Binder（channels.qr_binding.enabled）。
	QRBinding bool `json:"qr_binding,omitempty"`
}

// registry 是全部已接入的平台。顺序即管理面展示顺序。
var registry = []Descriptor{
	{
		Name:        Telegram,
		DisplayName: "Telegram Bot",
		Type:        channels.ChannelPolling,
		Capabilities: []channels.Capability{
			channels.CapText,
			channels.CapTextStream,
			channels.CapVoiceFile,
		},
		Fields: []Field{
			{Key: "bot_token", Label: "Bot Token", Required: true, Secret: true, Hint: "在 @BotFather 创建机器人后获取"},
		},
	},
	{
		Name:        WeCom,
		DisplayName: "企业微信智能机器人",
		Type:        channels.ChannelClient,
		Capabilities: []channels.Capability{
			channels.CapText,
			channels.CapTextStream,
		},
		QRBinding: true,
		Fields: []Field{
			{Key: "bot_id", Label: "Bot ID", Required: true, Hint: "智能机器人开启「API 模式 → 长连接」后获取"},
			{Key: "bot_secret", Label: "Secret", Required: true, Secret: true, Hint: "长连接专用密钥，与回调模式的 Token/EncodingAESKey 不同"},
		},
	},
}

// All 返回所有已注册平台，按名称排序，调用方拿到的是副本。
func All() []Descriptor {
	out := make([]Descriptor, len(registry))
	copy(out, registry)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Get 按标识查平台。
func Get(name string) (Descriptor, bool) {
	for _, d := range registry {
		if d.Name == name {
			return d, true
		}
	}
	return Descriptor{}, false
}

// Validate 校验一份配置：必填字段非空、不含未知字段、单值不超长。
// 错误信息直接回给调用方，不包装。
func (d Descriptor) Validate(config map[string]string) error {
	known := make(map[string]struct{}, len(d.Fields))
	for _, f := range d.Fields {
		known[f.Key] = struct{}{}
		value := config[f.Key]
		if f.Required && strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", f.Key)
		}
		if len(value) > maxFieldValueBytes {
			return fmt.Errorf("%s is too long (max %d bytes)", f.Key, maxFieldValueBytes)
		}
	}
	for key := range config {
		if _, ok := known[key]; !ok {
			return fmt.Errorf("unknown field %q", key)
		}
	}
	return nil
}

// Mask 返回用于公网回显的副本：Secret 字段只保留掩码，其余原样。
func (d Descriptor) Mask(config map[string]string) map[string]string {
	if len(config) == 0 {
		return nil
	}
	masked := make(map[string]string, len(config))
	for _, f := range d.Fields {
		value, ok := config[f.Key]
		if !ok {
			continue
		}
		if f.Secret {
			value = MaskCredential(value)
		}
		masked[f.Key] = value
	}
	return masked
}

// MaskCredential 保留凭证首尾各 4 个字符，过短的一律打满掩码。
func MaskCredential(credential string) string {
	if len(credential) <= 8 {
		return "********"
	}
	return credential[:4] + "..." + credential[len(credential)-4:]
}
