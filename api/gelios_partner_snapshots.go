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

// GenerateGeliosPartnerSnapshots — ручной триггер GELIOS partner-снимков за
// сегодня (UTC) для текущего тенанта. Ф3 (per-account, 100% guaranteed-data):
// force-sync gelios_users per-conn → читает units_count → пишет
// partner_daily_snapshots ВСЕГДА (warning при отсутствии данных).
// Идемпотентно (OnConflict upsert).
//
// POST /api/auth/gelios/partner-snapshots/generate
func GenerateGeliosPartnerSnapshots(c *gin.Context) {
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

	svc := services.NewGeliosPartnerSnapshotService(database.DB)
	created, err := svc.GenerateForTenant(tenantDB, time.Now().UTC())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "GELIOS partner snapshots сгенерированы",
		"created": created,
	})
}
