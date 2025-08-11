package main

import (
	"backend_axenta/api"
	"log"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	log.Println("Starting Axenta Backend Server...")

	r := gin.Default()
	r.Use(cors.Default())

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "success", "message": "pong"})
	})

	r.POST("/api/login", api.Login)
	r.POST("/api/test", api.TestAxentaAPI)

	log.Println("Server starting on port 8080...")
	r.Run(":8080")
}
