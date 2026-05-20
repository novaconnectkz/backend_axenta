package middleware

import (
	"backend_axenta/models"
	"backend_axenta/services"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func fgDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.PlatformPlan{}, &models.PlatformFeature{},
		&models.PlatformPlanFeature{}, &models.PlatformSubscription{},
		&models.CompanyEntitlement{},
	))
	return db
}

func runGate(ent *services.EntitlementService, companyID uint) int {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", func(c *gin.Context) {
		if companyID != 0 {
			c.Set("company_id", companyID) // эмулируем SetTenant
		}
	}, RequireFeature(ent, "advanced_reports"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/x", nil)
	r.ServeHTTP(w, req)
	return w.Code
}

func TestRequireFeature_EnabledByPlan_Allows(t *testing.T) {
	db := fgDB(t)
	db.Create(&models.PlatformPlan{ID: 1, Code: "pro"})
	db.Create(&models.PlatformFeature{Code: "advanced_reports", Name: "AR", IsActive: true})
	db.Create(&models.PlatformPlanFeature{PlanID: 1, FeatureCode: "advanced_reports"})
	db.Create(&models.PlatformSubscription{CompanyID: 5, PlanID: 1, Status: "active", StartsAt: time.Now().Add(-time.Hour)})
	ent := services.NewEntitlementService(db)
	require.Equal(t, http.StatusOK, runGate(ent, 5))
}

func TestRequireFeature_NoSubscription_403(t *testing.T) {
	ent := services.NewEntitlementService(fgDB(t))
	require.Equal(t, http.StatusForbidden, runGate(ent, 5))
}

func TestRequireFeature_OverrideDisabled_403(t *testing.T) {
	db := fgDB(t)
	db.Create(&models.PlatformPlan{ID: 1, Code: "pro"})
	db.Create(&models.PlatformFeature{Code: "advanced_reports", Name: "AR", IsActive: true})
	db.Create(&models.PlatformPlanFeature{PlanID: 1, FeatureCode: "advanced_reports"})
	db.Create(&models.PlatformSubscription{CompanyID: 5, PlanID: 1, Status: "active", StartsAt: time.Now().Add(-time.Hour)})
	db.Create(&models.CompanyEntitlement{CompanyID: 5, FeatureCode: "advanced_reports", Enabled: false})
	ent := services.NewEntitlementService(db)
	require.Equal(t, http.StatusForbidden, runGate(ent, 5), "override disable → гейт закрыт")
}

func TestRequireFeature_NoCompanyContext_403(t *testing.T) {
	ent := services.NewEntitlementService(fgDB(t))
	require.Equal(t, http.StatusForbidden, runGate(ent, 0), "нет company в контексте → deny")
}
