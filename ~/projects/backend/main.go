package main

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()
	r.Use(cors.Default()) // Для избежания CORS-ошибок
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "success", "message": "pong"})
	})
	r.Run(":8080")
}
