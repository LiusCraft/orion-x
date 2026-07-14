package main

import (
	"github.com/gin-gonic/gin"

	"github.com/liuscraft/orion-x/cmd/manager/handler"
	"github.com/liuscraft/orion-x/cmd/manager/middleware"
	"github.com/liuscraft/orion-x/internal/knowledge"
	"github.com/liuscraft/orion-x/internal/store"
)

func newRouter(
	jwtSecret []byte,
	users *store.UserStore,
	voicebots *store.VoicebotStore,
	devices *store.DeviceStore,
	providers *store.ProviderStore,
	models *store.AIModelStore,
	voices *store.ModelVoiceStore,
	langH *handler.LanguageHandler,
	mcpMarket *store.MCPMarketStore,
	mcpServers *store.MCPServerStore,
	mcpBindings *store.VoicebotMCPBindingStore,
	signToken func(userID string) (string, error),
	memStore *store.MemoryEntryStore,
	turnStore *store.TurnStore,
	kbSvc *knowledge.Service,
	kbStore *store.KnowledgeBaseStore,
	docStore *store.DocumentStore,
) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

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

	authH := handler.NewAuthHandler(users, signToken)
	botH := handler.NewVoicebotHandler(voicebots)
	devH := handler.NewDeviceHandler(voicebots, devices)
	providerH := handler.NewProviderHandler(providers)
	modelH := handler.NewModelHandler(models)
	voiceH := handler.NewVoiceHandler(voices)
	mcpH := handler.NewMCPHandler(mcpMarket, mcpServers, mcpBindings, voicebots)
	internalH := handler.NewInternalHandler(voicebots, devices, models, voices, mcpBindings)
	availableH := handler.NewAvailableHandler(providers, models, voices)
	memH := handler.NewMemoryHandler(memStore)
	dataMemH := handler.NewDataMemoryHandler(memStore, devices, voicebots)
	dataKnowH := handler.NewDataKnowledgeHandler(kbSvc, kbStore, docStore, devices, voicebots)
	turnH := handler.NewTurnHandler(turnStore)

	api := r.Group("/api")
	{
		auth := api.Group("/auth")
		auth.POST("/login", authH.Login)

		jwtMw := middleware.JWT(jwtSecret)

		authed := api.Group("/auth", jwtMw)
		authed.POST("/change-password", authH.ChangePassword)
		authed.POST("/bind-email", authH.BindEmail)
		authed.GET("/profile", authH.Profile)

		bots := api.Group("/voicebots", jwtMw)
		bots.GET("", botH.List)
		bots.POST("", botH.Create)
		bots.GET("/:id", botH.Get)
		bots.PUT("/:id", botH.Update)
		bots.DELETE("/:id", botH.Delete)

		bots.GET("/:id/devices", devH.List)
		bots.POST("/:id/devices", devH.Create)
		bots.DELETE("/:id/devices/:did", devH.Delete)

		pvd := api.Group("/providers", jwtMw)
		pvd.GET("", providerH.List)
		pvd.POST("", providerH.Create)
		pvd.GET("/slugs", providerH.Slugs)
		pvd.GET("/:id", providerH.Get)
		pvd.PUT("/:id", providerH.Update)
		pvd.DELETE("/:id", providerH.Delete)

		mdl := api.Group("/models", jwtMw)
		mdl.GET("", modelH.List)
		mdl.POST("", modelH.Create)
		mdl.GET("/types", modelH.Types)
		mdl.GET("/:id", modelH.Get)
		mdl.PUT("/:id", modelH.Update)
		mdl.DELETE("/:id", modelH.Delete)
		api.GET("/voices/system", jwtMw, voiceH.ListSystem)
		mdl.GET("/:id/voices", voiceH.List)
		mdl.POST("/:id/voices", voiceH.Create)
		mdl.POST("/:id/voices/clone", voiceH.Clone)
		mdl.GET("/:id/voices/:vid", voiceH.Get)
		mdl.PUT("/:id/voices/:vid", voiceH.Update)
		mdl.DELETE("/:id/voices/:vid", voiceH.Delete)

		// 可用资源（无 API key）
		api.GET("/available-resources", jwtMw, availableH.List)

		// 记忆管理（用户级）
		data := api.Group("/data/memory", jwtMw)
		data.GET("/agents", dataMemH.ListAgents)
		data.GET("/agents/:agent_id/devices", dataMemH.ListDevices)
		data.GET("/devices/:device_id/entries", dataMemH.ListEntries)
		data.DELETE("/:id", dataMemH.DeleteMemory)

		// 知识库管理
		knowledgeData := api.Group("/data/knowledge", jwtMw)
		knowledgeData.GET("/bots/:bot_id/knowledge_bases", dataKnowH.ListKBs)
		knowledgeData.POST("/bots/:bot_id/knowledge_bases", dataKnowH.CreateKB)
		knowledgeData.GET("/knowledge_bases/:kb_id", dataKnowH.GetKB)
		knowledgeData.DELETE("/knowledge_bases/:kb_id", dataKnowH.DeleteKB)
		knowledgeData.GET("/knowledge_bases/:kb_id/documents", dataKnowH.ListDocuments)
		knowledgeData.POST("/knowledge_bases/:kb_id/documents", dataKnowH.UploadDocument)
		knowledgeData.DELETE("/documents/:doc_id", dataKnowH.DeleteDocument)
		knowledgeData.GET("/documents/:doc_id/status", dataKnowH.GetDocumentStatus)

		// 预留：活跃会话列表
		api.GET("/sessions", jwtMw, func(c *gin.Context) {
			c.JSON(200, []any{})
		})

		// MCP 市场 & 用户级 MCP server CRUD
		api.GET("/mcp/market", jwtMw, mcpH.ListMarket)
		api.GET("/mcp/servers", jwtMw, mcpH.ListServers)
		api.POST("/mcp/servers", jwtMw, mcpH.CreateServer)
		api.POST("/mcp/test-connection", jwtMw, mcpH.TestConnection)
		api.POST("/mcp/list-tools", jwtMw, mcpH.ListTools)
		api.POST("/mcp/call-tool", jwtMw, mcpH.CallTool)
		api.GET("/mcp/servers/:serverID", jwtMw, mcpH.GetServer)
		api.PUT("/mcp/servers/:serverID", jwtMw, mcpH.UpdateServer)
		api.DELETE("/mcp/servers/:serverID", jwtMw, mcpH.DeleteServer)

		// voicebot MCP 绑定
		bots.GET("/:id/mcps", mcpH.ListVoicebotMCPServers)
		bots.POST("/:id/mcps", mcpH.BindMCP)
		bots.DELETE("/:id/mcps/:serverID", mcpH.UnbindMCP)
		bots.PATCH("/:id/mcps/:serverID/toggle", mcpH.ToggleBinding)

		// 语言字典（只读）
		api.GET("/languages", jwtMw, langH.List)
		api.GET("/languages/:code", jwtMw, langH.Get)
	}

	// Internal routes — intended for service-to-service calls within the same
	// network, not exposed to end users (no JWT required).
	internal := r.Group("/internal")
	{
		internal.GET("/device-config", internalH.DeviceConfig)
		internal.POST("/voices", voiceH.AdminCreate)
		internal.PATCH("/voices/:id", voiceH.AdminUpdate)
		internal.GET("/languages", langH.List)
		internal.POST("/languages", langH.AdminCreate)
		internal.PUT("/languages/:code", langH.AdminUpdate)
		internal.DELETE("/languages/:code", langH.AdminDelete)

		internal.GET("/devices/:device_id/memory", memH.GetMemory)
		internal.PUT("/devices/:device_id/memory", memH.PutMemory)
		internal.POST("/devices/:device_id/turns", turnH.CreateTurn)
		internal.GET("/devices/:device_id/turns", turnH.SearchTurns)
		internal.GET("/devices/:device_id/sessions/:session_id", turnH.GetSessionMessages)

		internal.GET("/knowledge/search", dataKnowH.Search)
	}

	return r
}
