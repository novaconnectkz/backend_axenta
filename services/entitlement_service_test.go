package services

import (
	"backend_axenta/models"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupEntitlementDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.PlatformPlan{}, &models.PlatformFeature{},
		&models.PlatformPlanFeature{}, &models.PlatformSubscription{},
		&models.CompanyEntitlement{},
	))
	return db
}

func TestEntitlement_NoSubscription_Disabled(t *testing.T) {
	svc := NewEntitlementService(setupEntitlementDB(t))
	require.False(t, svc.IsEnabled(1, "objects"))
}

func TestEntitlement_PlanFeatureEnabled(t *testing.T) {
	db := setupEntitlementDB(t)
	require.NoError(t, db.Create(&models.PlatformPlan{ID: 1, Code: "pro", Name: "Pro"}).Error)
	require.NoError(t, db.Create(&models.PlatformFeature{Code: "objects", Name: "o", IsActive: true}).Error)
	require.NoError(t, db.Create(&models.PlatformPlanFeature{
		PlanID: 1, FeatureCode: "objects", LimitsJSON: `{"max":1000}`}).Error)
	require.NoError(t, db.Create(&models.PlatformSubscription{
		CompanyID: 7, PlanID: 1, Status: "active", StartsAt: time.Now().Add(-time.Hour)}).Error)

	svc := NewEntitlementService(db)
	require.True(t, svc.IsEnabled(7, "objects"))
	require.Equal(t, `{"max":1000}`, svc.GetLimits(7, "objects"))
	require.False(t, svc.IsEnabled(7, "reports"), "фича не в плане")
	require.False(t, svc.IsEnabled(99, "objects"), "другая компания")
}

func TestEntitlement_OverrideDisablesPlanFeature(t *testing.T) {
	db := setupEntitlementDB(t)
	db.Create(&models.PlatformPlan{ID: 1, Code: "pro"})
	db.Create(&models.PlatformFeature{Code: "objects", Name: "o", IsActive: true})
	db.Create(&models.PlatformPlanFeature{PlanID: 1, FeatureCode: "objects"})
	db.Create(&models.PlatformSubscription{CompanyID: 7, PlanID: 1, Status: "active", StartsAt: time.Now().Add(-time.Hour)})
	db.Create(&models.CompanyEntitlement{CompanyID: 7, FeatureCode: "objects", Enabled: false, OverrideReason: "debt"})

	svc := NewEntitlementService(db)
	require.False(t, svc.IsEnabled(7, "objects"), "override должен выключить фичу плана")
}

func TestEntitlement_OverrideEnablesExtraFeature(t *testing.T) {
	db := setupEntitlementDB(t)
	db.Create(&models.PlatformPlan{ID: 1, Code: "basic"})
	db.Create(&models.PlatformSubscription{CompanyID: 7, PlanID: 1, Status: "active", StartsAt: time.Now().Add(-time.Hour)})
	db.Create(&models.PlatformFeature{Code: "beta_ai", Name: "b", IsActive: true})
	db.Create(&models.CompanyEntitlement{CompanyID: 7, FeatureCode: "beta_ai", Enabled: true})

	svc := NewEntitlementService(db)
	require.True(t, svc.IsEnabled(7, "beta_ai"), "override включает фичу вне плана")
}

func TestEntitlement_ExpiredSubscription_Disabled(t *testing.T) {
	db := setupEntitlementDB(t)
	db.Create(&models.PlatformPlan{ID: 1, Code: "pro"})
	db.Create(&models.PlatformFeature{Code: "objects", Name: "o", IsActive: true})
	db.Create(&models.PlatformPlanFeature{PlanID: 1, FeatureCode: "objects"})
	past := time.Now().Add(-time.Hour)
	db.Create(&models.PlatformSubscription{
		CompanyID: 7, PlanID: 1, Status: "active",
		StartsAt: time.Now().Add(-48 * time.Hour), EndsAt: &past})

	svc := NewEntitlementService(db)
	require.False(t, svc.IsEnabled(7, "objects"), "истёкшая подписка не даёт фич")
}

// BLK1: ошибка чтения company_entitlements (роняем таблицу) при
// активном плане → fail-CLOSED (фича выключена), НЕ выдаём бесплатно.
func TestEntitlement_DBError_FailClosed(t *testing.T) {
	db := setupEntitlementDB(t)
	db.Create(&models.PlatformPlan{ID: 1, Code: "pro"})
	db.Create(&models.PlatformFeature{Code: "objects", Name: "o", IsActive: true})
	db.Create(&models.PlatformPlanFeature{PlanID: 1, FeatureCode: "objects"})
	db.Create(&models.PlatformSubscription{CompanyID: 7, PlanID: 1, Status: "active", StartsAt: time.Now().Add(-time.Hour)})
	require.NoError(t, db.Migrator().DropTable(&models.CompanyEntitlement{}))

	svc := NewEntitlementService(db)
	require.False(t, svc.IsEnabled(7, "objects"), "BLK1: ошибка БД → deny, не выдавать платную фичу")
}

// BLK-F1: деактивированный план → фичи плана НЕ выдаются.
func TestEntitlement_InactivePlan_NoFeatures(t *testing.T) {
	db := setupEntitlementDB(t)
	db.Create(&models.PlatformPlan{ID: 1, Code: "pro"})
	db.Create(&models.PlatformFeature{Code: "objects", Name: "Obj", IsActive: true})
	db.Create(&models.PlatformFeature{Code: "objects", Name: "o", IsActive: true})
	db.Create(&models.PlatformPlanFeature{PlanID: 1, FeatureCode: "objects"})
	db.Create(&models.PlatformSubscription{CompanyID: 7, PlanID: 1, Status: "active", StartsAt: time.Now().Add(-time.Hour)})
	svc := NewEntitlementService(db)
	require.True(t, svc.IsEnabled(7, "objects"))

	require.NoError(t, db.Model(&models.PlatformPlan{}).Where("id = ?", 1).Update("is_active", false).Error)
	svc.InvalidateAll()
	require.False(t, svc.IsEnabled(7, "objects"), "деактивированный план не выдаёт фичи")
}

// BLK-F1: деактивированная фича выкидывается из effective (и из плана,
// и из override).
func TestEntitlement_InactiveFeature_Dropped(t *testing.T) {
	db := setupEntitlementDB(t)
	db.Create(&models.PlatformPlan{ID: 1, Code: "pro"})
	db.Create(&models.PlatformFeature{Code: "objects", Name: "Obj", IsActive: true})
	db.Create(&models.PlatformFeature{Code: "objects", Name: "o", IsActive: true})
	db.Create(&models.PlatformPlanFeature{PlanID: 1, FeatureCode: "objects"})
	db.Create(&models.PlatformSubscription{CompanyID: 7, PlanID: 1, Status: "active", StartsAt: time.Now().Add(-time.Hour)})
	svc := NewEntitlementService(db)
	require.True(t, svc.IsEnabled(7, "objects"))

	require.NoError(t, db.Model(&models.PlatformFeature{}).Where("code = ?", "objects").Update("is_active", false).Error)
	svc.InvalidateAll()
	require.False(t, svc.IsEnabled(7, "objects"), "деактивированная фича недоступна")

	// override на снятую с продажи фичу тоже не должен её воскрешать
	db.Create(&models.CompanyEntitlement{CompanyID: 7, FeatureCode: "objects", Enabled: true})
	svc.Invalidate(7)
	require.False(t, svc.IsEnabled(7, "objects"), "override не воскрешает деактивированную фичу")
}

func TestEntitlement_CacheInvalidate(t *testing.T) {
	db := setupEntitlementDB(t)
	db.Create(&models.PlatformPlan{ID: 1, Code: "pro"})
	db.Create(&models.PlatformFeature{Code: "objects", Name: "o", IsActive: true})
	db.Create(&models.PlatformPlanFeature{PlanID: 1, FeatureCode: "objects"})
	db.Create(&models.PlatformSubscription{CompanyID: 7, PlanID: 1, Status: "active", StartsAt: time.Now().Add(-time.Hour)})
	svc := NewEntitlementService(db)
	require.True(t, svc.IsEnabled(7, "objects"))

	// Отзываем подписку, кэш ещё держит true.
	require.NoError(t, db.Model(&models.PlatformSubscription{}).
		Where("company_id = ?", 7).Update("status", "canceled").Error)
	require.True(t, svc.IsEnabled(7, "objects"), "кэш ещё валиден")

	svc.Invalidate(7)
	require.False(t, svc.IsEnabled(7, "objects"), "после Invalidate пересчёт → выкл")
}
