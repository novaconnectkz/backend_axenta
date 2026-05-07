package api

import (
	"net/http"
	"strconv"

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
