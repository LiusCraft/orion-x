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
	internalH := handler.NewInternalHandler(voicebots, devices)

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

		// 预留：活跃会话列表
		api.GET("/sessions", jwtMw, func(c *gin.Context) {
			c.JSON(200, []any{})
		})
	}

	// Internal routes — intended for service-to-service calls within the same
	// network, not exposed to end users (no JWT required).
	r.GET("/internal/device-config", internalH.DeviceConfig)

	return r
}
