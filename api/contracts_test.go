package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"backend_axenta/database"
	"backend_axenta/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestCreateContract(t *testing.T) {
	// Настройка тестовой базы данных
	database.SetupTestDatabase()
	defer database.CleanupTestDatabase()

	// Создаем тарифный план для теста
	tariffPlan := models.BillingPlan{
		Name:          "Test Plan",
		Description:   "Test tariff plan",
		Price:         decimal.NewFromFloat(1000.0),
		Currency:      "RUB",
		BillingPeriod: "monthly",
		IsActive:      true,
	}
	database.DB.Create(&tariffPlan)

	// Настройка Gin в тестовом режиме
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/contracts", CreateContract)

	// Тестовые данные договора
	contract := models.Contract{
		Number:       "TEST-001",
		Title:        "Тестовый договор",
		ClientName:   "ООО Тест",
		ClientINN:    "1234567890",
		ClientEmail:  "test@example.com",
		ClientPhone:  "+7 (999) 123-45-67",
		StartDate:    time.Now(),
		EndDate:      time.Now().AddDate(1, 0, 0), // Год от текущей даты
		TariffPlanID: tariffPlan.ID,
		Status:       "draft",
		Currency:     "RUB",
		TotalAmount:  decimal.NewFromFloat(12000.0), // 1000 * 12 месяцев
		Notes:        "Тестовый договор для проверки API",
	}

	// Конвертируем в JSON
	contractJSON, _ := json.Marshal(contract)

	// Создаем HTTP запрос
	req, _ := http.NewRequest("POST", "/contracts", bytes.NewBuffer(contractJSON))
	req.Header.Set("Content-Type", "application/json")

	// Выполняем запрос
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Проверяем результат
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	assert.Equal(t, "success", response["status"])
	assert.NotNil(t, response["data"])

	// Проверяем, что договор создался в базе данных
	var createdContract models.Contract
	database.DB.First(&createdContract, "number = ?", "TEST-001")
	assert.Equal(t, "TEST-001", createdContract.Number)
	assert.Equal(t, "Тестовый договор", createdContract.Title)
	assert.Equal(t, "ООО Тест", createdContract.ClientName)
	assert.Equal(t, tariffPlan.ID, createdContract.TariffPlanID)
}

func TestGetContracts(t *testing.T) {
	// Настройка тестовой базы данных
	database.SetupTestDatabase()
	defer database.CleanupTestDatabase()

	// Создаем тарифный план
	tariffPlan := models.BillingPlan{
		Name:     "Test Plan",
		Price:    decimal.NewFromFloat(1000.0),
		Currency: "RUB",
		IsActive: true,
	}
	database.DB.Create(&tariffPlan)

	// Создаем тестовые договоры
	contract1 := models.Contract{
		Number:       "TEST-001",
		Title:        "Договор 1",
		ClientName:   "Клиент 1",
		StartDate:    time.Now(),
		EndDate:      time.Now().AddDate(1, 0, 0),
		TariffPlanID: tariffPlan.ID,
		Status:       "active",
		Currency:     "RUB",
		TotalAmount:  decimal.NewFromFloat(12000.0),
	}
	contract2 := models.Contract{
		Number:       "TEST-002",
		Title:        "Договор 2",
		ClientName:   "Клиент 2",
		StartDate:    time.Now(),
		EndDate:      time.Now().AddDate(1, 0, 0),
		TariffPlanID: tariffPlan.ID,
		Status:       "draft",
		Currency:     "RUB",
		TotalAmount:  decimal.NewFromFloat(12000.0),
	}

	database.DB.Create(&contract1)
	database.DB.Create(&contract2)

	// Настройка Gin
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/contracts", GetContracts)

	// Тест получения всех договоров
	req, _ := http.NewRequest("GET", "/contracts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	assert.Equal(t, "success", response["status"])
	assert.Equal(t, float64(2), response["count"])

	// Тест фильтрации по статусу
	req, _ = http.NewRequest("GET", "/contracts?status=active", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	json.Unmarshal(w.Body.Bytes(), &response)
	assert.Equal(t, "success", response["status"])
	assert.Equal(t, float64(1), response["count"])
}

func TestCalculateContractCost(t *testing.T) {
	// Настройка тестовой базы данных
	database.SetupTestDatabase()
	defer database.CleanupTestDatabase()

	// Создаем тарифный план
	tariffPlan := models.BillingPlan{
		Name:     "Test Plan",
		Price:    decimal.NewFromFloat(500.0), // 500 рублей за объект
		Currency: "RUB",
		IsActive: true,
	}
	database.DB.Create(&tariffPlan)

	// Создаем договор
	contract := models.Contract{
		Number:       "TEST-001",
		Title:        "Договор для расчета",
		ClientName:   "Тест Клиент",
		StartDate:    time.Now(),
		EndDate:      time.Now().AddDate(1, 0, 0),
		TariffPlanID: tariffPlan.ID,
		Status:       "active",
		Currency:     "RUB",
		TotalAmount:  decimal.NewFromFloat(6000.0),
	}
	database.DB.Create(&contract)

	// Создаем объекты по договору
	object1 := models.Object{
		Name:       "Объект 1",
		Type:       "vehicle",
		IMEI:       "123456789012345",
		ContractID: contract.ID,
		IsActive:   true,
		Status:     "active",
	}
	object2 := models.Object{
		Name:       "Объект 2",
		Type:       "equipment",
		IMEI:       "123456789012346",
		ContractID: contract.ID,
		IsActive:   false, // Неактивный объект
		Status:     "inactive",
	}
	object3 := models.Object{
		Name:       "Объект 3",
		Type:       "asset",
		IMEI:       "123456789012347",
		ContractID: contract.ID,
		IsActive:   true,
		Status:     "active",
	}

	database.DB.Create(&object1)
	database.DB.Create(&object2)
	database.DB.Create(&object3)

	// Настройка Gin
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/contracts/:contract_id/cost", CalculateContractCost)

	// Выполняем запрос расчета стоимости
	contractCostURL := fmt.Sprintf("/contracts/%d/cost", contract.ID)
	req, _ := http.NewRequest("GET", contractCostURL, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	assert.Equal(t, "success", response["status"])

	data := response["data"].(map[string]interface{})

	// Проверяем, что данные присутствуют
	assert.NotNil(t, data["total_objects"])
	assert.NotNil(t, data["active_objects"])
	assert.NotNil(t, data["inactive_objects"])
	assert.NotNil(t, data["calculated_cost"])

	// Проверяем расчет стоимости: активных * 500 + неактивных * 500 * 0.5
	calculatedCost := data["calculated_cost"].(string)

	// Получаем фактические значения для расчета
	totalObjects := int(data["total_objects"].(float64))
	activeObjects := int(data["active_objects"].(float64))
	inactiveObjects := int(data["inactive_objects"].(float64))

	// Рассчитываем ожидаемую стоимость
	expectedCostFloat := float64(activeObjects)*500 + float64(inactiveObjects)*500*0.5
	expectedCost := fmt.Sprintf("%.0f", expectedCostFloat)

	t.Logf("Total objects: %d, Active: %d, Inactive: %d", totalObjects, activeObjects, inactiveObjects)
	t.Logf("Expected cost: %s, Actual cost: %s", expectedCost, calculatedCost)

	assert.Equal(t, expectedCost, calculatedCost)
}

func TestContractAppendices(t *testing.T) {
	// Настройка тестовой базы данных
	database.SetupTestDatabase()
	defer database.CleanupTestDatabase()

	// Создаем тарифный план и договор
	tariffPlan := models.BillingPlan{
		Name:     "Test Plan",
		Price:    decimal.NewFromFloat(1000.0),
		Currency: "RUB",
		IsActive: true,
	}
	database.DB.Create(&tariffPlan)

	contract := models.Contract{
		Number:       "TEST-001",
		Title:        "Основной договор",
		ClientName:   "Тест Клиент",
		StartDate:    time.Now(),
		EndDate:      time.Now().AddDate(1, 0, 0),
		TariffPlanID: tariffPlan.ID,
		Status:       "active",
		Currency:     "RUB",
		TotalAmount:  decimal.NewFromFloat(12000.0),
	}
	database.DB.Create(&contract)

	// Настройка Gin
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/contracts/:contract_id/appendices", CreateContractAppendix)
	router.GET("/contracts/:contract_id/appendices", GetContractAppendices)

	// Создаем приложение к договору
	appendix := models.ContractAppendix{
		Number:      "APP-001",
		Title:       "Дополнительное соглашение №1",
		Description: "Изменение условий договора",
		StartDate:   time.Now(),
		EndDate:     time.Now().AddDate(0, 6, 0), // 6 месяцев
		Amount:      decimal.NewFromFloat(5000.0),
		Currency:    "RUB",
		Status:      "draft",
	}

	appendixJSON, _ := json.Marshal(appendix)

	// Создаем приложение
	contractIDStr := fmt.Sprintf("/contracts/%d/appendices", contract.ID)
	req, _ := http.NewRequest("POST", contractIDStr, bytes.NewBuffer(appendixJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	assert.Equal(t, "success", response["status"])

	// Получаем приложения договора
	contractIDStrGet := fmt.Sprintf("/contracts/%d/appendices", contract.ID)
	req, _ = http.NewRequest("GET", contractIDStrGet, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	json.Unmarshal(w.Body.Bytes(), &response)
	assert.Equal(t, "success", response["status"])
	assert.Equal(t, float64(1), response["count"])

	// Проверяем, что приложение создалось в базе данных
	var createdAppendix models.ContractAppendix
	err := database.DB.Where("contract_id = ? AND number = ?", contract.ID, "APP-001").First(&createdAppendix).Error
	assert.NoError(t, err)
	assert.Equal(t, "APP-001", createdAppendix.Number)
	assert.Equal(t, contract.ID, createdAppendix.ContractID)
}
