package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"
	"backend_axenta/services"
	"backend_axenta/utils"
)

// GELIOS Connections API — CRUD + test для GELIOS GPS интеграции.
//
// Routes регистрируются в main.go: /api/auth/gelios/connections/*
// База знаний: ACRM-Brain/wiki/sources/gelios-api/billing.md

func geliosService() *services.GeliosService {
	return services.NewGeliosService(database.DB)
}

// GetGeliosConnections возвращает список подключений текущей компании.
// GET /api/auth/gelios/connections
func GetGeliosConnections(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no company context"})
		return
	}
	var conns []models.GeliosConnection
	if err := database.DB.Where("company_id = ?", companyID).
		Order("id ASC").Find(&conns).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": conns})
}

// CreateGeliosConnection создаёт новое подключение.
// POST /api/auth/gelios/connections
// body: { name, base_url, username, password, sync_interval, auto_sync_enabled, sync_units }
func CreateGeliosConnection(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no company context"})
		return
	}
	var body struct {
		Name            string `json:"name" binding:"required"`
		Username        string `json:"username" binding:"required"`
		Password        string `json:"password" binding:"required"`
		SyncInterval    int    `json:"sync_interval"`
		AutoSyncEnabled bool   `json:"auto_sync_enabled"`
		SyncUnits       bool   `json:"sync_units"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.SyncInterval <= 0 {
		body.SyncInterval = 15
	}
	encPwd, err := utils.EncryptString(body.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "encrypt password: " + err.Error()})
		return
	}
	conn := models.GeliosConnection{
		CompanyID:       companyID,
		Name:            body.Name,
		BaseURL:         services.GeliosAllowedBaseURL, // SSRF: хост не настраиваемый
		Username:        body.Username,
		Password:        encPwd,
		SyncInterval:    body.SyncInterval,
		AutoSyncEnabled: body.AutoSyncEnabled,
		SyncUnits:       body.SyncUnits,
		IsActive:        true,
	}
	if err := database.DB.Create(&conn).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": conn})
}

// UpdateGeliosConnection обновляет подключение.
// PUT /api/auth/gelios/connections/:id
func UpdateGeliosConnection(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	conn, err := loadOwnedGeliosConn(c, companyID)
	if err != nil {
		return
	}
	// base_url намеренно НЕ принимаем (SSRF: GELIOS-хост фиксирован константой).
	var body struct {
		Name            *string `json:"name"`
		Username        *string `json:"username"`
		Password        *string `json:"password"`
		SyncInterval    *int    `json:"sync_interval"`
		AutoSyncEnabled *bool   `json:"auto_sync_enabled"`
		SyncUnits       *bool   `json:"sync_units"`
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
	if body.Username != nil {
		updates["username"] = *body.Username
		credsChanged = true
	}
	if body.Password != nil && *body.Password != "" {
		encPwd, err := utils.EncryptString(*body.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "encrypt password: " + err.Error()})
			return
		}
		updates["password"] = encPwd
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
	if body.IsActive != nil {
		updates["is_active"] = *body.IsActive
	}
	// При смене кредов сбрасываем сохранённый токен — следующий запрос сделает re-login.
	if credsChanged {
		updates["access_token"] = ""
		updates["refresh_token"] = ""
		updates["token_expires_at"] = nil
	}
	if err := database.DB.Model(conn).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	database.DB.First(conn, conn.ID)
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": conn})
}

// DeleteGeliosConnection удаляет подключение (cascade на gelios_units).
// DELETE /api/auth/gelios/connections/:id
func DeleteGeliosConnection(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	conn, err := loadOwnedGeliosConn(c, companyID)
	if err != nil {
		return
	}
	if err := database.DB.Delete(conn).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// TestGeliosConnection делает login + GET /units?limit=1 для проверки кредов.
// POST /api/auth/gelios/connections/:id/test
func TestGeliosConnection(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	conn, err := loadOwnedGeliosConn(c, companyID)
	if err != nil {
		return
	}
	info, err := geliosService().TestConnection(conn)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": info})
}

// SyncGeliosConnection триггерит синхронизацию объектов.
// POST /api/auth/gelios/connections/:id/sync
func SyncGeliosConnection(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	conn, err := loadOwnedGeliosConn(c, companyID)
	if err != nil {
		return
	}
	count, err := geliosService().SyncUnits(conn)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{"upserted": count}})
}

func loadOwnedGeliosConn(c *gin.Context, companyID uint) (*models.GeliosConnection, error) {
	if companyID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no company context"})
		return nil, fmt.Errorf("no company context")
	}
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return nil, err
	}
	// Грузим сразу с company_id-фильтром: чужое подключение → not found
	// (без 403/404-oracle, без бага «return nil,nil → nil-panic»).
	var conn models.GeliosConnection
	if err := database.DB.Where("id = ? AND company_id = ?", uint(id), companyID).
		First(&conn).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return nil, err
	}
	return &conn, nil
}
