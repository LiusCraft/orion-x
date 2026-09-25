package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/liuscraft/orion-x/internal/apikey"
)

// ScopeTable 是"哪条路由要哪个 scope"的中央声明（docs/api-key-design.md §5.1）。
//
// 键是 "METHOD /gin/pattern"（如 "GET /api/voicebots/:id"），值是 AND 关系的一组
// scope。做成中央表而不是写在每条路由上，是为了让"这个 scope 到底管什么"一眼可查；
// 代价是表与真实路由会漂移，所以有一条双向覆盖性测试钉住（§7 V3）。
type ScopeTable map[string][]string

// Required 返回某条路由要求的 scope；第二个返回值是"这条路由被声明过"。
func (t ScopeTable) Required(method, path string) ([]string, bool) {
	scopes, ok := t[method+" "+path]
	return scopes, ok
}

// RouteScopes 返回中央表的副本。调用方拿到的改动不影响服务端的事实源。
func RouteScopes() ScopeTable {
	out := make(ScopeTable, len(routeScopes))
	for k, v := range routeScopes {
		out[k] = append([]string(nil), v...)
	}
	return out
}

// routeScopes 是 P1 全量声明，与 §5.3 的 scope 表逐行对应。
//
// 加一条路由时这里必须同步加一行，否则覆盖性测试会红——先看到测试红，
// 而不是先上生产被人用一把只读 key 调了写接口。
var routeScopes = ScopeTable{
	// ── 智能体 ──
	"GET /api/voicebots":                {apikey.ScopeAgentRead},
	"POST /api/voicebots":               {apikey.ScopeAgentWrite},
	"GET /api/voicebots/:id":            {apikey.ScopeAgentRead},
	"PUT /api/voicebots/:id":            {apikey.ScopeAgentWrite},
	"DELETE /api/voicebots/:id":         {apikey.ScopeAgentWrite},
	"GET /api/agent-templates/system":   {apikey.ScopeAgentRead},
	"GET /api/agent-templates/:id":      {apikey.ScopeAgentRead},
	"POST /api/agent-templates/:id/use": {apikey.ScopeAgentWrite},

	// ── 设备与通道 ──
	"GET /api/voicebots/:id/devices":                           {apikey.ScopeDeviceRead},
	"POST /api/voicebots/:id/devices":                          {apikey.ScopeDeviceWrite},
	"DELETE /api/voicebots/:id/devices/:did":                   {apikey.ScopeDeviceWrite},
	"PUT /api/voicebots/:id/devices/:did/channels/telegram":    {apikey.ScopeDeviceWrite},
	"DELETE /api/voicebots/:id/devices/:did/channels/telegram": {apikey.ScopeDeviceWrite},

	// ── 供应商 / 模型 / 音色（音色克隆会扣费，所以它要 model:write） ──
	"GET /api/providers":                 {apikey.ScopeModelRead},
	"POST /api/providers":                {apikey.ScopeModelWrite},
	"GET /api/providers/slugs":           {apikey.ScopeModelRead},
	"GET /api/providers/:id":             {apikey.ScopeModelRead},
	"PUT /api/providers/:id":             {apikey.ScopeModelWrite},
	"DELETE /api/providers/:id":          {apikey.ScopeModelWrite},
	"GET /api/models":                    {apikey.ScopeModelRead},
	"POST /api/models":                   {apikey.ScopeModelWrite},
	"GET /api/models/types":              {apikey.ScopeModelRead},
	"GET /api/models/voice-cloning":      {apikey.ScopeModelRead},
	"GET /api/models/:id":                {apikey.ScopeModelRead},
	"PUT /api/models/:id":                {apikey.ScopeModelWrite},
	"DELETE /api/models/:id":             {apikey.ScopeModelWrite},
	"GET /api/models/:id/voices":         {apikey.ScopeModelRead},
	"POST /api/models/:id/voices":        {apikey.ScopeModelWrite},
	"POST /api/models/:id/voices/clone":  {apikey.ScopeModelWrite},
	"GET /api/models/:id/voices/:vid":    {apikey.ScopeModelRead},
	"PUT /api/models/:id/voices/:vid":    {apikey.ScopeModelWrite},
	"DELETE /api/models/:id/voices/:vid": {apikey.ScopeModelWrite},
	"GET /api/voices/system":             {apikey.ScopeModelRead},
	"GET /api/voices/mine":               {apikey.ScopeModelRead},
	"GET /api/available-resources":       {apikey.ScopeModelRead},
	"GET /api/languages":                 {apikey.ScopeModelRead},
	"GET /api/languages/:code":           {apikey.ScopeModelRead},

	// ── 数据：记忆 / 知识库 / 资源 ──
	"GET /api/data/memory/agents":                                         {apikey.ScopeDataRead},
	"GET /api/data/memory/agents/:agent_id/devices":                       {apikey.ScopeDataRead},
	"GET /api/data/memory/devices/:device_id/entries":                     {apikey.ScopeDataRead},
	"DELETE /api/data/memory/:id":                                         {apikey.ScopeDataWrite},
	"GET /api/data/knowledge/knowledge_bases":                             {apikey.ScopeDataRead},
	"GET /api/data/knowledge/knowledge_bases/:kb_id":                      {apikey.ScopeDataRead},
	"GET /api/data/knowledge/knowledge_bases/:kb_id/search":               {apikey.ScopeDataRead},
	"DELETE /api/data/knowledge/knowledge_bases/:kb_id":                   {apikey.ScopeDataWrite},
	"GET /api/data/knowledge/knowledge_bases/:kb_id/documents":            {apikey.ScopeDataRead},
	"POST /api/data/knowledge/knowledge_bases/:kb_id/documents":           {apikey.ScopeDataWrite},
	"POST /api/data/knowledge/knowledge_bases/:kb_id/documents/url":       {apikey.ScopeDataWrite},
	"GET /api/data/knowledge/documents/:doc_id/status":                    {apikey.ScopeDataRead},
	"DELETE /api/data/knowledge/documents/:doc_id":                        {apikey.ScopeDataWrite},
	"POST /api/data/knowledge/documents/:doc_id/retry":                    {apikey.ScopeDataWrite},
	"GET /api/data/knowledge/bots/:bot_id/knowledge_bases/bound":          {apikey.ScopeDataRead},
	"POST /api/data/knowledge/bots/:bot_id/knowledge_bases/bind":          {apikey.ScopeDataWrite},
	"DELETE /api/data/knowledge/bots/:bot_id/knowledge_bases/:kb_id/bind": {apikey.ScopeDataWrite},
	"GET /api/data/knowledge/bots/:bot_id/knowledge_bases":                {apikey.ScopeDataRead},
	"POST /api/data/knowledge/bots/:bot_id/knowledge_bases":               {apikey.ScopeDataWrite},
	"POST /api/assets":        {apikey.ScopeDataWrite},
	"GET /api/assets":         {apikey.ScopeDataRead},
	"GET /api/assets/:id":     {apikey.ScopeDataRead},
	"GET /api/assets/:id/url": {apikey.ScopeDataRead},
	"DELETE /api/assets/:id":  {apikey.ScopeDataWrite},
	"GET /api/sessions":       {apikey.ScopeDataRead},

	// ── MCP：绑定关系按"被改动的资源"归到 mcp ──
	"GET /api/mcp/market":                            {apikey.ScopeMCPRead},
	"GET /api/mcp/servers":                           {apikey.ScopeMCPRead},
	"POST /api/mcp/servers":                          {apikey.ScopeMCPWrite},
	"GET /api/mcp/servers/:serverID":                 {apikey.ScopeMCPRead},
	"PUT /api/mcp/servers/:serverID":                 {apikey.ScopeMCPWrite},
	"DELETE /api/mcp/servers/:serverID":              {apikey.ScopeMCPWrite},
	"GET /api/voicebots/:id/mcps":                    {apikey.ScopeMCPRead},
	"POST /api/voicebots/:id/mcps":                   {apikey.ScopeMCPWrite},
	"DELETE /api/voicebots/:id/mcps/:serverID":       {apikey.ScopeMCPWrite},
	"PATCH /api/voicebots/:id/mcps/:serverID/toggle": {apikey.ScopeMCPWrite},
	// 会真的执行外部工具，可能有副作用与成本：不与 mcp:write 合并。
	"POST /api/mcp/test-connection": {apikey.ScopeMCPCall},
	"POST /api/mcp/list-tools":      {apikey.ScopeMCPCall},
	"POST /api/mcp/call-tool":       {apikey.ScopeMCPCall},

	// ── 计费：key 只能看，不能动余额 ──
	"GET /api/billing/summary":        {apikey.ScopeBillingRead},
	"GET /api/billing/usage":          {apikey.ScopeBillingRead},
	"GET /api/billing/usage-by-model": {apikey.ScopeBillingRead},
	"GET /api/billing/prices":         {apikey.ScopeBillingRead},
}

// unreachableRoute 描述"API key 结构上到不了"的路由：它们压根没挂 middleware.Auth
// （只挂 JWT / RequireAdmin），所以没有 scope 可标，也不该出现在 scope 表里。
type unreachableRoute struct {
	// method 为空表示任何方法。
	method string
	// path 是前缀：匹配自身或以它 + "/" 开头的路由。
	path string
}

// keyUnreachableRoutes 与 §5.3 的"key 不可达的路由"一一对应。
//
// 两类：账号与凭证安全（改密码、绑定、签发凭证——不接受机器凭证），
// 以及动钱的动作（充值、退款、定价、调额）。两者都是"结构保证"，
// 不是靠 scope 判出来的：key 那条路连中间件都到不了。
var keyUnreachableRoutes = []unreachableRoute{
	{path: "/api/auth"},     // 改密码、绑邮箱、解绑 OAuth
	{path: "/api/api-keys"}, // 凭证管理：机器凭证不能给自己或别人签发凭证
	{path: "/api/billing/items"},
	{method: "POST", path: "/api/billing/prices"},
	{path: "/api/billing/prices/:id"},
	{path: "/api/billing/accounts"},
	{path: "/api/billing/ledger"},
	{path: "/api/billing/stats"},
	{path: "/api/billing/recharge"},
}

func isKeyUnreachable(method, path string) bool {
	for _, r := range keyUnreachableRoutes {
		if r.method != "" && r.method != method {
			continue
		}
		if path == r.path || strings.HasPrefix(path, r.path+"/") {
			return true
		}
	}
	return false
}

// ValidateCoverage 做 §7 V3 的双向覆盖性检查，返回所有问题（空 = 通过）：
//
//   - 正向：每条 /api/* 路由要么在 scope 表里（且声明了非空、目录里存在的 scope），
//     要么在"key 不可达"白名单里；
//   - 反向：目录里每个 scope 要么被至少一条路由使用，要么标了 Deprecated。
//
// 两个方向任一不满足都是越权或"给了但没生效"的温床，所以它跑在测试里而不是 review 里。
func ValidateCoverage(routes []gin.RouteInfo) []string {
	var problems []string

	used := make(map[string]bool, len(routeScopes))
	for spec, scopes := range routeScopes {
		if len(scopes) == 0 {
			problems = append(problems, "scope table: "+spec+" declares no scope")
		}
		for _, s := range scopes {
			used[s] = true
			if !catalogHas(s) {
				problems = append(problems, "scope table: "+spec+" requires unknown scope "+s)
			}
		}
	}

	for _, route := range routes {
		if !strings.HasPrefix(route.Path, "/api/") {
			continue
		}
		if _, declared := routeScopes[route.Method+" "+route.Path]; declared {
			continue
		}
		if isKeyUnreachable(route.Method, route.Path) {
			continue
		}
		problems = append(problems, "route "+route.Method+" "+route.Path+" has no required scope and is not in the key-unreachable list")
	}

	for _, scope := range apikey.Catalog() {
		if scope.Deprecated || used[scope.Value] {
			continue
		}
		problems = append(problems, "catalog scope "+scope.Value+" is used by no route and is not deprecated")
	}

	return problems
}

func catalogHas(value string) bool {
	for _, s := range apikey.Catalog() {
		if s.Value == value {
			return true
		}
	}
	return false
}
