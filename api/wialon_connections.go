package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// allAccountsCacheKey формирует ключ кэша Redis для /wialon/all-accounts по компании
func allAccountsCacheKey(companyID uint) string {
	return fmt.Sprintf("wialon:all-accounts:%d", companyID)
}

// allAccountsCacheTTL — длительность кэша. Scheduler рефрешит каждые 5 мин, TTL=15 мин даёт окно
// 3× refresh-cycle, чтобы при сбое scheduler пользователь не падал на live-fetch (18s+).
// При действиях, меняющих статус аккаунта (ToggleAccountStatus), кэш инвалидируется явно.
const allAccountsCacheTTL = 15 * time.Minute

// invalidateAllAccountsCache удаляет кэш списка для компании. Вызывается после изменений статуса.
func invalidateAllAccountsCache(companyID uint) {
	if database.RedisClient == nil {
		return
	}
	if err := database.RedisClient.Del(context.Background(), allAccountsCacheKey(companyID)).Err(); err != nil {
		log.Printf("⚠️ Ошибка инвалидации wialon all-accounts cache (company=%d): %v", companyID, err)
	}
}

// WialonConnectionAPI обработчики для подключений Wialon
type WialonConnectionAPI struct {
	service *services.WialonConnectionService
}

// NewWialonConnectionAPI создает новый экземпляр API
func NewWialonConnectionAPI(db *gorm.DB) *WialonConnectionAPI {
	return &WialonConnectionAPI{
		service: services.NewWialonConnectionService(db),
	}
}

// RegisterRoutes регистрирует маршруты API
func (api *WialonConnectionAPI) RegisterRoutes(r *gin.RouterGroup) {
	connections := r.Group("/wialon/connections")
	{
		connections.GET("", api.GetConnections)
		connections.POST("", api.CreateConnection)
		connections.GET("/:id", api.GetConnection)
		connections.PUT("/:id", api.UpdateConnection)
		connections.DELETE("/:id", api.DeleteConnection)
		connections.POST("/:id/test", api.TestConnection)
		connections.GET("/:id/units", api.GetConnectionUnits)
		connections.GET("/stats", api.GetConnectionStats)
	}

	// Обновленный endpoint для получения объектов из всех подключений
	r.GET("/wialon/all-units", api.GetAllUnits)

	// Endpoint для получения аккаунтов из всех подключений
	r.GET("/wialon/all-accounts", api.GetAllAccounts)

	// Endpoint для блокировки/разблокировки Wialon аккаунта
	r.POST("/wialon/accounts/:id/toggle-status", api.ToggleAccountStatus)

	// Endpoint для входа в мониторинг Wialon
	r.POST("/wialon/login-to-monitoring", api.LoginToMonitoring)

	// Endpoint для входа в CMS Wialon
	r.POST("/wialon/login-to-cms", api.LoginToCms)

	// Endpoint для удаления пользователя Wialon
	r.DELETE("/wialon/users/:id", api.DeleteWialonUser)

	// Endpoint для получения статистики объектов (фоновая загрузка)
	r.GET("/wialon/connections/:id/objects-stats", api.GetConnectionObjectsStats)

	// Endpoint для точечного обновления одной учётной записи (cache-invalidate + refresh stats для 1 ресурса)
	r.POST("/wialon/connections/:id/refresh-account/:user_id", api.RefreshSingleAccount)

	// Создание нового аккаунта в Wialon-подключении (билинг + dealer rights, 5-step flow)
	r.GET("/wialon/connections/:id/billing-plans", api.GetBillingPlans)
	r.POST("/wialon/connections/:id/accounts", api.CreateWialonAccount)

	log.Println("✅ Wialon Connections API routes registered: /api/wialon/connections/*")
}

// CreateConnectionRequest запрос на создание подключения
type CreateConnectionRequest struct {
	Name           string `json:"name" binding:"required"`
	ConnectionType string `json:"connection_type" binding:"required"` // "hosting" или "local"
	DataCenter     string `json:"data_center"`                        // Для hosting: com, us, eu, org, alt
	Host           string `json:"host"`                               // Для local: пользовательский URL
	Token          string `json:"token" binding:"required"`
	SyncInterval   int    `json:"sync_interval"`
	CmsURL         string `json:"cms_url"` // URL CMS-интерфейса (опционально)
}

// UpdateConnectionRequest запрос на обновление подключения
type UpdateConnectionRequest struct {
	Name            string `json:"name"`
	ConnectionType  string `json:"connection_type"`
	DataCenter      string `json:"data_center"`
	Host            string `json:"host"`
	Token           string `json:"token"`
	SyncInterval    int    `json:"sync_interval"`
	CmsURL          string `json:"cms_url"` // URL CMS-интерфейса (опционально)
	IsActive        *bool  `json:"is_active"`
	AutoSyncEnabled *bool  `json:"auto_sync_enabled"`
	SyncVehicles    *bool  `json:"sync_vehicles"`
	SyncSensors     *bool  `json:"sync_sensors"`
	SyncMaintenance *bool  `json:"sync_maintenance"`
	SyncDrivers     *bool  `json:"sync_drivers"`
	SyncGeozones    *bool  `json:"sync_geozones"`
}

// GetConnections возвращает список подключений компании
// @Summary Получить список подключений Wialon
// @Tags Wialon Connections
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/wialon/connections [get]
func (api *WialonConnectionAPI) GetConnections(c *gin.Context) {
	companyID, exists := c.Get("company_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Компания не определена"})
		return
	}

	connections, err := api.service.GetAllByCompany(companyID.(uint))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения подключений: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"connections": connections,
			"total":       len(connections),
		},
	})
}

// CreateConnection создает новое подключение
// @Summary Создать подключение Wialon
// @Tags Wialon Connections
// @Accept json
// @Produce json
// @Param request body CreateConnectionRequest true "Данные подключения"
// @Success 201 {object} map[string]interface{}
// @Router /api/wialon/connections [post]
func (api *WialonConnectionAPI) CreateConnection(c *gin.Context) {
	companyID, exists := c.Get("company_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Компания не определена"})
		return
	}

	var req CreateConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	// Валидация типа подключения
	if req.ConnectionType != "hosting" && req.ConnectionType != "local" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный тип подключения. Допустимые значения: hosting, local"})
		return
	}

	// Для local обязателен host
	if req.ConnectionType == "local" && req.Host == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Для Wialon Local обязательно указание URL хоста"})
		return
	}

	connection := &models.WialonConnection{
		CompanyID:      companyID.(uint),
		Name:           req.Name,
		ConnectionType: models.WialonConnectionType(req.ConnectionType),
		DataCenter:     req.DataCenter,
		Host:           req.Host,
		Token:          req.Token,
		SyncInterval:   req.SyncInterval,
		CmsURL:         req.CmsURL,
		IsActive:       true,
		SyncVehicles:   true,
	}

	// Устанавливаем интервал по умолчанию
	if connection.SyncInterval <= 0 {
		connection.SyncInterval = 5
	}

	if err := api.service.Create(connection); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка создания подключения: " + err.Error()})
		return
	}

	// Маскируем токен для ответа
	connection.MaskToken()

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Подключение успешно создано",
		"data":    connection,
	})
}

// GetConnection возвращает подключение по ID
// @Summary Получить подключение Wialon по ID
// @Tags Wialon Connections
// @Produce json
// @Param id path int true "ID подключения"
// @Success 200 {object} map[string]interface{}
// @Router /api/wialon/connections/{id} [get]
func (api *WialonConnectionAPI) GetConnection(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный ID подключения"})
		return
	}

	connection, err := api.service.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Подключение не найдено"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    connection,
	})
}

// UpdateConnection обновляет подключение
// @Summary Обновить подключение Wialon
// @Tags Wialon Connections
// @Accept json
// @Produce json
// @Param id path int true "ID подключения"
// @Param request body UpdateConnectionRequest true "Данные для обновления"
// @Success 200 {object} map[string]interface{}
// @Router /api/wialon/connections/{id} [put]
func (api *WialonConnectionAPI) UpdateConnection(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный ID подключения"})
		return
	}

	connection, err := api.service.GetByIDWithToken(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Подключение не найдено"})
		return
	}

	var req UpdateConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	// Обновляем поля
	if req.Name != "" {
		connection.Name = req.Name
	}
	if req.ConnectionType != "" {
		connection.ConnectionType = models.WialonConnectionType(req.ConnectionType)
	}
	if req.DataCenter != "" {
		connection.DataCenter = req.DataCenter
	}
	if req.Host != "" {
		connection.Host = req.Host
	}
	if req.Token != "" {
		connection.Token = req.Token
	}
	if req.SyncInterval > 0 {
		connection.SyncInterval = req.SyncInterval
	}
	// CmsURL обновляется всегда (может быть пустым для сброса значения)
	connection.CmsURL = req.CmsURL
	if req.IsActive != nil {
		connection.IsActive = *req.IsActive
	}
	if req.AutoSyncEnabled != nil {
		connection.AutoSyncEnabled = *req.AutoSyncEnabled
	}
	if req.SyncVehicles != nil {
		connection.SyncVehicles = *req.SyncVehicles
	}
	if req.SyncSensors != nil {
		connection.SyncSensors = *req.SyncSensors
	}
	if req.SyncMaintenance != nil {
		connection.SyncMaintenance = *req.SyncMaintenance
	}
	if req.SyncDrivers != nil {
		connection.SyncDrivers = *req.SyncDrivers
	}
	if req.SyncGeozones != nil {
		connection.SyncGeozones = *req.SyncGeozones
	}

	if err := api.service.Update(connection); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка обновления подключения: " + err.Error()})
		return
	}

	// Маскируем токен для ответа
	connection.MaskToken()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Подключение успешно обновлено",
		"data":    connection,
	})
}

// DeleteConnection удаляет подключение
// @Summary Удалить подключение Wialon
// @Tags Wialon Connections
// @Produce json
// @Param id path int true "ID подключения"
// @Success 200 {object} map[string]interface{}
// @Router /api/wialon/connections/{id} [delete]
func (api *WialonConnectionAPI) DeleteConnection(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный ID подключения"})
		return
	}

	if err := api.service.Delete(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка удаления подключения: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Подключение успешно удалено",
	})
}

// TestConnection проверяет подключение
// @Summary Тест подключения Wialon
// @Tags Wialon Connections
// @Produce json
// @Param id path int true "ID подключения"
// @Success 200 {object} map[string]interface{}
// @Router /api/wialon/connections/{id}/test [post]
func (api *WialonConnectionAPI) TestConnection(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный ID подключения"})
		return
	}

	connection, err := api.service.GetByIDWithToken(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Подключение не найдено"})
		return
	}

	result, err := api.service.TestConnection(connection)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка теста подключения: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": result.Success,
		"message": result.Message,
		"data": gin.H{
			"user_name":     result.UserName,
			"response_time": result.ResponseTime,
		},
	})
}

// GetConnectionUnits возвращает объекты из конкретного подключения
// @Summary Получить объекты подключения Wialon
// @Tags Wialon Connections
// @Produce json
// @Param id path int true "ID подключения"
// @Success 200 {object} map[string]interface{}
// @Router /api/wialon/connections/{id}/units [get]
func (api *WialonConnectionAPI) GetConnectionUnits(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный ID подключения"})
		return
	}

	units, err := api.service.GetUnitsFromConnection(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения объектов: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items": units,
			"total": len(units),
		},
	})
}

// GetAllUnits возвращает объекты из всех активных подключений
// @Summary Получить объекты из всех подключений Wialon
// @Tags Wialon Connections
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/wialon/all-units [get]
func (api *WialonConnectionAPI) GetAllUnits(c *gin.Context) {
	companyID, exists := c.Get("company_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Компания не определена"})
		return
	}

	units, err := api.service.GetAllUnitsFromActiveConnections(companyID.(uint))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения объектов: " + err.Error()})
		return
	}

	// Получаем статистику подключений
	stats, _ := api.service.GetConnectionStats(companyID.(uint))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items":             units,
			"total":             len(units),
			"connections_count": stats.Active,
		},
	})
}

// GetConnectionStats возвращает статистику подключений
// @Summary Получить статистику подключений Wialon
// @Tags Wialon Connections
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/wialon/connections/stats [get]
func (api *WialonConnectionAPI) GetConnectionStats(c *gin.Context) {
	companyID, exists := c.Get("company_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Компания не определена"})
		return
	}

	stats, err := api.service.GetConnectionStats(companyID.(uint))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения статистики: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}

// WialonAccountInfo информация об аккаунте Wialon
type WialonAccountInfo struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Type          string `json:"type"` // "client" или "partner"
	IsActive      bool   `json:"is_active"`
	DealerRights  bool   `json:"dealer_rights"` // Права дилера
	ObjectsTotal  int    `json:"objects_total"`
	ObjectsActive int    `json:"objects_active"` // Активные объекты
	Source        string `json:"source"`         // "wialon"
	SourceLabel   string `json:"source_label"`   // "WH(ACRM)" или "WL(Профмонитор)"
	Hierarchy     string `json:"hierarchy"`      // Иерархия: "WL(Профмонитор) > ИмяАккаунта"
	ConnectionID  uint   `json:"connection_id"`
	CreatedAt     string `json:"created_at,omitempty"` // Дата создания из Wialon
}

// GetAllAccounts получает аккаунты из всех подключений Wialon
// @Summary Получить аккаунты из всех подключений Wialon
// @Tags Wialon Connections
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/wialon/all-accounts [get]
func (api *WialonConnectionAPI) GetAllAccounts(c *gin.Context) {
	companyID, exists := c.Get("company_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Компания не определена"})
		return
	}
	cid := companyID.(uint)

	// Cache hit — отдаём из Redis. Источник правды — Wialon, но 5 минут stale-данных приемлемо
	// (новые/удалённые аккаунты появятся в CRM с задержкой до 5 мин). При toggle статуса cache
	// инвалидируется явно (см. invalidateAllAccountsCache).
	if database.RedisClient != nil {
		if cached, err := database.RedisClient.Get(context.Background(), allAccountsCacheKey(cid)).Bytes(); err == nil && len(cached) > 0 {
			log.Printf("⚡ Wialon CACHE HIT: /all-accounts company=%d, %d bytes", cid, len(cached))
			c.Data(http.StatusOK, "application/json", cached)
			return
		}
	}

	payload, err := buildAndCacheAllAccountsForCompany(cid, api.service)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения аккаунтов: " + err.Error()})
		return
	}
	c.Data(http.StatusOK, "application/json", payload)
}

// BuildAndCacheAllAccountsForCompany — exported wrapper для scheduler. Без gin.Context, чтобы scheduler мог дёргать в фоне.
func BuildAndCacheAllAccountsForCompany(companyID uint, db *gorm.DB) ([]byte, error) {
	connService := services.NewWialonConnectionService(db)
	return buildAndCacheAllAccountsForCompany(companyID, connService)
}

// buildAndCacheAllAccountsForCompany — общая логика fetch-всех-connections + Redis SET. Используется и handler'ом и scheduler'ом.
func buildAndCacheAllAccountsForCompany(cid uint, connService *services.WialonConnectionService) ([]byte, error) {
	connections, err := connService.GetAllByCompany(cid)
	if err != nil {
		return nil, fmt.Errorf("получение подключений: %w", err)
	}

	var allAccounts []WialonAccountInfo
	var totalActive, totalBlocked, totalObjects int

	wialonService := services.NewWialonService()

	// Параллелим загрузку по connections — WL и WH идут одновременно (Wialon разрешает до 5 concurrent sessions per token).
	// Раньше for-loop делал sum(time_WL, time_WH); теперь max(time_WL, time_WH).
	type connResult struct {
		conn        models.WialonConnection
		sourceLabel string
		accounts    []services.WialonAccount
		err         error
		duration    time.Duration
	}

	var wg sync.WaitGroup
	resultsCh := make(chan connResult, len(connections))

	tStart := time.Now()
	for _, conn := range connections {
		if !conn.IsActive {
			continue
		}
		wg.Add(1)
		go func(conn models.WialonConnection) {
			defer wg.Done()
			sourceLabel := "WL(" + conn.UserName + ")"
			if conn.ConnectionType == models.WialonConnectionTypeHosting {
				sourceLabel = "WH(" + conn.UserName + ")"
			}
			t0 := time.Now()
			accs, err := wialonService.GetAccountsBatchFromHost(conn.Host, conn.Token)
			resultsCh <- connResult{conn: conn, sourceLabel: sourceLabel, accounts: accs, err: err, duration: time.Since(t0)}
		}(conn)
	}
	wg.Wait()
	close(resultsCh)

	for r := range resultsCh {
		if r.err != nil {
			log.Printf("⚠️ Ошибка получения аккаунтов из %s: %v (за %s)", r.conn.Name, r.err, r.duration)
			continue
		}
		log.Printf("⚡ Wialon PARALLEL: connection=%s загружена за %s, %d аккаунтов", r.conn.Name, r.duration, len(r.accounts))

		for _, acc := range r.accounts {
			var hierarchy string
			if acc.ParentName != "" {
				hierarchy = r.sourceLabel + " > " + acc.ParentName + " > " + acc.Name
			} else {
				hierarchy = r.sourceLabel + " > " + acc.Name
			}

			accountType := "client"
			if acc.DealerRights {
				accountType = "partner"
			}

			allAccounts = append(allAccounts, WialonAccountInfo{
				ID:            int(acc.ID),
				Name:          acc.Name,
				Type:          accountType,
				IsActive:      acc.IsActive,
				DealerRights:  acc.DealerRights,
				ObjectsTotal:  acc.ObjectsTotal,
				ObjectsActive: acc.ObjectsActive,
				Source:        "wialon",
				SourceLabel:   r.sourceLabel,
				Hierarchy:     hierarchy,
				ConnectionID:  r.conn.ID,
				CreatedAt:     acc.CreatedAt,
			})

			if acc.IsActive {
				totalActive++
			} else {
				totalBlocked++
			}
			totalObjects += acc.ObjectsTotal
		}
	}
	log.Printf("⚡ Wialon PARALLEL: все connections обработаны за %s, всего %d аккаунтов", time.Since(tStart), len(allAccounts))

	// Собираем уникальные connectionIds для фоновой загрузки статистики
	connectionIds := make([]uint, 0)
	for _, conn := range connections {
		if conn.IsActive {
			connectionIds = append(connectionIds, conn.ID)
		}
	}

	responsePayload := gin.H{
		"success": true,
		"data": gin.H{
			"items":         allAccounts,
			"total":         len(allAccounts),
			"connectionIds": connectionIds, // Для фоновой загрузки статистики объектов
			"stats": gin.H{
				"total":         len(allAccounts),
				"active":        totalActive,
				"blocked":       totalBlocked,
				"objects_total": totalObjects,
			},
		},
	}

	// Сериализуем payload и сохраняем в Redis. Возвращаем bytes — handler отдаст клиенту, scheduler просто игнорирует
	payloadBytes, err := json.Marshal(responsePayload)
	if err != nil {
		return nil, fmt.Errorf("сериализация payload: %w", err)
	}
	if database.RedisClient != nil {
		if err := database.RedisClient.Set(context.Background(), allAccountsCacheKey(cid), payloadBytes, allAccountsCacheTTL).Err(); err != nil {
			log.Printf("⚠️ Ошибка записи wialon all-accounts cache (company=%d): %v", cid, err)
		} else {
			log.Printf("⚡ Wialon CACHE SET: /all-accounts company=%d, %d bytes, TTL=%s", cid, len(payloadBytes), allAccountsCacheTTL)
		}
	}
	return payloadBytes, nil
}

// RefreshSingleAccount — точечное обновление stats одной учётной записи.
// POST /api/wialon/connections/:id/refresh-account/:user_id
//
// Делает live-запрос к Wialon на 1 ресурс (~200-500 ms) + обновляет запись в wialon_object_stats
// + инвалидирует cache /all-accounts. Используется фронтом после действия пользователя
// (toggle, edit), чтобы пользователь сразу видел свежие данные без ожидания scheduler-цикла.
func (api *WialonConnectionAPI) RefreshSingleAccount(c *gin.Context) {
	connectionID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректный ID подключения"})
		return
	}
	userID, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректный user_id"})
		return
	}

	companyID, exists := c.Get("company_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Компания не определена"})
		return
	}

	// Проверка доступа
	conn, err := api.service.GetByID(uint(connectionID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Подключение не найдено"})
		return
	}
	if conn.CompanyID != companyID.(uint) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Нет доступа к этому подключению"})
		return
	}

	statsService := services.NewWialonStatsService()
	stat, err := statsService.RefreshSingleAccount(uint(connectionID), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка обновления: " + err.Error()})
		return
	}

	// Инвалидация cache /all-accounts чтобы при следующем заходе пользователь увидел свежий список
	invalidateAllAccountsCache(companyID.(uint))

	c.JSON(http.StatusOK, gin.H{
		"connectionId":       connectionID,
		"userId":             userID,
		"resourceId":         stat.ResourceID,
		"objectsTotal":       stat.ObjectsTotal,
		"objectsActive":      stat.ObjectsActive,
		"objectsDeactivated": stat.ObjectsDeactivated,
		"lastCollectedAt":    stat.LastCollectedAt,
	})
}

// ToggleAccountStatusRequest запрос на изменение статуса аккаунта
type ToggleAccountStatusRequest struct {
	Enable       bool `json:"enable"`        // true = активировать, false = заблокировать
	ConnectionID uint `json:"connection_id"` // ID подключения Wialon
}

// ToggleAccountStatus блокирует или активирует аккаунт Wialon
// @Summary Изменить статус аккаунта Wialon
// @Tags Wialon Connections
// @Accept json
// @Produce json
// @Param id path int true "ID аккаунта (ID ресурса в Wialon)"
// @Param request body ToggleAccountStatusRequest true "Данные для изменения статуса"
// @Success 200 {object} map[string]interface{}
// @Router /api/wialon/accounts/{id}/toggle-status [post]
func (api *WialonConnectionAPI) ToggleAccountStatus(c *gin.Context) {
	// Получаем ID аккаунта из URL
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Неверный ID аккаунта"})
		return
	}

	var req ToggleAccountStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Неверный формат данных: " + err.Error()})
		return
	}

	// Получаем подключение по ID
	connection, err := api.service.GetByIDWithToken(req.ConnectionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Подключение не найдено"})
		return
	}

	// Вызываем API Wialon для изменения статуса
	wialonService := services.NewWialonService()
	err = wialonService.EnableAccountWithHost(connection.Host, connection.Token, accountID, req.Enable)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Ошибка изменения статуса: " + err.Error(),
		})
		return
	}

	statusText := "активирован"
	if !req.Enable {
		statusText = "заблокирован"
	}

	// Инвалидируем cache /all-accounts — новый статус должен сразу отражаться в списке
	if companyID, exists := c.Get("company_id"); exists {
		invalidateAllAccountsCache(companyID.(uint))
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Аккаунт успешно " + statusText,
	})
}

// LoginToMonitoring генерирует URL для входа в мониторинг Wialon
func (api *WialonConnectionAPI) LoginToMonitoring(c *gin.Context) {
	var req struct {
		ConnectionID int64  `json:"connection_id" binding:"required"`
		UserName     string `json:"user_name"`   // Имя пользователя для входа (опционально, используется если указано)
		AccountID    int64  `json:"account_id"`  // ID ресурса (учётной записи) для поиска пользователя по bact
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Неверные параметры запроса: " + err.Error(),
		})
		return
	}

	log.Printf("🔐 Запрос входа в мониторинг Wialon: connection_id=%d, user_name=%s, account_id=%d", req.ConnectionID, req.UserName, req.AccountID)

	// Получаем подключение с токеном
	connection, err := api.service.GetByIDWithToken(uint(req.ConnectionID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Подключение не найдено: " + err.Error(),
		})
		return
	}

	// Получаем хост из подключения
	host := connection.Host

	// Создаем WialonService для работы с API
	wialonService := services.NewWialonService()

	// Определяем имя пользователя для входа
	userName := req.UserName
	
	// Если имя не указано, но указан account_id — ищем пользователя по bact
	if userName == "" && req.AccountID > 0 {
		log.Printf("🔍 Поиск пользователя с bact=%d", req.AccountID)
		foundUser, err := wialonService.FindUserByBillingAccountID(host, connection.Token, req.AccountID)
		if err != nil {
			log.Printf("⚠️ Не удалось найти пользователя по bact=%d: %v", req.AccountID, err)
			// Продолжаем без operateAs — войдём как основной пользователь
		} else if foundUser != "" {
			userName = foundUser
			log.Printf("✅ Найден пользователь: %s", userName)
		}
	}

	// Выполняем авторизацию и получаем SID (для основного пользователя или с operateAs)
	sid, err := wialonService.DuplicateSessionWithHost(host, connection.Token, userName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Ошибка создания сессии: " + err.Error(),
		})
		return
	}

	// Формируем URL для входа в мониторинг
	redirectURL := wialonService.GetMonitoringURL(host, sid)

	log.Printf("✅ URL для входа в мониторинг: %s", redirectURL)

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"redirectUrl": redirectURL,
	})
}

// LoginToCms генерирует URL для входа в CMS Wialon
func (api *WialonConnectionAPI) LoginToCms(c *gin.Context) {
	var req struct {
		ConnectionID int64  `json:"connection_id" binding:"required"`
		UserName     string `json:"user_name"`   // Имя пользователя для входа (опционально, используется если указано)
		AccountID    int64  `json:"account_id"`  // ID ресурса (учётной записи) для поиска пользователя
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Неверные параметры запроса: " + err.Error(),
		})
		return
	}

	log.Printf("🔐 Запрос входа в CMS Wialon: connection_id=%d, user_name=%s, account_id=%d", req.ConnectionID, req.UserName, req.AccountID)

	// Получаем подключение с токеном
	connection, err := api.service.GetByIDWithToken(uint(req.ConnectionID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Подключение не найдено: " + err.Error(),
		})
		return
	}

	// Получаем хост из подключения
	host := connection.Host

	// Создаем WialonService для работы с API
	wialonService := services.NewWialonService()

	// Определяем имя пользователя для входа
	userName := req.UserName
	
	// Если имя не указано, но указан account_id — ищем пользователя по ID
	if userName == "" && req.AccountID > 0 {
		log.Printf("🔍 Поиск пользователя с account_id=%d", req.AccountID)
		foundUser, err := wialonService.FindUserByBillingAccountID(host, connection.Token, req.AccountID)
		if err != nil {
			log.Printf("⚠️ Не удалось найти пользователя по account_id=%d: %v", req.AccountID, err)
			// Продолжаем без operateAs — войдём как основной пользователь
		} else if foundUser != "" {
			userName = foundUser
			log.Printf("✅ Найден пользователь: %s", userName)
		}
	}

	// Выполняем авторизацию и получаем SID
	sid, err := wialonService.DuplicateSessionWithHost(host, connection.Token, userName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Ошибка создания сессии: " + err.Error(),
		})
		return
	}

	// Получаем CMS URL из подключения
	cmsURL := connection.GetCmsURL()

	// Формируем URL для входа в CMS
	redirectURL := cmsURL + "?sid=" + sid + "&lang=ru"

	log.Printf("✅ URL для входа в CMS: %s", redirectURL)

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"redirectUrl": redirectURL,
	})
}

// DeleteWialonUser удаляет пользователя Wialon через API item/delete_item
// @Summary Удалить пользователя Wialon
// @Tags Wialon Users
// @Produce json
// @Param id path int true "ID пользователя Wialon"
// @Param connection_id query int true "ID подключения"
// @Success 200 {object} map[string]interface{}
// @Router /api/wialon/users/{id} [delete]
func (api *WialonConnectionAPI) DeleteWialonUser(c *gin.Context) {
	// Получаем ID пользователя из URL
	userIDStr := c.Param("id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Некорректный ID пользователя",
		})
		return
	}

	// Получаем connection_id из query параметров
	connectionIDStr := c.Query("connection_id")
	connectionID, err := strconv.Atoi(connectionIDStr)
	if err != nil || connectionID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Не указан connection_id",
		})
		return
	}

	// Получаем подключение с токеном
	connection, err := api.service.GetByIDWithToken(uint(connectionID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Подключение не найдено: " + err.Error(),
		})
		return
	}

	// Получаем хост из подключения
	host := connection.Host
	log.Printf("🗑️ Удаление пользователя %d через подключение %d (host: %s)", userID, connectionID, host)

	// Создаём WialonService
	wialonService := services.NewWialonService()

	// Авторизуемся в Wialon и получаем SID
	sid, err := wialonService.DuplicateSessionWithHost(host, connection.Token, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Ошибка авторизации в Wialon: " + err.Error(),
		})
		return
	}

	// Шаг 1: Получаем информацию о пользователе, чтобы узнать его billing account ID (bact)
	// Флаг 0x00000004 включает информацию о биллинге (bact - billing account id)
	searchParams := fmt.Sprintf(`{"id":%d,"flags":4}`, userID)
	searchURL := fmt.Sprintf("%s/wialon/ajax.html?svc=core/search_item&params=%s&sid=%s", host, url.QueryEscape(searchParams), sid)
	log.Printf("🔍 Получаем информацию о пользователе %d: %s", userID, searchURL)

	searchResp, err := http.Get(searchURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Ошибка получения информации о пользователе: " + err.Error(),
		})
		return
	}
	defer searchResp.Body.Close()

	searchBody, err := io.ReadAll(searchResp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Ошибка чтения ответа: " + err.Error(),
		})
		return
	}

	log.Printf("🔍 Информация о пользователе: %s", string(searchBody))

	// Парсим ответ для получения bact
	var userInfo struct {
		Item struct {
			ID   int64 `json:"id"`
			Bact int64 `json:"bact"` // Billing Account ID
		} `json:"item"`
		Error int `json:"error"`
	}
	if err := json.Unmarshal(searchBody, &userInfo); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Ошибка парсинга информации о пользователе: " + err.Error(),
		})
		return
	}

	if userInfo.Error != 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Ошибка Wialon при получении пользователя (код: %d)", userInfo.Error),
		})
		return
	}

	// Используем bact (billing account ID) для удаления учетной записи
	accountID := userInfo.Item.Bact
	if accountID == 0 {
		// Если bact нет, возможно это сам billing account — используем user ID
		accountID = userInfo.Item.ID
		log.Printf("⚠️ Нет bact, используем ID пользователя: %d", accountID)
	} else {
		log.Printf("✅ Найден billing account ID (bact): %d для пользователя %d", accountID, userID)
	}

	// Шаг 2: Удаляем учетную запись через account/delete_account
	// Получаем reason_key из query параметров (для Wialon Hosting)
	reasonKey := c.Query("reason_key")

	var params string
	if reasonKey != "" {
		// Wialon Hosting — указываем причину удаления
		params = fmt.Sprintf(`{"itemId":%d,"reasons":[{"reason_key":"%s"}]}`, accountID, reasonKey)
		log.Printf("🗑️ Удаление учетной записи %d с причиной: %s", accountID, reasonKey)
	} else {
		// Wialon Local или аккаунт без объектов
		params = fmt.Sprintf(`{"itemId":%d}`, accountID)
	}

	deleteURL := fmt.Sprintf("%s/wialon/ajax.html?svc=account/delete_account&params=%s&sid=%s", host, url.QueryEscape(params), sid)
	log.Printf("🗑️ URL для удаления учетной записи: %s", deleteURL)

	resp, err := http.Get(deleteURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Ошибка запроса к Wialon API: " + err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	// Читаем ответ
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Ошибка чтения ответа: " + err.Error(),
		})
		return
	}

	log.Printf("🗑️ Ответ Wialon на удаление: %s", string(body))

	// Проверяем ответ — успешное удаление возвращает пустой JSON {}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Некорректный ответ от Wialon",
		})
		return
	}

	// Проверяем на ошибку
	if errCode, ok := result["error"].(float64); ok {
		errorMessages := map[float64]string{
			1:    "Сессия недействительна",
			2:    "Неверное имя сервиса",
			3:    "Неверный результат",
			4:    "Неверные параметры",
			5:    "Ошибка выполнения запроса",
			6:    "Внутренняя ошибка сервера",
			7:    "Недостаточно прав для удаления",
			8:    "DNS ошибка",
			9:    "Недоступно по текущему тарифному плану",
			10:   "Ошибка биллинга",
			11:   "Исчерпан лимит запросов",
			14:   "Внутренняя ошибка",
			1001: "Нет прав доступа",
			1002: "Объект не найден",
			2014: "Невозможно удалить учетную запись",
		}
		errMsg := fmt.Sprintf("Код ошибки: %.0f", errCode)
		if msg, exists := errorMessages[errCode]; exists {
			errMsg = msg + fmt.Sprintf(" (код: %.0f)", errCode)
		}
		log.Printf("❌ Ошибка Wialon: %s, ответ: %s", errMsg, string(body))
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   errMsg,
		})
		return
	}

	log.Printf("✅ Пользователь %d успешно удалён", userID)

	// Удаляем запись из кэша wialon_object_stats и инвалидируем Redis cache /all-accounts
	// чтобы фронт сразу не видел удалённую учётку
	if database.DB != nil {
		if err := database.DB.Where("connection_id = ? AND user_id = ?", connectionID, userID).
			Delete(&models.WialonObjectStat{}).Error; err != nil {
			log.Printf("⚠️ Не удалось удалить wialon_object_stats для user=%d: %v", userID, err)
		}
	}
	if companyID, exists := c.Get("company_id"); exists {
		invalidateAllAccountsCache(companyID.(uint))
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Пользователь успешно удалён",
	})
}

// ObjectsStatsResponse структура ответа статистики объектов
type ObjectsStatsResponse struct {
	ConnectionID uint               `json:"connectionId"`
	Stats        map[int]ObjectStat `json:"stats"` // accountId -> stats
	TotalObjects int                `json:"totalObjects"`
}

// ObjectStat статистика объектов для аккаунта
type ObjectStat struct {
	ObjectsTotal       int `json:"objectsTotal"`
	ObjectsActive      int `json:"objectsActive"`
	ObjectsDeactivated int `json:"objectsDeactivated"`
}

// GetConnectionObjectsStats отдаёт статистику объектов из кэша БД (wialon_object_stats),
// заполняемого фоновым WialonStatsScheduler. Раньше делался live-запрос к Wialon — для WH
// с 3412 ресурсов это занимало 6.5 минут. Теперь — мгновенный SELECT из БД.
//
// Если ?force_refresh=true — синхронно собрать stats для одного подключения и записать в БД.
// Используется для ручного обновления (UI-кнопка / админка).
func (api *WialonConnectionAPI) GetConnectionObjectsStats(c *gin.Context) {
	connectionID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректный ID подключения"})
		return
	}

	companyID, exists := c.Get("company_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Компания не определена"})
		return
	}

	// Получаем подключение для проверки доступа
	conn, err := api.service.GetByID(uint(connectionID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Подключение не найдено"})
		return
	}
	if conn.CompanyID != companyID.(uint) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Нет доступа к этому подключению"})
		return
	}

	statsService := services.NewWialonStatsService()

	// Опциональный синхронный refresh (только если явно запрошен — иначе блокирует UI на минуты)
	if c.Query("force_refresh") == "true" {
		t0 := time.Now()
		upserted, err := statsService.CollectForConnectionID(uint(connectionID))
		if err != nil {
			log.Printf("⚠️ force_refresh stats для conn=%d: %v", connectionID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка обновления статистики: " + err.Error()})
			return
		}
		log.Printf("🔄 force_refresh: conn=%d, %d ресурсов upserted за %s", connectionID, upserted, time.Since(t0))
	}

	rows, err := statsService.GetStatsForConnection(uint(connectionID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка чтения statистики из БД: " + err.Error()})
		return
	}

	// Frontend ожидает map по userID (или resourceID, оба ключа исторически использовались).
	// Заполняем оба, чтобы не сломать совместимость.
	stats := make(map[int]ObjectStat, len(rows)*2)
	totalObjects := 0
	var oldestCollected time.Time
	for _, r := range rows {
		os := ObjectStat{
			ObjectsTotal:       r.ObjectsTotal,
			ObjectsActive:      r.ObjectsActive,
			ObjectsDeactivated: r.ObjectsDeactivated,
		}
		if r.UserID > 0 {
			stats[int(r.UserID)] = os
		}
		if r.ResourceID > 0 {
			stats[int(r.ResourceID)] = os
		}
		totalObjects += r.ObjectsTotal
		if oldestCollected.IsZero() || r.LastCollectedAt.Before(oldestCollected) {
			oldestCollected = r.LastCollectedAt
		}
	}

	log.Printf("📊 Wialon Stats (кэш БД): conn=%d, %d записей, %d объектов, oldest=%s",
		connectionID, len(rows), totalObjects, oldestCollected.Format(time.RFC3339))

	c.JSON(http.StatusOK, gin.H{
		"connectionId":    uint(connectionID),
		"stats":           stats,
		"totalObjects":    totalObjects,
		"lastCollectedAt": oldestCollected,
		"fromCache":       true,
	})
}

// GetBillingPlans возвращает список доступных тарифов для wialon-подключения.
// GET /api/wialon/connections/:id/billing-plans
// Используется фронтом при создании аккаунта — заполняет селектор «Тарифный план».
func (api *WialonConnectionAPI) GetBillingPlans(c *gin.Context) {
	connectionID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректный ID подключения"})
		return
	}
	companyID, exists := c.Get("company_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Компания не определена"})
		return
	}
	conn, err := api.service.GetByID(uint(connectionID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Подключение не найдено"})
		return
	}
	if conn.CompanyID != companyID.(uint) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Нет доступа"})
		return
	}

	svc := services.NewWialonAccountService()
	var plans []services.WialonBillingPlan
	var err2 error
	if c.Query("force_refresh") == "true" {
		// Принудительный sync: дёргаем Wialon, обновляем БД, возвращаем свежие
		plans, err2 = svc.SyncBillingPlans(uint(connectionID))
	} else {
		plans, err2 = svc.GetBillingPlans(uint(connectionID))
	}
	if err2 != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения тарифов: " + err2.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"plans": plans})
}

// CreateWialonAccount создаёт новую учётную запись в wialon-подключении.
// POST /api/wialon/connections/:id/accounts
// Body: { name, username, password, email, type: client|partner, billingPlan }
func (api *WialonConnectionAPI) CreateWialonAccount(c *gin.Context) {
	connectionID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректный ID подключения"})
		return
	}
	companyID, exists := c.Get("company_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Компания не определена"})
		return
	}
	conn, err := api.service.GetByID(uint(connectionID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Подключение не найдено"})
		return
	}
	if conn.CompanyID != companyID.(uint) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Нет доступа"})
		return
	}

	var req services.CreateWialonAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат: " + err.Error()})
		return
	}

	svc := services.NewWialonAccountService()
	result, err := svc.CreateAccount(uint(connectionID), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка создания: " + err.Error()})
		return
	}

	// Инвалидация cache /all-accounts чтобы новый аккаунт сразу появился в списке
	invalidateAllAccountsCache(companyID.(uint))

	c.JSON(http.StatusCreated, result)
}
