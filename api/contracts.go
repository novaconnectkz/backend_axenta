package api

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// GetContracts получает список всех договоров
func GetContracts(c *gin.Context) {
	// Проверяем demo-режим
	if isDemoMode(c) {
		demoContracts := []models.Contract{
			{
				ID:        24,
				Number:    "DOG-2024-001",
				Title:     "Договор с ООО Логистика Плюс",
				ClientName: "ООО Логистика Плюс",
				StartDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				EndDate:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				Status:    "active",
				Currency:  "RUB",
			},
		}

		c.JSON(http.StatusOK, gin.H{
			"status":      "success",
			"data":        demoContracts,
			"count":       len(demoContracts),
			"total":       1,
			"demo_notice": "Это демо-данные. Добавьте ?demo=0 для получения реальных данных.",
		})
		return
	}

	var contracts []models.Contract

	// Базовый запрос для фильтрации (без Preload для подсчета)
	baseQuery := database.DB.Model(&models.Contract{})

	// Фильтрация по статусу
	if status := c.Query("status"); status != "" {
		baseQuery = baseQuery.Where("status = ?", status)
	}

	// Фильтрация по активности
	if isActive := c.Query("is_active"); isActive != "" {
		switch isActive {
		case "true":
			baseQuery = baseQuery.Where("is_active = ?", true)
		case "false":
			baseQuery = baseQuery.Where("is_active = ?", false)
		}
	}

	// Фильтрация по истекающим договорам
	if expiring := c.Query("expiring"); expiring == "true" {
		baseQuery = baseQuery.Where("end_date <= ?", time.Now().AddDate(0, 0, 30))
	}

	// Поиск по номеру или названию (поддержка параметра q из roadmap)
	searchQuery := c.Query("q")
	if searchQuery == "" {
		searchQuery = c.Query("search") // Поддержка старого параметра search
	}
	if searchQuery != "" {
		baseQuery = baseQuery.Where("number ILIKE ? OR title ILIKE ? OR client_name ILIKE ?",
			"%"+searchQuery+"%", "%"+searchQuery+"%", "%"+searchQuery+"%")
	}

	// Пагинация
	page := 1
	limit := 20
	if p := c.Query("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	offset := (page - 1) * limit

	// Получаем общее количество (без Preload)
	var total int64
	if err := baseQuery.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при подсчете договоров",
		})
		return
	}

	// Оптимизированный запрос с Preload только для отображаемых данных
	query := baseQuery.Preload("TariffPlan", func(db *gorm.DB) *gorm.DB {
		return db.Select("id, name, price_per_month") // Загружаем только нужные поля
	}).Preload("Appendices", func(db *gorm.DB) *gorm.DB {
		return db.Select("id, title, created_at") // Загружаем только нужные поля
	}).Preload("Objects", func(db *gorm.DB) *gorm.DB {
		return db.Select("id, name, status") // Загружаем только нужные поля
	})

	// Получаем договоры с пагинацией
	if err := query.Offset(offset).Limit(limit).Find(&contracts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при получении договоров",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   contracts,
		"count":  len(contracts),
		"total":  total,
		"page":   page,
		"limit":  limit,
	})
}

// GetContract получает конкретный договор по ID
func GetContract(c *gin.Context) {
	id := c.Param("id")

	var contract models.Contract
	if err := database.DB.Preload("TariffPlan").Preload("Appendices").Preload("Objects").First(&contract, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Договор не найден",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   contract,
	})
}

// CreateContractRequest представляет запрос на создание договора
type CreateContractRequest struct {
	models.Contract
	AccountID *uint `json:"account_id"` // ID учетной записи Axenta для автоматической привязки объектов
}

// CreateContract создает новый договор
func CreateContract(c *gin.Context) {
	var request CreateContractRequest

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных",
		})
		return
	}

	contract := request.Contract

	// Валидация обязательных полей
	if contract.Number == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Номер договора обязателен",
		})
		return
	}

	if contract.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Название договора обязательно",
		})
		return
	}

	if contract.ClientName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Имя клиента обязательно",
		})
		return
	}

	if contract.TariffPlanID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Тарифный план обязателен",
		})
		return
	}

	if contract.StartDate.IsZero() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Дата начала договора обязательна",
		})
		return
	}

	if contract.EndDate.IsZero() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Дата окончания договора обязательна",
		})
		return
	}

	// Проверяем существование тарифного плана
	var tariffPlan models.BillingPlan
	if err := database.DB.First(&tariffPlan, contract.TariffPlanID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Тарифный план не найден",
		})
		return
	}

	// Устанавливаем значения по умолчанию
	if contract.Status == "" {
		contract.Status = "draft"
	}
	if contract.Currency == "" {
		contract.Currency = "RUB"
	}
	if contract.NotifyBefore == 0 {
		contract.NotifyBefore = 30
	}

	// Рассчитываем общую стоимость на основе тарифного плана
	if contract.TotalAmount.IsZero() {
		// Базовая стоимость из тарифного плана
		contract.TotalAmount = tariffPlan.Price

		// Если есть период, умножаем на количество периодов
		duration := contract.EndDate.Sub(contract.StartDate)
		months := int(duration.Hours() / (24 * 30))
		if months > 0 {
			contract.TotalAmount = contract.TotalAmount.Mul(decimal.NewFromInt(int64(months)))
		}
	}

	if err := database.DB.Create(&contract).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при создании договора",
		})
		return
	}

	// Если указан account_id, привязываем объекты этой учетной записи к договору
	if request.AccountID != nil && *request.AccountID > 0 {
		accountID := *request.AccountID
		
		// Получаем подключение к БД текущей компании для работы с объектами
		tenantDB := middleware.GetTenantDB(c)
		if tenantDB == nil {
			// Если нет tenantDB, используем обычную БД
			tenantDB = database.DB
		}

		// Находим все объекты, которые принадлежат этой учетной записи (CompanyID = account_id)
		var objectsToAttach []models.Object
		if err := tenantDB.Where("company_id = ?", accountID).
			Find(&objectsToAttach).Error; err == nil && len(objectsToAttach) > 0 {
			// Привязываем объекты к договору
			if err := tenantDB.Model(&models.Object{}).
				Where("company_id = ?", accountID).
				Update("contract_id", contract.ID).Error; err != nil {
				// Логируем ошибку, но не прерываем создание договора
				log.Printf("⚠️ Ошибка привязки объектов к договору %d: %v", contract.ID, err)
			} else {
				log.Printf("✅ Привязано %d объектов к договору %d", len(objectsToAttach), contract.ID)
			}
		}
	}

	// Загружаем связанные данные для ответа
	database.DB.Preload("TariffPlan").Preload("Objects").First(&contract, contract.ID)

	c.JSON(http.StatusCreated, gin.H{
		"status": "success",
		"data":   contract,
	})
}

// UpdateContract обновляет существующий договор
func UpdateContract(c *gin.Context) {
	id := c.Param("id")

	var contract models.Contract
	if err := database.DB.First(&contract, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Договор не найден",
		})
		return
	}

	var updateData models.Contract
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных",
		})
		return
	}

	// Проверяем тарифный план если он изменился
	if updateData.TariffPlanID != 0 && updateData.TariffPlanID != contract.TariffPlanID {
		var tariffPlan models.BillingPlan
		if err := database.DB.First(&tariffPlan, updateData.TariffPlanID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Тарифный план не найден",
			})
			return
		}
	}

	if err := database.DB.Model(&contract).Updates(updateData).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при обновлении договора",
		})
		return
	}

	// Загружаем обновленные данные
	database.DB.Preload("TariffPlan").Preload("Appendices").Preload("Objects").First(&contract, contract.ID)

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   contract,
	})
}

// DeleteContract удаляет договор
func DeleteContract(c *gin.Context) {
	id := c.Param("id")

	var contract models.Contract
	if err := database.DB.First(&contract, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Договор не найден",
		})
		return
	}

	// Проверяем, есть ли связанные объекты
	var objectCount int64
	database.DB.Model(&models.Object{}).Where("contract_id = ?", contract.ID).Count(&objectCount)

	if objectCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Нельзя удалить договор, к которому привязаны объекты",
		})
		return
	}

	if err := database.DB.Delete(&contract).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при удалении договора",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Договор успешно удален",
	})
}

// GetContractAppendices получает список приложений к договору
func GetContractAppendices(c *gin.Context) {
	contractID := c.Param("contract_id")

	var appendices []models.ContractAppendix
	if err := database.DB.Where("contract_id = ?", contractID).Find(&appendices).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при получении приложений",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   appendices,
		"count":  len(appendices),
	})
}

// CreateContractAppendix создает новое приложение к договору
func CreateContractAppendix(c *gin.Context) {
	contractID := c.Param("contract_id")

	// Проверяем существование договора
	var contract models.Contract
	if err := database.DB.Where("id = ?", contractID).First(&contract).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Договор не найден",
		})
		return
	}

	var appendix models.ContractAppendix
	if err := c.ShouldBindJSON(&appendix); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных",
		})
		return
	}

	// Устанавливаем ID договора
	contractIDUint, _ := strconv.ParseUint(contractID, 10, 32)
	appendix.ContractID = uint(contractIDUint)

	// Валидация обязательных полей
	if appendix.Number == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Номер приложения обязателен",
		})
		return
	}

	if appendix.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Название приложения обязательно",
		})
		return
	}

	// Устанавливаем значения по умолчанию
	if appendix.Status == "" {
		appendix.Status = "draft"
	}
	if appendix.Currency == "" {
		appendix.Currency = contract.Currency
	}

	// Обнуляем связанный объект Contract, чтобы GORM не создавал новый
	appendix.Contract = models.Contract{}

	if err := database.DB.Create(&appendix).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при создании приложения",
		})
		return
	}

	// Загружаем созданное приложение без связей для ответа
	var createdAppendix models.ContractAppendix
	database.DB.First(&createdAppendix, appendix.ID)

	c.JSON(http.StatusCreated, gin.H{
		"status": "success",
		"data":   createdAppendix,
	})
}

// UpdateContractAppendix обновляет приложение к договору
func UpdateContractAppendix(c *gin.Context) {
	id := c.Param("id")

	var appendix models.ContractAppendix
	if err := database.DB.First(&appendix, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Приложение не найдено",
		})
		return
	}

	var updateData models.ContractAppendix
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных",
		})
		return
	}

	if err := database.DB.Model(&appendix).Updates(updateData).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при обновлении приложения",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   appendix,
	})
}

// DeleteContractAppendix удаляет приложение к договору
func DeleteContractAppendix(c *gin.Context) {
	id := c.Param("id")

	var appendix models.ContractAppendix
	if err := database.DB.First(&appendix, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Приложение не найдено",
		})
		return
	}

	if err := database.DB.Delete(&appendix).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при удалении приложения",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Приложение успешно удалено",
	})
}

// CalculateContractCost рассчитывает стоимость договора на основе объектов
func CalculateContractCost(c *gin.Context) {
	contractID := c.Param("contract_id")

	// Получаем договор с тарифным планом
	var contract models.Contract
	if err := database.DB.Preload("TariffPlan").First(&contract, contractID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Договор не найден",
		})
		return
	}

	// Получаем количество объектов по договору
	var totalObjects int64
	var activeObjects int64
	var inactiveObjects int64

	database.DB.Model(&models.Object{}).Where("contract_id = ?", contractID).Count(&totalObjects)
	database.DB.Model(&models.Object{}).Where("contract_id = ? AND is_active = ?", contractID, true).Count(&activeObjects)
	inactiveObjects = totalObjects - activeObjects

	// Рассчитываем стоимость если есть TariffPlan с детальной тарификацией
	var calculatedCost decimal.Decimal
	if contract.TariffPlan.ID != 0 {
		// Создаем TariffPlan из BillingPlan для расчета
		tariffPlan := models.TariffPlan{
			BillingPlan:        contract.TariffPlan,
			PricePerObject:     contract.TariffPlan.Price,
			InactivePriceRatio: decimal.NewFromFloat(0.5), // 50% для неактивных объектов
		}

		calculatedCost = tariffPlan.CalculateObjectPrice(int(totalObjects), int(inactiveObjects))
	} else {
		calculatedCost = contract.TotalAmount
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"contract_id":      contract.ID,
			"total_objects":    totalObjects,
			"active_objects":   activeObjects,
			"inactive_objects": inactiveObjects,
			"calculated_cost":  calculatedCost,
			"current_cost":     contract.TotalAmount,
			"currency":         contract.Currency,
		},
	})
}

// GetExpiringContracts получает список истекающих договоров
func GetExpiringContracts(c *gin.Context) {
	// По умолчанию показываем договоры, истекающие в течение 30 дней
	days := 30
	if d := c.Query("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			days = parsed
		}
	}

	var contracts []models.Contract
	expiryDate := time.Now().AddDate(0, 0, days)

	if err := database.DB.Preload("TariffPlan").
		Where("end_date <= ? AND status = 'active'", expiryDate).
		Find(&contracts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при получении истекающих договоров",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   contracts,
		"count":  len(contracts),
		"days":   days,
	})
}

// ===== API ДЛЯ НУМЕРАТОРОВ ДОГОВОРОВ =====

// GetContractNumerators получает список нумераторов для компании
func GetContractNumerators(c *gin.Context) {
	companyIDStr := c.Query("company_id")
	if companyIDStr == "" {
		log.Printf("GetContractNumerators: company_id не указан\n")
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Параметр company_id обязателен",
		})
		return
	}

	companyID, err := strconv.ParseUint(companyIDStr, 10, 32)
	if err != nil {
		log.Printf("GetContractNumerators: ошибка парсинга company_id: %v\n", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Неверный формат company_id: %v", err),
		})
		return
	}
	log.Printf("GetContractNumerators: запрос для company_id=%d\n", uint(companyID))

	// Убеждаемся, что мы в схеме public для глобальных таблиц
	if err := database.DB.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("GetContractNumerators: ошибка установки search_path: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Ошибка подключения к базе данных: %v", err),
		})
		return
	}

	var numerators []models.ContractNumerator
	if err := database.DB.Where("company_id = ?", uint(companyID)).Order("is_default DESC, created_at ASC").Find(&numerators).Error; err != nil {
		log.Printf("GetContractNumerators: ОШИБКА при получении нумераторов: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Ошибка при получении нумераторов: %v", err),
		})
		return
	}

	log.Printf("GetContractNumerators: найдено %d нумераторов для company_id=%d\n", len(numerators), uint(companyID))
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   numerators,
		"count":  len(numerators),
	})
}

// GetContractNumerator получает конкретный нумератор по ID
func GetContractNumerator(c *gin.Context) {
	id := c.Param("id")

	var numerator models.ContractNumerator
	if err := database.DB.First(&numerator, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Нумератор не найден",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   numerator,
	})
}

// CreateContractNumerator создает новый нумератор
func CreateContractNumerator(c *gin.Context) {
	var numerator models.ContractNumerator

	if err := c.ShouldBindJSON(&numerator); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных",
		})
		return
	}

	// Валидация
	if numerator.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Название нумератора обязательно",
		})
		return
	}

	if numerator.Prefix == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Префикс нумератора обязателен",
		})
		return
	}

	if numerator.Template == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Шаблон номера обязателен",
		})
		return
	}

	// Если это нумератор по умолчанию, снимаем флаг с других
	if numerator.IsDefault {
		database.DB.Model(&models.ContractNumerator{}).
			Where("company_id = ? AND is_default = ?", numerator.CompanyID, true).
			Update("is_default", false)
	}

	if err := database.DB.Create(&numerator).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка при создании нумератора",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status": "success",
		"data":   numerator,
	})
}

// UpdateContractNumerator обновляет существующий нумератор
func UpdateContractNumerator(c *gin.Context) {
	id := c.Param("id")

	var numerator models.ContractNumerator
	if err := database.DB.First(&numerator, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Нумератор не найден",
		})
		return
	}

	var updateData models.ContractNumerator
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных",
		})
		return
	}

	// Если делаем нумератор по умолчанию, снимаем флаг с других
	if updateData.IsDefault && !numerator.IsDefault {
		database.DB.Model(&models.ContractNumerator{}).
			Where("company_id = ? AND is_default = ? AND id != ?", numerator.CompanyID, true, id).
			Update("is_default", false)
	}

	if err := database.DB.Model(&numerator).Updates(updateData).Error; err != nil {
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

// DeleteContractNumerator удаляет нумератор (мягкое удаление)
func DeleteContractNumerator(c *gin.Context) {
	id := c.Param("id")

	var numerator models.ContractNumerator
	if err := database.DB.First(&numerator, id).Error; err != nil {
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

// GenerateContractNumber генерирует номер договора по ID нумератора
func GenerateContractNumber(c *gin.Context) {
	numeratorIDStr := c.Param("numerator_id")
	numeratorID, err := strconv.ParseUint(numeratorIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат ID нумератора",
		})
		return
	}

	// Получаем параметры для генерации
	var req struct {
		ClientID   *uint `json:"client_id"`
		CompanyID  *uint `json:"company_id"`
		ContractID *uint `json:"contract_id"`
	}

	c.ShouldBindJSON(&req)

	var numerator models.ContractNumerator
	if err := database.DB.First(&numerator, uint(numeratorID)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Нумератор не найден",
		})
		return
	}

	// Используем значения по умолчанию
	clientID := uint(0)
	if req.ClientID != nil {
		clientID = *req.ClientID
	}

	companyID := numerator.CompanyID
	if req.CompanyID != nil {
		companyID = *req.CompanyID
	}

	contractID := uint(0)
	if req.ContractID != nil {
		contractID = *req.ContractID
	}

	// Генерируем номер
	number, err := numerator.GenerateNumber(clientID, companyID, contractID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка генерации номера",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"number": number,
			"counter": numerator.CounterValue,
		},
	})
}
