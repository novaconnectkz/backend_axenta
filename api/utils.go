package api

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GetCompanyID извлекает ID компании из контекста Gin
func GetCompanyID(c *gin.Context) uuid.UUID {
	if companyID, exists := c.Get("company_id"); exists {
		if id, ok := companyID.(uuid.UUID); ok {
			return id
		}
		if id, ok := companyID.(string); ok {
			if parsed, err := uuid.Parse(id); err == nil {
				return parsed
			}
		}
	}
	if tenantID, exists := c.Get("tenant_id"); exists {
		if id, ok := tenantID.(uuid.UUID); ok {
			return id
		}
		if id, ok := tenantID.(string); ok {
			if parsed, err := uuid.Parse(id); err == nil {
				return parsed
			}
		}
	}
	return uuid.Nil
}

// GetTenantIDFromContext извлекает ID компании из контекста (алиас для GetCompanyID)
func GetTenantIDFromContext(c *gin.Context) uuid.UUID {
	return GetCompanyID(c)
}
