package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ControlPlaneAPI — операторский CRUD монетизации (Фаза 2, S3).
// Под operatorAuth, БЕЗ tenant-middleware. Любая мутация подписки/
// override инвалидирует кэш EntitlementService (runtime-тоггл).
type ControlPlaneAPI struct {
	db  *gorm.DB
	ent *services.EntitlementService
}

func NewControlPlaneAPI(db *gorm.DB, ent *services.EntitlementService) *ControlPlaneAPI {
	return &ControlPlaneAPI{db: db, ent: ent}
}

func (a *ControlPlaneAPI) pdb() *gorm.DB {
	db := a.db.Session(&gorm.Session{})
	if err := db.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️ ControlPlaneAPI: search_path public: %v", err)
	}
	return db
}

func (a *ControlPlaneAPI) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/plans", a.ListPlans)
	g.POST("/plans", a.CreatePlan)
	g.PUT("/plans/:id", a.UpdatePlan)
	g.DELETE("/plans/:id", a.DeactivatePlan)
	g.GET("/plans/:id/features", a.ListPlanFeatures)
	g.POST("/plans/:id/features", a.SetPlanFeature)
	g.DELETE("/plans/:id/features/:code", a.RemovePlanFeature)

	g.GET("/features", a.ListFeatures)
	g.POST("/features", a.CreateFeature)
	g.PUT("/features/:id", a.UpdateFeature)
	g.DELETE("/features/:id", a.DeactivateFeature)

	g.GET("/companies", a.ListCompanies)
	g.GET("/companies/:id/subscription", a.GetSubscription)
	g.POST("/companies/:id/subscription", a.AssignSubscription)
	g.GET("/companies/:id/entitlements", a.GetEffective)
	g.PUT("/companies/:id/entitlements/:code", a.SetOverride)
	g.DELETE("/companies/:id/entitlements/:code", a.RemoveOverride)

	g.POST("/provision", a.Provision)
}

func cpErr(c *gin.Context, code int, msg string) {
	c.JSON(code, gin.H{"status": "error", "error": msg})
}
func cpOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": data})
}
func uintParam(c *gin.Context, name string) (uint, bool) {
	v, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil {
		return 0, false
	}
	return uint(v), true
}
func parseTimePtr(s string) *time.Time {
	if s == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t
	}
	return nil
}

// --- Plans ---

func (a *ControlPlaneAPI) ListPlans(c *gin.Context) {
	var plans []models.PlatformPlan
	if err := a.pdb().Order("id").Find(&plans).Error; err != nil {
		cpErr(c, 500, "db error")
		return
	}
	cpOK(c, plans)
}

type planReq struct {
	Code        string `json:"code" binding:"required,min=2,max=64"`
	Name        string `json:"name" binding:"required,min=1,max=128"`
	Description string `json:"description" binding:"omitempty,max=512"`
	PriceMinor  int64  `json:"price_minor"`
	Currency    string `json:"currency" binding:"omitempty,len=3"`
	Period      string `json:"period" binding:"omitempty,oneof=month year"`
	IsActive    *bool  `json:"is_active"`
}

func (a *ControlPlaneAPI) CreatePlan(c *gin.Context) {
	var r planReq
	if err := c.ShouldBindJSON(&r); err != nil {
		cpErr(c, 400, "Неверный формат: "+err.Error())
		return
	}
	p := models.PlatformPlan{
		Code: r.Code, Name: r.Name, Description: r.Description,
		PriceMinor: r.PriceMinor, Currency: orDef(r.Currency, "RUB"),
		Period: orDef(r.Period, "month"), IsActive: true,
	}
	if err := a.pdb().Create(&p).Error; err != nil {
		cpErr(c, 409, "Не удалось создать пакет (возможно, code занят)")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"status": "success", "data": p})
}

func (a *ControlPlaneAPI) UpdatePlan(c *gin.Context) {
	id, ok := uintParam(c, "id")
	if !ok {
		cpErr(c, 400, "bad id")
		return
	}
	var r planReq
	if err := c.ShouldBindJSON(&r); err != nil {
		cpErr(c, 400, "Неверный формат: "+err.Error())
		return
	}
	upd := map[string]any{
		"name": r.Name, "description": r.Description,
		"price_minor": r.PriceMinor, "period": orDef(r.Period, "month"),
	}
	if r.Currency != "" {
		upd["currency"] = r.Currency
	}
	if r.IsActive != nil {
		upd["is_active"] = *r.IsActive
	}
	if err := a.pdb().Model(&models.PlatformPlan{}).Where("id = ?", id).Updates(upd).Error; err != nil {
		cpErr(c, 500, "db error")
		return
	}
	a.ent.InvalidateAll() // смена плана влияет на всех подписчиков
	cpOK(c, gin.H{"updated": id})
}

func (a *ControlPlaneAPI) DeactivatePlan(c *gin.Context) {
	id, ok := uintParam(c, "id")
	if !ok {
		cpErr(c, 400, "bad id")
		return
	}
	if err := a.pdb().Model(&models.PlatformPlan{}).Where("id = ?", id).
		Update("is_active", false).Error; err != nil {
		cpErr(c, 500, "db error")
		return
	}
	a.ent.InvalidateAll() // BLK-F1: деактивация плана отзывает доступ у подписчиков
	cpOK(c, gin.H{"deactivated": id})
}

// --- Features ---

func (a *ControlPlaneAPI) ListFeatures(c *gin.Context) {
	var f []models.PlatformFeature
	if err := a.pdb().Order("id").Find(&f).Error; err != nil {
		cpErr(c, 500, "db error")
		return
	}
	cpOK(c, f)
}

type featureReq struct {
	Code        string `json:"code" binding:"required,min=2,max=64"`
	Name        string `json:"name" binding:"required,min=1,max=128"`
	Description string `json:"description" binding:"omitempty,max=512"`
	IsActive    *bool  `json:"is_active"`
}

func (a *ControlPlaneAPI) CreateFeature(c *gin.Context) {
	var r featureReq
	if err := c.ShouldBindJSON(&r); err != nil {
		cpErr(c, 400, "Неверный формат: "+err.Error())
		return
	}
	f := models.PlatformFeature{Code: r.Code, Name: r.Name, Description: r.Description, IsActive: true}
	if err := a.pdb().Create(&f).Error; err != nil {
		cpErr(c, 409, "Не удалось создать фичу (возможно, code занят)")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"status": "success", "data": f})
}

func (a *ControlPlaneAPI) UpdateFeature(c *gin.Context) {
	id, ok := uintParam(c, "id")
	if !ok {
		cpErr(c, 400, "bad id")
		return
	}
	var r featureReq
	if err := c.ShouldBindJSON(&r); err != nil {
		cpErr(c, 400, "Неверный формат: "+err.Error())
		return
	}
	upd := map[string]any{"name": r.Name, "description": r.Description}
	if r.IsActive != nil {
		upd["is_active"] = *r.IsActive
	}
	if err := a.pdb().Model(&models.PlatformFeature{}).Where("id = ?", id).Updates(upd).Error; err != nil {
		cpErr(c, 500, "db error")
		return
	}
	a.ent.InvalidateAll() // R1: UpdateFeature — альт-путь деактивации, отзываем кэш
	cpOK(c, gin.H{"updated": id})
}

func (a *ControlPlaneAPI) DeactivateFeature(c *gin.Context) {
	id, ok := uintParam(c, "id")
	if !ok {
		cpErr(c, 400, "bad id")
		return
	}
	if err := a.pdb().Model(&models.PlatformFeature{}).Where("id = ?", id).
		Update("is_active", false).Error; err != nil {
		cpErr(c, 500, "db error")
		return
	}
	a.ent.InvalidateAll() // BLK-F1: деактивация фичи отзывает её у всех
	cpOK(c, gin.H{"deactivated": id})
}

// --- Plan ↔ Feature ---

func (a *ControlPlaneAPI) ListPlanFeatures(c *gin.Context) {
	id, ok := uintParam(c, "id")
	if !ok {
		cpErr(c, 400, "bad id")
		return
	}
	var pf []models.PlatformPlanFeature
	if err := a.pdb().Where("plan_id = ?", id).Find(&pf).Error; err != nil {
		cpErr(c, 500, "db error")
		return
	}
	cpOK(c, pf)
}

type planFeatureReq struct {
	FeatureCode string `json:"feature_code" binding:"required,min=2,max=64"`
	LimitsJSON  string `json:"limits_json" binding:"omitempty"`
}

func (a *ControlPlaneAPI) SetPlanFeature(c *gin.Context) {
	id, ok := uintParam(c, "id")
	if !ok {
		cpErr(c, 400, "bad id")
		return
	}
	var r planFeatureReq
	if err := c.ShouldBindJSON(&r); err != nil {
		cpErr(c, 400, "Неверный формат: "+err.Error())
		return
	}
	db := a.pdb()
	// R3: каталог = source of truth — нельзя класть в пакет фичу, у
	// которой нет активной строки в platform_features.
	var fcat models.PlatformFeature
	if e := db.Where("code = ? AND is_active = ?", r.FeatureCode, true).First(&fcat).Error; e != nil {
		cpErr(c, 400, "Фича не найдена или деактивирована")
		return
	}
	var pf models.PlatformPlanFeature
	err := db.Where("plan_id = ? AND feature_code = ?", id, r.FeatureCode).First(&pf).Error
	if err == gorm.ErrRecordNotFound {
		pf = models.PlatformPlanFeature{PlanID: id, FeatureCode: r.FeatureCode, LimitsJSON: r.LimitsJSON}
		if err := db.Create(&pf).Error; err != nil {
			cpErr(c, 500, "db error")
			return
		}
	} else if err == nil {
		if e := db.Model(&pf).Update("limits_json", r.LimitsJSON).Error; e != nil {
			cpErr(c, 500, "db error")
			return
		}
	} else {
		cpErr(c, 500, "db error")
		return
	}
	a.ent.InvalidateAll()
	cpOK(c, pf)
}

func (a *ControlPlaneAPI) RemovePlanFeature(c *gin.Context) {
	id, ok := uintParam(c, "id")
	if !ok {
		cpErr(c, 400, "bad id")
		return
	}
	if err := a.pdb().Where("plan_id = ? AND feature_code = ?", id, c.Param("code")).
		Delete(&models.PlatformPlanFeature{}).Error; err != nil {
		cpErr(c, 500, "db error")
		return
	}
	a.ent.InvalidateAll()
	cpOK(c, gin.H{"removed": c.Param("code")})
}

// --- Companies / Subscriptions / Overrides ---

func (a *ControlPlaneAPI) ListCompanies(c *gin.Context) {
	var companies []models.Company
	if err := a.pdb().Select("id", "name", "company_type", "is_active", "database_schema").
		Order("id").Find(&companies).Error; err != nil {
		cpErr(c, 500, "db error")
		return
	}
	cpOK(c, companies)
}

func (a *ControlPlaneAPI) GetSubscription(c *gin.Context) {
	id, ok := uintParam(c, "id")
	if !ok {
		cpErr(c, 400, "bad id")
		return
	}
	var subs []models.PlatformSubscription
	a.pdb().Preload("Plan").Where("company_id = ?", id).Order("starts_at DESC").Find(&subs)
	cpOK(c, subs)
}

type assignSubReq struct {
	PlanID      uint   `json:"plan_id" binding:"required"`
	Status      string `json:"status" binding:"omitempty,oneof=active trial past_due canceled"`
	EndsAt      string `json:"ends_at"`
	TrialEndsAt string `json:"trial_ends_at"`
}

func (a *ControlPlaneAPI) AssignSubscription(c *gin.Context) {
	companyID, ok := uintParam(c, "id")
	if !ok {
		cpErr(c, 400, "bad id")
		return
	}
	var r assignSubReq
	if err := c.ShouldBindJSON(&r); err != nil {
		cpErr(c, 400, "Неверный формат: "+err.Error())
		return
	}
	db := a.pdb()
	var plan models.PlatformPlan
	if err := db.First(&plan, r.PlanID).Error; err != nil {
		cpErr(c, 404, "Пакет не найден")
		return
	}
	if !plan.IsActive {
		// BLK-F2: нельзя назначить деактивированный пакет.
		cpErr(c, 400, "Пакет деактивирован — назначение запрещено")
		return
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		// Закрываем прежние активные подписки компании.
		if err := tx.Model(&models.PlatformSubscription{}).
			Where("company_id = ? AND status IN ?", companyID, []string{"active", "trial"}).
			Update("status", "canceled").Error; err != nil {
			return err
		}
		sub := models.PlatformSubscription{
			CompanyID: companyID, PlanID: r.PlanID,
			Status: orDef(r.Status, "active"), StartsAt: time.Now(),
			EndsAt: parseTimePtr(r.EndsAt), TrialEndsAt: parseTimePtr(r.TrialEndsAt),
		}
		return tx.Create(&sub).Error
	})
	if err != nil {
		cpErr(c, 500, "Не удалось назначить подписку")
		return
	}
	a.ent.Invalidate(companyID) // runtime-тоггл
	cpOK(c, gin.H{"company_id": companyID, "plan_id": r.PlanID})
}

func (a *ControlPlaneAPI) GetEffective(c *gin.Context) {
	id, ok := uintParam(c, "id")
	if !ok {
		cpErr(c, 400, "bad id")
		return
	}
	cpOK(c, a.ent.Effective(id))
}

type overrideReq struct {
	Enabled        *bool  `json:"enabled" binding:"required"`
	LimitsJSON     string `json:"limits_json"`
	OverrideReason string `json:"override_reason" binding:"omitempty,max=256"`
	EndsAt         string `json:"ends_at"`
}

func (a *ControlPlaneAPI) SetOverride(c *gin.Context) {
	companyID, ok := uintParam(c, "id")
	if !ok {
		cpErr(c, 400, "bad id")
		return
	}
	code := c.Param("code")
	var r overrideReq
	if err := c.ShouldBindJSON(&r); err != nil {
		cpErr(c, 400, "Неверный формат: "+err.Error())
		return
	}
	db := a.pdb()
	// R3: override можно ставить только на существующую активную фичу
	// каталога (нельзя продать снятую с продажи / неизвестную).
	var fcat models.PlatformFeature
	if e := db.Where("code = ? AND is_active = ?", code, true).First(&fcat).Error; e != nil {
		cpErr(c, 400, "Фича не найдена или деактивирована")
		return
	}
	var ov models.CompanyEntitlement
	err := db.Where("company_id = ? AND feature_code = ?", companyID, code).First(&ov).Error
	if err == gorm.ErrRecordNotFound {
		ov = models.CompanyEntitlement{
			CompanyID: companyID, FeatureCode: code, Enabled: *r.Enabled,
			LimitsJSON: r.LimitsJSON, OverrideReason: r.OverrideReason,
			EndsAt: parseTimePtr(r.EndsAt),
		}
		if err := db.Create(&ov).Error; err != nil {
			cpErr(c, 500, "db error")
			return
		}
	} else if err == nil {
		// Updates с map — false-bool пишется явно (без GORM-ловушки).
		if e := db.Model(&ov).Updates(map[string]any{
			"enabled": *r.Enabled, "limits_json": r.LimitsJSON,
			"override_reason": r.OverrideReason, "ends_at": parseTimePtr(r.EndsAt),
		}).Error; e != nil {
			cpErr(c, 500, "db error")
			return
		}
	} else {
		cpErr(c, 500, "db error")
		return
	}
	a.ent.Invalidate(companyID)
	cpOK(c, ov)
}

func (a *ControlPlaneAPI) RemoveOverride(c *gin.Context) {
	companyID, ok := uintParam(c, "id")
	if !ok {
		cpErr(c, 400, "bad id")
		return
	}
	if err := a.pdb().Where("company_id = ? AND feature_code = ?", companyID, c.Param("code")).
		Delete(&models.CompanyEntitlement{}).Error; err != nil {
		cpErr(c, 500, "db error")
		return
	}
	a.ent.Invalidate(companyID)
	cpOK(c, gin.H{"removed": c.Param("code")})
}

// --- Provisioning нового tenant'а при продаже ---

type provisionReq struct {
	CompanyName   string `json:"company_name" binding:"required,min=1,max=100"`
	AdminUsername string `json:"admin_username" binding:"required,min=3,max=64"`
	AdminEmail    string `json:"admin_email" binding:"required,email"`
	AdminPassword string `json:"admin_password" binding:"required,min=10,max=128"`
	PlanID        uint   `json:"plan_id"`
}

// Provision создаёт компанию + tenant-схему + tenant-админа + (опц.)
// подписку. Транзакция + компенсирующий откат при сбое схемы
// (паттерн BLK4 из bootstrap): без осиротевших схем/«битых» аккаунтов.
func (a *ControlPlaneAPI) Provision(c *gin.Context) {
	var r provisionReq
	if err := c.ShouldBindJSON(&r); err != nil {
		cpErr(c, 400, "Неверный формат: "+err.Error())
		return
	}
	db := a.pdb()
	var createdCompany models.Company
	var adminID uint

	txErr := db.Transaction(func(tx *gorm.DB) error {
		var uCnt int64
		if err := tx.Model(&models.LocalUser{}).Where("username = ?", r.AdminUsername).Count(&uCnt).Error; err != nil {
			return err
		}
		if uCnt > 0 {
			return errProvUsernameTaken
		}
		rnd := make([]byte, 6)
		_, _ = rand.Read(rnd)
		h := hex.EncodeToString(rnd)
		company := models.Company{
			Name: r.CompanyName, DatabaseSchema: "tenant_init_" + h,
			Domain: "tenant-" + h, AxetnaLogin: r.AdminUsername,
			AxetnaPassword: "", ContactEmail: r.AdminEmail,
			IsActive: true, CompanyType: "client",
		}
		if err := tx.Create(&company).Error; err != nil {
			return fmt.Errorf("company create: %w", err)
		}
		schema := fmt.Sprintf("tenant_%d", company.ID)
		if err := tx.Model(&company).Update("database_schema", schema).Error; err != nil {
			return fmt.Errorf("schema update: %w", err)
		}
		company.DatabaseSchema = schema

		admin := models.LocalUser{
			Username: r.AdminUsername, Email: r.AdminEmail, Name: r.AdminUsername,
			CompanyID: strconv.FormatUint(uint64(company.ID), 10),
			Role:      models.RoleAdmin, IsActive: true, TokenVersion: 1,
		}
		if err := admin.SetPassword(r.AdminPassword); err != nil {
			return fmt.Errorf("hash: %w", err)
		}
		if err := tx.Create(&admin).Error; err != nil {
			return fmt.Errorf("admin create: %w", err)
		}
		if r.PlanID > 0 {
			// R2: provisioning — тоже write-path подписки; нельзя
			// назначить несуществующий/деактивированный пакет.
			var pl models.PlatformPlan
			if err := tx.First(&pl, r.PlanID).Error; err != nil {
				return errProvPlanInvalid
			}
			if !pl.IsActive {
				return errProvPlanInvalid
			}
			if err := tx.Create(&models.PlatformSubscription{
				CompanyID: company.ID, PlanID: r.PlanID,
				Status: "active", StartsAt: time.Now(),
			}).Error; err != nil {
				return fmt.Errorf("subscription: %w", err)
			}
		}
		createdCompany = company
		adminID = admin.ID
		return nil
	})
	switch txErr {
	case nil:
	case errProvUsernameTaken:
		cpErr(c, 409, "Имя tenant-админа занято")
		return
	case errProvPlanInvalid:
		cpErr(c, 400, "Пакет не найден или деактивирован")
		return
	default:
		log.Printf("❌ Provision tx failed: %v", txErr)
		cpErr(c, 500, "Ошибка provisioning")
		return
	}

	// Схема — после коммита; при сбое компенсирующий откат (BLK4).
	if err := database.CreateTenantSchema(createdCompany.ID, createdCompany.DatabaseSchema); err != nil {
		log.Printf("❌ Provision: схема %s не создана: %v — откат", createdCompany.DatabaseSchema, err)
		rb := a.pdb()
		rb.Exec(fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", createdCompany.DatabaseSchema))
		rb.Unscoped().Where("company_id = ?", createdCompany.ID).Delete(&models.PlatformSubscription{})
		rb.Unscoped().Delete(&models.LocalUser{}, adminID)
		rb.Unscoped().Delete(&models.Company{}, createdCompany.ID)
		cpErr(c, 500, "Не удалось создать схему tenant — provisioning откатён, повторите")
		return
	}
	a.ent.Invalidate(createdCompany.ID)
	log.Printf("✅ Provision: company #%d (%s) + admin #%d", createdCompany.ID, createdCompany.DatabaseSchema, adminID)
	c.JSON(http.StatusCreated, gin.H{"status": "success", "data": gin.H{
		"company_id": createdCompany.ID, "schema": createdCompany.DatabaseSchema,
		"admin_id": adminID,
	}})
}

var errProvUsernameTaken = fmt.Errorf("provision username taken")
var errProvPlanInvalid = fmt.Errorf("provision plan invalid")

func orDef(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
