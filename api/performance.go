package api

import (
	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/services"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// PerformanceAPI API для управления производительностью и безопасностью
type PerformanceAPI struct {
	cacheService    *services.PerformanceCacheService
	auditService    *services.AuditService
	loadTestService *services.LoadTestService
}

// NewPerformanceAPI создает новый API для управления производительностью
func NewPerformanceAPI() *PerformanceAPI {
	redis := database.GetRedis()
	cacheService := services.NewPerformanceCacheService(redis, nil)
	auditService := services.NewAuditService(database.DB, nil)
	loadTestService := services.NewLoadTestService("http://localhost:8080", nil)

	return &PerformanceAPI{
		cacheService:    cacheService,
		auditService:    auditService,
		loadTestService: loadTestService,
	}
}

// RegisterRoutes регистрирует маршруты для API производительности
func (api *PerformanceAPI) RegisterRoutes(router *gin.RouterGroup) {
	performance := router.Group("/performance")
	{
		// Кэширование
		performance.GET("/cache/metrics", api.getCacheMetrics)
		performance.POST("/cache/warmup", api.warmupCache)
		performance.DELETE("/cache/clear", api.clearCache)
		performance.GET("/cache/stats", api.getCacheStats)

		// Индексы БД
		performance.GET("/database/indexes", api.getDatabaseIndexes)
		performance.POST("/database/indexes/create", api.createIndexes)
		performance.GET("/database/indexes/usage", api.getIndexUsage)
		performance.POST("/database/optimize", api.optimizeDatabase)
		performance.GET("/database/stats", api.getDatabaseStats)

		// Rate Limiting
		performance.GET("/rate-limit/info", api.getRateLimitInfo)
		performance.DELETE("/rate-limit/clear", api.clearRateLimit)

		// Аудит логи
		performance.GET("/audit/logs", api.getAuditLogs)
		performance.GET("/audit/stats", api.getAuditStats)
		performance.GET("/audit/security-alerts", api.getSecurityAlerts)
		performance.POST("/audit/export", api.exportAuditLogs)
		performance.DELETE("/audit/cleanup", api.cleanupAuditLogs)

		// Системная информация
		performance.GET("/system/info", api.getSystemInfo)
		performance.GET("/system/health", api.getSystemHealth)

		// Нагрузочное тестирование
		performance.GET("/load-test/configs", api.getLoadTestConfigs)
		performance.POST("/load-test/run", api.runLoadTest)
		performance.GET("/load-test/results/:id", api.getLoadTestResult)
		performance.POST("/load-test/results/:id/export", api.exportLoadTestResult)
	}
}

// getCacheMetrics получает метрики кэширования
func (api *PerformanceAPI) getCacheMetrics(c *gin.Context) {
	metrics, err := api.cacheService.GetCacheMetrics()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    metrics,
	})
}

// warmupCache прогревает кэш
func (api *PerformanceAPI) warmupCache(c *gin.Context) {
	tenantID := getTenantID(c)

	if err := api.cacheService.CacheHotData(tenantID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Cache warmed up successfully",
	})
}

// clearCache очищает кэш
func (api *PerformanceAPI) clearCache(c *gin.Context) {
	redis := database.GetRedis()
	if redis == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Redis not available"})
		return
	}

	tenantID := getTenantID(c)
	pattern := fmt.Sprintf("tenant:%d:*", tenantID)

	keys, err := redis.Keys(database.Ctx, pattern).Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if len(keys) > 0 {
		if err := redis.Del(database.Ctx, keys...).Err(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Cleared %d cache keys", len(keys)),
	})
}

// getCacheStats получает статистику кэша
func (api *PerformanceAPI) getCacheStats(c *gin.Context) {
	stats, err := api.cacheService.GetCacheStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}

// getDatabaseIndexes получает информацию об индексах
func (api *PerformanceAPI) getDatabaseIndexes(c *gin.Context) {
	tableName := c.Query("table")
	if tableName == "" {
		// Получаем индексы для всех основных таблиц
		tables := []string{"objects", "users", "contracts", "installations", "equipment", "invoices", "reports"}
		result := make(map[string]interface{})

		for _, table := range tables {
			indexes, err := database.GetIndexInfo(database.DB, table)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			result[table] = indexes
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    result,
		})
		return
	}

	indexes, err := database.GetIndexInfo(database.DB, tableName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    indexes,
	})
}

// createIndexes создает индексы для оптимизации
func (api *PerformanceAPI) createIndexes(c *gin.Context) {
	if err := database.CreatePerformanceIndexes(database.DB); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Performance indexes created successfully",
	})
}

// getIndexUsage получает статистику использования индексов
func (api *PerformanceAPI) getIndexUsage(c *gin.Context) {
	usage, err := database.AnalyzeIndexUsage(database.DB)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    usage,
	})
}

// optimizeDatabase оптимизирует базу данных
func (api *PerformanceAPI) optimizeDatabase(c *gin.Context) {
	if err := database.OptimizeDatabase(database.DB); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Database optimized successfully",
	})
}

// getDatabaseStats получает статистику таблиц
func (api *PerformanceAPI) getDatabaseStats(c *gin.Context) {
	stats, err := database.GetTableStats(database.DB)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}

// getRateLimitInfo получает информацию о rate limiting
func (api *PerformanceAPI) getRateLimitInfo(c *gin.Context) {
	config := middleware.RateLimitConfig{
		Requests: 100,
		Window:   time.Minute,
	}

	info, err := middleware.GetRateLimitInfo(middleware.UserKeyGenerator, config, c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    info,
	})
}

// clearRateLimit очищает rate limit для пользователя
func (api *PerformanceAPI) clearRateLimit(c *gin.Context) {
	if err := middleware.ClearRateLimit(middleware.UserKeyGenerator, c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Rate limit cleared successfully",
	})
}

// getAuditLogs получает аудит логи
func (api *PerformanceAPI) getAuditLogs(c *gin.Context) {
	tenantID := getTenantID(c)

	filters := services.AuditFilters{
		Limit:  50,
		Offset: 0,
	}

	// Парсим параметры запроса
	if userIDStr := c.Query("user_id"); userIDStr != "" {
		if userID, err := strconv.ParseUint(userIDStr, 10, 32); err == nil {
			uid := uint(userID)
			filters.UserID = &uid
		}
	}

	if action := c.Query("action"); action != "" {
		filters.Action = action
	}

	if resource := c.Query("resource"); resource != "" {
		filters.Resource = resource
	}

	if limitStr := c.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			filters.Limit = limit
		}
	}

	if offsetStr := c.Query("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil {
			filters.Offset = offset
		}
	}

	logs, err := api.auditService.GetAuditLogs(tenantID, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    logs,
	})
}

// getAuditStats получает статистику аудит логов
func (api *PerformanceAPI) getAuditStats(c *gin.Context) {
	tenantID := getTenantID(c)
	period := c.DefaultQuery("period", "week")

	stats, err := api.auditService.GetAuditStats(tenantID, period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}

// getSecurityAlerts получает алерты безопасности
func (api *PerformanceAPI) getSecurityAlerts(c *gin.Context) {
	tenantID := getTenantID(c)
	hoursStr := c.DefaultQuery("hours", "24")

	hours, err := strconv.Atoi(hoursStr)
	if err != nil {
		hours = 24
	}

	alerts, err := api.auditService.GetSecurityAlerts(tenantID, hours)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    alerts,
	})
}

// exportAuditLogs экспортирует аудит логи
func (api *PerformanceAPI) exportAuditLogs(c *gin.Context) {
	tenantID := getTenantID(c)

	filters := services.AuditFilters{
		Limit: 10000, // Максимум для экспорта
	}

	// Парсим параметры из тела запроса
	var requestFilters struct {
		UserID    *uint     `json:"user_id"`
		Action    string    `json:"action"`
		Resource  string    `json:"resource"`
		StartDate time.Time `json:"start_date"`
		EndDate   time.Time `json:"end_date"`
		IPAddress string    `json:"ip_address"`
	}

	if err := c.ShouldBindJSON(&requestFilters); err == nil {
		filters.UserID = requestFilters.UserID
		filters.Action = requestFilters.Action
		filters.Resource = requestFilters.Resource
		filters.StartDate = requestFilters.StartDate
		filters.EndDate = requestFilters.EndDate
		filters.IPAddress = requestFilters.IPAddress
	}

	data, err := api.auditService.ExportAuditLogs(tenantID, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filename := fmt.Sprintf("audit_logs_%d_%s.json", tenantID, time.Now().Format("20060102_150405"))

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Data(http.StatusOK, "application/json", data)
}

// cleanupAuditLogs очищает старые аудит логи
func (api *PerformanceAPI) cleanupAuditLogs(c *gin.Context) {
	tenantID := getTenantID(c)
	retentionDaysStr := c.DefaultQuery("retention_days", "90")

	retentionDays, err := strconv.Atoi(retentionDaysStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid retention_days parameter"})
		return
	}

	if err := api.auditService.CleanupOldLogs(tenantID, retentionDays); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Cleaned up audit logs older than %d days", retentionDays),
	})
}

// getSystemInfo получает системную информацию
func (api *PerformanceAPI) getSystemInfo(c *gin.Context) {
	redis := database.GetRedis()
	redisStatus := "disabled"
	if redis != nil {
		if err := redis.Ping(database.Ctx).Err(); err == nil {
			redisStatus = "connected"
		} else {
			redisStatus = "error"
		}
	}

	dbStatus := "connected"
	if sqlDB, err := database.DB.DB(); err == nil {
		if err := sqlDB.Ping(); err != nil {
			dbStatus = "error"
		}
	} else {
		dbStatus = "error"
	}

	info := map[string]interface{}{
		"database": map[string]interface{}{
			"status": dbStatus,
			"driver": "postgresql",
		},
		"cache": map[string]interface{}{
			"status": redisStatus,
			"type":   "redis",
		},
		"performance": map[string]interface{}{
			"indexes_enabled":       true,
			"rate_limiting_enabled": true,
			"audit_logging_enabled": true,
		},
		"timestamp": time.Now(),
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    info,
	})
}

// getSystemHealth проверяет здоровье системы
func (api *PerformanceAPI) getSystemHealth(c *gin.Context) {
	health := map[string]interface{}{
		"status": "healthy",
		"checks": map[string]interface{}{},
	}

	overallStatus := true

	// Проверка базы данных
	dbStatus := true
	if sqlDB, err := database.DB.DB(); err == nil {
		if err := sqlDB.Ping(); err != nil {
			dbStatus = false
			overallStatus = false
		}
	} else {
		dbStatus = false
		overallStatus = false
	}

	health["checks"].(map[string]interface{})["database"] = map[string]interface{}{
		"status": dbStatus,
		"name":   "PostgreSQL Database",
	}

	// Проверка Redis
	redisStatus := true
	redis := database.GetRedis()
	if redis != nil {
		if err := redis.Ping(database.Ctx).Err(); err != nil {
			redisStatus = false
			// Redis не критичен для работы системы
		}
	} else {
		redisStatus = false
	}

	health["checks"].(map[string]interface{})["cache"] = map[string]interface{}{
		"status": redisStatus,
		"name":   "Redis Cache",
	}

	if !overallStatus {
		health["status"] = "unhealthy"
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"data":    health,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    health,
	})
}

// getLoadTestConfigs получает предустановленные конфигурации тестов
func (api *PerformanceAPI) getLoadTestConfigs(c *gin.Context) {
	configs := api.loadTestService.GetPredefinedConfigs()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    configs,
	})
}

// runLoadTest запускает нагрузочное тестирование
func (api *PerformanceAPI) runLoadTest(c *gin.Context) {
	var config services.LoadTestConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Запускаем тест в горутине
	go func() {
		ctx := context.Background()
		result, err := api.loadTestService.RunLoadTest(ctx, config)
		if err != nil {
			// В реальном приложении здесь нужно сохранить ошибку
			return
		}

		// В реальном приложении здесь нужно сохранить результат в БД
		_ = result
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"message": "Load test started",
		"test_id": fmt.Sprintf("test_%d", time.Now().Unix()),
	})
}

// getLoadTestResult получает результат нагрузочного тестирования
func (api *PerformanceAPI) getLoadTestResult(c *gin.Context) {
	testID := c.Param("id")

	// В реальном приложении здесь нужно получить результат из БД
	// Для демо возвращаем мокированный результат
	mockResult := api.getMockLoadTestResult()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"test_id": testID,
			"result":  mockResult,
		},
	})
}

// exportLoadTestResult экспортирует результат нагрузочного тестирования
func (api *PerformanceAPI) exportLoadTestResult(c *gin.Context) {
	testID := c.Param("id")

	// В реальном приложении здесь нужно получить результат из БД
	mockResult := api.getMockLoadTestResult()

	data, err := api.loadTestService.ExportResults(mockResult)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filename := fmt.Sprintf("load_test_result_%s.json", testID)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Data(http.StatusOK, "application/json", data)
}

// getMockLoadTestResult возвращает мокированный результат нагрузочного тестирования
func (api *PerformanceAPI) getMockLoadTestResult() *services.LoadTestResult {
	config := services.LoadTestConfig{
		ConcurrentUsers: 50,
		DurationSeconds: 300,
		RampUpSeconds:   30,
		Endpoints: []string{
			"/api/objects",
			"/api/users",
			"/api/contracts",
			"/api/installations",
			"/api/dashboard/stats",
		},
		ThinkTimeMs: 500,
	}

	return &services.LoadTestResult{
		Config:              config,
		StartTime:           time.Now().Add(-5 * time.Minute),
		EndTime:             time.Now(),
		TotalRequests:       15420,
		SuccessfulRequests:  14890,
		FailedRequests:      530,
		AverageResponseTime: 145.6,
		MaxResponseTime:     2340.5,
		MinResponseTime:     12.3,
		RequestsPerSecond:   51.4,
		ErrorRate:           3.44,
		ResultsByEndpoint: map[string]*services.EndpointResult{
			"/api/objects": {
				Endpoint:            "/api/objects",
				Requests:            3850,
				SuccessfulRequests:  3720,
				FailedRequests:      130,
				AverageResponseTime: 125.4,
				MaxResponseTime:     1890.2,
				MinResponseTime:     15.6,
				ErrorRate:           3.38,
			},
			"/api/users": {
				Endpoint:            "/api/users",
				Requests:            3100,
				SuccessfulRequests:  2980,
				FailedRequests:      120,
				AverageResponseTime: 98.7,
				MaxResponseTime:     1456.8,
				MinResponseTime:     12.3,
				ErrorRate:           3.87,
			},
			"/api/contracts": {
				Endpoint:            "/api/contracts",
				Requests:            2890,
				SuccessfulRequests:  2790,
				FailedRequests:      100,
				AverageResponseTime: 156.3,
				MaxResponseTime:     2340.5,
				MinResponseTime:     18.9,
				ErrorRate:           3.46,
			},
			"/api/installations": {
				Endpoint:            "/api/installations",
				Requests:            2940,
				SuccessfulRequests:  2850,
				FailedRequests:      90,
				AverageResponseTime: 134.2,
				MaxResponseTime:     1987.4,
				MinResponseTime:     16.2,
				ErrorRate:           3.06,
			},
			"/api/dashboard/stats": {
				Endpoint:            "/api/dashboard/stats",
				Requests:            2640,
				SuccessfulRequests:  2550,
				FailedRequests:      90,
				AverageResponseTime: 189.8,
				MaxResponseTime:     2145.7,
				MinResponseTime:     25.4,
				ErrorRate:           3.41,
			},
		},
		ResponseTimeHistogram: map[string]int64{
			"0-50ms":    2340,
			"50-100ms":  3890,
			"100-200ms": 5670,
			"200-500ms": 2890,
			"500ms-1s":  520,
			"1s-2s":     95,
			"2s+":       15,
		},
		ErrorsByType: map[string]int64{
			"network_error": 180,
			"server_error":  250,
			"client_error":  100,
		},
	}
}

// getTenantID получает ID тенанта из контекста
func getTenantID(c *gin.Context) uint {
	if tenantID, exists := c.Get("tenant_id"); exists {
		if id, ok := tenantID.(uint); ok {
			return id
		}
	}
	return 1 // Дефолтный тенант для демо
}
