package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

var bitrix24Service *services.Bitrix24IntegrationService

// InitBitrix24Service инициализирует сервис интеграции с Битрикс24
func InitBitrix24Service() {
	logger := log.New(os.Stdout, "[BITRIX24_API] ", log.LstdFlags|log.Lshortfile)
	bitrix24Service = services.NewBitrix24IntegrationService(logger)
}

// Bitrix24SetupRequest запрос для настройки интеграции с Битрикс24
type Bitrix24SetupRequest struct {
	WebhookURL   string `json:"webhook_url" binding:"required"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// Bitrix24SyncRequest запрос для синхронизации с Битрикс24
type Bitrix24SyncRequest struct {
	SyncObjects bool   `json:"sync_objects"`
	SyncUsers   bool   `json:"sync_users"`
	Direction   string `json:"direction"` // to_bitrix, from_bitrix, bidirectional
}

// SetupBitrix24Integration настраивает интеграцию с Битрикс24
func SetupBitrix24Integration(c *gin.Context) {
	tenantID := getTenantIDFromContext(c)
	if tenantID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Не удалось определить компанию",
		})
		return
	}

	var req Bitrix24SetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат запроса: " + err.Error(),
		})
		return
	}

	// Настраиваем интеграцию
	if err := bitrix24Service.SetupCompanyCredentials(c.Request.Context(), tenantID, req.WebhookURL); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка настройки интеграции: " + err.Error(),
		})
		return
	}

	// Если указаны дополнительные параметры OAuth, сохраняем их
	if req.ClientID != "" || req.ClientSecret != "" {
		db := database.GetDB()
		updates := map[string]interface{}{}
		if req.ClientID != "" {
			updates["bitrix24_client_id"] = req.ClientID
		}
		if req.ClientSecret != "" {
			updates["bitrix24_client_secret"] = req.ClientSecret
		}

		if err := db.Model(&models.Company{}).Where("id = ?", tenantID).Updates(updates).Error; err != nil {
			log.Printf("Предупреждение: не удалось сохранить дополнительные параметры OAuth: %v", err)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Интеграция с Битрикс24 успешно настроена",
	})
}

// CheckBitrix24Health проверяет состояние интеграции с Битрикс24
func CheckBitrix24Health(c *gin.Context) {
	tenantID := getTenantIDFromContext(c)
	if tenantID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Не удалось определить компанию",
		})
		return
	}

	// Проверяем доступность API
	err := bitrix24Service.CheckHealth(c.Request.Context(), tenantID)

	status := gin.H{
		"service":   "bitrix24",
		"healthy":   err == nil,
		"timestamp": time.Now(),
	}

	if err != nil {
		status["error"] = err.Error()
	}

	httpStatus := http.StatusOK
	if err != nil {
		httpStatus = http.StatusServiceUnavailable
	}

	c.JSON(httpStatus, gin.H{
		"status": "success",
		"data":   status,
	})
}

// SyncToBitrix24 запускает синхронизацию данных в Битрикс24
func SyncToBitrix24(c *gin.Context) {
	tenantID := getTenantIDFromContext(c)
	if tenantID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Не удалось определить компанию",
		})
		return
	}

	var req Bitrix24SyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат запроса: " + err.Error(),
		})
		return
	}

	// Получаем БД компании
	tenantDB := database.GetTenantDBByID(tenantID)
	if tenantDB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Не удалось получить базу данных компании",
		})
		return
	}

	results := gin.H{
		"objects_synced": 0,
		"users_synced":   0,
		"errors":         []string{},
	}

	// Синхронизируем объекты
	if req.SyncObjects {
		var objects []models.Object
		if err := tenantDB.Find(&objects).Error; err != nil {
			results["errors"] = append(results["errors"].([]string), "Ошибка получения объектов: "+err.Error())
		} else {
			objectsCount := 0
			for _, object := range objects {
				if err := bitrix24Service.SyncObjectToBitrix24(c.Request.Context(), tenantID, &object); err != nil {
					log.Printf("Ошибка синхронизации объекта %d: %v", object.ID, err)
					results["errors"] = append(results["errors"].([]string), "Объект "+strconv.Itoa(int(object.ID))+": "+err.Error())
				} else {
					objectsCount++
				}
			}
			results["objects_synced"] = objectsCount
		}
	}

	// Синхронизируем пользователей
	if req.SyncUsers {
		var users []models.User
		if err := tenantDB.Find(&users).Error; err != nil {
			results["errors"] = append(results["errors"].([]string), "Ошибка получения пользователей: "+err.Error())
		} else {
			usersCount := 0
			for _, user := range users {
				if err := bitrix24Service.SyncUserToBitrix24(c.Request.Context(), tenantID, &user); err != nil {
					log.Printf("Ошибка синхронизации пользователя %d: %v", user.ID, err)
					results["errors"] = append(results["errors"].([]string), "Пользователь "+strconv.Itoa(int(user.ID))+": "+err.Error())
				} else {
					usersCount++
				}
			}
			results["users_synced"] = usersCount
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Синхронизация завершена",
		"data":    results,
	})
}

// SyncFromBitrix24 запускает синхронизацию данных из Битрикс24
func SyncFromBitrix24(c *gin.Context) {
	tenantID := getTenantIDFromContext(c)
	if tenantID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Не удалось определить компанию",
		})
		return
	}

	// Запускаем синхронизацию из Битрикс24
	if err := bitrix24Service.SyncFromBitrix24(c.Request.Context(), tenantID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка синхронизации из Битрикс24: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Синхронизация из Битрикс24 завершена",
	})
}

// GetBitrix24Mappings получает маппинги синхронизации
func GetBitrix24Mappings(c *gin.Context) {
	tenantID := getTenantIDFromContext(c)
	if tenantID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Не удалось определить компанию",
		})
		return
	}

	// Параметры пагинации
	page := 1
	limit := 20

	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	offset := (page - 1) * limit

	// Получаем маппинги
	mappings, total, err := bitrix24Service.GetSyncMappings(tenantID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка получения маппингов: " + err.Error(),
		})
		return
	}

	// Подготавливаем ответ с пагинацией
	totalPages := int((total + int64(limit) - 1) / int64(limit))

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"mappings": mappings,
			"pagination": gin.H{
				"page":        page,
				"limit":       limit,
				"total":       total,
				"total_pages": totalPages,
				"has_next":    page < totalPages,
				"has_prev":    page > 1,
			},
		},
	})
}

// GetBitrix24Stats получает статистику интеграции с Битрикс24
func GetBitrix24Stats(c *gin.Context) {
	tenantID := getTenantIDFromContext(c)
	if tenantID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Не удалось определить компанию",
		})
		return
	}

	db := database.GetDB()

	// Статистика маппингов
	var stats struct {
		TotalMappings    int64      `json:"total_mappings"`
		ObjectMappings   int64      `json:"object_mappings"`
		UserMappings     int64      `json:"user_mappings"`
		ContractMappings int64      `json:"contract_mappings"`
		LastSyncTime     *time.Time `json:"last_sync_time"`
	}

	// Общее количество маппингов
	db.Model(&services.Bitrix24SyncMapping{}).Where("tenant_id = ?", tenantID).Count(&stats.TotalMappings)

	// Маппинги по типам
	db.Model(&services.Bitrix24SyncMapping{}).Where("tenant_id = ? AND local_type = ?", tenantID, "object").Count(&stats.ObjectMappings)
	db.Model(&services.Bitrix24SyncMapping{}).Where("tenant_id = ? AND local_type = ?", tenantID, "user").Count(&stats.UserMappings)
	db.Model(&services.Bitrix24SyncMapping{}).Where("tenant_id = ? AND local_type = ?", tenantID, "contract").Count(&stats.ContractMappings)

	// Время последней синхронизации
	var lastMapping services.Bitrix24SyncMapping
	if err := db.Where("tenant_id = ?", tenantID).Order("last_sync_at DESC").First(&lastMapping).Error; err == nil {
		stats.LastSyncTime = &lastMapping.LastSyncAt
	}

	// Статистика ошибок интеграции
	errorStats, err := models.GetIntegrationErrorStats(db, tenantID, 10)
	if err != nil {
		log.Printf("Ошибка получения статистики ошибок: %v", err)
	}

	// Фильтруем ошибки только для Битрикс24
	bitrix24ErrorCount := int64(0)
	if errorStats != nil {
		if count, exists := errorStats.ErrorsByService[models.IntegrationServiceBitrix24]; exists {
			bitrix24ErrorCount = count
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"mappings":     stats,
			"errors_count": bitrix24ErrorCount,
			"service_name": "Битрикс24",
			"generated_at": time.Now(),
		},
	})
}

// ClearBitrix24Cache очищает кэш интеграции с Битрикс24
func ClearBitrix24Cache(c *gin.Context) {
	tenantID := getTenantIDFromContext(c)
	if tenantID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Не удалось определить компанию",
		})
		return
	}

	// Очищаем кэш для компании
	bitrix24Service.ClearCredentialsCache(tenantID)

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Кэш интеграции с Битрикс24 очищен",
	})
}

// AutoSyncObjectToBitrix24 автоматически синхронизирует объект при создании/обновлении
func AutoSyncObjectToBitrix24(tenantID uint, object *models.Object) {
	if bitrix24Service == nil {
		return
	}

	// Проверяем, настроена ли интеграция для этой компании
	db := database.GetDB()
	var company models.Company
	if err := db.First(&company, tenantID).Error; err != nil {
		return
	}

	if company.Bitrix24WebhookURL == "" {
		return // Интеграция не настроена
	}

	// Запускаем синхронизацию асинхронно
	go func() {
		ctx := context.Background() // Используем background context для горутины
		if err := bitrix24Service.SyncObjectToBitrix24(ctx, tenantID, object); err != nil {
			log.Printf("Ошибка автоматической синхронизации объекта %d с Битрикс24: %v", object.ID, err)
		}
	}()
}

// AutoSyncUserToBitrix24 автоматически синхронизирует пользователя при создании/обновлении
func AutoSyncUserToBitrix24(tenantID uint, user *models.User) {
	if bitrix24Service == nil {
		return
	}

	// Проверяем, настроена ли интеграция для этой компании
	db := database.GetDB()
	var company models.Company
	if err := db.First(&company, tenantID).Error; err != nil {
		return
	}

	if company.Bitrix24WebhookURL == "" {
		return // Интеграция не настроена
	}

	// Запускаем синхронизацию асинхронно
	go func() {
		ctx := context.Background() // Используем background context для горутины
		if err := bitrix24Service.SyncUserToBitrix24(ctx, tenantID, user); err != nil {
			log.Printf("Ошибка автоматической синхронизации пользователя %d с Битрикс24: %v", user.ID, err)
		}
	}()
}

// getTenantIDFromContext извлекает ID компании из контекста Gin
func getTenantIDFromContext(c *gin.Context) uint {
	if tenantID, exists := c.Get("tenant_id"); exists {
		if id, ok := tenantID.(uint); ok {
			return id
		}
	}
	return 0
}
