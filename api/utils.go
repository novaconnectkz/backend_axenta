package api

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

// GetCompanyID извлекает ID компании из контекста Gin
func GetCompanyID(c *gin.Context) uint {
	if companyID, exists := c.Get("company_id"); exists {
		if id, ok := companyID.(uint); ok {
			return id
		}
		if id, ok := companyID.(string); ok {
			if parsed, err := strconv.ParseUint(id, 10, 32); err == nil {
				return uint(parsed)
			}
		}
	}
	if tenantID, exists := c.Get("tenant_id"); exists {
		if id, ok := tenantID.(uint); ok {
			return id
		}
		if id, ok := tenantID.(string); ok {
			if parsed, err := strconv.ParseUint(id, 10, 32); err == nil {
				return uint(parsed)
			}
		}
	}
	return 0
}

// GetTenantIDFromContext извлекает ID компании из контекста (алиас для GetCompanyID)
func GetTenantIDFromContext(c *gin.Context) uint {
	return GetCompanyID(c)
}
