package apikey

import (
	"errors"
	"strings"
)

// scope 是授权单位：`resource:action`，deny-by-default，不支持通配（§1 D5）。
//
// 值一经发布不得改变含义，只能废弃（Deprecated）：否则历史 key 的授权面会静默变化。
// 命名深度默认两段；只有当一个资源家族的动作超过约 15 个、或需要把有副作用的动作
// 拆出来时才允许第三段，且不允许超过三段（§5.3 兼容规则）。
const (
	ScopeAgentRead   = "agent:read"
	ScopeAgentWrite  = "agent:write"
	ScopeDeviceRead  = "device:read"
	ScopeDeviceWrite = "device:write"
	ScopeModelRead   = "model:read"
	ScopeModelWrite  = "model:write"
	ScopeDataRead    = "data:read"
	ScopeDataWrite   = "data:write"
	ScopeMCPRead     = "mcp:read"
	ScopeMCPWrite    = "mcp:write"
	ScopeMCPCall     = "mcp:call"
	ScopeBillingRead = "billing:read"
)

// 创建时的校验错误。HTTP 层按它们回 400（§5.3 的"管理面还有三类 400"）。
var (
	ErrNameRequired    = errors.New("apikey: name is required")
	ErrNameTooLong     = errors.New("apikey: name is too long")
	ErrNoScopes        = errors.New("apikey: at least one scope is required")
	ErrUnknownScope    = errors.New("apikey: unknown scope")
	ErrExpiresAtPast   = errors.New("apikey: expires_at must be in the future")
	ErrKeyLimitReached = errors.New("apikey: too many keys for this account")
)

// UnknownScopeError 带上被拒的 scope 名单：**不静默丢弃**，否则用户会以为自己
// 授了权（§5.3 的 400 表）。Deprecated 与 Unknown 分开列：前者是"曾经存在、
// 不再可授"，后者是"根本不认识"。
type UnknownScopeError struct {
	Unknown    []string
	Deprecated []string
}

func (e *UnknownScopeError) Error() string { return ErrUnknownScope.Error() }

// Unwrap 让 errors.Is(err, ErrUnknownScope) 继续成立。
func (e *UnknownScopeError) Unwrap() error { return ErrUnknownScope }

// ScopeInfo 是目录项。目录是服务端单一事实源：路由与前端都从它取（§5.3）。
type ScopeInfo struct {
	Value string `json:"value"` // "agent:write"，唯一标识，一经发布不变
	Group string `json:"group"` // 分组，只用于展示与批量选择，不参与授权判定
	Title string `json:"title"` // 中文短标题，控制台展示
	// Description 是"授予后会发生什么"的人话（要提示会扣费就写在这里，§5.3）。
	// 线上的字段名是 desc——契约以 §5.3 的 curl 示例为准。
	Description string `json:"desc"`
	Default     bool   `json:"default"`    // 是否进默认预设
	Deprecated  bool   `json:"deprecated"` // 不再挂新路由，但历史 key 仍可持有
}

// GroupInfo 是分组的展示名。分组本身不参与授权判定，但组名也该由服务端给，
// 否则前端要为每一个新组发版（FR-10 的同一条理由）。
type GroupInfo struct {
	Value string `json:"value"`
	Title string `json:"title"`
}

// PresetInfo 是创建 Key 时的预设组合（§1 N8）：只是 scope 列表的快捷方式，
// 不引入新的授权语义。落库只存展开后的显式列表，所以改预设不影响历史 key。
// 字段名对齐 §5.3 的 curl 示例（name/title/scopes）；description 是增量字段，
// 供控制台做"描述提示"。
type PresetInfo struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Scopes      []string `json:"scopes"`
}

// catalog 是 P1 全量 scope（§5.3 的 scope 表）。只增不改：改分组/标题/描述随时可以，
// 改 Value 或它的含义不行。财务口径与"会不会扣费"不进这里——那是计费侧的事实，
// 计费可以整个关掉、价格会变，写进凭证目录就等于写下一份注定过期的东西。
var catalog = []ScopeInfo{
	{
		Value: ScopeAgentRead, Group: "agent", Title: "查看智能体", Default: true,
		Description: "查看智能体、智能体配置与广场模板",
	},
	{
		Value: ScopeAgentWrite, Group: "agent", Title: "管理智能体",
		Description: "创建、修改、删除智能体，从广场模板创建智能体",
	},
	{
		Value: ScopeDeviceRead, Group: "device", Title: "查看设备", Default: true,
		Description: "查看智能体下的设备与 Telegram 通道状态（不回显通道凭据）",
	},
	{
		Value: ScopeDeviceWrite, Group: "device", Title: "管理设备",
		Description: "注册、删除设备，设置或解除 Telegram 通道",
	},
	{
		Value: ScopeModelRead, Group: "model", Title: "查看模型与音色", Default: true,
		Description: "查看供应商、模型、系统音色与可用资源列表",
	},
	{
		Value: ScopeModelWrite, Group: "model", Title: "管理模型与音色",
		Description: "管理供应商、模型与音色；含音色克隆，会产生费用",
	},
	{
		Value: ScopeDataRead, Group: "data", Title: "查看数据", Default: true,
		Description: "查看记忆、知识库、文档、会话记录与上传的资源",
	},
	{
		Value: ScopeDataWrite, Group: "data", Title: "管理数据",
		Description: "删除记忆，上传或删除知识库文档，调整知识库绑定，上传或删除资源",
	},
	{
		Value: ScopeMCPRead, Group: "mcp", Title: "查看 MCP 服务", Default: true,
		Description: "查看 MCP 市场、自己的 MCP 服务与工具清单",
	},
	{
		Value: ScopeMCPWrite, Group: "mcp", Title: "管理 MCP 服务",
		Description: "增删改 MCP 服务，调整智能体的 MCP 绑定",
	},
	{
		Value: ScopeMCPCall, Group: "mcp", Title: "调用 MCP 工具",
		Description: "测试连接并真正执行 MCP 工具；可能有副作用与费用",
	},
	{
		Value: ScopeBillingRead, Group: "billing", Title: "查看用量与余额", Default: true,
		Description: "查看余额、用量明细与当前生效的价格",
	},
}

// groups 是分组的展示顺序与名称。
var groups = []GroupInfo{
	{Value: "agent", Title: "智能体"},
	{Value: "device", Title: "设备"},
	{Value: "model", Title: "模型与音色"},
	{Value: "data", Title: "数据"},
	{Value: "mcp", Title: "MCP 服务"},
	{Value: "billing", Title: "计费"},
}

// presets 是内置预设。"只读"的集合刻意等于目录里 Default 标记的那批（有测试钉住），
// 这样"默认勾选什么"和"只读预设给什么"不会漂移成两套说法。
var presets = []PresetInfo{
	{
		Name: "readonly", Title: "只读",
		Description: "只能查看智能体、设备、数据与用量，改不了任何配置",
		Scopes: []string{
			ScopeAgentRead, ScopeDeviceRead, ScopeModelRead,
			ScopeDataRead, ScopeMCPRead, ScopeBillingRead,
		},
	},
	{
		Name: "integration", Title: "集成",
		Description: "在脚本或流水线里管理智能体、设备与数据；不含音色克隆等会产生费用的动作",
		Scopes: []string{
			ScopeAgentRead, ScopeAgentWrite, ScopeDeviceRead, ScopeDeviceWrite,
			ScopeModelRead, ScopeDataRead, ScopeDataWrite,
			ScopeMCPRead, ScopeMCPWrite, ScopeMCPCall,
		},
	},
}

// Catalog 返回 scope 目录副本；调用方改不动服务端的事实源。
func Catalog() []ScopeInfo {
	out := make([]ScopeInfo, len(catalog))
	copy(out, catalog)
	return out
}

// Groups 返回分组展示名。
func Groups() []GroupInfo {
	out := make([]GroupInfo, len(groups))
	copy(out, groups)
	return out
}

// Presets 返回预设组合。
func Presets() []PresetInfo {
	out := make([]PresetInfo, len(presets))
	for i, p := range presets {
		out[i] = p
		out[i].Scopes = append([]string(nil), p.Scopes...)
	}
	return out
}

// Missing 返回 required 里未被 granted 覆盖的 scope（AND 语义，deny-by-default）。
// 空 required 表示"这条路由没声明要求"——那不是放行，调用方必须自己决定（见 §7 V3）。
func Missing(granted, required []string) []string {
	have := make(map[string]struct{}, len(granted))
	for _, s := range granted {
		have[s] = struct{}{}
	}
	var missing []string
	for _, s := range required {
		if _, ok := have[s]; !ok {
			missing = append(missing, s)
		}
	}
	return missing
}

// HasAll 是 Missing 的布尔形式。
func HasAll(granted, required []string) bool {
	return len(Missing(granted, required)) == 0
}

// NormalizeScopes 校验并归一化创建时的显式 scope 列表：去空、去重、按目录顺序排列。
// 未知或已废弃的 scope 一律拒绝——废弃的意思是"不再授予新 key"；被拒的值全部
// 收集进 UnknownScopeError，让调用方能一次告诉用户改哪几个。
func NormalizeScopes(scopes []string) ([]string, error) {
	idx := make(map[string]ScopeInfo, len(catalog))
	for _, s := range catalog {
		idx[s.Value] = s
	}

	seen := make(map[string]struct{}, len(scopes))
	var rejected UnknownScopeError
	for _, raw := range scopes {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		info, ok := idx[v]
		if !ok {
			rejected.Unknown = append(rejected.Unknown, v)
			continue
		}
		if info.Deprecated {
			rejected.Deprecated = append(rejected.Deprecated, v)
			continue
		}
		seen[v] = struct{}{}
	}
	if len(rejected.Unknown) > 0 || len(rejected.Deprecated) > 0 {
		return nil, &rejected
	}
	if len(seen) == 0 {
		return nil, ErrNoScopes
	}

	out := make([]string, 0, len(seen))
	for _, info := range catalog { // 目录顺序：列表与创建响应的展示顺序稳定
		if _, ok := seen[info.Value]; ok {
			out = append(out, info.Value)
		}
	}
	return out, nil
}

// ExpandPreset 把预设名展开成显式 scope 列表。展开在服务端完成、落库只存展开结果：
// 存"预设名"的话，预设定义一改，历史 key 的权限就会静默变化（§5.3）。
func ExpandPreset(name string) ([]string, bool) {
	for _, p := range presets {
		if p.Name == strings.TrimSpace(name) {
			return append([]string(nil), p.Scopes...), true
		}
	}
	return nil, false
}
