package api

import (
	"net/http"
	"time"

	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GenerateSkifPartnerSnapshots — ручной триггер SKIF partner-снимков за сегодня (UTC)
// для текущего тенанта. Ф1: агрегирует объекты дилера (Σ units компаний) →
// partner_daily_snapshots (source=skif). Идемпотентно (OnConflict upsert).
//
// POST /api/auth/skif/partner-snapshots/generate
func GenerateSkifPartnerSnapshots(c *gin.Context) {
	if _, err := middleware.GetAdminAccountID(c); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": err.Error()})
		return
	}

	tenantDBVal, exists := c.Get("tenant_db")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Tenant DB не найдена"})
		return
	}
	tenantDB := tenantDBVal.(*gorm.DB)

	svc := services.NewSkifPartnerSnapshotService(database.DB)
	created, err := svc.GenerateForTenant(tenantDB, time.Now().UTC())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "SKIF partner snapshots сгенерированы",
		"created": created,
	})
}
