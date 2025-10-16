package main

import (
	"backend_axenta/api"

	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()

	// Простые тестовые endpoints
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "success", "message": "pong"})
	})

	r.GET("/api/test", api.TestEndpoint)
	r.POST("/api/cms/users", api.CreateCmsUserWithCurrentToken)
	r.POST("/api/cms/users/", api.CreateCmsUserWithCurrentToken)

	r.Run(":8081")
}
