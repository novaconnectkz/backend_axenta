package api

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

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

// GetGeliosHealth — операционная наблюдаемость per-connection (Gemini #3):
// sync-drift (overdue), login-rate, error-state, empty-streak. Read-only,
// tenant-scoped. Bloat gelios_units = Postgres autovacuum (ops, не код).
// GET /api/auth/gelios/health
func GetGeliosHealth(c *gin.Context) {
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
	now := time.Now()
	ageSec := func(t *time.Time) *int64 {
		if t == nil {
			return nil
		}
		v := int64(now.Sub(*t).Seconds())
		return &v
	}
	type connHealth struct {
		ID              uint   `json:"id"`
		Name            string `json:"name"`
		IsActive        bool   `json:"is_active"`
		AutoSyncEnabled bool   `json:"auto_sync_enabled"`
		SyncIntervalMin int    `json:"sync_interval_min"`
		UnitsCount      int    `json:"units_count"`
		LastSyncAgeSec  *int64 `json:"last_sync_age_sec"`
		Overdue         bool   `json:"overdue"` // нет успешного синка > 2×интервал
		LastLoginAgeSec *int64 `json:"last_login_age_sec"`
		LoginCount      int64  `json:"login_count"` // full-login (не refresh) — рост = refresh ломается
		ErrorCount      int    `json:"error_count"`
		LastError       string `json:"last_error,omitempty"`
		LastErrorAgeSec *int64 `json:"last_error_age_sec"`
		EmptySyncStreak int    `json:"empty_sync_streak"` // >0 = подозрение API-flap
		Status          string `json:"status"`            // ok | overdue | error | idle
	}
	out := make([]connHealth, 0, len(conns))
	for i := range conns {
		cn := &conns[i]
		interval := cn.SyncInterval
		if interval <= 0 {
			interval = 15
		}
		overdue := false
		if cn.IsActive && cn.AutoSyncEnabled {
			if cn.LastSyncAt == nil ||
				now.Sub(*cn.LastSyncAt) > time.Duration(2*interval)*time.Minute {
				overdue = true
			}
		}
		status := "ok"
		switch {
		case !cn.IsActive || !cn.AutoSyncEnabled:
			status = "idle"
		case cn.ErrorMessage != "":
			status = "error"
		case overdue:
			status = "overdue"
		}
		out = append(out, connHealth{
			ID: cn.ID, Name: cn.Name, IsActive: cn.IsActive,
			AutoSyncEnabled: cn.AutoSyncEnabled, SyncIntervalMin: interval,
			UnitsCount:      cn.UnitsCount,
			LastSyncAgeSec:  ageSec(cn.LastSyncAt),
			Overdue:         overdue,
			LastLoginAgeSec: ageSec(cn.LastLoginAt),
			LoginCount:      cn.LoginCount,
			ErrorCount:      cn.ErrorCount,
			LastError:       cn.ErrorMessage,
			LastErrorAgeSec: ageSec(cn.LastErrorAt),
			EmptySyncStreak: cn.EmptySyncStreak,
			Status:          status,
		})
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": out})
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
		SyncUsers       *bool  `json:"sync_users"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.SyncInterval <= 0 {
		body.SyncInterval = 15
	}
	syncUsers := true // дефолт ON (смысл фичи — accounts/users)
	if body.SyncUsers != nil {
		syncUsers = *body.SyncUsers
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
		SyncUsersFlag:   syncUsers,
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
		SyncUsers       *bool   `json:"sync_users"`
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
	if body.SyncUsers != nil {
		updates["sync_users"] = *body.SyncUsers
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
	svc := geliosService()
	count, err := svc.SyncUnits(conn)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "error": err.Error()})
		return
	}
	// Users sync — best-effort: ошибка не валит ответ (units уже синканы).
	usersCount := 0
	var usersErr string
	if conn.SyncUsersFlag {
		if uc, e := svc.SyncGeliosUsers(conn); e != nil {
			usersErr = e.Error()
		} else {
			usersCount = uc
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{
		"upserted": count, "users_upserted": usersCount, "users_error": usersErr,
	}})
}

// CreateGeliosUserHandler — POST /gelios/connections/:id/users.
// Защита от дурака:
//   - tenant-scope: connection только своей компании (loadOwnedGeliosConn);
//   - creator_id обязан быть в дереве ЭТОГО connection (синканные
//     gelios_user_id ∪ их creator_id = узлы+корень) — нельзя создать под
//     произвольным GELIOS id; GELIOS 403 — финальный страховочный net;
//   - длины login/password валидируются в сервисе до сетевого вызова.
//
// После успеха — синхронный re-sync users (новый юзер сразу в выдаче).
func CreateGeliosUserHandler(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	conn, err := loadOwnedGeliosConn(c, companyID)
	if err != nil {
		return
	}
	var req services.GeliosUserCreateReq
	if e := c.ShouldBindJSON(&req); e != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": e.Error()})
		return
	}

	// creator_id ∈ {gelios_user_id} ∪ {creator_id} для этого connection
	// = «синканные узлы + корень дерева». Сравнение текст-в-текст по
	// gelios_user_id (varchar — БЕЗ CAST: нечисловой id иначе сломал бы
	// весь запрос, Codex High) и числом по creator_id. .Error проверяем.
	creatorStr := strconv.FormatInt(req.CreatorID, 10)
	var allowed int64
	if e := database.DB.Raw(`
		SELECT COUNT(*) FROM gelios_users
		WHERE connection_id = ? AND gelios_deleted_at IS NULL
		  AND (gelios_user_id = ? OR creator_id = ?)
	`, conn.ID, creatorStr, req.CreatorID).Scan(&allowed).Error; e != nil {
		log.Printf("⚠️ GELIOS create: creator allow-list query conn=%d: %v", conn.ID, e)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ошибка проверки creator_id"})
		return
	}
	if allowed == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "creator_id вне дерева подключения (выберите существующего GELIOS-пользователя как создателя)",
		})
		return
	}

	newID, e := geliosService().CreateGeliosUser(conn, req)
	if e != nil {
		// Полировка статусов: локальная валидация → 400; GELIOS 403
		// (дубль login / structurally impossible) → 409 Conflict; GELIOS
		// 422 (field) → 422; прочий upstream-сбой → 502.
		status := http.StatusBadGateway
		if oe, ok := e.(*services.GeliosUserOpError); ok {
			switch {
			case oe.Local:
				status = http.StatusBadRequest
			case oe.UpstreamStatus == http.StatusForbidden:
				status = http.StatusConflict
			case oe.UpstreamStatus == http.StatusUnprocessableEntity:
				status = http.StatusUnprocessableEntity
			}
		}
		c.JSON(status, gin.H{"status": "error", "error": e.Error()})
		return
	}
	// Сразу подтянуть дерево (новый юзер появится в unified/users|accounts).
	if _, se := geliosService().SyncGeliosUsers(conn); se != nil {
		log.Printf("⚠️ GELIOS post-create re-sync conn=%d: %v", conn.ID, se)
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{"gelios_user_id": newID}})
}

// ListGeliosCreatorsHandler — GET /gelios/connections/:id/creators.
// Допустимые «создатели» для нового юзера = тот же allow-list что
// форсит CreateGeliosUserHandler: синканные узлы дерева + корень
// (creator_id top-level, ≈ ГАРАЖ24/token-owner). Tenant-scoped.
func ListGeliosCreatorsHandler(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	conn, err := loadOwnedGeliosConn(c, companyID)
	if err != nil {
		return
	}
	type creatorOpt struct {
		GeliosID int64  `json:"gelios_id"`
		Login    string `json:"login"`
	}
	var opts []creatorOpt
	// Узлы: gelios_user_id (varchar→bigint безопасно, id GELIOS числовой)
	// + корень: creator_id, которого нет среди gelios_user_id (top-level).
	if e := database.DB.Raw(`
		SELECT DISTINCT gelios_user_id::bigint AS gelios_id, login
		FROM gelios_users
		WHERE connection_id = ? AND gelios_deleted_at IS NULL
		  AND gelios_user_id ~ '^[0-9]+$'
		UNION
		SELECT DISTINCT gu.creator_id AS gelios_id, gu.creator_login AS login
		FROM gelios_users gu
		WHERE gu.connection_id = ? AND gu.gelios_deleted_at IS NULL
		  AND gu.creator_id > 0
		  AND NOT EXISTS (
		    SELECT 1 FROM gelios_users x
		    WHERE x.connection_id = gu.connection_id
		      AND x.gelios_deleted_at IS NULL
		      AND x.gelios_user_id = gu.creator_id::text
		  )
		ORDER BY login
	`, conn.ID, conn.ID).Scan(&opts).Error; e != nil {
		log.Printf("⚠️ GELIOS creators list conn=%d: %v", conn.ID, e)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ошибка списка создателей"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": opts})
}

// DeleteGeliosUserHandler — DELETE /gelios/connections/:id/users/:userId.
// Защита от дурака:
//   - tenant-scope (loadOwnedGeliosConn);
//   - удаляем ТОЛЬКО известный синканный gelios_users-узел этого connection
//     (gelios_deleted_at IS NULL). root/token-user НЕ входит в gelios_users
//     (GET /users его не отдаёт) → структурно недостижим для удаления;
//   - GELIOS DELETE = hard + idempotent (403 Access denied = уже нет);
//   - после успеха soft-mark gelios_deleted_at локально (мгновенный UI) +
//     re-sync (consistency с реальным деревом).
func DeleteGeliosUserHandler(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	conn, err := loadOwnedGeliosConn(c, companyID)
	if err != nil {
		return
	}
	geliosUserID := c.Param("userId")
	if geliosUserID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "userId обязателен"})
		return
	}

	var row models.GeliosUser
	if e := database.DB.Where(
		"connection_id = ? AND gelios_user_id = ? AND gelios_deleted_at IS NULL",
		conn.ID, geliosUserID).First(&row).Error; e != nil {
		// Не наш синканный узел (или root, или чужой) → отказ. Никаких
		// произвольных GELIOS id, никакого удаления корня.
		c.JSON(http.StatusNotFound, gin.H{
			"error": "пользователь не найден среди синканных узлов этого подключения",
		})
		return
	}

	if e := geliosService().DeleteGeliosUser(conn, geliosUserID); e != nil {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "error": e.Error()})
		return
	}
	// НЕ ставим soft-mark вручную: idempotent-403 мог быть не «уже нет», а
	// потеря прав/scope (Codex Med). re-sync = единственный источник правды:
	// completeness-gate NOT-IN пометит реально удалённого, реально живого
	// оставит (никакой лжи в БД). Если re-sync «занят»/упал — scheduler
	// согласует ≤интервал; UI кратко устаревший, но ПРАВДИВЫЙ.
	resynced := true
	if _, se := geliosService().SyncGeliosUsers(conn); se != nil {
		resynced = false
		log.Printf("⚠️ GELIOS post-delete re-sync conn=%d: %v (scheduler согласует)", conn.ID, se)
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{"resynced": resynced}})
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
