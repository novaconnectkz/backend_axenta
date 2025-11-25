package api

import (
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// GetBillingPlans получает список всех тарифных планов
func GetBillingPlans(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	var plans []models.BillingPlan

	// Убеждаемся, что мы в схеме public для глобальных таблиц
	if err := database.DB.Exec("SET search_path TO public").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подключения к базе данных",
		})
		return
	}

	// Получаем только активные планы по умолчанию
	query := database.DB.Where("is_active = ?", true).
		Where("admin_account_id = ?", adminAccountID)

	// Дополнительная фильтрация по company_id (опционально)
	if companyID := c.Query("company_id"); companyID != "" {
		if companyIDUint, err := strconv.ParseUint(companyID, 10, 32); err == nil {
			query = query.Where("company_id = ?", uint(companyIDUint))
		} else {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  fmt.Sprintf("Неверный формат company_id: %v", err),
			})
			return
		}
	}

	if err := query.Find(&plans).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при получении тарифных планов",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   plans,
		"count":  len(plans),
	})
}

// GetBillingPlan получает конкретный тарифный план по ID
func GetBillingPlan(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	id := c.Param("id")
	companyIDStr := c.Query("company_id")

	if err := database.DB.Exec("SET search_path TO public").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подключения к базе данных",
		})
		return
	}

	var plan models.BillingPlan
	query := database.DB.Where("id = ?", id).Where("admin_account_id = ?", adminAccountID)

	// Проверяем принадлежность к компании
	if companyIDStr != "" {
		query = query.Where("company_id = ?", companyIDStr)
	} else {
		// Если company_id не указан, возвращаем ошибку (безопасность)
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Параметр company_id обязателен",
		})
		return
	}

	if err := query.First(&plan).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Тарифный план не найден или не принадлежит вашей компании",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   plan,
	})
}

// CreateBillingPlan создает новый тарифный план
func CreateBillingPlan(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	var plan models.BillingPlan

	if err := c.ShouldBindJSON(&plan); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Неверный формат данных: %v", err),
		})
		return
	}

	// Валидация обязательных полей
	if plan.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Название тарифного плана обязательно",
		})
		return
	}

	// Проверяем, что Price корректно установлен
	if plan.Price.IsZero() && plan.Price.IsNegative() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Цена должна быть положительным числом",
		})
		return
	}

	if plan.Price.LessThan(decimal.Zero) {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Цена не может быть отрицательной",
		})
		return
	}

	// Устанавливаем значения по умолчанию
	if plan.Currency == "" {
		plan.Currency = "RUB"
	}
	if plan.BillingPeriod == "" {
		plan.BillingPeriod = "monthly"
	}

	// Устанавливаем company_id из query параметра (если не указан в теле запроса)
	if plan.CompanyID == nil {
		companyIDStr := c.Query("company_id")
		if companyIDStr != "" {
			if companyID, parseErr := strconv.ParseUint(companyIDStr, 10, 32); parseErr == nil {
				companyIDUint := uint(companyID)
				plan.CompanyID = &companyIDUint
			} else {
				c.JSON(http.StatusBadRequest, gin.H{
					"status": "error",
					"error":  fmt.Sprintf("Неверный формат company_id: %v", parseErr),
				})
				return
			}
		}
	}

	plan.AdminAccountID = adminAccountID

	// Убеждаемся, что мы в схеме public для глобальных таблиц
	if err := database.DB.Exec("SET search_path TO public").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Ошибка подключения к базе данных: %v", err),
		})
		return
	}

	// Проверяем, нет ли уже плана с таким же именем
	// ВАЖНО: Уникальный индекс idx_billing_plans_name в БД действует глобально по имени,
	// поэтому проверяем ВСЕ планы с таким именем, независимо от company_id
	var existingPlan models.BillingPlan
	// Сначала проверяем планы для текущей компании
	if plan.CompanyID != nil {
		// Проверяем активные планы для этой компании
		if err := database.DB.Where("name = ? AND company_id = ? AND admin_account_id = ? AND is_active = ?", plan.Name, *plan.CompanyID, adminAccountID, true).First(&existingPlan).Error; err == nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  fmt.Sprintf("Тарифный план с названием '%s' уже существует для вашей компании", plan.Name),
			})
			return
		}
		// Проверяем неактивные планы для этой компании
		if err := database.DB.Where("name = ? AND company_id = ? AND admin_account_id = ? AND is_active = ?", plan.Name, *plan.CompanyID, adminAccountID, false).First(&existingPlan).Error; err == nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  fmt.Sprintf("Тарифный план с названием '%s' уже существует (неактивен). Пожалуйста, активируйте существующий план или удалите его перед созданием нового.", plan.Name),
			})
			return
		}
	}

	// ВАЖНО: Также проверяем планы с таким именем глобально (включая удаленные)
	// Уникальный индекс в БД действует на все записи, включая удаленные (soft delete)
	// Используем Unscoped() чтобы найти даже удаленные планы
	var deletedPlan models.BillingPlan
	if err := database.DB.Unscoped().Where("name = ? AND admin_account_id = ?", plan.Name, adminAccountID).First(&deletedPlan).Error; err == nil {
		// Если план удален (мягкое удаление)
		if deletedPlan.DeletedAt.Valid {
			// Если удаленный план принадлежит той же компании, физически удаляем его
			if plan.CompanyID != nil && deletedPlan.CompanyID != nil && *deletedPlan.CompanyID == *plan.CompanyID {
				// Физически удаляем старый план, чтобы освободить имя
				database.DB.Unscoped().Delete(&deletedPlan)
			} else if deletedPlan.CompanyID == nil {
				// Удаленный план с company_id=NULL (глобальный план) - можно физически удалить, если создаем для конкретной компании
				if plan.CompanyID != nil {
					database.DB.Unscoped().Delete(&deletedPlan)
				} else {
					// Создаем глобальный план, а удаленный тоже глобальный - нельзя
					c.JSON(http.StatusBadRequest, gin.H{
						"status": "error",
						"error":  fmt.Sprintf("Тарифный план с названием '%s' уже существует в системе. Пожалуйста, выберите другое название.", plan.Name),
					})
					return
				}
			} else {
				// Удаленный план другой компании - нельзя использовать это имя
				c.JSON(http.StatusBadRequest, gin.H{
					"status": "error",
					"error":  fmt.Sprintf("Тарифный план с названием '%s' уже существует в системе. Пожалуйста, выберите другое название.", plan.Name),
				})
				return
			}
		} else {
			// План не удален
			// Если план принадлежит другой компании или не имеет company_id
			if deletedPlan.CompanyID == nil || (plan.CompanyID != nil && *deletedPlan.CompanyID != *plan.CompanyID) {
				c.JSON(http.StatusBadRequest, gin.H{
					"status": "error",
					"error":  fmt.Sprintf("Тарифный план с названием '%s' уже существует в системе. Пожалуйста, выберите другое название.", plan.Name),
				})
				return
			}
			// Если это тот же план (что маловероятно, так как мы уже проверили выше)
			if deletedPlan.CompanyID != nil && plan.CompanyID != nil && *deletedPlan.CompanyID == *plan.CompanyID {
				if !deletedPlan.IsActive {
					c.JSON(http.StatusBadRequest, gin.H{
						"status": "error",
						"error":  fmt.Sprintf("Тарифный план с названием '%s' уже существует (неактивен). Пожалуйста, активируйте существующий план или удалите его перед созданием нового.", plan.Name),
					})
					return
				}
			}
		}
	}

	if err := database.DB.Create(&plan).Error; err != nil {
		// Проверяем тип ошибки для более понятного сообщения
		errorMsg := "Ошибка при создании тарифного плана"
		errStr := err.Error()

		// Проверяем на дубликат имени (может быть из-за старого индекса или глобального уникального индекса)
		if strings.Contains(errStr, "duplicate key") || strings.Contains(errStr, "UNIQUE constraint") || strings.Contains(errStr, "violates unique constraint") {
			// Пытаемся найти существующий план для более информативного сообщения
			var existingPlan models.BillingPlan
			if err := database.DB.Where("name = ? AND admin_account_id = ?", plan.Name, adminAccountID).First(&existingPlan).Error; err == nil {
				errorMsg = fmt.Sprintf("Тарифный план с названием '%s' уже существует (ID: %d, компания: %v, активен: %v)",
					plan.Name, existingPlan.ID, existingPlan.CompanyID, existingPlan.IsActive)
			} else {
				errorMsg = fmt.Sprintf("Тарифный план с названием '%s' уже существует", plan.Name)
			}
			// Возвращаем 400 Bad Request вместо 500 Internal Server Error для ошибок валидации
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  errorMsg,
			})
			return
		} else if errStr != "" {
			errorMsg = fmt.Sprintf("Ошибка при создании тарифного плана: %v", err)
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  errorMsg,
		})
		return
	}

	fmt.Printf("CreateBillingPlan: план успешно создан с ID: %d\n", plan.ID)

	c.JSON(http.StatusCreated, gin.H{
		"status": "success",
		"data":   plan,
	})
}

// UpdateBillingPlan обновляет существующий тарифный план
func UpdateBillingPlan(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	id := c.Param("id")
	companyIDStr := c.Query("company_id")

	if err := database.DB.Exec("SET search_path TO public").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подключения к базе данных",
		})
		return
	}

	var plan models.BillingPlan
	query := database.DB.Where("id = ?", id).Where("admin_account_id = ?", adminAccountID)

	// Проверяем принадлежность к компании
	if companyIDStr != "" {
		query = query.Where("company_id = ?", companyIDStr)
	} else {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Параметр company_id обязателен",
		})
		return
	}

	if err := query.First(&plan).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Тарифный план не найден или не принадлежит вашей компании",
		})
		return
	}

	var updateData models.BillingPlan
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных",
		})
		return
	}

	// Валидация
	if updateData.Price.LessThan(decimal.Zero) {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Цена не может быть отрицательной",
		})
		return
	}

	// Игнорируем попытку изменить company_id через обновление
	updateData.CompanyID = nil
	updateData.AdminAccountID = 0

	if err := database.DB.Model(&plan).Updates(updateData).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при обновлении тарифного плана",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   plan,
	})
}

// DeleteBillingPlan удаляет тарифный план (мягкое удаление)
func DeleteBillingPlan(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	id := c.Param("id")
	companyIDStr := c.Query("company_id")

	// Параметр company_id обязателен
	if companyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Параметр company_id обязателен",
		})
		return
	}

	// Парсим company_id в uint
	companyIDUint, parseErr := strconv.ParseUint(companyIDStr, 10, 32)
	if parseErr != nil {
		fmt.Printf("DeleteBillingPlan: ошибка парсинга company_id: %v\n", parseErr)
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Неверный формат company_id: %v", parseErr),
		})
		return
	}
	companyID := uint(companyIDUint)

	if err := database.DB.Exec("SET search_path TO public").Error; err != nil {
		fmt.Printf("DeleteBillingPlan: ошибка установки search_path: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подключения к базе данных",
		})
		return
	}

	var plan models.BillingPlan
	query := database.DB.Where("id = ? AND company_id = ? AND admin_account_id = ?", id, companyID, adminAccountID)

	if err := query.First(&plan).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Тарифный план не найден или не принадлежит вашей компании",
		})
		return
	}

	// Проверяем, есть ли активные подписки на этот план (только для этой компании)
	var subscriptionCount int64
	database.DB.Model(&models.Subscription{}).
		Where("billing_plan_id = ? AND company_id = ? AND admin_account_id = ? AND status = 'active'", plan.ID, companyID, adminAccountID).
		Count(&subscriptionCount)

	if subscriptionCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Нельзя удалить тарифный план с активными подписками",
		})
		return
	}

	if err := database.DB.Delete(&plan).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при удалении тарифного плана",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Тарифный план успешно удален",
	})
}

// GetSubscriptions получает список подписок для компании
func GetSubscriptions(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	companyIDStr := c.Query("company_id")
	if companyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Параметр company_id обязателен",
		})
		return
	}

	companyID, err := strconv.ParseUint(companyIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат company_id",
		})
		return
	}

	// Работаем в схеме public, где хранится таблица subscriptions
	db := database.DB.Session(&gorm.Session{})
	if err := db.Exec("SET search_path TO public").Error; err != nil {
		fmt.Printf("GetSubscriptions: ошибка установки search_path: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подключения к базе данных",
		})
		return
	}

	var subscriptions []models.Subscription
	if err := db.Preload("BillingPlan", "admin_account_id = ?", adminAccountID).
		Where("company_id = ? AND admin_account_id = ?", uint(companyID), adminAccountID).
		Find(&subscriptions).Error; err != nil {
		fmt.Printf("GetSubscriptions: ОШИБКА при получении подписок (admin_account_id=%d, company_id=%d): %v\n",
			adminAccountID, companyID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при получении подписок",
		})
		return
	}

	// Загружаем информацию о договорах для подписок (из tenant-схемы)
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB != nil {
		for i := range subscriptions {
			if subscriptions[i].ContractID != nil {
				var contract models.Contract
			if err := tenantDB.Where("id = ? AND admin_account_id = ?", *subscriptions[i].ContractID, adminAccountID).
				First(&contract).Error; err == nil {
				subscriptions[i].Contract = &contract
			}
			}
		}
	}

	// Создаем ответ с объектами для каждой подписки
	subscriptionsResponse := make([]map[string]interface{}, 0, len(subscriptions))

	// Собираем все уникальные ObjectID и CompanyID для batch-загрузки названий
	type ObjectKey struct {
		ObjectID  uint
		CompanyID uint
	}
	objectKeysSet := make(map[ObjectKey]bool)

	// Сначала собираем все объекты из подписок
	for _, sub := range subscriptions {
		if sub.ContractID != nil && tenantDB != nil {
			var contractObjects []models.ContractObject
			// Фильтруем объекты по subscription_id, чтобы получить только те, которые принадлежат этой подписке
			if err := tenantDB.Where("contract_id = ? AND subscription_id = ? AND status = ?", *sub.ContractID, sub.ID, "active").
				Find(&contractObjects).Error; err == nil {
				for _, co := range contractObjects {
					objectKeysSet[ObjectKey{ObjectID: co.ObjectID, CompanyID: co.ObjectCompanyID}] = true
				}
			}
		}
	}

	// Загружаем названия объектов из Axenta Cloud (batch)
	objectNamesMap := make(map[ObjectKey]string)
	if len(objectKeysSet) > 0 {
		// Получаем токен пользователя для запроса к Axenta Cloud
		authHeader := c.GetHeader("Authorization")
		var userToken string
		if strings.HasPrefix(authHeader, "Token ") {
			userToken = strings.TrimPrefix(authHeader, "Token ")
		} else if strings.HasPrefix(authHeader, "Bearer ") {
			userToken = strings.TrimPrefix(authHeader, "Bearer ")
		} else {
			userToken = authHeader
		}

		// Группируем объекты по CompanyID для batch-запросов
		objectsByCompany := make(map[uint][]uint)
		for key := range objectKeysSet {
			objectsByCompany[key.CompanyID] = append(objectsByCompany[key.CompanyID], key.ObjectID)
		}

		// Загружаем объекты по компаниям
		if userToken != "" {
			for companyID, objectIDs := range objectsByCompany {
				if len(objectIDs) > 50 {
					// Если объектов слишком много, берем только первые 50
					objectIDs = objectIDs[:50]
				}

				axentaObjects, err := fetchObjectsFromAxentaCloud(userToken, int(companyID), objectIDs)
				if err != nil {
					log.Printf("⚠️ Не удалось загрузить названия объектов для компании %d: %v", companyID, err)
					// Используем плейсхолдеры для этой компании
					for _, objectID := range objectIDs {
						objectNamesMap[ObjectKey{ObjectID: objectID, CompanyID: companyID}] = fmt.Sprintf("Объект #%d", objectID)
					}
				} else {
					// Сохраняем названия в карту
					for _, obj := range axentaObjects {
						objectNamesMap[ObjectKey{ObjectID: uint(obj.ID), CompanyID: companyID}] = obj.Name
					}
					log.Printf("✅ Загружено %d названий объектов для компании %d", len(axentaObjects), companyID)
				}
			}
		} else {
			log.Printf("⚠️ Токен пользователя не найден, используем плейсхолдеры для названий объектов")
			for key := range objectKeysSet {
				objectNamesMap[key] = fmt.Sprintf("Объект #%d", key.ObjectID)
			}
		}
	}

	for _, sub := range subscriptions {
		subMap := make(map[string]interface{})

		// Преобразуем подписку в map для добавления объектов
		subJSON, _ := json.Marshal(sub)
		json.Unmarshal(subJSON, &subMap)

		// Добавляем объекты, если они есть
		if sub.ContractID != nil && tenantDB != nil {
			var contractObjects []models.ContractObject
			// Фильтруем объекты по subscription_id, чтобы получить только те, которые принадлежат этой подписке
			if err := tenantDB.Where("contract_id = ? AND subscription_id = ? AND status = ?", *sub.ContractID, sub.ID, "active").
				Find(&contractObjects).Error; err == nil && len(contractObjects) > 0 {
				objectIDs := make([]uint, 0, len(contractObjects))
				objects := make([]map[string]interface{}, 0, len(contractObjects))

				for _, co := range contractObjects {
					objectIDs = append(objectIDs, co.ObjectID)

					// Получаем название объекта из карты
					key := ObjectKey{ObjectID: co.ObjectID, CompanyID: co.ObjectCompanyID}
					name := objectNamesMap[key]
					if name == "" {
						name = fmt.Sprintf("Объект #%d", co.ObjectID)
					}

					objects = append(objects, map[string]interface{}{
						"id":         co.ObjectID,
						"name":       name,
						"company_id": co.ObjectCompanyID,
					})
				}

				subMap["object_ids"] = objectIDs
				subMap["objects_count"] = len(objectIDs)
				subMap["objects"] = objects
				log.Printf("✅ Добавлено %d объектов для подписки %d", len(objectIDs), sub.ID)
			} else {
				subMap["object_ids"] = []uint{}
				subMap["objects_count"] = 0
				subMap["objects"] = []map[string]interface{}{}
			}
		} else {
			subMap["object_ids"] = []uint{}
			subMap["objects_count"] = 0
			subMap["objects"] = []map[string]interface{}{}
		}

		subscriptionsResponse = append(subscriptionsResponse, subMap)
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   subscriptionsResponse,
		"count":  len(subscriptionsResponse),
	})
}

// CreateSubscriptionData представляет данные для создания подписки
type CreateSubscriptionData struct {
	CompanyID                  uint   `json:"company_id" binding:"required"`
	BillingPlanID              uint   `json:"billing_plan_id" binding:"required"`
	StartDate                  string `json:"start_date"`
	StartTime                  string `json:"start_time"` // Время начала (HH:MM)
	EndDate                    string `json:"end_date"`
	Status                     string `json:"status"`
	IsAutoRenew                bool   `json:"is_auto_renew"`
	PaymentMethod              string `json:"payment_method"`
	ContractID                 *uint  `json:"contract_id"`
	SplitPeriod                bool   `json:"split_period"`
	TransferFromSubscriptionID *uint  `json:"transfer_from_subscription_id"`
	AccountID                  *uint  `json:"account_id"`             // Учетная запись (опционально)
	ObjectIDs                  []uint `json:"object_ids"`             // Список ID объектов (опционально)
	ContractPeriodMonths       *int   `json:"contract_period_months"` // Период подписки в месяцах (опционально)
}

// recalculateContractTotalAmount пересчитывает total_amount договора на основе всех его активных подписок
func recalculateContractTotalAmount(tenantDB, publicDB *gorm.DB, contractID uint, adminAccountID uint) error {
	// Загружаем все активные подписки для договора
	var subscriptions []models.Subscription
	if err := publicDB.
		Where("contract_id = ? AND admin_account_id = ? AND deleted_at IS NULL AND status NOT IN (?, ?)", 
			contractID, adminAccountID, "cancelled", "expired").
		Find(&subscriptions).Error; err != nil {
		return fmt.Errorf("ошибка загрузки подписок: %w", err)
	}

	if len(subscriptions) == 0 {
		log.Printf("ℹ️ Нет активных подписок для договора %d, сумма не пересчитывается", contractID)
		return nil
	}

	// Загружаем договор для получения периода
	var contract models.Contract
	if err := tenantDB.First(&contract, contractID).Error; err != nil {
		return fmt.Errorf("ошибка загрузки договора: %w", err)
	}

	// Рассчитываем количество месяцев в договоре
	months := 1
	if contract.StartDate != nil && contract.EndDate != nil {
		duration := contract.EndDate.Sub(*contract.StartDate)
		days := int(duration.Hours() / 24)
		if days > 0 {
			months = days / 30
			if months == 0 {
				months = 1
			}
		}
	}

	totalAmount := decimal.Zero
	log.Printf("📊 Расчет суммы договора %d (подписок: %d, месяцев: %d):", contractID, len(subscriptions), months)

	// Суммируем стоимость каждой подписки
	for _, subscription := range subscriptions {
		// Загружаем тарифный план подписки
		var billingPlan models.BillingPlan
		if err := publicDB.First(&billingPlan, subscription.BillingPlanID).Error; err != nil {
			log.Printf("⚠️ Ошибка загрузки тарифного плана %d: %v", subscription.BillingPlanID, err)
			continue
		}

		// Подсчитываем количество объектов в этой подписке
		var objectsCount int64
		if err := tenantDB.Model(&models.ContractObject{}).
			Where("contract_id = ? AND subscription_id = ? AND status = ?", contractID, subscription.ID, "active").
			Count(&objectsCount).Error; err != nil {
			log.Printf("⚠️ Ошибка подсчета объектов для подписки %d: %v", subscription.ID, err)
			continue
		}

		// Рассчитываем стоимость подписки
		var subscriptionAmount decimal.Decimal
		if billingPlan.BillingPeriod == "yearly" {
			// Годовой тариф: (цена / 12) × количество месяцев договора × количество объектов
			// Делим годовую стоимость на 12 и умножаем на фактический период договора
			pricePerMonth := billingPlan.Price.Div(decimal.NewFromInt(12))
			subscriptionAmount = pricePerMonth.
				Mul(decimal.NewFromInt(int64(months))).
				Mul(decimal.NewFromInt(int64(objectsCount)))
			
			log.Printf("  - Подписка #%d (%s, yearly): %d объектов × (%s / 12) × %d мес = %s",
				subscription.ID,
				billingPlan.Name,
				objectsCount,
				billingPlan.Price.String(),
				months,
				subscriptionAmount.String())
		} else {
			// Месячный тариф с пролонгацией: цена × количество объектов
			// Каждый месяц будет выставляться новый счет
			subscriptionAmount = billingPlan.Price.Mul(decimal.NewFromInt(int64(objectsCount)))
			
			log.Printf("  - Подписка #%d (%s, monthly): %d объектов × %s = %s (ежемесячно)",
				subscription.ID,
				billingPlan.Name,
				objectsCount,
				billingPlan.Price.String(),
				subscriptionAmount.String())
		}

		totalAmount = totalAmount.Add(subscriptionAmount)
	}

	// Обновляем total_amount договора
	contract.TotalAmount = totalAmount
	if err := tenantDB.Save(&contract).Error; err != nil {
		return fmt.Errorf("ошибка обновления total_amount: %w", err)
	}

	log.Printf("✅ Обновлена сумма договора %d: %s ₽", contractID, totalAmount.String())
	return nil
}

// CreateSubscription создает новую подписку
func CreateSubscription(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	var data CreateSubscriptionData

	if err := c.ShouldBindJSON(&data); err != nil {
		log.Printf("❌ Ошибка биндинга JSON данных: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Неверный формат данных: %v", err),
		})
		return
	}

	log.Printf("📥 CreateSubscription: получены данные: company_id=%d, billing_plan_id=%d, contract_id=%v, account_id=%v, object_ids=%v, contract_period_months=%v",
		data.CompanyID, data.BillingPlanID, data.ContractID, data.AccountID, data.ObjectIDs, data.ContractPeriodMonths)

	// Валидация обязательных полей
	if data.CompanyID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Поле company_id обязательно",
		})
		return
	}

	if data.BillingPlanID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Поле billing_plan_id обязательно",
		})
		return
	}

	// Проверяем существование тарифного плана (billing_plans тоже в схеме public)
	var plan models.BillingPlan
	planDB := database.DB.Session(&gorm.Session{})
	if err := planDB.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("CreateSubscription: ошибка установки search_path для проверки плана: %v\n", err)
	}
	if err := planDB.Where("id = ? AND admin_account_id = ?", data.BillingPlanID, adminAccountID).
		First(&plan).Error; err != nil {
		log.Printf("❌ Тарифный план не найден: billing_plan_id=%d, admin_account_id=%d, ошибка: %v",
			data.BillingPlanID, adminAccountID, err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Тарифный план не найден",
		})
		return
	}

	// Проверяем права на тарифы по договору, если указан contract_id
	if data.ContractID != nil {
		// Получаем tenant DB для работы с договорами
		tenantDB := middleware.GetTenantDB(c)
		if tenantDB == nil {
			log.Printf("⚠️ Не удалось получить tenant DB из контекста, используем основную БД")
			tenantDB = database.DB
		}

		var contract models.Contract
		if err := tenantDB.Where("id = ? AND admin_account_id = ?", *data.ContractID, adminAccountID).
			First(&contract).Error; err != nil {
			log.Printf("❌ Договор не найден: contract_id=%d, admin_account_id=%d, ошибка: %v", *data.ContractID, adminAccountID, err)
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Договор не найден",
			})
			return
		}

		log.Printf("✅ Договор найден: id=%d, number=%s, tariff_plan_id=%v", contract.ID, contract.Number, contract.TariffPlanID)

		// Если у договора нет tariff_plan_id, устанавливаем его из billing_plan_id
		if contract.TariffPlanID == nil || *contract.TariffPlanID == 0 {
			log.Printf("📝 У договора нет tariff_plan_id, устанавливаем из billing_plan_id=%d", data.BillingPlanID)
			contract.TariffPlanID = &data.BillingPlanID
			if err := tenantDB.Save(&contract).Error; err != nil {
				log.Printf("❌ Ошибка сохранения tariff_plan_id в договоре: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{
					"status": "error",
					"error":  "Ошибка обновления договора",
				})
				return
			}
			log.Printf("✅ tariff_plan_id установлен в договоре id=%d", contract.ID)
		}
	}

	// Обрабатываем перенос из существующей подписки
	if data.TransferFromSubscriptionID != nil {
		var oldSubscription models.Subscription
		if err := database.DB.Where("id = ? AND admin_account_id = ?", *data.TransferFromSubscriptionID, adminAccountID).
			First(&oldSubscription).Error; err == nil {
			// Отменяем старую подписку
			oldSubscription.Status = "cancelled"
			database.DB.Save(&oldSubscription)
		}
	}

	// Парсим дату начала
	var startDate time.Time
	if data.StartDate != "" {
		parsedDate, err := time.Parse("2006-01-02", data.StartDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Неверный формат даты начала",
			})
			return
		}
		startDate = parsedDate

		// Если указано время начала, добавляем его
		if data.StartTime != "" {
			// Парсим время в формате HH:MM
			timeParts := strings.Split(data.StartTime, ":")
			if len(timeParts) == 2 {
				hour, errH := strconv.Atoi(timeParts[0])
				minute, errM := strconv.Atoi(timeParts[1])
				if errH == nil && errM == nil && hour >= 0 && hour < 24 && minute >= 0 && minute < 60 {
					// Устанавливаем время
					startDate = time.Date(startDate.Year(), startDate.Month(), startDate.Day(), hour, minute, 0, 0, startDate.Location())
				}
			}
		}
	} else {
		startDate = time.Now()
	}

	// Определяем статус: если дата/время начала в будущем, устанавливаем "scheduled"
	// НО если статус явно передан как 'active' (например, "Запустить немедленно"), не перезаписываем его
	status := data.Status
	now := time.Now()

	if startDate.After(now) && status != "active" {
		// Если дата в будущем И статус НЕ установлен явно как 'active', делаем 'scheduled'
		status = "scheduled"
		log.Printf("📅 Дата/время начала в будущем (%v > %v), статус = scheduled", startDate, now)
	} else if status == "" {
		status = "active"
		log.Printf("✅ Дата/время начала не в будущем (%v <= %v) или статус явно установлен, статус = active", startDate, now)
	} else if status == "active" {
		log.Printf("✅ Статус явно установлен как 'active' (запуск немедленно), сохраняем его")
	}

	// Обработка разбиения периода (если указано)
	endDate := (*time.Time)(nil)
	if data.EndDate != "" {
		parsedEndDate, err := time.Parse("2006-01-02", data.EndDate)
		if err == nil {
			endDate = &parsedEndDate
		}
	}

	// Если указан contract_period_months, вычисляем end_date на основе периода
	if data.ContractPeriodMonths != nil && *data.ContractPeriodMonths > 0 && endDate == nil {
		calculatedEndDate := startDate.AddDate(0, *data.ContractPeriodMonths, 0)
		endDate = &calculatedEndDate
		log.Printf("📅 Вычислена дата окончания на основе contract_period_months=%d: %v", *data.ContractPeriodMonths, endDate)
	}

	// Если разбит период для месячного тарифа, устанавливаем конец месяца
	if data.SplitPeriod && plan.BillingPeriod == "monthly" {
		// Конец месяца для даты начала
		lastDayOfMonth := time.Date(startDate.Year(), startDate.Month()+1, 0, 23, 59, 59, 0, startDate.Location())
		endDate = &lastDayOfMonth
		log.Printf("📅 Установлена дата окончания (конец месяца) для split_period: %v", endDate)
	}

	// Устанавливаем значение по умолчанию для PaymentMethod, если оно пустое
	paymentMethod := data.PaymentMethod
	if paymentMethod == "" {
		paymentMethod = ""
	}

	// Создаем подписку
	subscription := models.Subscription{
		AdminAccountID: adminAccountID,
		CompanyID:      data.CompanyID,
		BillingPlanID:  data.BillingPlanID,
		ContractID:     data.ContractID, // Сохраняем contract_id
		StartDate:      startDate,
		EndDate:        endDate,
		Status:         status,
		IsAutoRenew:    data.IsAutoRenew,
		PaymentMethod:  paymentMethod,
	}

	// Вычисляем дату следующего платежа
	if plan.BillingPeriod != "one-time" {
		var nextPayment time.Time
		switch plan.BillingPeriod {
		case "monthly":
			if endDate != nil {
				// Если период разбит, следующая оплата после конца текущего периода
				nextPayment = (*endDate).AddDate(0, 0, 1)
			} else {
				nextPayment = startDate.AddDate(0, 1, 0)
			}
		case "yearly":
			nextPayment = startDate.AddDate(1, 0, 0)
		default:
			nextPayment = startDate.AddDate(0, 1, 0)
		}
		subscription.NextPaymentDate = &nextPayment
		log.Printf("📅 Вычислена дата следующего платежа: %v (период: %s)", nextPayment, plan.BillingPeriod)
	}

	log.Printf("📝 Создаем подписку: company_id=%d, billing_plan_id=%d, contract_id=%v, start_date=%v, end_date=%v, status=%s, is_auto_renew=%v, payment_method=%s",
		data.CompanyID, data.BillingPlanID, data.ContractID, startDate, endDate, status, data.IsAutoRenew, data.PaymentMethod)
	log.Printf("📝 Данные подписки: AdminAccountID=%d, CompanyID=%d, BillingPlanID=%d, ContractID=%v, StartDate=%v, EndDate=%v, Status=%s, IsAutoRenew=%v, PaymentMethod=%s, NextPaymentDate=%v",
		subscription.AdminAccountID, subscription.CompanyID, subscription.BillingPlanID, subscription.ContractID, subscription.StartDate, subscription.EndDate, subscription.Status, subscription.IsAutoRenew, subscription.PaymentMethod, subscription.NextPaymentDate)

	// Работаем в схеме public, где хранится таблица subscriptions
	db := database.DB.Session(&gorm.Session{})
	if err := db.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("CreateSubscription: ошибка установки search_path: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подключения к базе данных",
		})
		return
	}

	if err := db.Create(&subscription).Error; err != nil {
		log.Printf("❌ Ошибка создания подписки в БД: %v", err)
		log.Printf("❌ Детали ошибки: SQL State: %v, Error: %v", err, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Ошибка при создании подписки: %v", err),
		})
		return
	}

	log.Printf("✅ Подписка создана: id=%d", subscription.ID)

	// Если подписка создана для договора, автоматически переводим его в статус "active"
	// обновляем период из подписки и привязываем объекты, если они указаны
	if data.ContractID != nil && *data.ContractID > 0 {
		// Получаем tenant DB для работы с договорами
		tenantDB := middleware.GetTenantDB(c)
		if tenantDB == nil {
			log.Printf("⚠️ Не удалось получить tenant DB для обновления договора")
		} else {
			var contract models.Contract
			// Проверяем, что договор существует и принадлежит этому admin_account
			if err := tenantDB.Where("id = ? AND admin_account_id = ?", *data.ContractID, adminAccountID).
				First(&contract).Error; err == nil {
				// Обновляем период договора из подписки (всегда, если подписка создана для договора)
				contractUpdated := false
				// Всегда обновляем start_date из подписки
				if contract.StartDate == nil || !contract.StartDate.Equal(startDate) {
					contract.StartDate = &startDate
					contractUpdated = true
					log.Printf("📅 Установлен start_date договора %d из подписки: %v", contract.ID, startDate)
				}
				// Обновляем end_date из подписки, если он указан
				if endDate != nil {
					if contract.EndDate == nil || !contract.EndDate.Equal(*endDate) {
						contract.EndDate = endDate
						contractUpdated = true
						log.Printf("📅 Установлен end_date договора %d из подписки: %v", contract.ID, endDate)
					}
				}

				// Если договор в статусе "draft" (черновик), переводим в "active" (активный)
				if contract.Status == "draft" {
					contract.Status = "active"
					contractUpdated = true
				}

				// Сохраняем изменения договора, если были обновления
				if contractUpdated {
					if err := tenantDB.Save(&contract).Error; err != nil {
						log.Printf("⚠️ Не удалось обновить договор %d: %v", contract.ID, err)
					} else {
						if contract.Status == "active" {
							log.Printf("✅ Договор %d (№%s) автоматически переведен в статус 'active' после создания подписки",
								contract.ID, contract.Number)
						}
						if contract.StartDate != nil || contract.EndDate != nil {
							log.Printf("✅ Период договора %d обновлен из подписки: start_date=%v, end_date=%v",
								contract.ID, contract.StartDate, contract.EndDate)
						}
					}
				}

				// Привязываем объекты к договору, если они указаны
				log.Printf("🔍 Проверка привязки объектов: len(data.ObjectIDs)=%d, data.ObjectIDs=%v", len(data.ObjectIDs), data.ObjectIDs)
				if len(data.ObjectIDs) > 0 {
					log.Printf("🔗 Привязываем объекты %v к договору %d через подписку", data.ObjectIDs, contract.ID)

					// Убеждаемся, что таблица contract_objects существует
					if err := ensureContractObjectsTable(tenantDB); err != nil {
						log.Printf("⚠️ Не удалось создать таблицу contract_objects: %v", err)
					} else {
						// Определяем targetAccountID для привязки объектов
						var targetAccountID uint
						if data.AccountID != nil && *data.AccountID > 0 {
							targetAccountID = *data.AccountID
							log.Printf("ℹ️ AccountID %d указан для привязки объектов из Axenta Cloud", targetAccountID)
						} else {
							// Если account_id не указан, используем company_id договора (fallback)
							targetAccountID = contract.CompanyID
							log.Printf("⚠️ AccountID не указан, используем CompanyID договора: %d", targetAccountID)
						}

						// Получаем информацию о компании для определения схемы
						var company models.Company
						publicDB := database.DB.Session(&gorm.Session{})
						if err := publicDB.Exec("SET search_path TO public").Error; err != nil {
							log.Printf("⚠️ Не удалось переключиться на схему public: %v", err)
						}

						if err := publicDB.First(&company, targetAccountID).Error; err != nil {
							log.Printf("⚠️ Компания с ID %d не найдена: %v", targetAccountID, err)
						}

						// Получаем токен из заголовка Authorization для запроса к Axenta Cloud API
						authHeader := c.GetHeader("Authorization")
						var userToken string
						if authHeader != "" {
							if strings.HasPrefix(authHeader, "Token ") {
								userToken = strings.TrimPrefix(authHeader, "Token ")
							} else if strings.HasPrefix(authHeader, "Bearer ") {
								userToken = strings.TrimPrefix(authHeader, "Bearer ")
							} else {
								userToken = authHeader
							}
						}

						// Получаем объекты из Axenta Cloud API для проверки их существования (если токен есть)
						var axentaObjects []axentaCloudObjectForContract
						if userToken != "" {
							axentaObjects, err = fetchObjectsFromAxentaCloud(userToken, int(targetAccountID), data.ObjectIDs)
							if err != nil {
								log.Printf("⚠️ Ошибка получения объектов из Axenta Cloud API для account_id %d: %v", targetAccountID, err)
								log.Printf("ℹ️ Продолжаем привязку объектов без проверки через API")
							} else {
								log.Printf("✅ Получено %d объектов из Axenta Cloud API для account_id %d", len(axentaObjects), targetAccountID)
							}
						}

						// Используем даты из подписки для привязки объектов
						objStartDate := startDate
						var objEndDate *time.Time = endDate

						// Если у договора есть даты, используем их
						if contract.StartDate != nil {
							objStartDate = *contract.StartDate
						}
						if contract.EndDate != nil {
							objEndDate = contract.EndDate
						}

						attachedCount := int64(0)
						var objectErrors []string

						// Создаем записи в junction table для каждого объекта
						for _, objectID := range data.ObjectIDs {
							// Проверяем, существует ли объект в Axenta Cloud (если удалось получить список)
							if len(axentaObjects) > 0 {
								objectExists := false
								for _, axentaObj := range axentaObjects {
									if axentaObj.ID == int(objectID) {
										objectExists = true
										break
									}
								}
								if !objectExists {
									log.Printf("⚠️ Объект %d не найден в Axenta Cloud для account_id %d, пропускаем", objectID, targetAccountID)
									objectErrors = append(objectErrors, fmt.Sprintf("Объект %d не найден в Axenta Cloud", objectID))
									continue
								}
							}

							// Проверяем, не существует ли уже такая связь с этим договором
							var existingSameContract models.ContractObject
							if err := tenantDB.Where("contract_id = ? AND object_id = ? AND object_company_id = ?",
								contract.ID, objectID, targetAccountID).First(&existingSameContract).Error; err == nil {
								// Объект уже привязан к договору - обновляем subscription_id
								existingSameContract.SubscriptionID = &subscription.ID
								existingSameContract.StartDate = objStartDate
								existingSameContract.EndDate = objEndDate
								existingSameContract.Status = "active"
								
								if err := tenantDB.Save(&existingSameContract).Error; err != nil {
									log.Printf("⚠️ Ошибка обновления связи для объекта %d: %v", objectID, err)
									objectErrors = append(objectErrors, fmt.Sprintf("Ошибка обновления объекта %d: %v", objectID, err))
								} else {
									attachedCount++
									log.Printf("✅ Обновлена связь: договор %d <-> объект %d (subscription_id=%d, account_id %d)", 
										contract.ID, objectID, subscription.ID, targetAccountID)
								}
								continue
							}

							// Создаем связь в junction table
							contractObject := models.ContractObject{
								ContractID:      contract.ID,
								SubscriptionID:  &subscription.ID, // Привязываем объект к конкретной подписке
								ObjectID:        objectID,
								ObjectCompanyID: targetAccountID,
								ObjectSchema:    company.DatabaseSchema,
								Status:          "active",
								StartDate:       objStartDate,
								EndDate:         objEndDate,
							}

							if err := tenantDB.Create(&contractObject).Error; err != nil {
								log.Printf("⚠️ Ошибка создания связи для объекта %d: %v", objectID, err)
								objectErrors = append(objectErrors, fmt.Sprintf("Ошибка привязки объекта %d: %v", objectID, err))
							} else {
								attachedCount++
								log.Printf("✅ Создана связь: договор %d <-> объект %d (subscription_id=%d, account_id %d, схема %s)", 
									contract.ID, objectID, subscription.ID, targetAccountID, company.DatabaseSchema)
							}
						}

						// Логируем результаты привязки
						if len(objectErrors) > 0 {
							log.Printf("⚠️ При привязке объектов к договору %d возникло %d ошибок", contract.ID, len(objectErrors))
						}
						if attachedCount != int64(len(data.ObjectIDs)) {
							log.Printf("⚠️ Не все объекты привязаны: ожидалось %d, создано %d", len(data.ObjectIDs), attachedCount)
						} else {
							log.Printf("✅ Привязано %d объектов к договору %d через подписку", attachedCount, contract.ID)
						}

					// Пересчитываем сумму договора на основе ВСЕХ подписок
					if attachedCount > 0 {
						if err := recalculateContractTotalAmount(tenantDB, publicDB, contract.ID, adminAccountID); err != nil {
							log.Printf("⚠️ Ошибка пересчета total_amount договора %d: %v", contract.ID, err)
						}
					}
					}
				} else {
					log.Printf("ℹ️ Объекты не указаны в подписке, пропускаем привязку")
				}
			} else {
				log.Printf("⚠️ Не удалось найти договор %d для обновления статуса: %v", *data.ContractID, err)
			}
		}
	} else if len(data.ObjectIDs) > 0 {
		log.Printf("⚠️ Объекты указаны (%v), но contract_id не указан. Объекты не могут быть привязаны без договора", data.ObjectIDs)
	}

	// Загружаем связанные данные для ответа (используем ту же сессию с установленным search_path)
	// Убеждаемся, что search_path установлен перед Preload
	if err := db.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️ Ошибка установки search_path перед Preload: %v", err)
	}
	if err := db.Preload("BillingPlan", "admin_account_id = ?", adminAccountID).
		Where("id = ? AND admin_account_id = ?", subscription.ID, adminAccountID).
		First(&subscription).Error; err != nil {
		log.Printf("⚠️ Ошибка загрузки подписки с BillingPlan: %v", err)
		// Не возвращаем ошибку, так как подписка уже создана
	}

	c.JSON(http.StatusCreated, gin.H{
		"status": "success",
		"data":   subscription,
	})
}

// UpdateSubscription обновляет статус подписки
func UpdateSubscription(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	id := c.Param("id")

	var subscription models.Subscription
	if err := database.DB.Where("id = ? AND admin_account_id = ?", id, adminAccountID).First(&subscription).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Подписка не найдена",
		})
		return
	}

	var updateData models.Subscription
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных",
		})
		return
	}

	// Сохраняем старый статус
	oldStatus := subscription.Status

	if err := database.DB.Model(&subscription).Updates(updateData).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при обновлении подписки",
		})
		return
	}

	// Если статус изменился на cancelled или expired, обнуляем subscription_id у объектов
	if updateData.Status != "" && updateData.Status != oldStatus && 
		(updateData.Status == "cancelled" || updateData.Status == "expired") {
		
		tenantDB := middleware.GetTenantDB(c)
		if tenantDB != nil && subscription.ContractID != nil {
			result := tenantDB.Model(&models.ContractObject{}).
				Where("contract_id = ? AND subscription_id = ?", *subscription.ContractID, subscription.ID).
				Update("subscription_id", nil)
			
			if result.Error != nil {
				log.Printf("⚠️ Ошибка обнуления subscription_id у объектов подписки %d: %v", subscription.ID, result.Error)
			} else if result.RowsAffected > 0 {
				log.Printf("✅ Обнулен subscription_id у %d объектов подписки %d (статус изменен на %s)", 
					result.RowsAffected, subscription.ID, updateData.Status)
			}
		}
	}

	// Пересчитываем сумму договора после обновления подписки
	if subscription.ContractID != nil {
		tenantDB := middleware.GetTenantDB(c)
		if tenantDB != nil {
			if err := recalculateContractTotalAmount(tenantDB, database.DB, *subscription.ContractID, adminAccountID); err != nil {
				log.Printf("⚠️ Ошибка пересчета total_amount договора %d после обновления подписки: %v", *subscription.ContractID, err)
			}
		}
	}

	// Загружаем обновленные данные с связями
	database.DB.Preload("BillingPlan", "admin_account_id = ?", adminAccountID).
		Where("id = ? AND admin_account_id = ?", subscription.ID, adminAccountID).
		First(&subscription)

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   subscription,
	})
}

// DeleteSubscription удаляет подписку
func DeleteSubscription(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	id := c.Param("id")
	subscriptionID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат ID подписки",
		})
		return
	}

	var subscription models.Subscription
	if err := database.DB.Where("id = ? AND admin_account_id = ?", id, adminAccountID).First(&subscription).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Подписка не найдена",
		})
		return
	}

	// Сохраняем contractID для пересчета после удаления
	var contractID *uint
	if subscription.ContractID != nil {
		contractID = subscription.ContractID
	}

	// Получаем tenant DB для работы с объектами договора
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB != nil && subscription.ContractID != nil {
		// Обнуляем subscription_id у всех объектов этой подписки
		result := tenantDB.Model(&models.ContractObject{}).
			Where("contract_id = ? AND subscription_id = ?", *subscription.ContractID, uint(subscriptionID)).
			Update("subscription_id", nil)
		
		if result.Error != nil {
			log.Printf("⚠️ Ошибка обнуления subscription_id у объектов подписки %d: %v", subscriptionID, result.Error)
		} else if result.RowsAffected > 0 {
			log.Printf("✅ Обнулен subscription_id у %d объектов подписки %d", result.RowsAffected, subscriptionID)
		}
	}

	if err := database.DB.Delete(&subscription).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при удалении подписки",
		})
		return
	}

	log.Printf("✅ Подписка %d успешно удалена", subscriptionID)

	// Пересчитываем сумму договора после удаления подписки
	if contractID != nil && tenantDB != nil {
		if err := recalculateContractTotalAmount(tenantDB, database.DB, *contractID, adminAccountID); err != nil {
			log.Printf("⚠️ Ошибка пересчета total_amount договора %d после удаления подписки: %v", *contractID, err)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Подписка успешно удалена",
	})
}

// ===== НОВЫЕ ENDPOINTS ДЛЯ СИСТЕМЫ БИЛЛИНГА =====

// CalculateBilling рассчитывает стоимость биллинга для договора
func CalculateBilling(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	contractIDStr := c.Param("contract_id")
	contractID, err := strconv.ParseUint(contractIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат contract_id",
		})
		return
	}

	// Получаем параметры периода
	periodStartStr := c.Query("period_start")
	periodEndStr := c.Query("period_end")

	var periodStart, periodEnd time.Time

	if periodStartStr != "" && periodEndStr != "" {
		periodStart, err = time.Parse("2006-01-02", periodStartStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Неверный формат period_start (ожидается YYYY-MM-DD)",
			})
			return
		}

		periodEnd, err = time.Parse("2006-01-02", periodEndStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Неверный формат period_end (ожидается YYYY-MM-DD)",
			})
			return
		}
	} else {
		// По умолчанию текущий месяц
		now := time.Now()
		periodStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		periodEnd = periodStart.AddDate(0, 1, -1)
	}

	// Создаем сервис биллинга
	billingService := services.NewBillingService(adminAccountID)

	// Рассчитываем биллинг
	calculation, err := billingService.CalculateBillingForContract(uint(contractID), periodStart, periodEnd)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   calculation,
	})
}

// GenerateInvoice создает счет для договора
func GenerateInvoice(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	contractIDStr := c.Param("contract_id")
	contractID, err := strconv.ParseUint(contractIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат contract_id",
		})
		return
	}

	log.Printf("📥 GenerateInvoice: contract_id=%d, admin_account_id=%d", contractID, adminAccountID)

	// Получаем параметры из тела запроса
	var requestData struct {
		PeriodStart string `json:"period_start"`
		PeriodEnd   string `json:"period_end"`
	}

	if err := c.ShouldBindJSON(&requestData); err != nil {
		log.Printf("❌ Ошибка парсинга JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных",
		})
		return
	}

	log.Printf("📅 Получены даты: period_start=%s, period_end=%s", requestData.PeriodStart, requestData.PeriodEnd)

	var periodStart, periodEnd time.Time

	if requestData.PeriodStart != "" && requestData.PeriodEnd != "" {
		periodStart, err = time.Parse("2006-01-02", requestData.PeriodStart)
		if err != nil {
			log.Printf("❌ Ошибка парсинга period_start: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Неверный формат period_start (ожидается YYYY-MM-DD)",
			})
			return
		}

		periodEnd, err = time.Parse("2006-01-02", requestData.PeriodEnd)
		if err != nil {
			log.Printf("❌ Ошибка парсинга period_end: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Неверный формат period_end (ожидается YYYY-MM-DD)",
			})
			return
		}
	} else {
		// По умолчанию текущий месяц
		now := time.Now()
		periodStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		periodEnd = periodStart.AddDate(0, 1, -1)
		log.Printf("⏰ Используются даты по умолчанию: %s - %s", periodStart.Format("2006-01-02"), periodEnd.Format("2006-01-02"))
	}

	log.Printf("✅ Период биллинга: %s - %s", periodStart.Format("2006-01-02"), periodEnd.Format("2006-01-02"))

	// Получаем tenant DB для работы с договорами
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		log.Printf("⚠️ Не удалось получить tenant DB из контекста, используем основную БД")
		tenantDB = database.DB
	}

	// Создаем сервис биллинга
	billingService := services.NewBillingService(adminAccountID)

	// Генерируем счет
	log.Printf("🔄 Генерация счета для договора %d...", contractID)
	invoice, err := billingService.GenerateInvoiceForContractWithTenantDB(uint(contractID), periodStart, periodEnd, tenantDB)
	if err != nil {
		log.Printf("❌ Ошибка генерации счета: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	log.Printf("✅ Счет успешно создан: %s", invoice.Number)

	c.JSON(http.StatusCreated, gin.H{
		"status": "success",
		"data":   invoice,
	})
}

// GetInvoices получает список счетов
func GetInvoices(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	// Проверяем demo-режим
	if isDemoMode(c) {
		demoInvoices := []models.Invoice{
			{
				ID:             1,
				Number:         "INV-2024-001",
				Title:          "Счет за услуги мониторинга",
				InvoiceDate:    time.Now().AddDate(0, 0, -5),
				DueDate:        time.Now().AddDate(0, 0, 9),
				SubtotalAmount: decimal.NewFromInt(10000),
				TaxAmount:      decimal.NewFromInt(2000),
				TotalAmount:    decimal.NewFromInt(12000),
				PaidAmount:     decimal.Zero,
				Currency:       "RUB",
				Status:         "sent",
			},
		}

		c.JSON(http.StatusOK, gin.H{
			"status":      "success",
			"data":        demoInvoices,
			"count":       len(demoInvoices),
			"total":       1,
			"demo_notice": "Это демо-данные. Добавьте ?demo=0 для получения реальных данных.",
		})
		return
	}

	companyIDStr := c.Query("company_id")
	contractIDStr := c.Query("contract_id")
	status := c.Query("status")

	// Пагинация
	limitStr := c.DefaultQuery("limit", "50")
	offsetStr := c.DefaultQuery("offset", "0")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 50
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	// Убеждаемся, что мы в схеме public для глобальных таблиц
	if err := database.DB.Exec("SET search_path TO public").Error; err != nil {
		fmt.Printf("GetInvoices: ошибка установки search_path: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Ошибка подключения к базе данных: %v", err),
		})
		return
	}

	// Базовый запрос - без Preload сначала для фильтрации
	query := database.DB.Model(&models.Invoice{}).
		Where("admin_account_id = ?", adminAccountID)

	if companyIDStr != "" {
		companyID, parseErr := strconv.ParseUint(companyIDStr, 10, 32)
		if parseErr != nil {
			fmt.Printf("GetInvoices: ошибка парсинга company_id: %v\n", parseErr)
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  fmt.Sprintf("Неверный формат company_id: %v", parseErr),
			})
			return
		}
		fmt.Printf("GetInvoices: фильтруем по company_id=%d\n", uint(companyID))
		query = query.Where("company_id = ?", uint(companyID))
	}

	if contractIDStr != "" {
		contractID, err := strconv.ParseUint(contractIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Неверный формат contract_id",
			})
			return
		}
		query = query.Where("contract_id = ?", uint(contractID))
	}

	if status != "" {
		query = query.Where("status = ?", status)
	}

	// Подсчитываем общее количество (без Preload)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		fmt.Printf("GetInvoices: ОШИБКА при подсчете счетов: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Ошибка при подсчете счетов: %v", err),
		})
		return
	}

	fmt.Printf("GetInvoices: найдено счетов: %d\n", total)

	// Добавляем Preload для Items
	queryWithPreload := query.
		Preload("Items")

	// Получаем счета с пагинацией
	var invoices []models.Invoice
	if err := queryWithPreload.Limit(limit).Offset(offset).Order("created_at DESC").Find(&invoices).Error; err != nil {
		fmt.Printf("GetInvoices: ОШИБКА при получении счетов: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Ошибка при получении счетов: %v", err),
		})
		return
	}

	fmt.Printf("GetInvoices: успешно получено %d счетов\n", len(invoices))

	// Загружаем Contract для каждого счета отдельно из тенантной схемы
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB != nil {
		for i := range invoices {
			if invoices[i].ContractID != nil && *invoices[i].ContractID > 0 {
				var contract models.Contract
				err := tenantDB.Where("id = ?", *invoices[i].ContractID).
					Select("id, number, client_name, client_short_name, client_email").
					First(&contract).Error
				if err == nil {
					invoices[i].Contract = &contract
				} else {
					fmt.Printf("GetInvoices: ошибка загрузки договора для счета %d: %v\n", invoices[i].ID, err)
				}
			}
		}
	} else {
		fmt.Printf("GetInvoices: не удалось получить тенантную БД\n")
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   invoices,
		"count":  len(invoices),
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetInvoice получает конкретный счет по ID
func GetInvoice(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	id := c.Param("id")

	var invoice models.Invoice
	if err := database.DB.
		Preload("Contract", "admin_account_id = ?", adminAccountID).
		Preload("TariffPlan", "admin_account_id = ?", adminAccountID).
		Preload("Items").
		Where("id = ? AND admin_account_id = ?", id, adminAccountID).
		First(&invoice).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Счет не найден",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   invoice,
	})
}

// SendInvoice отправляет счет клиенту (POST /api/invoices/:id/send согласно roadmap)
func SendInvoice(c *gin.Context) {
	log.Printf("🔔 SendInvoice вызвана! Path: %s, Method: %s", c.Request.URL.Path, c.Request.Method)
	
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		log.Printf("❌ SendInvoice: ошибка получения adminAccountID: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}
	
	log.Printf("✅ SendInvoice: adminAccountID = %d", adminAccountID)

	id := c.Param("id")
	log.Printf("📋 SendInvoice: invoice ID from param = %s", id)
	invoiceID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат ID счета",
		})
		return
	}

	// Получаем параметры отправки из тела запроса
	var requestData struct {
		Channels    []string          `json:"channels"`     // Список каналов: email, telegram, max
		ContactInfo map[string]string `json:"contact_info"` // Контактная информация для каждого канала
	}

	if err := c.ShouldBindJSON(&requestData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных",
		})
		return
	}

	// Проверяем, что указан хотя бы один канал
	if len(requestData.Channels) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Не указан ни один канал отправки",
		})
		return
	}

	// Получаем счет из схемы public
	log.Printf("🔍 Ищем счет ID=%d для adminAccountID=%d", invoiceID, adminAccountID)
	
	var invoice models.Invoice
	// Используем прямой запрос к таблице public.invoices
	if err := database.DB.Table("public.invoices").
		Where("id = ? AND admin_account_id = ?", invoiceID, adminAccountID).
		First(&invoice).Error; err != nil {
		log.Printf("❌ Счет не найден: %v", err)
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Счет не найден",
		})
		return
	}
	
	// Загружаем связанные данные отдельно из схемы public
	if invoice.ContractID != nil && *invoice.ContractID > 0 {
		database.DB.Table("public.contracts").Where("id = ?", *invoice.ContractID).First(&invoice.Contract)
	}
	if invoice.TariffPlanID > 0 {
		database.DB.Table("public.billing_plans").Where("id = ?", invoice.TariffPlanID).First(&invoice.TariffPlan)
	}
	
	log.Printf("✅ Счет найден: ID=%d, Сумма=%s, Статус=%s", invoice.ID, invoice.TotalAmount, invoice.Status)

	// Проверяем, можно ли отправить счет
	if invoice.Status == "paid" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Нельзя отправить уже оплаченный счет",
		})
		return
	}

	if invoice.Status == "cancelled" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Нельзя отправить отмененный счет",
		})
		return
	}

	// Получаем контактную информацию из договора, если она не указана в запросе
	if requestData.ContactInfo == nil {
		requestData.ContactInfo = make(map[string]string)
	}

	// Дополняем контактную информацию из договора
	if invoice.Contract != nil {
		if _, ok := requestData.ContactInfo["email"]; !ok && invoice.Contract.ClientEmail != "" {
			requestData.ContactInfo["email"] = invoice.Contract.ClientEmail
		}
		// Telegram и MAX ID можно получить из связанного пользователя, если есть
	}

	// Создаем сервис отправки счетов
	senderService := services.NewInvoiceSenderService(database.DB)

	// Отправляем счет через выбранные каналы
	err = senderService.SendInvoiceToClient(&invoice, requestData.Channels, requestData.ContactInfo)

	if err != nil {
		// Даже если есть ошибка, проверяем, были ли успешные отправки
		if invoice.LastSentAt != nil {
			// Частичная отправка - возвращаем предупреждение
			c.JSON(http.StatusOK, gin.H{
				"status":        "partial_success",
				"message":       fmt.Sprintf("Счет отправлен частично: %v", err),
				"data":          invoice,
				"sent_channels": invoice.LastSentChannels,
			})
			return
		}

		// Проверяем, является ли это ошибкой конфигурации
		errorMsg := err.Error()
		if strings.Contains(errorMsg, "не настроена") || strings.Contains(errorMsg, "не настроен") ||
			strings.Contains(errorMsg, "отключены") || strings.Contains(errorMsg, "отключен") {
			// Ошибка конфигурации - возвращаем 400 Bad Request
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  errorMsg,
			})
			return
		}

		// Другая ошибка отправки - возвращаем 500
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Ошибка отправки счета: %v", err),
		})
		return
	}

	// Успешная отправка
	c.JSON(http.StatusOK, gin.H{
		"status":        "success",
		"message":       "Счет успешно отправлен",
		"data":          invoice,
		"sent_channels": invoice.LastSentChannels,
	})
}

// ProcessPayment обрабатывает платеж по счету (POST /api/invoices/:id/pay согласно roadmap)
func ProcessPayment(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	id := c.Param("id")
	invoiceID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат ID счета",
		})
		return
	}

	var paymentData struct {
		Amount        string `json:"amount" binding:"required"`
		PaymentMethod string `json:"payment_method" binding:"required"`
		Notes         string `json:"notes"`
	}

	if err := c.ShouldBindJSON(&paymentData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных",
		})
		return
	}

	// Парсим сумму
	amount, err := decimal.NewFromString(paymentData.Amount)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат суммы",
		})
		return
	}

	if amount.LessThanOrEqual(decimal.Zero) {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Сумма должна быть больше нуля",
		})
		return
	}

	// Создаем сервис биллинга
	billingService := services.NewBillingService(adminAccountID)

	// Обрабатываем платеж
	if err := billingService.ProcessPayment(uint(invoiceID), amount, paymentData.PaymentMethod, paymentData.Notes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	// Получаем обновленный счет
	var invoice models.Invoice
	if err := database.DB.
		Preload("Contract", "admin_account_id = ?", adminAccountID).
		Preload("TariffPlan", "admin_account_id = ?", adminAccountID).
		Preload("Items").
		Where("id = ? AND admin_account_id = ?", invoiceID, adminAccountID).
		First(&invoice).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка получения обновленного счета",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Платеж успешно обработан",
		"data":    invoice,
	})
}

// CancelInvoice отменяет счет
func CancelInvoice(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	id := c.Param("id")
	invoiceID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат ID счета",
		})
		return
	}

	var cancelData struct {
		Reason string `json:"reason" binding:"required"`
	}

	if err := c.ShouldBindJSON(&cancelData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных",
		})
		return
	}

	// Создаем сервис биллинга
	billingService := services.NewBillingService(adminAccountID)

	// Отменяем счет
	if err := billingService.CancelInvoice(uint(invoiceID), cancelData.Reason); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Счет успешно отменен",
	})
}

// DeleteInvoice удаляет счет
func DeleteInvoice(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	id := c.Param("id")
	invoiceID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат ID счета",
		})
		return
	}

	// Убеждаемся, что мы в схеме public для глобальных таблиц
	if err := database.DB.Exec("SET search_path TO public").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подключения к базе данных",
		})
		return
	}

	// Получаем счет
	var invoice models.Invoice
	if err := database.DB.Where("id = ? AND admin_account_id = ?", invoiceID, adminAccountID).First(&invoice).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"status": "error",
				"error":  "Счет не найден",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при получении счета",
		})
		return
	}

	// Проверяем, можно ли удалить счет
	if invoice.Status == "paid" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Нельзя удалить оплаченный счет",
		})
		return
	}

	// Удаляем сначала все позиции счета
	if err := database.DB.Where("invoice_id = ?", invoiceID).Delete(&models.InvoiceItem{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при удалении позиций счета",
		})
		return
	}

	// Удаляем сам счет
	if err := database.DB.Delete(&invoice).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при удалении счета",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Счет успешно удален",
	})
}

// GetBillingHistory получает историю биллинга
func GetBillingHistory(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	companyIDStr := c.Query("company_id")
	if companyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Параметр company_id обязателен",
		})
		return
	}

	companyID, err := uuid.Parse(companyIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат company_id",
		})
		return
	}

	// Пагинация
	limitStr := c.DefaultQuery("limit", "50")
	offsetStr := c.DefaultQuery("offset", "0")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 50
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	// Создаем сервис биллинга
	billingService := services.NewBillingService(adminAccountID)

	// Получаем историю
	history, total, err := billingService.GetBillingHistory(companyID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   history,
		"count":  len(history),
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetOverdueInvoices получает просроченные счета
func GetOverdueInvoices(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	companyIDStr := c.Query("company_id")
	var companyID *uint

	if companyIDStr != "" {
		cIDUint, err := strconv.ParseUint(companyIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Неверный формат company_id",
			})
			return
		}
		cID := uint(cIDUint)
		companyID = &cID
	}

	// Создаем сервис биллинга
	billingService := services.NewBillingService(adminAccountID)

	// Получаем просроченные счета
	invoices, err := billingService.GetOverdueInvoices(companyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   invoices,
		"count":  len(invoices),
	})
}

// GetBillingSettings получает настройки биллинга для компании
func GetBillingSettings(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		fmt.Printf("GetBillingSettings: не удалось определить admin_account_id: %v\n", err)
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	companyIDStr := c.Query("company_id")
	if companyIDStr == "" {
		fmt.Printf("GetBillingSettings: company_id не указан\n")
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Параметр company_id обязателен",
		})
		return
	}

	companyIDUint, err := strconv.ParseUint(companyIDStr, 10, 32)
	if err != nil {
		fmt.Printf("GetBillingSettings: ошибка парсинга company_id: %v\n", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Неверный формат company_id: %v", err),
		})
		return
	}
	companyID := uint(companyIDUint)
	fmt.Printf("GetBillingSettings: запрос для admin_account_id=%d, company_id=%d\n", adminAccountID, companyID)

	// Убеждаемся, что мы в схеме public для глобальных таблиц
	db := database.DB.Session(&gorm.Session{})

	if err := db.Exec("SET search_path TO public").Error; err != nil {
		fmt.Printf("GetBillingSettings: ошибка установки search_path: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Ошибка подключения к базе данных: %v", err),
		})
		return
	}

	var settings models.BillingSettings

	if err := db.Where("company_id = ? AND admin_account_id = ?", companyID, adminAccountID).First(&settings).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			fmt.Printf("GetBillingSettings: ОШИБКА при загрузке настроек (admin_account_id=%d, company_id=%d): %v\n", adminAccountID, companyID, err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  fmt.Sprintf("Ошибка получения настроек биллинга: %v", err),
			})
			return
		}

		// Настройки для данной пары admin/company не найдены. Проверяем, есть ли настройки для компании без учета admin_account_id
		var companySettings models.BillingSettings
		if err := db.Where("company_id = ?", companyID).First(&companySettings).Error; err == nil {
			// Нашли настройки компании, привязываем их к текущему администратору (если требуется)
			if companySettings.AdminAccountID != adminAccountID {
				fmt.Printf("GetBillingSettings: найден существующий набор настроек для company_id=%d с другим admin_account_id=%d, обновляем на %d\n",
					companyID, companySettings.AdminAccountID, adminAccountID)
				companySettings.AdminAccountID = adminAccountID
				if saveErr := db.Save(&companySettings).Error; saveErr != nil {
					fmt.Printf("GetBillingSettings: ОШИБКА при обновлении admin_account_id для company_id=%d: %v\n", companyID, saveErr)
					c.JSON(http.StatusInternalServerError, gin.H{
						"status": "error",
						"error":  fmt.Sprintf("Ошибка обновления настроек биллинга: %v", saveErr),
					})
					return
				}
			}
			settings = companySettings
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
		// Нет настроек даже на уровне компании — создаем с дефолтными значениями
		// Получаем валюту из настроек компании
		var company models.Company
		companyCurrency := "RUB" // По умолчанию
		if err := db.Where("id = ?", companyID).First(&company).Error; err == nil {
			companyCurrency = company.Currency
			fmt.Printf("GetBillingSettings: используем валюту компании: %s\n", companyCurrency)
		}

		settings = models.BillingSettings{
			AdminAccountID:             adminAccountID,
			CompanyID:                  companyID,
			AutoGenerateInvoices:       true,
			InvoiceGenerationDay:       1,
			InvoicePaymentTermDays:     14,
			DefaultTaxRate:             decimal.NewFromFloat(20),
			TaxIncluded:                false,
			VATRatePreset:              "russia",
			VATRateCustom:              decimal.NewFromFloat(20),
			NotifyBeforeInvoice:        3,
			NotifyBeforeDue:            3,
			NotifyOverdue:              1,
			InvoiceNumberPrefix:        "INV",
			InvoiceNumberFormat:        "%s-%04d",
			Currency:                   companyCurrency,
			AllowPartialPayments:       true,
			RequirePaymentConfirm:      false,
			EnableInactiveDiscounts:    true,
			InactiveDiscountRatio:      decimal.NewFromFloat(0.5),
			ContractNumberingMethod:    "manual",
			ContractDefaultNumeratorID: nil,
			Bitrix24DealNumberField:    "",
			AutopilotEnabled:           true, // Автопилот включен по умолчанию
		}

			fmt.Printf("GetBillingSettings: настройки не найдены, создаем по умолчанию для admin_account_id=%d, company_id=%d\n", adminAccountID, companyID)
			if err := db.Create(&settings).Error; err != nil {
				if isUniqueConstraintError(err) {
					// Параллельно создали настройки или существует запись с другим admin_account_id
					if err := db.Where("company_id = ?", companyID).First(&companySettings).Error; err == nil {
						if companySettings.AdminAccountID != adminAccountID {
							fmt.Printf("GetBillingSettings: дубликат настроек для company_id=%d, обновляем admin_account_id с %d на %d\n",
								companyID, companySettings.AdminAccountID, adminAccountID)
							companySettings.AdminAccountID = adminAccountID
							if saveErr := db.Save(&companySettings).Error; saveErr != nil {
								fmt.Printf("GetBillingSettings: ОШИБКА при обновлении admin_account_id после дубликата для company_id=%d: %v\n", companyID, saveErr)
								c.JSON(http.StatusInternalServerError, gin.H{
									"status": "error",
									"error":  fmt.Sprintf("Ошибка обновления настроек биллинга: %v", saveErr),
								})
								return
							}
						}
						settings = companySettings
					} else {
						fmt.Printf("GetBillingSettings: ОШИБКА при поиске существующих настроек после дубликата (company_id=%d): %v\n", companyID, err)
						c.JSON(http.StatusInternalServerError, gin.H{
							"status": "error",
							"error":  fmt.Sprintf("Ошибка создания настроек биллинга: %v", err),
						})
						return
					}
				} else {
					fmt.Printf("GetBillingSettings: ОШИБКА при создании настроек: %v\n", err)
					c.JSON(http.StatusInternalServerError, gin.H{
						"status": "error",
						"error":  fmt.Sprintf("Ошибка создания настроек биллинга: %v", err),
					})
					return
				}
			} else {
				fmt.Printf("GetBillingSettings: настройки успешно созданы для company_id=%d\n", companyID)
			}
		} else {
			fmt.Printf("GetBillingSettings: ОШИБКА при поиске настроек компании (company_id=%d): %v\n", companyID, err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  fmt.Sprintf("Ошибка получения настроек биллинга: %v", err),
			})
			return
		}
	}

	// Вычисляем DefaultTaxRate на основе пресета
	switch settings.VATRatePreset {
	case "russia":
		settings.DefaultTaxRate = decimal.NewFromFloat(20)
	case "kazakhstan":
		settings.DefaultTaxRate = decimal.NewFromFloat(12)
	case "none":
		settings.DefaultTaxRate = decimal.NewFromFloat(0)
	case "custom":
		settings.DefaultTaxRate = settings.VATRateCustom
	default:
		// Если пресет не установлен или неизвестен, используем существующий DefaultTaxRate или 20%
		if settings.VATRatePreset == "" {
			settings.VATRatePreset = "russia"
			settings.DefaultTaxRate = decimal.NewFromFloat(20)
		}
	}

	fmt.Printf("GetBillingSettings: возвращаем настройки для admin_account_id=%d, company_id=%d\n", adminAccountID, companyID)
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   settings,
	})
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()
	return strings.Contains(errMsg, "duplicate key") ||
		strings.Contains(errMsg, "UNIQUE constraint") ||
		strings.Contains(errMsg, "unique constraint") ||
		strings.Contains(errMsg, "already exists")
}

// UpdateBillingSettings обновляет настройки биллинга
func UpdateBillingSettings(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	companyIDStr := c.Query("company_id")
	if companyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Параметр company_id обязателен",
		})
		return
	}

	companyID, err := strconv.ParseUint(companyIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат company_id",
		})
		return
	}

	// Устанавливаем search_path для работы с глобальными таблицами
	db := database.DB.Session(&gorm.Session{})
	if err := db.Exec("SET search_path TO public").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подключения к базе данных",
		})
		return
	}

	var settings models.BillingSettings
	// Пытаемся найти настройки для данной пары admin/company
	err = db.Where("company_id = ? AND admin_account_id = ?", uint(companyID), adminAccountID).First(&settings).Error
	
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Ошибка получения настроек биллинга",
			})
			return
		}

		// Настройки не найдены - пытаемся найти по company_id без учета admin_account_id
		var companySettings models.BillingSettings
		if err := db.Where("company_id = ?", uint(companyID)).First(&companySettings).Error; err == nil {
			// Нашли настройки компании, обновляем admin_account_id
			companySettings.AdminAccountID = adminAccountID
			if saveErr := db.Save(&companySettings).Error; saveErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"status": "error",
					"error":  "Ошибка обновления настроек биллинга",
				})
				return
			}
			settings = companySettings
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
		// Нет настроек вообще - создаем дефолтные
		// Получаем валюту из настроек компании
		var company models.Company
		companyCurrency := "RUB" // По умолчанию
		if err := db.Where("id = ?", uint(companyID)).First(&company).Error; err == nil {
			companyCurrency = company.Currency
			fmt.Printf("UpdateBillingSettings: используем валюту компании: %s\n", companyCurrency)
		}

		settings = models.BillingSettings{
			AdminAccountID:             adminAccountID,
			CompanyID:                  uint(companyID),
			AutoGenerateInvoices:       true,
			InvoiceGenerationDay:       1,
			InvoicePaymentTermDays:     14,
			DefaultTaxRate:             decimal.NewFromFloat(20),
			TaxIncluded:                false,
			VATRatePreset:              "russia",
			VATRateCustom:              decimal.NewFromFloat(20),
			NotifyBeforeInvoice:        3,
			NotifyBeforeDue:            3,
			NotifyOverdue:              1,
			InvoiceNumberPrefix:        "INV",
			InvoiceNumberFormat:        "%s-%04d",
			Currency:                   companyCurrency,
			AllowPartialPayments:       true,
			RequirePaymentConfirm:      false,
			EnableInactiveDiscounts:    true,
			InactiveDiscountRatio:      decimal.NewFromFloat(0.5),
			ContractNumberingMethod:    "manual",
			ContractDefaultNumeratorID: nil,
			Bitrix24DealNumberField:    "",
			AutopilotEnabled:           true, // Автопилот включен по умолчанию
		}

			if err := db.Create(&settings).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"status": "error",
					"error":  "Ошибка создания настроек биллинга",
				})
				return
			}
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Ошибка поиска настроек биллинга",
			})
			return
		}
	}

	var updateData models.BillingSettings
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных",
		})
		return
	}

	// Логируем полученные данные для отладки
	fmt.Printf("UpdateBillingSettings: получены данные обновления для company_id=%d:\n", uint(companyID))
	fmt.Printf("  - contract_numbering_method: '%s'\n", updateData.ContractNumberingMethod)
	fmt.Printf("  - autopilot_enabled: %v\n", updateData.AutopilotEnabled)
	fmt.Printf("  - invoice_generation_day: %d\n", updateData.InvoiceGenerationDay)
	fmt.Printf("  - invoice_payment_term_days: %d\n", updateData.InvoicePaymentTermDays)

	// Валидация (пропускаем, если значения по умолчанию 0)
	if updateData.InvoiceGenerationDay != 0 && (updateData.InvoiceGenerationDay < 1 || updateData.InvoiceGenerationDay > 28) {
		fmt.Printf("❌ Ошибка валидации: InvoiceGenerationDay=%d должен быть от 1 до 28\n", updateData.InvoiceGenerationDay)
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "День генерации счета должен быть от 1 до 28",
		})
		return
	}

	if updateData.InvoicePaymentTermDays != 0 && updateData.InvoicePaymentTermDays < 1 {
		fmt.Printf("❌ Ошибка валидации: InvoicePaymentTermDays=%d должен быть больше 0\n", updateData.InvoicePaymentTermDays)
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Срок оплаты должен быть больше 0 дней",
		})
		return
	}

	// Вычисляем DefaultTaxRate на основе пресета перед обновлением
	if updateData.VATRatePreset != "" {
		switch updateData.VATRatePreset {
		case "russia":
			updateData.DefaultTaxRate = decimal.NewFromFloat(20)
		case "kazakhstan":
			updateData.DefaultTaxRate = decimal.NewFromFloat(12)
		case "none":
			updateData.DefaultTaxRate = decimal.NewFromFloat(0)
		case "custom":
			updateData.DefaultTaxRate = updateData.VATRateCustom
		}
	}

	updateData.AdminAccountID = 0

	// ВАЖНО: Updates() пропускает zero values (включая false для bool).
	// Используем Select() для явного указания полей, которые нужно обновить
	fmt.Printf("🔄 Обновляем настройки для settings.ID=%d\n", settings.ID)
	if err := db.Model(&settings).Select("*").Updates(updateData).Error; err != nil {
		fmt.Printf("❌ ОШИБКА при обновлении настроек: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Ошибка при обновлении настроек биллинга: %v", err),
		})
		return
	}
	fmt.Printf("✅ Настройки успешно обновлены\n")

	// Перезагружаем обновленные настройки из БД
	fmt.Printf("🔄 Перезагружаем настройки из БД...\n")
	if err := db.Where("id = ?", settings.ID).First(&settings).Error; err != nil {
		fmt.Printf("❌ ОШИБКА при получении обновленных настроек: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Ошибка получения обновленных настроек: %v", err),
		})
		return
	}
	fmt.Printf("✅ Настройки успешно перезагружены\n")

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   settings,
	})
}

// ===== ДОПОЛНИТЕЛЬНЫЕ ENDPOINTS ДЛЯ АВТОМАТИЗАЦИИ БИЛЛИНГА =====

// RunInvoicesGeneration запускает генерацию счетов (POST /api/invoices/run согласно roadmap)
func RunInvoicesGeneration(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	// Получаем параметры из запроса
	var requestData struct {
		CompanyID uint   `json:"company_id"`
		Year      int    `json:"year"`
		Month     int    `json:"month"`
		Period    string `json:"period"` // Формат YYYY-MM, альтернатива year+month
	}

	if err := c.ShouldBindJSON(&requestData); err != nil {
		// Если JSON не передан, пробуем получить из query параметров
		if companyIDStr := c.Query("company_id"); companyIDStr != "" {
			if id, err := strconv.ParseUint(companyIDStr, 10, 32); err == nil {
				requestData.CompanyID = uint(id)
			}
		}
		if periodStr := c.Query("period"); periodStr != "" {
			requestData.Period = periodStr
		}
		if yearStr := c.Query("year"); yearStr != "" {
			if y, err := strconv.Atoi(yearStr); err == nil {
				requestData.Year = y
			}
		}
		if monthStr := c.Query("month"); monthStr != "" {
			if m, err := strconv.Atoi(monthStr); err == nil {
				requestData.Month = m
			}
		}
	}

	// Определяем год и месяц
	var year, month int
	if requestData.Period != "" {
		// Парсим период в формате YYYY-MM
		periodTime, err := time.Parse("2006-01", requestData.Period)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Неверный формат периода (ожидается YYYY-MM)",
			})
			return
		}
		year = periodTime.Year()
		month = int(periodTime.Month())
	} else {
		if requestData.Year == 0 {
			year = time.Now().Year()
		} else {
			year = requestData.Year
		}
		if requestData.Month == 0 {
			month = int(time.Now().Month())
		} else {
			month = requestData.Month
		}
	}

	// Создаем сервис автоматизации биллинга
	automationService := services.NewBillingAutomationService(adminAccountID)

	// Генерируем счета
	if err := automationService.AutoGenerateInvoicesForMonth(year, month); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": fmt.Sprintf("Счета за %d-%02d успешно сгенерированы", year, month),
		"year":    year,
		"month":   month,
	})
}

// AutoGenerateInvoices автоматически генерирует счета за месяц
func AutoGenerateInvoices(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	yearStr := c.Query("year")
	monthStr := c.Query("month")

	if yearStr == "" || monthStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Параметры year и month обязательны",
		})
		return
	}

	year, err := strconv.Atoi(yearStr)
	if err != nil || year < 2000 || year > 3000 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат года",
		})
		return
	}

	month, err := strconv.Atoi(monthStr)
	if err != nil || month < 1 || month > 12 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат месяца (1-12)",
		})
		return
	}

	// Создаем сервис автоматизации биллинга
	automationService := services.NewBillingAutomationService(adminAccountID)

	// Генерируем счета
	if err := automationService.AutoGenerateInvoicesForMonth(year, month); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": fmt.Sprintf("Счета за %d-%02d успешно сгенерированы", year, month),
	})
}

// ProcessScheduledDeletions обрабатывает плановые удаления объектов
func ProcessScheduledDeletions(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	// Создаем сервис автоматизации биллинга
	automationService := services.NewBillingAutomationService(adminAccountID)

	// Обрабатываем плановые удаления
	if err := automationService.ProcessScheduledDeletions(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Плановые удаления успешно обработаны",
	})
}

// ActivateScheduledSubscriptions активирует запланированные подписки
func ActivateScheduledSubscriptions(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	// Создаем сервис автоматизации биллинга
	automationService := services.NewBillingAutomationService(adminAccountID)

	// Активируем запланированные подписки
	if err := automationService.ActivateScheduledSubscriptions(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Запланированные подписки успешно активированы",
	})
}

// AutoRenewMonthlyContracts автоматически продлевает месячные договоры
// Продлевает договоры с тарифным планом billing_period = "monthly" и статусом "active"
// Продлевает end_date на +1 месяц, если end_date уже наступил или близок
func AutoRenewMonthlyContracts(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	// Создаем сервис автоматизации биллинга
	automationService := services.NewBillingAutomationService(adminAccountID)

	// Автоматически продлеваем месячные договоры
	if err := automationService.AutoRenewMonthlyContracts(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Автоматическая пролонгация месячных договоров выполнена",
	})
}

// GetBillingStatistics получает статистику биллинга
func GetBillingStatistics(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	companyIDStr := c.Query("company_id")
	if companyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Параметр company_id обязателен",
		})
		return
	}

	companyID, err := strconv.ParseUint(companyIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат company_id",
		})
		return
	}

	yearStr := c.Query("year")
	monthStr := c.Query("month")

	if yearStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Параметр year обязателен",
		})
		return
	}

	year, err := strconv.Atoi(yearStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат года",
		})
		return
	}

	var month *int
	if monthStr != "" {
		m, err := strconv.Atoi(monthStr)
		if err != nil || m < 1 || m > 12 {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Неверный формат месяца (1-12)",
			})
			return
		}
		month = &m
	}

	// Создаем сервис автоматизации биллинга
	automationService := services.NewBillingAutomationService(adminAccountID)

	// Получаем статистику
	stats, err := automationService.GetBillingStatistics(uint(companyID), year, month)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   stats,
	})
}

// GetInvoicesByPeriod получает счета за период
func GetInvoicesByPeriod(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")
	companyIDStr := c.Query("company_id")

	if startDateStr == "" || endDateStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Параметры start_date и end_date обязательны",
		})
		return
	}

	startDate, err := time.Parse("2006-01-02", startDateStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат start_date (ожидается YYYY-MM-DD)",
		})
		return
	}

	endDate, err := time.Parse("2006-01-02", endDateStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат end_date (ожидается YYYY-MM-DD)",
		})
		return
	}

	var companyID *uint
	if companyIDStr != "" {
		cID, err := strconv.ParseUint(companyIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Неверный формат company_id",
			})
			return
		}
		companyIDUint := uint(cID)
		companyID = &companyIDUint
	}

	// Создаем сервис автоматизации биллинга
	automationService := services.NewBillingAutomationService(adminAccountID)

	// Получаем счета за период
	invoices, err := automationService.GetInvoicesByPeriod(companyID, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   invoices,
		"count":  len(invoices),
	})
}

// GetInvoiceNumerators получает список нумераторов счетов
func GetInvoiceNumerators(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	companyIDStr := c.Query("company_id")
	if companyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Параметр company_id обязателен",
		})
		return
	}

	companyID, err := strconv.ParseUint(companyIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат company_id",
		})
		return
	}

	// Убеждаемся, что мы в схеме public для глобальных таблиц
	if err := database.DB.Exec("SET search_path TO public").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подключения к базе данных",
		})
		return
	}

	var numerators []models.InvoiceNumerator
	if err := database.DB.
		Where("admin_account_id = ? AND company_id = ?", adminAccountID, uint(companyID)).
		Order("is_default DESC, name ASC").
		Find(&numerators).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при получении нумераторов",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   numerators,
		"count":  len(numerators),
	})
}

// CreateInvoiceNumerator создает новый нумератор счетов
func CreateInvoiceNumerator(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	var input struct {
		CompanyID   uint   `json:"company_id" binding:"required"`
		Name        string `json:"name" binding:"required"`
		Prefix      string `json:"prefix" binding:"required"`
		Template    string `json:"template" binding:"required"`
		Description string `json:"description"`
		IsDefault   bool   `json:"is_default"`
		IsActive    bool   `json:"is_active"`
		AutoReset   bool   `json:"auto_reset"`
		ResetPeriod string `json:"reset_period"`
		Notes       string `json:"notes"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	// Убеждаемся, что мы в схеме public
	if err := database.DB.Exec("SET search_path TO public").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подключения к базе данных",
		})
		return
	}

	// Если новый нумератор должен быть по умолчанию, снимаем флаг с других
	if input.IsDefault {
		database.DB.Model(&models.InvoiceNumerator{}).
			Where("admin_account_id = ? AND company_id = ? AND is_default = ?", adminAccountID, input.CompanyID, true).
			Update("is_default", false)
	}

	numerator := models.InvoiceNumerator{
		AdminAccountID: adminAccountID,
		CompanyID:      input.CompanyID,
		Name:           input.Name,
		Prefix:         input.Prefix,
		Template:       input.Template,
		Description:    input.Description,
		IsDefault:      input.IsDefault,
		IsActive:       input.IsActive,
		AutoReset:      input.AutoReset,
		ResetPeriod:    input.ResetPeriod,
		Notes:          input.Notes,
		CounterValue:   0,
	}

	if err := database.DB.Create(&numerator).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при создании нумератора",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   numerator,
	})
}

// UpdateInvoiceNumerator обновляет нумератор счетов
func UpdateInvoiceNumerator(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	id := c.Param("id")
	numeratorID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат ID",
		})
		return
	}

	var input struct {
		Name        *string `json:"name"`
		Prefix      *string `json:"prefix"`
		Template    *string `json:"template"`
		Description *string `json:"description"`
		IsDefault   *bool   `json:"is_default"`
		IsActive    *bool   `json:"is_active"`
		AutoReset   *bool   `json:"auto_reset"`
		ResetPeriod *string `json:"reset_period"`
		Notes       *string `json:"notes"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	// Убеждаемся, что мы в схеме public
	if err := database.DB.Exec("SET search_path TO public").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подключения к базе данных",
		})
		return
	}

	var numerator models.InvoiceNumerator
	if err := database.DB.Where("id = ? AND admin_account_id = ?", uint(numeratorID), adminAccountID).First(&numerator).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Нумератор не найден",
		})
		return
	}

	// Если нумератор устанавливается как по умолчанию, снимаем флаг с других
	if input.IsDefault != nil && *input.IsDefault {
		database.DB.Model(&models.InvoiceNumerator{}).
			Where("admin_account_id = ? AND company_id = ? AND id != ? AND is_default = ?",
				adminAccountID, numerator.CompanyID, uint(numeratorID), true).
			Update("is_default", false)
	}

	// Обновляем поля
	updates := make(map[string]interface{})
	if input.Name != nil {
		updates["name"] = *input.Name
	}
	if input.Prefix != nil {
		updates["prefix"] = *input.Prefix
	}
	if input.Template != nil {
		updates["template"] = *input.Template
	}
	if input.Description != nil {
		updates["description"] = *input.Description
	}
	if input.IsDefault != nil {
		updates["is_default"] = *input.IsDefault
	}
	if input.IsActive != nil {
		updates["is_active"] = *input.IsActive
	}
	if input.AutoReset != nil {
		updates["auto_reset"] = *input.AutoReset
	}
	if input.ResetPeriod != nil {
		updates["reset_period"] = *input.ResetPeriod
	}
	if input.Notes != nil {
		updates["notes"] = *input.Notes
	}

	if err := database.DB.Model(&numerator).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при обновлении нумератора",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   numerator,
	})
}

// DeleteInvoiceNumerator удаляет нумератор счетов
func DeleteInvoiceNumerator(c *gin.Context) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	id := c.Param("id")
	numeratorID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат ID",
		})
		return
	}

	// Убеждаемся, что мы в схеме public
	if err := database.DB.Exec("SET search_path TO public").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подключения к базе данных",
		})
		return
	}

	var numerator models.InvoiceNumerator
	if err := database.DB.Where("id = ? AND admin_account_id = ?", uint(numeratorID), adminAccountID).First(&numerator).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Нумератор не найден",
		})
		return
	}

	if err := database.DB.Delete(&numerator).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при удалении нумератора",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Нумератор успешно удален",
	})
}
