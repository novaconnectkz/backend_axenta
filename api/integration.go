package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
	"context"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetIntegrationHealth проверяет статус интеграций
func GetIntegrationHealth(c *gin.Context) {
	integrationService := services.GetIntegrationService()
	if integrationService == nil {
		c.JSON(500, gin.H{"status": "error", "error": "Сервис интеграций недоступен"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	healthResults := integrationService.CheckHealth(ctx)

	allHealthy := true
	for _, err := range healthResults {
		if err != nil {
			allHealthy = false
			break
		}
	}

	status := "healthy"
	if !allHealthy {
		status = "unhealthy"
	}

	// Преобразуем ошибки в строки для JSON
	results := make(map[string]interface{})
	for service, err := range healthResults {
		if err != nil {
			results[service] = map[string]string{
				"status": "error",
				"error":  err.Error(),
			}
		} else {
			results[service] = map[string]string{
				"status": "ok",
			}
		}
	}

	c.JSON(200, gin.H{
		"status":             "success",
		"overall":            status,
		"services":           results,
		"cached_credentials": integrationService.GetCachedCredentialsCount(),
	})
}

// GetIntegrationErrors получает список ошибок интеграции для текущей компании
func GetIntegrationErrors(c *gin.Context) {
	// Получаем tenant ID из контекста
	tenantID, exists := c.Get("tenant_id")
	if !exists {
		c.JSON(400, gin.H{"status": "error", "error": "Не удалось определить компанию"})
		return
	}

	tid, ok := tenantID.(uint)
	if !ok {
		c.JSON(400, gin.H{"status": "error", "error": "Некорректный ID компании"})
		return
	}

	// Получаем параметры запроса
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	status := c.Query("status")
	service := c.Query("service")
	operation := c.Query("operation")
	retryableOnly := c.DefaultQuery("retryable_only", "false") == "true"

	// Базовый запрос
	db := database.GetDB()
	query := db.Model(&models.IntegrationError{}).Where("tenant_id = ?", tid)

	// Фильтры
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if service != "" {
		query = query.Where("service = ?", service)
	}
	if operation != "" {
		query = query.Where("operation = ?", operation)
	}
	if retryableOnly {
		query = query.Where("retryable = ? AND retry_count < max_retries", true)
	}

	// Подсчет общего количества
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка подсчета ошибок интеграции: " + err.Error()})
		return
	}

	// Получение ошибок с пагинацией
	var errors []models.IntegrationError
	offset := (page - 1) * limit
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&errors).Error; err != nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка получения ошибок интеграции: " + err.Error()})
		return
	}

	// Формируем ответ
	response := gin.H{
		"status": "success",
		"data": gin.H{
			"items":       errors,
			"total":       total,
			"page":        page,
			"limit":       limit,
			"total_pages": (total + int64(limit) - 1) / int64(limit),
		},
	}

	c.JSON(200, response)
}

// GetIntegrationErrorStats получает статистику ошибок интеграции
func GetIntegrationErrorStats(c *gin.Context) {
	// Получаем tenant ID из контекста
	tenantID, exists := c.Get("tenant_id")
	if !exists {
		c.JSON(400, gin.H{"status": "error", "error": "Не удалось определить компанию"})
		return
	}

	tid, ok := tenantID.(uint)
	if !ok {
		c.JSON(400, gin.H{"status": "error", "error": "Некорректный ID компании"})
		return
	}

	// Получаем лимит для последних ошибок
	recentLimit, _ := strconv.Atoi(c.DefaultQuery("recent_limit", "10"))

	db := database.GetDB()
	stats, err := models.GetIntegrationErrorStats(db, tid, recentLimit)
	if err != nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка получения статистики: " + err.Error()})
		return
	}

	c.JSON(200, gin.H{"status": "success", "data": stats})
}

// RetryIntegrationError повторяет обработку ошибки интеграции
func RetryIntegrationError(c *gin.Context) {
	// Получаем ID ошибки
	errorID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"status": "error", "error": "Некорректный ID ошибки"})
		return
	}

	// Получаем tenant ID из контекста
	tenantID, exists := c.Get("tenant_id")
	if !exists {
		c.JSON(400, gin.H{"status": "error", "error": "Не удалось определить компанию"})
		return
	}

	tid, ok := tenantID.(uint)
	if !ok {
		c.JSON(400, gin.H{"status": "error", "error": "Некорректный ID компании"})
		return
	}

	// Находим ошибку
	db := database.GetDB()
	var integrationError models.IntegrationError
	if err := db.Where("id = ? AND tenant_id = ?", errorID, tid).First(&integrationError).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(404, gin.H{"status": "error", "error": "Ошибка интеграции не найдена"})
		} else {
			c.JSON(500, gin.H{"status": "error", "error": "Ошибка поиска ошибки интеграции: " + err.Error()})
		}
		return
	}

	// Проверяем, можно ли повторить
	if !integrationError.CanRetry() {
		c.JSON(400, gin.H{
			"status": "error",
			"error":  "Ошибку нельзя повторить",
			"reason": gin.H{
				"retryable":   integrationError.Retryable,
				"retry_count": integrationError.RetryCount,
				"max_retries": integrationError.MaxRetries,
				"status":      integrationError.Status,
				"next_retry":  integrationError.NextRetryAt,
			},
		})
		return
	}

	// Получаем сервис интеграций
	integrationService := services.GetIntegrationService()
	if integrationService == nil {
		c.JSON(500, gin.H{"status": "error", "error": "Сервис интеграций недоступен"})
		return
	}

	// Отмечаем как обрабатываемую
	integrationError.MarkAsProcessing()
	if err := db.Save(&integrationError).Error; err != nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка обновления статуса: " + err.Error()})
		return
	}

	// Получаем объект для повторной синхронизации
	tenantDB := database.GetTenantDBByID(tid)
	var object models.Object
	if err := tenantDB.First(&object, integrationError.ObjectID).Error; err != nil {
		// Если объект не найден, отмечаем ошибку как неразрешимую
		integrationError.MarkAsFailed()
		db.Save(&integrationError)
		c.JSON(400, gin.H{"status": "error", "error": "Связанный объект не найден"})
		return
	}

	// Запускаем повторную синхронизацию асинхронно
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var syncErr error
		switch integrationError.Operation {
		case models.IntegrationOperationCreate:
			syncErr = integrationService.SyncObjectCreate(ctx, tid, &object)
		case models.IntegrationOperationUpdate:
			syncErr = integrationService.SyncObjectUpdate(ctx, tid, &object)
		case models.IntegrationOperationDelete:
			syncErr = integrationService.SyncObjectDelete(ctx, tid, &object)
		default:
			syncErr = &services.IntegrationError{
				TenantID:  tid,
				Operation: integrationError.Operation,
				ObjectID:  integrationError.ObjectID,
				Message:   "Неизвестная операция: " + integrationError.Operation,
				Retryable: false,
			}
		}

		// Обновляем статус ошибки
		if syncErr != nil {
			// Увеличиваем счетчик повторов
			integrationError.IncrementRetryCount(integrationError.GetRetryDelay())
			if !integrationError.CanRetry() {
				integrationError.MarkAsFailed()
			}
		} else {
			// Операция успешна
			integrationError.MarkAsResolved("manual_retry")
		}

		db.Save(&integrationError)
	}()

	c.JSON(200, gin.H{
		"status":  "success",
		"message": "Повторная обработка ошибки запущена",
		"data": gin.H{
			"error_id":    integrationError.ID,
			"retry_count": integrationError.RetryCount + 1,
			"max_retries": integrationError.MaxRetries,
		},
	})
}

// ResolveIntegrationError отмечает ошибку как решенную вручную
func ResolveIntegrationError(c *gin.Context) {
	// Получаем ID ошибки
	errorID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"status": "error", "error": "Некорректный ID ошибки"})
		return
	}

	// Получаем tenant ID из контекста
	tenantID, exists := c.Get("tenant_id")
	if !exists {
		c.JSON(400, gin.H{"status": "error", "error": "Не удалось определить компанию"})
		return
	}

	tid, ok := tenantID.(uint)
	if !ok {
		c.JSON(400, gin.H{"status": "error", "error": "Некорректный ID компании"})
		return
	}

	// Находим ошибку
	db := database.GetDB()
	var integrationError models.IntegrationError
	if err := db.Where("id = ? AND tenant_id = ?", errorID, tid).First(&integrationError).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(404, gin.H{"status": "error", "error": "Ошибка интеграции не найдена"})
		} else {
			c.JSON(500, gin.H{"status": "error", "error": "Ошибка поиска ошибки интеграции: " + err.Error()})
		}
		return
	}

	// Отмечаем как решенную
	integrationError.MarkAsResolved("manual_resolve")
	if err := db.Save(&integrationError).Error; err != nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка обновления статуса: " + err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"status":  "success",
		"message": "Ошибка интеграции отмечена как решенная",
		"data": gin.H{
			"error_id":    integrationError.ID,
			"status":      integrationError.Status,
			"resolved_at": integrationError.ResolvedAt,
			"resolved_by": integrationError.ResolvedBy,
		},
	})
}

// SetupCompanyCredentials настраивает учетные данные компании для интеграций
func SetupCompanyCredentials(c *gin.Context) {
	// Получаем tenant ID из контекста
	tenantID, exists := c.Get("tenant_id")
	if !exists {
		c.JSON(400, gin.H{"status": "error", "error": "Не удалось определить компанию"})
		return
	}

	tid, ok := tenantID.(uint)
	if !ok {
		c.JSON(400, gin.H{"status": "error", "error": "Некорректный ID компании"})
		return
	}

	// Парсим данные из запроса
	var request struct {
		AxetnaLogin    string `json:"axetna_login" binding:"required"`
		AxetnaPassword string `json:"axetna_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(400, gin.H{"status": "error", "error": "Некорректные данные: " + err.Error()})
		return
	}

	// Получаем сервис интеграций
	integrationService := services.GetIntegrationService()
	if integrationService == nil {
		c.JSON(500, gin.H{"status": "error", "error": "Сервис интеграций недоступен"})
		return
	}

	// Настраиваем учетные данные
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := integrationService.SetupCompanyCredentials(ctx, tid, request.AxetnaLogin, request.AxetnaPassword)
	if err != nil {
		c.JSON(400, gin.H{"status": "error", "error": "Ошибка настройки учетных данных: " + err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"status":  "success",
		"message": "Учетные данные для интеграции с Axetna.cloud успешно настроены",
		"data": gin.H{
			"tenant_id": tid,
			"login":     request.AxetnaLogin,
		},
	})
}

// ClearIntegrationCache очищает кэш интеграций
func ClearIntegrationCache(c *gin.Context) {
	// Получаем tenant ID из контекста (опционально)
	tenantID, exists := c.Get("tenant_id")
	var tid uint = 0
	if exists {
		if t, ok := tenantID.(uint); ok {
			tid = t
		}
	}

	// Получаем сервис интеграций
	integrationService := services.GetIntegrationService()
	if integrationService == nil {
		c.JSON(500, gin.H{"status": "error", "error": "Сервис интеграций недоступен"})
		return
	}

	// Очищаем кэш
	integrationService.ClearCredentialsCache(tid)

	message := "Кэш интеграций очищен"
	if tid > 0 {
		message = "Кэш интеграций очищен для текущей компании"
	} else {
		message = "Весь кэш интеграций очищен"
	}

	c.JSON(200, gin.H{
		"status":  "success",
		"message": message,
		"data": gin.H{
			"tenant_id":          tid,
			"cached_credentials": integrationService.GetCachedCredentialsCount(),
		},
	})
}
