package main

import (
	"ai-prelander-builder/backend/internal/config"
	"ai-prelander-builder/backend/internal/handlers"
	"github.com/gin-gonic/gin"
)

func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", config.FrontendURL)
		c.Header("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
func main() {
	r := gin.Default()
	r.Use(cors())
	r.Static("/static", "./static")
	r.POST("/api/prelanders/generate", handlers.GeneratePrelanders)
	r.GET("/api/prelanders", handlers.ListPrelanders)
	r.GET("/preview/:prelander_id", handlers.Preview)
	_ = r.Run(":" + config.Port)
}
