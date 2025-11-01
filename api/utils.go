package api

import "github.com/gin-gonic/gin"

// isDemoMode проверяет, включен ли демо-режим через параметр ?demo=1 или ?demo=true
func isDemoMode(c *gin.Context) bool {
	return c.Query("demo") == "1" || c.Query("demo") == "true"
}
