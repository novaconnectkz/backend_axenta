package api

import (
	"backend_axenta/database"
	"backend_axenta/models"

	"github.com/gin-gonic/gin"
)

func GetObjects(c *gin.Context) {
	var objects []models.Object
	if err := database.DB.Find(&objects).Error; err != nil {
		c.JSON(500, gin.H{"status": "error", "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "success", "data": objects})
}
