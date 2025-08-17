package api

import (
	"net/http"
	"strconv"
	"time"

	"backend_axenta/database"
	"backend_axenta/models"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// GetContracts получает список всех договоров
func GetContracts(c *gin.Context) {
	var contracts []models.Contract

	query := database.DB.Preload("TariffPlan").Preload("Appendices").Preload("Objects")

	// Фильтрация по статусу
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	// Фильтрация по активности
	if isActive := c.Query("is_active"); isActive != "" {
		if isActive == "true" {
			query = query.Where("is_active = ?", true)
		} else if isActive == "false" {
			query = query.Where("is_active = ?", false)
		}
	}

	// Фильтрация по истекающим договорам
	if expiring := c.Query("expiring"); expiring == "true" {
		query = query.Where("end_date <= ?", time.Now().AddDate(0, 0, 30))
	}

	// Поиск по номеру или названию
	if search := c.Query("search"); search != "" {
		query = query.Where("number ILIKE ? OR title ILIKE ? OR client_name ILIKE ?",
			"%"+search+"%", "%"+search+"%", "%"+search+"%")
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

	// Получаем общее количество
	var total int64
	database.DB.Model(&models.Contract{}).Count(&total)

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

// CreateContract создает новый договор
func CreateContract(c *gin.Context) {
	var contract models.Contract

	if err := c.ShouldBindJSON(&contract); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных",
		})
		return
	}

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
		contract.TotalAmount = decimal.NewFromFloat(tariffPlan.Price)

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

	// Загружаем связанные данные для ответа
	database.DB.Preload("TariffPlan").First(&contract, contract.ID)

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
			PricePerObject:     decimal.NewFromFloat(contract.TariffPlan.Price),
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
