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
}

// GetAccounts получает список учетных записей через прокси к Axenta API
func (h *AccountsHandler) GetAccounts(c *gin.Context) {
	// Получаем токен из заголовка
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		authHeader = c.GetHeader("authorization")
	}

	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Authorization header is required",
		})
		return
	}

	// Получаем параметры запроса
	page := c.DefaultQuery("page", "1")
	perPage := c.DefaultQuery("per_page", "50")
	ordering := c.DefaultQuery("ordering", "name")
	search := c.Query("search")
	accountType := c.Query("type")
	isActive := c.Query("is_active")

	// Сначала пробуем читать из локального snapshot (axenta_account_snapshots)
	// Берём БД из tenant-контекста (snapshot живёт в tenant_<id>, не в public)
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		tenantDB = database.DB
	}
	if resp, ok := tryServeAccountsFromSnapshot(tenantDB, page, perPage, ordering, search, accountType, isActive); ok {
		c.JSON(http.StatusOK, resp)
		return
	}

	// Строим URL для Axenta API
	axentaURL := "https://axenta.cloud/api/cms/accounts/"

	// Создаем HTTP клиент
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Создаем запрос
	req, err := http.NewRequest("GET", axentaURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to create request: " + err.Error(),
		})
		return
	}

	// Добавляем параметры запроса
	q := req.URL.Query()
	q.Add("page", page)
	q.Add("per_page", perPage)
	q.Add("ordering", ordering)

	if search != "" {
		q.Add("search", search)
	}
	if accountType != "" {
		q.Add("type", accountType)
	}
	if isActive != "" {
		q.Add("is_active", isActive)
	}

	req.URL.RawQuery = q.Encode()

	// Добавляем заголовки авторизации
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Добавляем X-Tenant-ID если есть
	tenantID := c.GetHeader("X-Tenant-ID")
	if tenantID != "" {
		req.Header.Set("X-Tenant-ID", tenantID)
	}

	// Логируем запрос
	fmt.Printf("🔄 Proxy request to Axenta API: %s\n", req.URL.String())
	fmt.Printf("📋 Headers: Authorization=%s, X-Tenant-ID=%s\n",
		authHeader[:min(len(authHeader), 20)]+"...", tenantID)

	// Выполняем запрос
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("❌ Axenta API request failed: %v\n", err)
		axentaMutationUnavailable(c, err)
		return
	}
	defer resp.Body.Close()

	// Читаем ответ
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("❌ Failed to read Axenta API response: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to read Axenta API response: " + err.Error(),
		})
		return
	}

	// Логируем ответ
	fmt.Printf("✅ Axenta API response: status=%d, size=%d bytes\n", resp.StatusCode, len(body))

	// Если статус не 200, возвращаем ошибку
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("❌ Axenta API error: status=%d, body=%s\n", resp.StatusCode, string(body))

		// Пытаемся распарсить ошибку
		var errorResponse map[string]interface{}
		if err := json.Unmarshal(body, &errorResponse); err == nil {
			c.JSON(resp.StatusCode, gin.H{
				"status":  "error",
				"error":   "Axenta API error",
				"details": errorResponse,
			})
		} else {
			c.JSON(resp.StatusCode, gin.H{
				"status": "error",
				"error":  "Axenta API error: " + string(body),
			})
		}
		return
	}

	// Парсим успешный ответ
	var accountsResponse AccountsResponse
	if err := json.Unmarshal(body, &accountsResponse); err != nil {
		fmt.Printf("❌ Failed to parse Axenta API response: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to parse Axenta API response: " + err.Error(),
		})
		return
	}

	fmt.Printf("✅ Successfully proxied accounts: count=%d, results=%d\n",
		accountsResponse.Count, len(accountsResponse.Results))

	// Возвращаем данные
	c.JSON(http.StatusOK, accountsResponse)
}

// GetAccount получает конкретную учетную запись по ID
func (h *AccountsHandler) GetAccount(c *gin.Context) {
	// Получаем ID из параметров
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid account ID",
		})
		return
	}

	// Получаем токен из заголовка
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		authHeader = c.GetHeader("authorization")
	}

	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Authorization header is required",
		})
		return
	}

	// Строим URL для Axenta API
	axentaURL := fmt.Sprintf("https://axenta.cloud/api/cms/accounts/%d/", id)

	// Создаем HTTP клиент
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Создаем запрос
	req, err := http.NewRequest("GET", axentaURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to create request: " + err.Error(),
		})
		return
	}

	// Добавляем заголовки авторизации
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Добавляем X-Tenant-ID если есть
	tenantID := c.GetHeader("X-Tenant-ID")
	if tenantID != "" {
		req.Header.Set("X-Tenant-ID", tenantID)
	}

	// Выполняем запрос
	resp, err := client.Do(req)
	if err != nil {
		axentaMutationUnavailable(c, err)
		return
	}
	defer resp.Body.Close()

	// Читаем ответ
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to read Axenta API response: " + err.Error(),
		})
		return
	}

	// Если статус не 200, возвращаем ошибку
	if resp.StatusCode != http.StatusOK {
		var errorResponse map[string]interface{}
		if err := json.Unmarshal(body, &errorResponse); err == nil {
			c.JSON(resp.StatusCode, gin.H{
				"status":  "error",
				"error":   "Axenta API error",
				"details": errorResponse,
			})
		} else {
			c.JSON(resp.StatusCode, gin.H{
				"status": "error",
				"error":  "Axenta API error: " + string(body),
			})
		}
		return
	}

	// Парсим успешный ответ
	var account Account
	if err := json.Unmarshal(body, &account); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to parse Axenta API response: " + err.Error(),
		})
		return
	}

	// Возвращаем данные
	c.JSON(http.StatusOK, account)
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

	// Downstream 401 — server-токен компании отозван: сброс + 1 retry.
	if statusCode == http.StatusUnauthorized {
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

	// Downstream 401 — server-токен компании отозван: сброс + 1 retry.
	if statusCode == http.StatusUnauthorized {
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

	// Downstream 401 — server-токен компании отозван: сброс + 1 retry.
	if statusCode == http.StatusUnauthorized {
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

// tryServeAccountsFromSnapshot читает учётные записи из локального snapshot (axenta_account_snapshots).
// Возвращает (response, true) если snapshot непуст и достаточно свежий, иначе (nil, false) — caller сделает fallback на Axenta proxy.
//
// TTL свежести задан общей константой SnapshotTTL (60 мин). Если последняя
// синхронизация старше — считаем устаревшим и идём в Axenta.
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

	// TTL: общая константа SnapshotTTL. Если snapshot старше — fallback на Axenta.
	if time.Since(lastSync) > SnapshotTTL {
		fmt.Printf("⏰ Snapshot устарел (last_synced_at=%v, age=%v > TTL=%v), fallback на Axenta proxy\n",
			lastSync.Format(time.RFC3339), time.Since(lastSync).Round(time.Second), SnapshotTTL)
		return nil, false
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
		var acc Account
		// RawPayload содержит оригинальный JSON от Axenta — desearializaция даёт все поля
		if r.RawPayload != "" {
			_ = json.Unmarshal([]byte(r.RawPayload), &acc)
		}
		// Подстраховка: если RawPayload неполон — заполняем из колонок
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
		results = append(results, acc)
	}

	resp := &AccountsResponse{
		Count:    int(total),
		Next:     nil,
		Previous: nil,
		Results:  results,
	}
	fmt.Printf("⚡ Snapshot served: total=%d, page=%d, returned=%d (last_sync=%v)\n", total, pageNum, len(results), lastSync.Format(time.RFC3339))
	return resp, true
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
// Если snapshot пуст / устарел >60м — fallback на 4 проксированных запроса к Axenta (старое поведение).
func (h *AccountsHandler) GetAccountsStats(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		authHeader = c.GetHeader("authorization")
	}
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": "Authorization header is required"})
		return
	}

	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		tenantDB = database.DB
	}

	if stats, ok := computeAccountsStatsFromSnapshot(tenantDB); ok {
		c.JSON(http.StatusOK, stats)
		return
	}

	// Fallback: дёргаем /accounts proxy с разными фильтрами параллельно (старая логика, но в одном handler'е)
	type result struct {
		key string
		val int
	}
	out := make(chan result, 4)
	go func() { out <- result{"total", proxyAccountsCount(authHeader, c.GetHeader("X-Tenant-ID"), "")} }()
	go func() {
		out <- result{"active", proxyAccountsCount(authHeader, c.GetHeader("X-Tenant-ID"), "is_active=true&active=true&status=active")}
	}()
	go func() {
		out <- result{"clients", proxyAccountsCount(authHeader, c.GetHeader("X-Tenant-ID"), "type=client")}
	}()
	go func() {
		out <- result{"partners", proxyAccountsCount(authHeader, c.GetHeader("X-Tenant-ID"), "type=partner")}
	}()

	stats := gin.H{}
	for i := 0; i < 4; i++ {
		r := <-out
		stats[r.key] = r.val
	}
	if total, ok := stats["total"].(int); ok {
		if active, ok := stats["active"].(int); ok {
			stats["blocked"] = total - active
		}
	}
	c.JSON(http.StatusOK, stats)
}

// computeAccountsStatsFromSnapshot — один SELECT с COUNT FILTER (...) на snapshot. Быстрее чем 4 отдельных запроса.
func computeAccountsStatsFromSnapshot(db *gorm.DB) (gin.H, bool) {
	if db == nil {
		return nil, false
	}

	// Проверяем свежесть snapshot (TTL общая константа SnapshotTTL)
	var lastSync time.Time
	if err := db.Model(&models.AxentaAccountSnapshot{}).Select("MAX(last_synced_at)").Scan(&lastSync).Error; err != nil || lastSync.IsZero() {
		return nil, false
	}
	if time.Since(lastSync) > SnapshotTTL {
		return nil, false
	}

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

	return gin.H{
		"total":    c.Total,
		"active":   c.Active,
		"blocked":  c.Total - c.Active,
		"clients":  c.Clients,
		"partners": c.Partners,
	}, true
}

// proxyAccountsCount — fallback для случая когда snapshot недоступен. Дёргает Axenta API per_page=1, парсит count.
func proxyAccountsCount(authHeader, tenantID, extraQuery string) int {
	url := "https://axenta.cloud/api/cms/accounts/?page=1&per_page=1&ordering=name"
	if extraQuery != "" {
		url += "&" + extraQuery
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0
	}
	req.Header.Set("Authorization", authHeader)
	if tenantID != "" {
		req.Header.Set("X-Tenant-ID", tenantID)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0
	}
	var ar AccountsResponse
	if err := json.Unmarshal(body, &ar); err != nil {
		return 0
	}
	return ar.Count
}
