package api

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"
	"backend_axenta/services"
)

// SKIF Connections API — CRUD + test/sync для multi-account SKIF.PRO интеграции.
//
// Routes регистрируются в main.go: /api/skif/connections/*

func skifService() *services.SkifService {
	return services.NewSkifService(database.DB)
}

// GetSkifConnections возвращает список подключений текущей компании.
// GET /api/auth/skif/connections
func GetSkifConnections(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no company context"})
		return
	}
	var conns []models.SkifConnection
	if err := database.DB.Where("company_id = ?", companyID).
		Order("id ASC").Find(&conns).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": conns})
}

// CreateSkifConnection создаёт новое подключение.
// POST /api/auth/skif/connections
// body: { name, base_url, login, password, sync_interval, auto_sync_enabled, sync_units, sync_terminals }
func CreateSkifConnection(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no company context"})
		return
	}
	var body struct {
		Name            string `json:"name" binding:"required"`
		BaseURL         string `json:"base_url"`
		Login           string `json:"login" binding:"required"`
		Password        string `json:"password" binding:"required"`
		SyncInterval    int    `json:"sync_interval"`
		AutoSyncEnabled bool   `json:"auto_sync_enabled"`
		SyncUnits       bool   `json:"sync_units"`
		SyncTerminals   bool   `json:"sync_terminals"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.BaseURL == "" {
		body.BaseURL = "https://app.skif.pro"
	}
	if body.SyncInterval <= 0 {
		body.SyncInterval = 15
	}
	conn := models.SkifConnection{
		CompanyID:       companyID,
		Name:            body.Name,
		BaseURL:         body.BaseURL,
		Login:           body.Login,
		Password:        body.Password,
		SyncInterval:    body.SyncInterval,
		AutoSyncEnabled: body.AutoSyncEnabled,
		SyncUnits:       body.SyncUnits,
		SyncTerminals:   body.SyncTerminals,
		IsActive:        true,
	}
	if err := database.DB.Create(&conn).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": conn})
}

// UpdateSkifConnection обновляет подключение.
// PUT /api/auth/skif/connections/:id
func UpdateSkifConnection(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	conn, err := loadOwnedSkifConn(c, companyID)
	if err != nil {
		return
	}
	var body struct {
		Name            *string `json:"name"`
		BaseURL         *string `json:"base_url"`
		Login           *string `json:"login"`
		Password        *string `json:"password"`
		SyncInterval    *int    `json:"sync_interval"`
		AutoSyncEnabled *bool   `json:"auto_sync_enabled"`
		SyncUnits       *bool   `json:"sync_units"`
		SyncTerminals   *bool   `json:"sync_terminals"`
		IsActive        *bool   `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updates := map[string]interface{}{}
	credsChanged := false
	if body.Name != nil {
		updates["name"] = *body.Name
	}
	if body.BaseURL != nil {
		updates["base_url"] = *body.BaseURL
		credsChanged = true
	}
	if body.Login != nil {
		updates["login"] = *body.Login
		credsChanged = true
	}
	if body.Password != nil && *body.Password != "" {
		updates["password"] = *body.Password
		credsChanged = true
	}
	if body.SyncInterval != nil {
		updates["sync_interval"] = *body.SyncInterval
	}
	if body.AutoSyncEnabled != nil {
		updates["auto_sync_enabled"] = *body.AutoSyncEnabled
	}
	if body.SyncUnits != nil {
		updates["sync_units"] = *body.SyncUnits
	}
	if body.SyncTerminals != nil {
		updates["sync_terminals"] = *body.SyncTerminals
	}
	if body.IsActive != nil {
		updates["is_active"] = *body.IsActive
	}
	// При смене кредов сбрасываем сохранённую сессию — следующий запрос сделает re-login.
	if credsChanged {
		updates["session_cookie"] = ""
	}
	if err := database.DB.Model(conn).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	database.DB.First(conn, conn.ID)
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": conn})
}

// DeleteSkifConnection удаляет подключение (cascade на skif_units).
// DELETE /api/auth/skif/connections/:id
func DeleteSkifConnection(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	conn, err := loadOwnedSkifConn(c, companyID)
	if err != nil {
		return
	}
	if err := database.DB.Delete(conn).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// TestSkifConnection делает login + /me для проверки кредов.
// POST /api/auth/skif/connections/:id/test
func TestSkifConnection(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	conn, err := loadOwnedSkifConn(c, companyID)
	if err != nil {
		return
	}
	me, err := skifService().TestConnection(conn)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{"me": me}})
}

// SyncSkifConnection триггерит синхронизацию объектов.
// POST /api/auth/skif/connections/:id/sync
func SyncSkifConnection(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	conn, err := loadOwnedSkifConn(c, companyID)
	if err != nil {
		return
	}
	count, err := skifService().SyncUnits(conn)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{"upserted": count}})
}

// BackfillSkifHistory запускает синхронный backfill истории units (POST /company/updates/query).
// POST /api/auth/skif/connections/:id/history/backfill?from=2025-05-01&to=2026-05-07
func BackfillSkifHistory(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	conn, err := loadOwnedSkifConn(c, companyID)
	if err != nil {
		return
	}
	fromStr := c.Query("from")
	toStr := c.Query("to")
	if fromStr == "" || toStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from и to обязательны (YYYY-MM-DD)"})
		return
	}
	from, err1 := parseHistoryDate(fromStr)
	to, err2 := parseHistoryDate(toStr)
	if err1 != nil || err2 != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from/to должны быть YYYY-MM-DD"})
		return
	}
	if !from.Before(to) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from должно быть раньше to"})
		return
	}
	events, upserted, err := skifService().BackfillHistory(conn, from, to)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   gin.H{"events": events, "upserted_days": upserted},
	})
}

// CreateSkifCompany создаёт новую дилерскую компанию в SKIF через POST /api_v1/company/create.
// POST /api/auth/skif/connections/:id/companies
// body: { company_name, timezone, dateformat?, timeformat?, with_user?, user_email?, user_password?, user_name? }
func CreateSkifCompany(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	conn, err := loadOwnedSkifConn(c, companyID)
	if err != nil {
		return
	}
	var body struct {
		CompanyName  string `json:"company_name" binding:"required"`
		Timezone     string `json:"timezone" binding:"required"`
		DateFormat   string `json:"dateformat"`
		TimeFormat   string `json:"timeformat"`
		WithUser     bool   `json:"with_user"`
		UserEmail    string `json:"user_email"`
		UserPassword string `json:"user_password"`
		UserName     string `json:"user_name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	params := services.SkifCreateCompanyParams{
		CompanyName: body.CompanyName,
		Timezone:    body.Timezone,
		DateFormat:  body.DateFormat,
		TimeFormat:  body.TimeFormat,
	}
	if body.WithUser {
		if body.UserEmail == "" || body.UserPassword == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "with_user=true требует user_email и user_password"})
			return
		}
		params.User = &services.SkifCreateCompanyUser{
			Email:    body.UserEmail,
			Type:     "EMAIL",
			Password: body.UserPassword,
			Name:     body.UserName,
		}
	}
	svc := skifService()
	resp, err := svc.CreateCompany(conn, params)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "error": err.Error()})
		return
	}
	// Триггерим sync чтобы новая компания + её юниты (если есть) попали в локальный реестр.
	// Sync асинхронный — делаем в горутине, ответ юзеру не задерживаем.
	go func() {
		if count, err := svc.SyncUnits(conn); err != nil {
			log.Printf("⚠️ SKIF post-create sync conn=%d: %v", conn.ID, err)
		} else {
			log.Printf("🔁 SKIF post-create sync conn=%d: upserted=%d", conn.ID, count)
		}
		// Свежая компания должна сразу попасть в skif_company_statuses
		// чтобы /unified/accounts отдал её даже без юнитов.
		if _, err := svc.SyncCompanyStatuses(conn); err != nil {
			log.Printf("⚠️ SKIF post-create statuses: %v", err)
		}
	}()
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": resp})
}

// DeleteSkifCompany планирует удаление SKIF-компании (schedule_delete).
// DELETE /api/auth/skif/connections/:id/companies/:companyId
func DeleteSkifCompany(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	conn, err := loadOwnedSkifConn(c, companyID)
	if err != nil {
		return
	}
	skifCompanyID := c.Param("companyId")
	if skifCompanyID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "companyId обязателен"})
		return
	}
	if err := skifService().DeleteCompany(conn, skifCompanyID); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// CancelDeleteSkifCompany отменяет запланированное удаление компании.
// POST /api/auth/skif/connections/:id/companies/:companyId/cancel-delete
func CancelDeleteSkifCompany(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	conn, err := loadOwnedSkifConn(c, companyID)
	if err != nil {
		return
	}
	skifCompanyID := c.Param("companyId")
	if skifCompanyID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "companyId обязателен"})
		return
	}
	if err := skifService().CancelDeleteCompany(conn, skifCompanyID); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// CreateSkifSubdealer регистрирует нового субинтегратора через app/api_v1/registrate_dealer.
// POST /api/auth/skif/connections/:id/subdealers
// body: { type_key, name, email, inn, phone, contact_person, password, address }
func CreateSkifSubdealer(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	conn, err := loadOwnedSkifConn(c, companyID)
	if err != nil {
		return
	}
	var body services.SkifRegisterSubdealerParams
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp, err := skifService().RegisterSubdealer(conn, body)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": resp})
}

// SyncSkifSubdealers триггерит ручной sync субинтеграторов.
// POST /api/auth/skif/connections/:id/sync-subdealers
func SyncSkifSubdealers(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	conn, err := loadOwnedSkifConn(c, companyID)
	if err != nil {
		return
	}
	count, err := skifService().SyncSubdealers(conn)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{"upserted": count}})
}

// SyncSkifStatuses триггерит ручной sync billing.company_status для всех компаний.
// POST /api/auth/skif/connections/:id/sync-statuses
func SyncSkifStatuses(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	conn, err := loadOwnedSkifConn(c, companyID)
	if err != nil {
		return
	}
	count, err := skifService().SyncCompanyStatuses(conn)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{"upserted": count}})
}

// BlockSkifCompany блокирует компанию SKIF.
// POST /api/auth/skif/connections/:id/companies/:companyId/block
// body: { block_type?: "terminals_not_block"|"terminals_block", pending?: bool }
func BlockSkifCompany(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	conn, err := loadOwnedSkifConn(c, companyID)
	if err != nil {
		return
	}
	skifCompanyID := c.Param("companyId")
	if skifCompanyID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "companyId обязателен"})
		return
	}
	var body struct {
		BlockType string `json:"block_type"`
		Pending   bool   `json:"pending"`
	}
	c.ShouldBindJSON(&body)
	if err := skifService().BlockCompany(conn, skifCompanyID, body.BlockType, body.Pending); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// UnblockSkifCompany снимает блокировку компании.
// POST /api/auth/skif/connections/:id/companies/:companyId/unblock
func UnblockSkifCompany(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	conn, err := loadOwnedSkifConn(c, companyID)
	if err != nil {
		return
	}
	skifCompanyID := c.Param("companyId")
	if skifCompanyID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "companyId обязателен"})
		return
	}
	if err := skifService().UnblockCompany(conn, skifCompanyID); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// GetSkifUnits возвращает юниты подключения из локального реестра.
// GET /api/auth/skif/connections/:id/units
func GetSkifUnits(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	conn, err := loadOwnedSkifConn(c, companyID)
	if err != nil {
		return
	}
	var units []models.SkifUnit
	if err := database.DB.Where("connection_id = ?", conn.ID).
		Order("name ASC").Limit(1000).Find(&units).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": units, "count": len(units)})
}

// parseHistoryDate парсит YYYY-MM-DD в начало дня UTC.
func parseHistoryDate(s string) (t time.Time, err error) {
	t, err = time.Parse("2006-01-02", s)
	return
}

// loadOwnedSkifConn загружает connection и проверяет company_id.
// Если ошибка — пишет JSON ответ и возвращает err.
func loadOwnedSkifConn(c *gin.Context, companyID uint) (*models.SkifConnection, error) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return nil, err
	}
	var conn models.SkifConnection
	if err := database.DB.First(&conn, uint(id)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return nil, err
	}
	if conn.CompanyID != companyID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return nil, err
	}
	return &conn, nil
}
