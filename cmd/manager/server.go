package main

import (
	"github.com/gin-gonic/gin"

	"github.com/liuscraft/orion-x/cmd/manager/handler"
	"github.com/liuscraft/orion-x/cmd/manager/middleware"
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
	signToken func(userID string) (string, error),
) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	// CORS — 前端开发时允许跨域
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
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
	internalH := handler.NewInternalHandler(voicebots, devices, models, voices)
	availableH := handler.NewAvailableHandler(providers, models, voices)

	api := r.Group("/api")
	{
		auth := api.Group("/auth")
		auth.POST("/login", authH.Login)

		jwtMw := middleware.JWT(jwtSecret)

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

		// 预留：活跃会话列表
		api.GET("/sessions", jwtMw, func(c *gin.Context) {
			c.JSON(200, []any{})
		})

		// 语言字典（只读）
		api.GET("/languages", jwtMw, langH.List)
		api.GET("/languages/:code", jwtMw, langH.Get)
	}

	// Internal routes — intended for service-to-service calls within the same
	// network, not exposed to end users (no JWT required).
	r.GET("/internal/device-config", internalH.DeviceConfig)
	r.POST("/internal/voices", voiceH.AdminCreate)
	r.PATCH("/internal/voices/:id", voiceH.AdminUpdate)
	r.GET("/internal/languages", langH.List)
	r.POST("/internal/languages", langH.AdminCreate)
	r.PUT("/internal/languages/:code", langH.AdminUpdate)
	r.DELETE("/internal/languages/:code", langH.AdminDelete)

	return r
}
