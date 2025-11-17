package api

import (
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
			fmt.Printf("GetBillingPlans: ошибка парсинга company_id: %v\n", err)
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

	// Логируем входящие данные для отладки
	fmt.Printf("CreateBillingPlan: получен запрос, query params: %v\n", c.Request.URL.Query())

	if err := c.ShouldBindJSON(&plan); err != nil {
		fmt.Printf("CreateBillingPlan: ошибка парсинга JSON: %v\n", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Неверный формат данных: %v", err),
		})
		return
	}

	fmt.Printf("CreateBillingPlan: распарсенные данные плана: Name=%s, Price=%s, CompanyID=%v\n",
		plan.Name, plan.Price.String(), plan.CompanyID)

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
		fmt.Printf("CreateBillingPlan: company_id из query: %s\n", companyIDStr)
		if companyIDStr != "" {
			if companyID, parseErr := strconv.ParseUint(companyIDStr, 10, 32); parseErr == nil {
				companyIDUint := uint(companyID)
				plan.CompanyID = &companyIDUint
				fmt.Printf("CreateBillingPlan: установлен company_id: %d\n", companyIDUint)
			} else {
				fmt.Printf("CreateBillingPlan: ошибка парсинга company_id: %v\n", parseErr)
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
		fmt.Printf("CreateBillingPlan: ошибка установки search_path: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Ошибка подключения к базе данных: %v", err),
		})
		return
	}

	fmt.Printf("CreateBillingPlan: создаем план с данными: Name=%s, Price=%s, CompanyID=%v\n",
		plan.Name, plan.Price.String(), plan.CompanyID)

	// Проверяем, нет ли уже плана с таким же именем
	// ВАЖНО: Уникальный индекс idx_billing_plans_name в БД действует глобально по имени,
	// поэтому проверяем ВСЕ планы с таким именем, независимо от company_id
	var existingPlan models.BillingPlan
	// Сначала проверяем планы для текущей компании
	if plan.CompanyID != nil {
		// Проверяем активные планы для этой компании
		if err := database.DB.Where("name = ? AND company_id = ? AND admin_account_id = ? AND is_active = ?", plan.Name, *plan.CompanyID, adminAccountID, true).First(&existingPlan).Error; err == nil {
			fmt.Printf("CreateBillingPlan: активный план с именем '%s' уже существует для компании %d (ID: %d)\n", plan.Name, *plan.CompanyID, existingPlan.ID)
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  fmt.Sprintf("Тарифный план с названием '%s' уже существует для вашей компании", plan.Name),
			})
			return
		}
		// Проверяем неактивные планы для этой компании
		if err := database.DB.Where("name = ? AND company_id = ? AND admin_account_id = ? AND is_active = ?", plan.Name, *plan.CompanyID, adminAccountID, false).First(&existingPlan).Error; err == nil {
			fmt.Printf("CreateBillingPlan: найден неактивный план с именем '%s' для компании %d (ID: %d)\n",
				plan.Name, *plan.CompanyID, existingPlan.ID)
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
		// Найден план (активный или удаленный)
		fmt.Printf("CreateBillingPlan: ⚠️ найден план с именем '%s' глобально (ID: %d, company_id: %v, is_active: %v, deleted_at: %v)\n",
			plan.Name, deletedPlan.ID, deletedPlan.CompanyID, deletedPlan.IsActive, deletedPlan.DeletedAt)

		// Если план удален (мягкое удаление)
		if deletedPlan.DeletedAt.Valid {
			// Если удаленный план принадлежит той же компании, физически удаляем его
			if plan.CompanyID != nil && deletedPlan.CompanyID != nil && *deletedPlan.CompanyID == *plan.CompanyID {
				fmt.Printf("CreateBillingPlan: найден удаленный план '%s' для той же компании %d, физически удаляем его\n",
					plan.Name, *plan.CompanyID)
				// Физически удаляем старый план, чтобы освободить имя
				if err := database.DB.Unscoped().Delete(&deletedPlan).Error; err != nil {
					fmt.Printf("CreateBillingPlan: ⚠️ ошибка физического удаления старого плана: %v\n", err)
					// Продолжаем создание, возможно БД разрешит
				} else {
					fmt.Printf("CreateBillingPlan: ✅ старый удаленный план физически удален, можно создавать новый\n")
				}
			} else if deletedPlan.CompanyID == nil {
				// Удаленный план с company_id=NULL (глобальный план) - можно физически удалить, если создаем для конкретной компании
				if plan.CompanyID != nil {
					fmt.Printf("CreateBillingPlan: найден удаленный глобальный план '%s', физически удаляем его для создания плана компании %d\n",
						plan.Name, *plan.CompanyID)
					if err := database.DB.Unscoped().Delete(&deletedPlan).Error; err != nil {
						fmt.Printf("CreateBillingPlan: ⚠️ ошибка физического удаления старого глобального плана: %v\n", err)
					} else {
						fmt.Printf("CreateBillingPlan: ✅ старый удаленный глобальный план физически удален, можно создавать новый\n")
					}
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
		// Логируем детальную информацию об ошибке
		fmt.Printf("CreateBillingPlan: ОШИБКА при создании тарифного плана: %v\n", err)
		fmt.Printf("CreateBillingPlan: Данные плана: Name=%s, Price=%s, CompanyID=%v, Currency=%s, BillingPeriod=%s\n",
			plan.Name, plan.Price.String(), plan.CompanyID, plan.Currency, plan.BillingPeriod)

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

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   subscriptions,
		"count":  len(subscriptions),
	})
}

// CreateSubscriptionData представляет данные для создания подписки
type CreateSubscriptionData struct {
	CompanyID                  uint   `json:"company_id" binding:"required"`
	BillingPlanID              uint   `json:"billing_plan_id" binding:"required"`
	StartDate                  string `json:"start_date"`
	EndDate                    string `json:"end_date"`
	Status                     string `json:"status"`
	IsAutoRenew                bool   `json:"is_auto_renew"`
	PaymentMethod              string `json:"payment_method"`
	ContractID                 *uint  `json:"contract_id"`
	SplitPeriod                bool   `json:"split_period"`
	TransferFromSubscriptionID *uint  `json:"transfer_from_subscription_id"`
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
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных",
		})
		return
	}

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

	// Проверяем существование тарифного плана
	var plan models.BillingPlan
	if err := database.DB.Where("id = ? AND admin_account_id = ?", data.BillingPlanID, adminAccountID).
		First(&plan).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Тарифный план не найден",
		})
		return
	}

	// Проверяем права на тарифы по договору, если указан contract_id
	if data.ContractID != nil {
		var contract models.Contract
		if err := database.DB.Where("id = ? AND admin_account_id = ?", *data.ContractID, adminAccountID).
			First(&contract).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Договор не найден",
			})
			return
		}

		// Проверяем, что у договора есть доступ к тарифам (если tariff_plan_id = nil или 0, значит нет прав)
		if contract.TariffPlanID == nil || *contract.TariffPlanID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Нет прав на тарифы в этом договоре",
			})
			return
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
	} else {
		startDate = time.Now()
	}

	// Определяем статус: если дата начала в будущем, устанавливаем "scheduled"
	status := data.Status
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startOfDayStartDate := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())

	if startOfDayStartDate.After(startOfDay) {
		status = "scheduled"
	} else if status == "" {
		status = "active"
	}

	// Обработка разбиения периода (если указано)
	endDate := (*time.Time)(nil)
	if data.EndDate != "" {
		parsedEndDate, err := time.Parse("2006-01-02", data.EndDate)
		if err == nil {
			endDate = &parsedEndDate
		}
	}

	// Если разбит период для месячного тарифа, устанавливаем конец месяца
	if data.SplitPeriod && plan.BillingPeriod == "monthly" {
		// Конец месяца для даты начала
		lastDayOfMonth := time.Date(startDate.Year(), startDate.Month()+1, 0, 23, 59, 59, 0, startDate.Location())
		endDate = &lastDayOfMonth
	}

	// Создаем подписку
	subscription := models.Subscription{
		AdminAccountID: adminAccountID,
		CompanyID:      data.CompanyID,
		BillingPlanID:  data.BillingPlanID,
		StartDate:      startDate,
		EndDate:        endDate,
		Status:         status,
		IsAutoRenew:    data.IsAutoRenew,
		PaymentMethod:  data.PaymentMethod,
	}

	// Вычисляем дату следующего платежа
	if subscription.NextPaymentDate == nil && plan.BillingPeriod != "one-time" {
		var nextPayment time.Time
		switch plan.BillingPeriod {
		case "monthly":
			if endDate != nil {
				// Если период разбит, следующая оплата после конца текущего периода
				nextPayment = (*endDate).AddDate(0, 0, 1)
			} else {
				nextPayment = subscription.StartDate.AddDate(0, 1, 0)
			}
		case "yearly":
			nextPayment = subscription.StartDate.AddDate(1, 0, 0)
		default:
			nextPayment = subscription.StartDate.AddDate(0, 1, 0)
		}
		subscription.NextPaymentDate = &nextPayment
	}

	if err := database.DB.Create(&subscription).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при создании подписки",
		})
		return
	}

	// Если подписка создана для договора, автоматически переводим его в статус "active"
	if data.ContractID != nil && *data.ContractID > 0 {
		var contract models.Contract
		// Проверяем, что договор существует и принадлежит этому admin_account
		if err := database.DB.Where("id = ? AND admin_account_id = ?", *data.ContractID, adminAccountID).
			First(&contract).Error; err == nil {
			// Если договор в статусе "draft" (черновик), переводим в "active" (активный)
			if contract.Status == "draft" {
				contract.Status = "active"
				if err := database.DB.Save(&contract).Error; err != nil {
					log.Printf("⚠️ Не удалось обновить статус договора %d на 'active': %v", contract.ID, err)
				} else {
					log.Printf("✅ Договор %d (№%s) автоматически переведен в статус 'active' после создания подписки", 
						contract.ID, contract.Number)
				}
			}
		} else {
			log.Printf("⚠️ Не удалось найти договор %d для обновления статуса: %v", *data.ContractID, err)
		}
	}

	// Загружаем связанные данные для ответа
	database.DB.Preload("BillingPlan", "admin_account_id = ?", adminAccountID).
		Where("id = ? AND admin_account_id = ?", subscription.ID, adminAccountID).
		First(&subscription)

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

	if err := database.DB.Model(&subscription).Updates(updateData).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при обновлении подписки",
		})
		return
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

	var subscription models.Subscription
	if err := database.DB.Where("id = ? AND admin_account_id = ?", id, adminAccountID).First(&subscription).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Подписка не найдена",
		})
		return
	}

	if err := database.DB.Delete(&subscription).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при удалении подписки",
		})
		return
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

	// Получаем параметры из тела запроса
	var requestData struct {
		PeriodStart string `json:"period_start"`
		PeriodEnd   string `json:"period_end"`
	}

	if err := c.ShouldBindJSON(&requestData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных",
		})
		return
	}

	var periodStart, periodEnd time.Time

	if requestData.PeriodStart != "" && requestData.PeriodEnd != "" {
		periodStart, err = time.Parse("2006-01-02", requestData.PeriodStart)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Неверный формат period_start (ожидается YYYY-MM-DD)",
			})
			return
		}

		periodEnd, err = time.Parse("2006-01-02", requestData.PeriodEnd)
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

	// Генерируем счет
	invoice, err := billingService.GenerateInvoiceForContract(uint(contractID), periodStart, periodEnd)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

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

	// Добавляем Preload только для основных записей (без Contract, так как он в tenant схеме)
	// TariffPlan нужно загружать из billing_plans, но в модели Invoice используется TariffPlan
	// Попробуем загрузить только Items, а TariffPlan попробуем через Join или отдельно
	queryWithPreload := query.
		Preload("Items").
		Preload("Contract", "admin_account_id = ?", adminAccountID).
		Preload("TariffPlan", "admin_account_id = ?", adminAccountID)

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

	// Получаем счет
	var invoice models.Invoice
	if err := database.DB.Where("id = ? AND admin_account_id = ?", invoiceID, adminAccountID).First(&invoice).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Счет не найден",
		})
		return
	}

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

	// Обновляем статус на "sent"
	invoice.Status = "sent"
	if err := database.DB.Save(&invoice).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка обновления статуса счета",
		})
		return
	}

	// TODO: Здесь можно добавить отправку email/уведомления клиенту

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Счет успешно отправлен",
		"data":    invoice,
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
			settings = models.BillingSettings{
				AdminAccountID:             adminAccountID,
				CompanyID:                  companyID,
				AutoGenerateInvoices:       true,
				InvoiceGenerationDay:       1,
				InvoicePaymentTermDays:     14,
				DefaultTaxRate:             decimal.NewFromFloat(20),
				TaxIncluded:                false,
				NotifyBeforeInvoice:        3,
				NotifyBeforeDue:            3,
				NotifyOverdue:              1,
				InvoiceNumberPrefix:        "INV",
				InvoiceNumberFormat:        "%s-%04d",
				Currency:                   "RUB",
				AllowPartialPayments:       true,
				RequirePaymentConfirm:      false,
				EnableInactiveDiscounts:    true,
				InactiveDiscountRatio:      decimal.NewFromFloat(0.5),
				ContractNumberingMethod:    "manual",
				ContractDefaultNumeratorID: nil,
				Bitrix24DealNumberField:    "",
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

	var settings models.BillingSettings
	if err := database.DB.Where("company_id = ? AND admin_account_id = ?", uint(companyID), adminAccountID).First(&settings).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Настройки биллинга не найдены",
		})
		return
	}

	var updateData models.BillingSettings
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных",
		})
		return
	}

	// Валидация
	if updateData.InvoiceGenerationDay < 1 || updateData.InvoiceGenerationDay > 28 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "День генерации счета должен быть от 1 до 28",
		})
		return
	}

	if updateData.InvoicePaymentTermDays < 1 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Срок оплаты должен быть больше 0 дней",
		})
		return
	}

	updateData.AdminAccountID = 0

	if err := database.DB.Model(&settings).Updates(updateData).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при обновлении настроек биллинга",
		})
		return
	}

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
