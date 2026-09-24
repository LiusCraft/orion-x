package main

import (
	"strings"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/liuscraft/orion-x/cmd/manager/handler"
	"github.com/liuscraft/orion-x/cmd/manager/middleware"
	_ "github.com/liuscraft/orion-x/docs/manager"
	"github.com/liuscraft/orion-x/internal/apikey"
	"github.com/liuscraft/orion-x/internal/assets"
	"github.com/liuscraft/orion-x/internal/billing"
	"github.com/liuscraft/orion-x/internal/billing/service"
	"github.com/liuscraft/orion-x/internal/knowledge"
	"github.com/liuscraft/orion-x/internal/store"
)

func newRouter(
	jwtSecret []byte,
	users *store.UserStore,
	bindings *store.OAuthBindingStore,
	voicebots *store.VoicebotStore,
	devices *store.DeviceStore,
	providers *store.ProviderStore,
	models *store.AIModelStore,
	voices *store.ModelVoiceStore,
	mcpMarket *store.MCPMarketStore,
	mcpServers *store.MCPServerStore,
	mcpBindings *store.VoicebotMCPBindingStore,
	signToken func(userID string, isAdmin bool) (string, error),
	memStore *store.MemoryEntryStore,
	turnStore *store.TurnStore,
	kbSvc *knowledge.Service,
	kbStore *store.KnowledgeBaseStore,
	docStore *store.DocumentStore,
	voicebotKBs *store.VoicebotKBStore,
	agentTemplates *store.AgentTemplateStore,
	assetSvc *assets.Service,
	internalToken string,
	billingSvc *service.Service,
	paymentSvc *service.PaymentService,
	apikeySvc *apikey.Service,
	apikeyAdminOnly bool,
) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.GET("/healthz", func(c *gin.Context) { c.Status(200) })
	r.GET("/api-docs", apiDocs)
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// CORS — 前端开发时允许跨域
	r.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		allowed := origin == "http://localhost:5173" || origin == "http://localhost:3000" || origin == "http://127.0.0.1:5173"
		if allowed {
			c.Header("Access-Control-Allow-Origin", origin)
		}
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	authH := handler.NewAuthHandler(users, bindings, signToken)
	botH := handler.NewVoicebotHandler(voicebots)
	devH := handler.NewDeviceHandler(voicebots, devices)
	providerH := handler.NewProviderHandler(providers)
	modelH := handler.NewModelHandler(models)
	// 计费关闭（billing.enabled: false）时 billingSvc 为 nil：业务模块拿到的是 nil
	// 接口（不是“非 nil 接口 + nil 指针”，那样每个方法都会 panic），所有计费路由回 503。
	var billingMeter billing.Meter
	if billingSvc != nil {
		billingMeter = billingSvc
	}
	voiceH := handler.NewVoiceHandler(voices, models, assetSvc, billingMeter)
	langH := handler.NewLanguageHandler()
	mcpH := handler.NewMCPHandler(mcpMarket, mcpServers, mcpBindings, voicebots)
	internalH := handler.NewInternalHandler(voicebots, devices, models, voices, mcpBindings)
	oauthH := handler.NewOAuthHandler(users, bindings, signToken)
	billingInternalH := handler.NewInternalBillingHandler(billingSvc)
	billingAdminH := handler.NewBillingAdminHandler(billingSvc)
	billingUserH := handler.NewBillingUserHandler(billingSvc)
	// 充值通道没接（payment 段没配）时 paymentSvc 为 nil：路由照样注册，每个请求
	// 回 503，前端不用为「这部署接没接支付」写分支。
	paymentH := handler.NewPaymentHandler(paymentSvc)
	// apikey.enabled: false 时 apikeySvc 为 nil：管理面回 503，key 中间件也拿不到
	// service（key 请求一律 401，与没这个功能时一致，§7.3、FR-9）。
	apiKeyH := handler.NewAPIKeyHandler(apikeySvc, apikeyAdminOnly)

	availableH := handler.NewAvailableHandler(providers, models, voices, assetSvc)
	tplH := handler.NewAgentTemplateHandler(agentTemplates)
	memH := handler.NewMemoryHandler(memStore)
	dataMemH := handler.NewDataMemoryHandler(memStore, devices, voicebots)
	dataKnowH := handler.NewDataKnowledgeHandler(kbSvc, kbStore, docStore, devices, voicebots, voicebotKBs)
	turnH := handler.NewTurnHandler(turnStore)
	assetH := handler.NewAssetHandler(assetSvc)

	api := r.Group("/api")
	{
		auth := api.Group("/auth")
		auth.POST("/register", authH.Register)
		auth.POST("/login", authH.Login)

		// 第三方 OAuth 登录 — 平台由 internal/oauth 注册表提供，
		// 未注册的平台在 handler 内返回 404
		auth.GET("/oauth/providers", oauthH.Providers)
		auth.GET("/oauth/:provider/login", oauthH.Login)
		auth.GET("/oauth/:provider/callback", oauthH.Callback)

		jwtMw := middleware.JWT(jwtSecret)
		// 资源路由上的两种凭证：带 ox_sk_ 前缀的走 API key，其余走原来的 JWT
		// （§7.2：老客户端行为零变化）。
		authMw := middleware.Auth(jwtSecret, apikeySvc)
		// scope 判定只对 API key 生效（JWT 是人）。表与真实路由的一致性由
		// middleware.ValidateCoverage 的测试钉住（§7 V3）。
		scopeMw := middleware.RequireScopes(middleware.RouteScopes())

		authed := api.Group("/auth", jwtMw)
		authed.POST("/change-password", authH.ChangePassword)
		authed.POST("/bind-email", authH.BindEmail)
		authed.POST("/oauth/:provider/unbind", oauthH.Unbind)
		authed.GET("/profile", authH.Profile)

		// 凭证管理：不接受机器凭证（机器不能给自己签发凭证），所以只挂 JWT。
		apiKeys := api.Group("/api-keys", jwtMw)
		apiKeys.GET("", apiKeyH.List)
		apiKeys.POST("", apiKeyH.Create)
		apiKeys.GET("/scopes", apiKeyH.Scopes)
		apiKeys.DELETE("/:id", apiKeyH.Revoke)

		bots := api.Group("/voicebots", authMw, scopeMw)
		bots.GET("", botH.List)
		bots.POST("", botH.Create)
		bots.GET("/:id", botH.Get)
		bots.PUT("/:id", botH.Update)
		bots.DELETE("/:id", botH.Delete)

		bots.GET("/:id/devices", devH.List)
		bots.POST("/:id/devices", devH.Create)
		bots.DELETE("/:id/devices/:did", devH.Delete)
		bots.PUT("/:id/devices/:did/channels/telegram", devH.SetTelegramChannel)
		bots.DELETE("/:id/devices/:did/channels/telegram", devH.DeleteTelegramChannel)

		pvd := api.Group("/providers", authMw, scopeMw)
		pvd.GET("", providerH.List)
		pvd.POST("", providerH.Create)
		pvd.GET("/slugs", providerH.Slugs)
		pvd.GET("/:id", providerH.Get)
		pvd.PUT("/:id", providerH.Update)
		pvd.DELETE("/:id", providerH.Delete)

		mdl := api.Group("/models", authMw, scopeMw)
		mdl.GET("", modelH.List)
		mdl.POST("", modelH.Create)
		mdl.GET("/types", modelH.Types)
		mdl.GET("/voice-cloning", voiceH.ListCloneableModels)
		mdl.GET("/:id", modelH.Get)
		mdl.PUT("/:id", modelH.Update)
		mdl.DELETE("/:id", modelH.Delete)
		api.GET("/voices/system", authMw, scopeMw, voiceH.ListSystem)
		api.GET("/voices/mine", authMw, scopeMw, voiceH.ListMine)
		mdl.GET("/:id/voices", voiceH.List)
		mdl.POST("/:id/voices", voiceH.Create)
		mdl.POST("/:id/voices/clone", voiceH.Clone)
		mdl.GET("/:id/voices/:vid", voiceH.Get)
		mdl.PUT("/:id/voices/:vid", voiceH.Update)
		mdl.DELETE("/:id/voices/:vid", voiceH.Delete)

		// 可用资源（需要 agent/model 的读权限，key 可用）
		api.GET("/available-resources", authMw, scopeMw, availableH.List)

		// 记忆管理（用户级）
		data := api.Group("/data/memory", authMw, scopeMw)
		data.GET("/agents", dataMemH.ListAgents)
		data.GET("/agents/:agent_id/devices", dataMemH.ListDevices)
		data.GET("/devices/:device_id/entries", dataMemH.ListEntries)
		data.DELETE("/:id", dataMemH.DeleteMemory)

		// 知识库管理
		knowledgeData := api.Group("/data/knowledge", authMw, scopeMw)
		knowledgeData.GET("/knowledge_bases", dataKnowH.ListAllKBs)
		knowledgeData.GET("/knowledge_bases/:kb_id", dataKnowH.GetKB)
		knowledgeData.GET("/knowledge_bases/:kb_id/search", dataKnowH.SearchKB)
		knowledgeData.DELETE("/knowledge_bases/:kb_id", dataKnowH.DeleteKB)
		knowledgeData.GET("/knowledge_bases/:kb_id/documents", dataKnowH.ListDocuments)
		knowledgeData.POST("/knowledge_bases/:kb_id/documents", dataKnowH.UploadDocument)
		knowledgeData.POST("/knowledge_bases/:kb_id/documents/url", dataKnowH.IngestURL)
		knowledgeData.DELETE("/documents/:doc_id", dataKnowH.DeleteDocument)
		knowledgeData.GET("/documents/:doc_id/status", dataKnowH.GetDocumentStatus)
		knowledgeData.POST("/documents/:doc_id/retry", dataKnowH.RetryDocument)
		// KB-bot binding
		knowledgeData.GET("/bots/:bot_id/knowledge_bases/bound", dataKnowH.ListBoundKBs)
		knowledgeData.POST("/bots/:bot_id/knowledge_bases/bind", dataKnowH.BindKB)
		knowledgeData.DELETE("/bots/:bot_id/knowledge_bases/:kb_id/bind", dataKnowH.UnbindKB)
		knowledgeData.GET("/bots/:bot_id/knowledge_bases", dataKnowH.ListKBs)
		knowledgeData.POST("/bots/:bot_id/knowledge_bases", dataKnowH.CreateKB)

		// 资源（上传文件）：上传 / 浏览 / 预签名访问
		assetRoutes := api.Group("/assets", authMw, scopeMw)
		assetRoutes.POST("", assetH.Upload)
		assetRoutes.GET("", assetH.List)
		assetRoutes.GET("/:id", assetH.Get)
		assetRoutes.GET("/:id/url", assetH.URL)
		assetRoutes.DELETE("/:id", assetH.Delete)

		// 智能体广场
		api.GET("/agent-templates/system", authMw, scopeMw, tplH.ListSystem)
		api.GET("/agent-templates/:id", authMw, scopeMw, tplH.Get)
		api.POST("/agent-templates/:id/use", authMw, scopeMw, tplH.Use)

		// 预留：活跃会话列表
		api.GET("/sessions", authMw, scopeMw, func(c *gin.Context) {
			c.JSON(200, []any{})
		})

		// MCP 市场 & 用户级 MCP server CRUD
		api.GET("/mcp/market", authMw, scopeMw, mcpH.ListMarket)
		api.GET("/mcp/servers", authMw, scopeMw, mcpH.ListServers)
		api.POST("/mcp/servers", authMw, scopeMw, mcpH.CreateServer)
		api.POST("/mcp/test-connection", authMw, scopeMw, mcpH.TestConnection)
		api.POST("/mcp/list-tools", authMw, scopeMw, mcpH.ListTools)
		api.POST("/mcp/call-tool", authMw, scopeMw, mcpH.CallTool)
		api.GET("/mcp/servers/:serverID", authMw, scopeMw, mcpH.GetServer)
		api.PUT("/mcp/servers/:serverID", authMw, scopeMw, mcpH.UpdateServer)
		api.DELETE("/mcp/servers/:serverID", authMw, scopeMw, mcpH.DeleteServer)

		// voicebot MCP 绑定
		bots.GET("/:id/mcps", mcpH.ListVoicebotMCPServers)
		bots.POST("/:id/mcps", mcpH.BindMCP)
		bots.DELETE("/:id/mcps/:serverID", mcpH.UnbindMCP)
		bots.PATCH("/:id/mcps/:serverID/toggle", mcpH.ToggleBinding)

		// TG 绑定管理

		// 语言字典（只读）
		api.GET("/languages", authMw, scopeMw, langH.List)
		api.GET("/languages/:code", authMw, scopeMw, langH.Get)

		// 计费（§8）。管理端要 is_admin，用户端只要凭证。
		// GET /api/billing/prices 在两个面里都是同一个路径（§8 的设计表就是这样），
		// Gin 不允许同一路径注册两次，所以只注册一条、按 is_admin 分流：管理员看全部
		// 价格版本（带筛选参数），普通用户（与 API key）只看当前生效的平台标准价。
		billingAdmin := api.Group("/billing", jwtMw, middleware.RequireAdmin())
		billingAdmin.GET("/items", billingAdminH.Items)
		billingAdmin.PUT("/items/:code", billingAdminH.SetItem)
		billingAdmin.POST("/prices", billingAdminH.CreatePrice)
		billingAdmin.PUT("/prices/:id", billingAdminH.UpdatePrice)
		billingAdmin.DELETE("/prices/:id", billingAdminH.DeletePrice)
		billingAdmin.GET("/accounts", billingAdminH.Accounts)
		billingAdmin.POST("/accounts/:id/adjust", billingAdminH.AdjustAccount)
		billingAdmin.GET("/ledger", billingAdminH.Ledger)
		billingAdmin.GET("/stats", billingAdminH.Stats)

		// 只读的计费面：key 可达（billing:read）。
		billingRead := api.Group("/billing", authMw, scopeMw)
		billingRead.GET("/summary", billingUserH.Summary)
		billingRead.GET("/usage", billingUserH.Usage)
		// 模型监控页的数据源：按模型 × 计费项聚合的用量，账户由控制面自己推。
		billingRead.GET("/usage-by-model", billingUserH.ModelUsage)
		api.GET("/billing/prices", authMw, scopeMw, func(c *gin.Context) {
			if middleware.IsAdmin(c) {
				billingAdminH.ListPrices(c)
				return
			}
			billingUserH.Prices(c)
		})

		// 充值会变更余额，API key 不可达：只挂 JWT（§5.3 的"key 不可达"清单）。
		billingWallet := api.Group("/billing", jwtMw)
		billingWallet.POST("/recharge", paymentH.CreateRecharge)
		billingWallet.GET("/recharge", paymentH.ListRecharges)
		billingWallet.GET("/recharge/config", paymentH.RechargeConfig)
		billingWallet.GET("/recharge/:out_trade_no", paymentH.GetRecharge)
		// 退款是真把钱退出去，只给 admin。
		billingAdmin.POST("/recharge/:out_trade_no/refund", paymentH.RefundRecharge)
	}

	// 支付网关的回调：公网匿名路由（网关不带 token），安全性来自签名而不是身份。
	// 这两条不能挂在 /api 组下，也不能挂任何鉴权中间件。
	r.POST("/pay/epay/notify", paymentH.Notify)
	r.GET("/pay/epay/notify", paymentH.Notify) // 有些实现用 GET 通知
	r.GET("/pay/epay/return", paymentH.Return)

	// Internal routes — intended for service-to-service calls within the same
	// network, not exposed to end users (no JWT required).
	internal := r.Group("/internal")
	{
		internal.GET("/device-config", internalH.DeviceConfig)
		internal.POST("/voices", voiceH.AdminCreate)
		internal.PATCH("/voices/:id", voiceH.AdminUpdate)

		internal.GET("/devices/:device_id/memory", memH.GetMemory)
		internal.PUT("/devices/:device_id/memory", memH.PutMemory)
		internal.POST("/devices/:device_id/turns", turnH.CreateTurn)
		internal.GET("/devices/:device_id/turns", turnH.SearchTurns)
		internal.GET("/devices/:device_id/sessions/:session_id", turnH.GetSessionMessages)

		internal.GET("/knowledge/search", dataKnowH.Search)
		internal.GET("/devices/tg-bots", internalH.DeviceTGBots)

		internal.POST("/agent-templates", tplH.AdminCreate)
		internal.PUT("/agent-templates/:id", tplH.AdminUpdate)
		internal.DELETE("/agent-templates/:id", tplH.AdminDelete)

		// 数据面调的三个计费接口（§6.1）：路径常量在 internal/billing，这里只把
		// 开头的 /internal 去掉（路由组已经带了）。
		//
		// 只给这三条挂 InternalAuth：存量 /internal/* 的调用方还带着一轮跟计费无关
		// 的回归，P1 不动它们。计费关闭时不注册——不存在的端点该是 404，而不是一个
		// 永远回 503 的空壳。
		if billingSvc != nil {
			internalAuth := middleware.InternalAuth(internalToken)
			internal.POST(strings.TrimPrefix(billing.PathAuthorize, "/internal"), internalAuth, billingInternalH.Authorize)
			internal.POST(strings.TrimPrefix(billing.PathUsageEvents, "/internal"), internalAuth, billingInternalH.UsageEvents)
			internal.POST(strings.TrimPrefix(billing.PathSettle, "/internal"), internalAuth, billingInternalH.Settle)
		}
	}
	return r
}
