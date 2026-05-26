package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"
	"backend_axenta/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AccountsHandler обрабатывает запросы к API учетных записей Axenta
type AccountsHandler struct{}

// NewAccountsHandler создает новый обработчик учетных записей
func NewAccountsHandler() *AccountsHandler {
	return &AccountsHandler{}
}

// Account представляет структуру учетной записи из Axenta API
type Account struct {
	ID                 int     `json:"id"`
	Name               string  `json:"name"`
	Type               string  `json:"type"`
	AdminFullname      string  `json:"adminFullname"`
	AdminID            int     `json:"adminId"`
	AdminIsActive      bool    `json:"adminIsActive"`
	ParentAccountName  string  `json:"parentAccountName"`
	ObjectsActive      int     `json:"objectsActive"`
	ObjectsTotal       int     `json:"objectsTotal"`
	ObjectsDeleted     int     `json:"objectsDeleted"`
	Comment            *string `json:"comment"`
	IsActive           bool    `json:"isActive"`
	BlockingDatetime   *string `json:"blockingDatetime"`
	Hierarchy          string  `json:"hierarchy"`
	DaysBeforeBlocking *int    `json:"daysBeforeBlocking"`
	CreationDatetime   string  `json:"creationDatetime"`
}

// AccountsResponse представляет ответ API со списком учетных записей
type AccountsResponse struct {
	Count    int       `json:"count"`
	Next     *string   `json:"next"`
	Previous *string   `json:"previous"`
	Results  []Account `json:"results"`
	// Ф3-E/Codex Q2: true когда snapshot устарел (>TTL) — отдаём
	// последние известные строки + флаг (паритет с objects/users
	// Ф3-B: stale → данные+degraded, НЕ пусто). omitempty: на свежем
	// ответе поля нет (1:1 со старым proxy-форматом).
	Degraded bool `json:"degraded,omitempty"`
}

// GetAccounts получает список учётных записей из локального snapshot.
//
// Ф3-E/Codex: РАНЬШЕ при пустом/устаревшем snapshot
// (tryServeAccountsFromSnapshot=false) handler ПРОКСИРОВАЛ в
// axenta.cloud по request-токену. После Ф1 request-токен = локальный
// JWT → axenta.cloud отдаёт 401 → экран аккаунтов залочен (ровно
// cutover-сценарий). Это была дыра Ф3-B (objects/users list рерайчены,
// accounts list — нет; найдено на staging Ф3-E). Теперь контракт как
// у objects/users: всегда 200, snapshot-only, ноль request-token proxy.
//
//   - snapshot свежий+есть → реальные данные (tryServe, degraded=false);
//   - snapshot устарел (>TTL) → последние известные строки +
//     degraded:true (Ф3-E/Codex Q2 паритет с objects/users — НЕ пусто);
//   - нет БД / пустой snapshot / SQL-ошибка → 200 + пустой список +
//     degraded:true (НЕ 4xx/5xx, НЕ proxy). Axenta Sync Scheduler
//     наполнит snapshot в течение цикла — экран сам оживёт.
func (h *AccountsHandler) GetAccounts(c *gin.Context) {
	page := c.DefaultQuery("page", "1")
	perPage := c.DefaultQuery("per_page", "50")
	ordering := c.DefaultQuery("ordering", "name")
	search := c.Query("search")
	accountType := c.Query("type")
	isActive := c.Query("is_active")

	// snapshot живёт в tenant-схеме (tenant_<id>), НЕ в public.
	// Ф3-D #4/Codex Q4-симметрия: nil tenantDB → degraded, public не трогаем.
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB != nil {
		if resp, ok := tryServeAccountsFromSnapshot(tenantDB, page, perPage, ordering, search, accountType, isActive); ok {
			c.JSON(http.StatusOK, resp)
			return
		}
	}

	// Snapshot не отдал (пусто/устарел/nil/SQL-err) — деградируем,
	// НЕ проксируем в axenta.cloud (после Ф1 = 401).
	pageNum, _ := strconv.Atoi(page)
	if pageNum < 1 {
		pageNum = 1
	}
	c.JSON(http.StatusOK, gin.H{
		"count":     0,
		"next":      nil,
		"previous":  nil,
		"results":   []Account{},
		"degraded":  true,
		"page":      pageNum,
		"page_size": perPage,
	})
}

// GetAccount получает конкретную учетную запись по ID.
//
// Ф3-D #4 (2026-05-19): раньше — live-proxy GET axenta.cloud/api/cms/
// accounts/:id/ по request-`Authorization`. После Ф1 request-токен =
// локальный JWT → axenta.cloud вернул бы 401. Переведено на snapshot-only
// (см. serveAccountFromSnapshot), как read-path Ф3-B. Тонкая обёртка —
// вся логика и контракт деградации в одной функции.
func (h *AccountsHandler) GetAccount(c *gin.Context) {
	serveAccountFromSnapshot(c)
}

// CreateAccount создает новую учетную запись через прокси к Axenta API
func (h *AccountsHandler) CreateAccount(c *gin.Context) {
	// Читаем тело запроса
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Failed to read request body: " + err.Error(),
		})
		return
	}

	// Ф3-C: server-side Axenta-токен (креды компании), НЕ request-токен.
	// ok=false → degraded-ответ записан, мутацию НЕ делаем.
	axToken, ok := axentaServerTokenFor(c)
	if !ok {
		return
	}

	axentaURL := "https://axenta.cloud/api/cms/accounts/"
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	doReq := func(token string) (int, []byte, error) {
		httpReq, e := http.NewRequest("POST", axentaURL, bytes.NewBuffer(body))
		if e != nil {
			return 0, nil, e
		}
		httpReq.Header.Set("Authorization", "Token "+token)
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "application/json")
		rsp, e := client.Do(httpReq)
		if e != nil {
			return 0, nil, e
		}
		defer rsp.Body.Close()
		b, e := io.ReadAll(rsp.Body)
		return rsp.StatusCode, b, e
	}

	statusCode, respBody, err := doReq(axToken)
	if err != nil {
		axentaMutationUnavailable(c, err)
		return
	}

	// Downstream 401/403 — server-токен компании отозван: сброс + 1 retry.
	if isAxentaAuthError(statusCode) {
		invalidateAxentaServerToken(c)
		axToken2, ok2 := axentaServerTokenFor(c)
		if !ok2 {
			return
		}
		statusCode, respBody, err = doReq(axToken2)
		if err != nil {
			axentaMutationUnavailable(c, err)
			return
		}
	}

	fmt.Printf("📥 CreateAccount via server-token: %d\n", statusCode)

	// Если Axenta API вернул ошибку, передаем её клиенту
	if statusCode >= 400 {
		c.JSON(statusCode, gin.H{
			"status": "error",
			"error":  "Axenta API error: " + string(respBody),
		})
		return
	}

	// Парсим успешный ответ
	var account Account
	if err := json.Unmarshal(respBody, &account); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to parse Axenta API response: " + err.Error(),
		})
		return
	}

	// Триггерим резинк snapshot'ов чтобы новый аккаунт появился в /unified/accounts
	// и в KPI dashboard без ожидания scheduled cron.
	if adminID := middleware.GetTrustedAdminAccountID(c); adminID != 0 {
		services.GetSnapshotInvalidator().Invalidate(adminID, "account.create")

		// Точечный upsert AxentaUserSnapshot для admin'а нового аккаунта.
		// Без этого hook'а свежий admin не появится в /users до следующего полного
		// AxentaSync (а sync пишет в логах "обновлено=N", но реально INSERT'ом
		// свежие composite (admin_account_id, external_user_id) не добавляются —
		// см. project_acrm_unified_accounts. Этот hook гарантирует моментальное
		// появление admin'а в /users сразу после создания аккаунта).
		if account.AdminID > 0 {
			tenantDB := middleware.GetTenantDB(c)
			if tenantDB != nil {
				upsertAdminUserSnapshot(tenantDB, adminID, account)
			}
		}
	}

	// Возвращаем данные
	c.JSON(http.StatusCreated, account)
}

// upsertAdminUserSnapshot записывает admin-юзера созданного аккаунта в
// axenta_user_snapshots, чтобы /users показал его сразу без ожидания AxentaSync.
// raw_payload содержит JSON с полями account-info, чтобы downstream-логика
// (которая может ожидать конкретные поля) не падала на пустом payload.
func upsertAdminUserSnapshot(db *gorm.DB, adminAccountID uint, account Account) {
	rawPayload, _ := json.Marshal(map[string]any{
		"id":                account.AdminID,
		"username":          account.AdminFullname,
		"name":              account.AdminFullname,
		"is_active":         account.AdminIsActive,
		"creation_datetime": account.CreationDatetime,
		"account_id":        account.ID,
		"account_name":      account.Name,
		"source":            "create_account_hook",
	})

	snapshot := models.AxentaUserSnapshot{
		AdminAccountID:   adminAccountID,
		ExternalUserID:   int64(account.AdminID),
		Username:         account.AdminFullname,
		Name:             account.AdminFullname,
		AccountType:      account.Type,
		IsActive:         account.AdminIsActive,
		CreationDatetime: account.CreationDatetime,
		LastSyncedAt:     time.Now().UTC(),
		RawPayload:       string(rawPayload),
	}

	if err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "admin_account_id"}, {Name: "external_user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"username", "name", "account_type", "is_active",
			"creation_datetime", "last_synced_at", "raw_payload",
		}),
	}).Create(&snapshot).Error; err != nil {
		log.Printf("⚠️ upsertAdminUserSnapshot account=%d admin=%d: %v", account.ID, account.AdminID, err)
		return
	}
	log.Printf("✅ upsertAdminUserSnapshot: admin %d (%s) аккаунта %d записан в /users snapshot",
		account.AdminID, account.AdminFullname, account.ID)
}

// AccountMoveRequest представляет запрос на перемещение учетной записи
type AccountMoveRequest struct {
	AccountID       int `json:"accountId" binding:"required,min=1"`
	TargetAccountID int `json:"targetAccountId" binding:"required,min=1"`
}

// MoveAccount перемещает учетную запись и все её данные к другому партнеру
func (h *AccountsHandler) MoveAccount(c *gin.Context) {
	// Парсим запрос
	var moveRequest AccountMoveRequest
	if err := c.ShouldBindJSON(&moveRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid request data: " + err.Error(),
		})
		return
	}

	// Проверяем, что аккаунты разные
	if moveRequest.AccountID == moveRequest.TargetAccountID {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Account cannot be moved to itself",
		})
		return
	}

	// Ф3-C: server-side Axenta-токен (креды компании), НЕ request-токен.
	axToken, ok := axentaServerTokenFor(c)
	if !ok {
		return
	}

	axentaURL := "https://axenta.cloud/api/cms/accounts/change_account/"
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	requestData := map[string]interface{}{
		"accountId":       moveRequest.AccountID,
		"targetAccountId": moveRequest.TargetAccountID,
	}
	jsonData, err := json.Marshal(requestData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to prepare request data: " + err.Error(),
		})
		return
	}

	doReq := func(token string) (int, []byte, error) {
		httpReq, e := http.NewRequest("POST", axentaURL, bytes.NewBuffer(jsonData))
		if e != nil {
			return 0, nil, e
		}
		httpReq.Header.Set("Authorization", "Token "+token)
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "application/json")
		rsp, e := client.Do(httpReq)
		if e != nil {
			return 0, nil, e
		}
		defer rsp.Body.Close()
		b, e := io.ReadAll(rsp.Body)
		return rsp.StatusCode, b, e
	}

	fmt.Printf("🔄 Moving account %d → %d via Axenta (server-token)\n",
		moveRequest.AccountID, moveRequest.TargetAccountID)

	statusCode, body, err := doReq(axToken)
	if err != nil {
		axentaMutationUnavailable(c, err)
		return
	}

	// Downstream 401/403 — server-токен компании отозван: сброс + 1 retry.
	if isAxentaAuthError(statusCode) {
		invalidateAxentaServerToken(c)
		axToken2, ok2 := axentaServerTokenFor(c)
		if !ok2 {
			return
		}
		statusCode, body, err = doReq(axToken2)
		if err != nil {
			axentaMutationUnavailable(c, err)
			return
		}
	}

	// Проверяем статус ответа
	if statusCode != http.StatusOK {
		c.JSON(statusCode, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Axenta API error: %s", string(body)),
		})
		return
	}

	fmt.Printf("✅ Successfully moved account %d to target account %d\n",
		moveRequest.AccountID, moveRequest.TargetAccountID)

	// Триггерим резинк snapshot'ов — иерархия аккаунтов изменилась.
	if adminID := middleware.GetTrustedAdminAccountID(c); adminID != 0 {
		services.GetSnapshotInvalidator().Invalidate(adminID, "account.move")
	}

	// Возвращаем успешный ответ
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Account moved successfully",
		"data": gin.H{
			"accountId":       moveRequest.AccountID,
			"targetAccountId": moveRequest.TargetAccountID,
		},
	})
}

// ToggleAccountStatus — proxy к Axenta Cloud API /cms/accounts/:id/activate/.
// Без proxy frontend дёргал axenta.cloud напрямую, мы не знали о мутации,
// snapshot оставался устаревшим до cron (10 мин). Теперь после успешного
// тогла триггерим SnapshotInvalidator → snapshot обновляется через 5-10s.
//
// Body: {"state": true|false}. Возвращает ответ Axenta как есть.
func (h *AccountsHandler) ToggleAccountStatus(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "ID не указан"})
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Failed to read request body: " + err.Error()})
		return
	}

	// Ф3-C: server-side Axenta-токен (креды компании), НЕ request-токен.
	axToken, ok := axentaServerTokenFor(c)
	if !ok {
		return
	}

	axentaURL := fmt.Sprintf("https://axenta.cloud/api/cms/accounts/%s/activate/", id)
	client := &http.Client{Timeout: 30 * time.Second}

	doReq := func(tk string) (int, []byte, error) {
		httpReq, e := http.NewRequest("POST", axentaURL, bytes.NewBuffer(body))
		if e != nil {
			return 0, nil, e
		}
		httpReq.Header.Set("Authorization", "Token "+tk)
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "application/json")
		rsp, e := client.Do(httpReq)
		if e != nil {
			return 0, nil, e
		}
		defer rsp.Body.Close()
		b, e := io.ReadAll(rsp.Body)
		return rsp.StatusCode, b, e
	}

	statusCode, respBody, err := doReq(axToken)
	if err != nil {
		axentaMutationUnavailable(c, err)
		return
	}

	// Downstream 401/403 — server-токен компании отозван: сброс + 1 retry.
	if isAxentaAuthError(statusCode) {
		invalidateAxentaServerToken(c)
		axToken2, ok2 := axentaServerTokenFor(c)
		if !ok2 {
			return
		}
		axToken = axToken2
		statusCode, respBody, err = doReq(axToken)
		if err != nil {
			axentaMutationUnavailable(c, err)
			return
		}
	}

	if statusCode >= 400 {
		c.Data(statusCode, "application/json", respBody)
		return
	}

	// Точечный re-fetch конкретного аккаунта — намного быстрее SyncAdmin (~200ms vs ~30s).
	// СИНХРОННО до ответа, чтобы frontend сразу после 201 видел обновлённый snapshot.
	// При ошибке refresh — fallback на async SnapshotInvalidator.
	// Ф3-C: RefreshAccount тоже на server-токене (request-токен после Ф1 невалиден).
	token := axToken

	// Ф3-C/Codex: adminID ТОЛЬКО из доверенного claim. getAccountIDFromToken
	// на server-токене вернул бы КОРНЕВОЙ аккаунт компании (не tenant-admin) →
	// refresh/invalidate ушёл бы не в ту схему. adminID==0 → sync-refresh
	// пропускаем (guard ниже), cron подхватит.
	adminID := middleware.GetTrustedAdminAccountID(c)

	// Ф3-C/Codex: ДОВЕРЕННЫЙ tenantDB из SetTenant (signed claim) — ПЕРЕД
	// клиентским X-Tenant-ID. После zone2-фикса cmsGroup имеет SetTenant,
	// поэтому GetTenantDB(c) непуст и привязан к аутентифицированной компании.
	// X-Tenant-ID (client-controlled) — только legacy-fallback если trusted
	// почему-то пуст; иначе аутентифицированный юзер форжил бы чужую схему
	// (cross-tenant запись через RefreshAccount).
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		if tenantIDStr := c.GetHeader("X-Tenant-ID"); tenantIDStr != "" {
			if tid, err := strconv.ParseUint(tenantIDStr, 10, 32); err == nil {
				tenantDB = database.GetTenantDBByID(uint(tid))
			}
		}
	}
	if tenantDB == nil {
		tenantDB = database.GetDB()
	}

	accIDInt, _ := strconv.Atoi(id)
	refreshed := false
	if adminID > 0 && accIDInt > 0 {
		if syncSvc := services.GetAxentaSyncService(); syncSvc != nil {
			if _, rErr := syncSvc.RefreshAccount(token, adminID, accIDInt, tenantDB); rErr == nil {
				refreshed = true
			} else {
				log.Printf("⚠️ RefreshAccount fail (account=%d): %v — fallback на async SyncAdmin", accIDInt, rErr)
			}
		}
		if !refreshed {
			services.GetSnapshotInvalidator().Invalidate(adminID, "account.toggle_status.fallback")
		}
	}

	c.Data(statusCode, "application/json", respBody)
}

// PatchAccount — proxy PATCH /api/cms/accounts/:id в Axenta Cloud (через
// server-токен компании). Используется FE для частичных правок снимка
// (clearBlockingDatetime, name, comment и т.п.) — раньше FE ходил напрямую
// в axenta.cloud, после AUTH_MODE=local cutover это 401.
//
// Body: произвольный JSON (передаём как есть). Axenta вернёт обновлённый
// объект. После успеха триггерим SnapshotInvalidator чтоб snapshot не
// отставал.
func (h *AccountsHandler) PatchAccount(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "ID не указан"})
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Failed to read request body: " + err.Error()})
		return
	}

	axToken, ok := axentaServerTokenFor(c)
	if !ok {
		return
	}

	axentaURL := fmt.Sprintf("https://axenta.cloud/api/cms/accounts/%s/", id)
	client := &http.Client{Timeout: 30 * time.Second}

	doReq := func(tk string) (int, []byte, error) {
		httpReq, e := http.NewRequest("PATCH", axentaURL, bytes.NewBuffer(body))
		if e != nil {
			return 0, nil, e
		}
		httpReq.Header.Set("Authorization", "Token "+tk)
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "application/json")
		rsp, e := client.Do(httpReq)
		if e != nil {
			return 0, nil, e
		}
		defer rsp.Body.Close()
		b, e := io.ReadAll(rsp.Body)
		return rsp.StatusCode, b, e
	}

	statusCode, respBody, err := doReq(axToken)
	if err != nil {
		axentaMutationUnavailable(c, err)
		return
	}
	if isAxentaAuthError(statusCode) {
		invalidateAxentaServerToken(c)
		tk2, ok2 := axentaServerTokenFor(c)
		if !ok2 {
			return
		}
		statusCode, respBody, err = doReq(tk2)
		if err != nil {
			axentaMutationUnavailable(c, err)
			return
		}
	}

	if statusCode < 400 {
		if adminID := middleware.GetTrustedAdminAccountID(c); adminID > 0 {
			services.GetSnapshotInvalidator().Invalidate(adminID, "account.patch")
		}
	}

	c.Data(statusCode, "application/json", respBody)
}

// RefreshSingleAccount — manual точечный refresh учётки по ID. Полезно когда
// пользователь поменял статус в Axenta CMS снаружи и хочет увидеть актуальные
// данные в ACRM не дожидаясь cron (10 мин).
//
// POST /api/auth/accounts/:id/refresh
// Возвращает обновлённую запись из snapshot или 404 если не найдена.
func (h *AccountsHandler) RefreshSingleAccount(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "ID не указан"})
		return
	}
	accIDInt, err := strconv.Atoi(id)
	if err != nil || accIDInt <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Некорректный ID"})
		return
	}

	// Ф3-C: server-side Axenta-токен (креды компании), НЕ request-токен —
	// после Ф1 логин = локальный JWT, невалиден для axenta.cloud/RefreshAccount.
	token, ok := axentaServerTokenFor(c)
	if !ok {
		return
	}

	// Ф3-C/Codex: adminID ТОЛЬКО из доверенного claim. getAccountIDFromToken
	// на server-токене вернул бы корневой аккаунт компании → refresh ушёл бы
	// не в ту схему. Нет adminID — не угадываем, честная ошибка.
	adminID := middleware.GetTrustedAdminAccountID(c)
	if adminID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Не удалось определить admin account из сессии"})
		return
	}

	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		tenantDB = database.GetDB()
	}

	syncSvc := services.GetAxentaSyncService()
	if syncSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "error": "AxentaSyncService недоступен"})
		return
	}

	// Один attempt (как до Ф3-C): RefreshAccount возвращает нетипизированную
	// ошибку — retry-on-any-error плодил бы повторный логин+запрос на 404/5xx.
	acc, err := syncSvc.RefreshAccount(token, adminID, accIDInt, tenantDB)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "error": "Refresh failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"id":       acc.ID,
			"name":     acc.Name,
			"isActive": acc.IsActive,
		},
	})
}

// tryServeAccountsFromSnapshot читает учётные записи из локального snapshot
// (axenta_account_snapshots). Возвращает (response, true) если в snapshot
// ЕСТЬ данные (свежие ИЛИ устаревшие), иначе (nil, false) — caller отдаёт
// пустой degraded-ответ. Ноль обращений в axenta.cloud (после Ф1
// request-токен невалиден — см. GetAccounts).
//
// Ф3-E/Codex Q2: TTL (SnapshotTTL 60м) больше НЕ повод вернуть false.
// Устарел → отдаём последние известные строки + resp.Degraded=true
// (паритет с objects/users Ф3-B: stale → данные+degraded, НЕ пусто).
// false только при реальном отсутствии данных (нет БД / пустой snapshot /
// SQL-ошибка).
func tryServeAccountsFromSnapshot(db *gorm.DB, page, perPage, ordering, search, accountType, isActive string) (*AccountsResponse, bool) {
	if db == nil {
		return nil, false
	}

	// Проверяем наличие хотя бы одной записи и свежесть последнего sync
	var lastSync time.Time
	if err := db.
		Model(&models.AxentaAccountSnapshot{}).
		Select("MAX(last_synced_at)").
		Scan(&lastSync).Error; err != nil || lastSync.IsZero() {
		return nil, false
	}

	// Устарел? — НЕ повод проксировать (после Ф1 = 401). Отдаём что есть
	// с degraded:true. Axenta Sync Scheduler освежит в течение цикла.
	stale := time.Since(lastSync) > SnapshotTTL
	if stale {
		fmt.Printf("⏰ Accounts snapshot устарел (last_synced_at=%v, age=%v > TTL=%v) — отдаём из snapshot + degraded (без proxy)\n",
			lastSync.Format(time.RFC3339), time.Since(lastSync).Round(time.Second), SnapshotTTL)
	}

	q := db.Model(&models.AxentaAccountSnapshot{})

	// Фильтры
	if search != "" {
		pattern := "%" + strings.ToLower(search) + "%"
		q = q.Where("LOWER(account_name) LIKE ? OR LOWER(admin_fullname) LIKE ? OR LOWER(parent_account_name) LIKE ?", pattern, pattern, pattern)
	}
	if accountType != "" {
		q = q.Where("account_type = ?", accountType)
	}
	if isActive != "" {
		if isActive == "true" {
			q = q.Where("is_active = ?", true)
		} else if isActive == "false" {
			q = q.Where("is_active = ?", false)
		}
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		fmt.Printf("⚠️ Snapshot count failed: %v\n", err)
		return nil, false
	}

	// Сортировка: маппим имена полей фронта (camelCase) на колонки snapshot
	orderClause := snapshotOrderClause(ordering)

	// Пагинация
	pageNum, _ := strconv.Atoi(page)
	if pageNum < 1 {
		pageNum = 1
	}
	pageSize, _ := strconv.Atoi(perPage)
	if pageSize <= 0 {
		pageSize = 50
	}
	offset := (pageNum - 1) * pageSize

	var rows []models.AxentaAccountSnapshot
	if err := q.Order(orderClause).Limit(pageSize).Offset(offset).Find(&rows).Error; err != nil {
		fmt.Printf("⚠️ Snapshot query failed: %v\n", err)
		return nil, false
	}

	// Преобразуем snapshot rows в формат Axenta API (Account)
	results := make([]Account, 0, len(rows))
	for _, r := range rows {
		results = append(results, accountFromSnapshotRow(r))
	}

	resp := &AccountsResponse{
		Count:    int(total),
		Next:     nil,
		Previous: nil,
		Results:  results,
		Degraded: stale,
	}
	fmt.Printf("⚡ Accounts snapshot served: total=%d, page=%d, returned=%d, degraded=%v (last_sync=%v)\n", total, pageNum, len(results), stale, lastSync.Format(time.RFC3339))
	return resp, true
}

// accountFromSnapshotRow конвертирует одну строку axenta_account_snapshots в
// формат Axenta API (Account). ЕДИНЫЙ источник маппинга для списка
// (tryServeAccountsFromSnapshot) и детали (serveAccountFromSnapshot) — чтобы
// карточка и строка списка не разъезжались (тот же принцип что фильтр-хелперы
// Ф3-B B4). RawPayload (оригинальный JSON Axenta) — основной источник, колонки
// — подстраховка при неполном payload.
func accountFromSnapshotRow(r models.AxentaAccountSnapshot) Account {
	var acc Account
	if r.RawPayload != "" {
		_ = json.Unmarshal([]byte(r.RawPayload), &acc)
	}
	if acc.ID == 0 {
		acc.ID = int(r.ExternalAccountID)
	}
	if acc.Name == "" {
		acc.Name = r.AccountName
	}
	if acc.Type == "" {
		acc.Type = r.AccountType
	}
	if acc.AdminFullname == "" {
		acc.AdminFullname = r.AdminFullname
	}
	if acc.Hierarchy == "" {
		acc.Hierarchy = r.Hierarchy
	}
	if acc.ParentAccountName == "" {
		acc.ParentAccountName = r.ParentAccountName
	}
	if acc.ObjectsTotal == 0 {
		acc.ObjectsTotal = r.ObjectsTotal
	}
	if acc.ObjectsActive == 0 {
		acc.ObjectsActive = r.ObjectsActive
	}
	if !acc.IsActive {
		acc.IsActive = r.IsActive
	}
	if acc.CreationDatetime == "" && !r.CreatedAt.IsZero() {
		acc.CreationDatetime = r.CreatedAt.Format(time.RFC3339)
	}
	return acc
}

// serveAccountFromSnapshot — Ф3-D #4: detail-read GetAccount читается из
// локального snapshot, БЕЗ proxy в axenta.cloud по request-токену (после Ф1
// request-токен = локальный JWT, для axenta.cloud невалиден → был бы 401).
//
// Контракт (производный от Ф3-B, адаптирован под by-id resource):
//   - снапшот свежий + строка есть → 200 Account (как старый proxy: bare JSON,
//     FE читает response.data напрямую);
//   - снапшот есть, но устарел (>TTL) → ВСЁ РАВНО 200 Account (отдаём что
//     есть, НЕ проксируем — принцип Ф3-B грабля #1), факт логируем;
//   - снапшот свежий, строки для :id нет → 404 (честный resource-not-found,
//     НЕ infra-сбой; FE catch это уже обрабатывает как раньше proxy-404);
//   - tenantDB нет / таблицы нет / SQL-ошибка / снапшот ещё не наполнен →
//     200 {degraded:true} (как Ф3-B list: не 5xx из-за отсутствия infra;
//     этот edge практически недостижим — без списка нет клика в деталь).
//
// Никогда: 401/500 из-за proxy, обращение в axenta.cloud по request-токену.
func serveAccountFromSnapshot(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Invalid account ID"})
		return
	}

	// snapshot живёт в tenant-схеме (tenant_<id>), НЕ в public.
	// Ф3-D #4/Codex Q4: НЕТ fallback на database.DB (public). Если
	// доверенный SetTenant не дал tenant_db — читать из public нельзя
	// (там этой tenant-таблицы нет / search_path-quirk долга #15 →
	// чужая/пустая выборка). Контракт: tenantDB nil → сразу degraded.
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(http.StatusOK, gin.H{"degraded": true})
		return
	}

	// Свежесть snapshot (как tryServeAccountsFromSnapshot). Пустой/недоступный
	// snapshot → degraded (не 5xx). Устаревший — НЕ повод (отдаём ниже что есть).
	var lastSync time.Time
	if err := tenantDB.Model(&models.AxentaAccountSnapshot{}).
		Select("MAX(last_synced_at)").Scan(&lastSync).Error; err != nil || lastSync.IsZero() {
		c.JSON(http.StatusOK, gin.H{"degraded": true})
		return
	}

	// Ф3-D #4/Codex Q2: composite-unique (admin_account_id, external_account_id)
	// → в одной tenant-схеме теоретически >1 строки с одним external_account_id.
	// НЕ добавляем admin-фильтр: GetTrustedAdminAccountID=company.ID ≠
	// legacy-sync firstCompany.ID (закрытый долг #2) → admin-фильтр дал бы
	// 0 строк. Дубли в проде = транзиентный orphan от смены axetna_login,
	// чистится 7д orphan-cleanup (долг #2). Детерминизм: берём свежайшую по
	// last_synced_at (= строка актуального admin'а, не протухший orphan) —
	// стабильный, предсказуемый выбор; расхождение со списком (он Find'ит
	// все, pre-existing list-показ-дублей вне scope #4) ограничено и
	// само-исцеляется.
	var row models.AxentaAccountSnapshot
	if err := tenantDB.Where("external_account_id = ?", int64(id)).
		Order("last_synced_at DESC").
		Order("id ASC"). // явный tiebreak при равном last_synced_at (mass-sync = один timestamp) — ORM-независимый детерминизм (Codex Q2 minor)
		First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// snapshot наполнен, но этой учётки в нём нет — честный not-found
			c.JSON(http.StatusNotFound, gin.H{
				"status": "error",
				"error":  "Учётная запись не найдена в snapshot",
			})
			return
		}
		// SQL-ошибка — деградируем, НЕ 500 (контракт Ф3-B)
		fmt.Printf("⚠️ serveAccountFromSnapshot query failed (id=%d): %v\n", id, err)
		c.JSON(http.StatusOK, gin.H{"degraded": true})
		return
	}

	if time.Since(lastSync) > SnapshotTTL {
		fmt.Printf("⏰ GetAccount snapshot устарел (id=%d, last_sync=%v, age=%v > TTL=%v) — отдаём из snapshot без proxy\n",
			id, lastSync.Format(time.RFC3339), time.Since(lastSync).Round(time.Second), SnapshotTTL)
	}

	c.JSON(http.StatusOK, accountFromSnapshotRow(row))
}

// snapshotOrderClause маппит ordering из фронта (camelCase, опционально с минусом) на SQL ORDER BY для snapshot
func snapshotOrderClause(ordering string) string {
	desc := false
	if strings.HasPrefix(ordering, "-") {
		desc = true
		ordering = strings.TrimPrefix(ordering, "-")
	}
	col := "account_name"
	switch ordering {
	case "name":
		col = "account_name"
	case "id":
		col = "external_account_id"
	case "type":
		col = "account_type"
	case "isActive", "is_active":
		col = "is_active"
	case "objectsTotal", "objects_total":
		col = "objects_total"
	case "creationDatetime", "creation_datetime", "created_at":
		col = "created_at"
	case "adminFullname", "admin_fullname":
		col = "admin_fullname"
	}
	if desc {
		return col + " DESC"
	}
	return col + " ASC"
}

// GetAccountsStats возвращает агрегированную статистику snapshot одним запросом.
// Заменяет 4 параллельных вызова /accounts с per_page=1 (~4 × middleware overhead = ~6с).
//
// GET /api/auth/accounts/stats → { total, active, blocked, clients, partners }
//
// Ф3-E/Codex: РАНЬШЕ при пустом/устаревшем snapshot — fallback на 4
// проксированных запроса к axenta.cloud (proxyAccountsCount). После Ф1
// request-токен невалиден → 401/мусор → KPI accounts-экрана сломан
// (та же дыра Ф3-B что list, найдена на staging Ф3-E). Теперь
// snapshot-only, ноль request-token proxy, контракт как у list:
//   - свежий → реальные цифры (degraded=false);
//   - устарел (>TTL) → последние известные + degraded:true;
//   - нет БД / пусто / SQL-ошибка → нули + degraded:true. НЕ 4xx/5xx.
func (h *AccountsHandler) GetAccountsStats(c *gin.Context) {
	// snapshot живёт в tenant-схеме; nil tenantDB → degraded, public не трогаем
	// (симметрия с GetAccounts / GetAccount #4).
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB != nil {
		if stats, ok := computeAccountsStatsFromSnapshot(tenantDB); ok {
			c.JSON(http.StatusOK, stats)
			return
		}
	}
	// Нет данных — нули + degraded (НЕ proxy, после Ф1 = 401).
	c.JSON(http.StatusOK, gin.H{
		"total": 0, "active": 0, "blocked": 0, "clients": 0, "partners": 0,
		"degraded": true,
	})
}

// computeAccountsStatsFromSnapshot — один SELECT с COUNT FILTER (...) на snapshot. Быстрее чем 4 отдельных запроса.
func computeAccountsStatsFromSnapshot(db *gorm.DB) (gin.H, bool) {
	if db == nil {
		return nil, false
	}

	// Ф3-E/Codex Q2: stale → НЕ false (иначе caller проксировал бы / отдал
	// нули). Считаем по snapshot и помечаем degraded. false только при
	// реальном отсутствии данных (нет БД / пустой snapshot / SQL-ошибка).
	var lastSync time.Time
	if err := db.Model(&models.AxentaAccountSnapshot{}).Select("MAX(last_synced_at)").Scan(&lastSync).Error; err != nil || lastSync.IsZero() {
		return nil, false
	}
	stale := time.Since(lastSync) > SnapshotTTL

	type counts struct {
		Total    int64
		Active   int64
		Clients  int64
		Partners int64
	}
	var c counts
	err := db.Raw(`
		SELECT
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE is_active = true) AS active,
			COUNT(*) FILTER (WHERE account_type = 'client') AS clients,
			COUNT(*) FILTER (WHERE account_type = 'partner') AS partners
		FROM axenta_account_snapshots
		WHERE deleted_at IS NULL
	`).Scan(&c).Error
	if err != nil {
		fmt.Printf("⚠️ accounts/stats snapshot query failed: %v\n", err)
		return nil, false
	}

	out := gin.H{
		"total":    c.Total,
		"active":   c.Active,
		"blocked":  c.Total - c.Active,
		"clients":  c.Clients,
		"partners": c.Partners,
	}
	if stale {
		out["degraded"] = true
		fmt.Printf("⏰ accounts/stats snapshot устарел (last_sync=%v) — отдаём + degraded (без proxy)\n", lastSync.Format(time.RFC3339))
	}
	return out, true
}

// Ф3-E: proxyAccountsCount удалён — был request-token fallback к
// axenta.cloud для accounts/stats (после Ф1 = 401). Снапшот-only, см.
// computeAccountsStatsFromSnapshot / GetAccountsStats.
