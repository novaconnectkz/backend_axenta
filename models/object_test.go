package models

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestObjectModel тестирует модель Object
func TestObjectModel(t *testing.T) {
	db := setupTestDB(t)

	t.Run("Создание объекта мониторинга", func(t *testing.T) {
		// Создаем необходимые связанные сущности
		location := Location{
			City:     "Москва",
			Region:   "Московская область",
			Country:  "Russia",
			Timezone: "Europe/Moscow",
			IsActive: true,
		}
		err := db.Create(&location).Error
		require.NoError(t, err)

		billingPlan := BillingPlan{
			Name:          "Базовый тариф",
			Description:   "Базовый тарифный план",
			Price:         1000.0,
			Currency:      "RUB",
			BillingPeriod: "monthly",
			IsActive:      true,
		}
		err = db.Create(&billingPlan).Error
		require.NoError(t, err)

		contract := Contract{
			Number:       "TEST-001",
			Title:        "Тестовый договор",
			ClientName:   "ООО Тест",
			StartDate:    time.Now(),
			EndDate:      time.Now().AddDate(1, 0, 0),
			TariffPlanID: billingPlan.ID,
			TotalAmount:  decimal.NewFromFloat(12000.0),
			Status:       "active",
			IsActive:     true,
		}
		err = db.Create(&contract).Error
		require.NoError(t, err)

		// Создаем объект
		object := Object{
			Name:         "Тестовый объект",
			Type:         "vehicle",
			Description:  "Тестовый объект для мониторинга",
			Latitude:     floatPtr(55.7558),
			Longitude:    floatPtr(37.6176),
			Address:      "Москва, Красная площадь",
			IMEI:         "123456789012345",
			PhoneNumber:  "+7-900-123-45-67",
			SerialNumber: "SN123456",
			Status:       "active",
			IsActive:     true,
			ContractID:   contract.ID,
			LocationID:   location.ID,
			Settings:     `{"monitoring": {"interval": 60, "alerts": true}}`,
			Tags:         []string{"test", "vehicle", "moscow"},
			Notes:        "Тестовые заметки",
			ExternalID:   "EXT123",
		}

		err = db.Create(&object).Error
		require.NoError(t, err)
		assert.NotZero(t, object.ID)
		assert.Equal(t, "Тестовый объект", object.Name)
		assert.Equal(t, "123456789012345", object.IMEI)
	})

	t.Run("Уникальность IMEI", func(t *testing.T) {
		// Создаем договор для объектов
		billingPlan := BillingPlan{
			Name:          "Уникальный тариф",
			Price:         500.0,
			Currency:      "RUB",
			BillingPeriod: "monthly",
			IsActive:      true,
		}
		err := db.Create(&billingPlan).Error
		require.NoError(t, err)

		contract := Contract{
			Number:       "UNIQUE-001",
			Title:        "Договор для уникальности",
			ClientName:   "ООО Уникальность",
			StartDate:    time.Now(),
			EndDate:      time.Now().AddDate(1, 0, 0),
			TariffPlanID: billingPlan.ID,
			TotalAmount:  decimal.NewFromFloat(6000.0),
			Status:       "active",
			IsActive:     true,
		}
		err = db.Create(&contract).Error
		require.NoError(t, err)

		// Создаем первый объект
		object1 := Object{
			Name:       "Объект 1",
			Type:       "equipment",
			IMEI:       "unique123456789",
			ContractID: contract.ID,
			IsActive:   true,
		}
		err = db.Create(&object1).Error
		require.NoError(t, err)

		// Попытка создать объект с тем же IMEI
		object2 := Object{
			Name:       "Объект 2",
			Type:       "equipment",
			IMEI:       "unique123456789",
			ContractID: contract.ID,
			IsActive:   true,
		}
		err = db.Create(&object2).Error
		assert.Error(t, err, "Должна быть ошибка из-за дублирования IMEI")
	})

	t.Run("Связи объекта", func(t *testing.T) {
		// Создаем все необходимые связанные сущности
		location := Location{
			City:     "Санкт-Петербург",
			Region:   "Ленинградская область",
			Country:  "Russia",
			IsActive: true,
		}
		err := db.Create(&location).Error
		require.NoError(t, err)

		billingPlan := BillingPlan{
			Name:          "Премиум тариф",
			Price:         2000.0,
			Currency:      "RUB",
			BillingPeriod: "monthly",
			IsActive:      true,
		}
		err = db.Create(&billingPlan).Error
		require.NoError(t, err)

		contract := Contract{
			Number:       "RELATIONS-001",
			Title:        "Договор для связей",
			ClientName:   "ООО Связи",
			StartDate:    time.Now(),
			EndDate:      time.Now().AddDate(1, 0, 0),
			TariffPlanID: billingPlan.ID,
			TotalAmount:  decimal.NewFromFloat(24000.0),
			Status:       "active",
			IsActive:     true,
		}
		err = db.Create(&contract).Error
		require.NoError(t, err)

		objectTemplate := ObjectTemplate{
			Name:        "Шаблон для связей",
			Description: "Тестовый шаблон объекта",
			Category:    "vehicle",
			IsActive:    true,
		}
		err = db.Create(&objectTemplate).Error
		require.NoError(t, err)

		// Создаем объект со всеми связями
		object := Object{
			Name:       "Объект со связями",
			Type:       "vehicle",
			IMEI:       "relations123456",
			ContractID: contract.ID,
			LocationID: location.ID,
			TemplateID: &objectTemplate.ID,
			IsActive:   true,
		}
		err = db.Create(&object).Error
		require.NoError(t, err)

		// Загружаем объект со всеми связями
		var objectWithRelations Object
		err = db.Preload("Contract").Preload("Location").Preload("Template").First(&objectWithRelations, object.ID).Error
		require.NoError(t, err)

		assert.NotNil(t, objectWithRelations.Contract)
		assert.Equal(t, "RELATIONS-001", objectWithRelations.Contract.Number)

		assert.NotNil(t, objectWithRelations.Location)
		assert.Equal(t, "Санкт-Петербург", objectWithRelations.Location.City)

		assert.NotNil(t, objectWithRelations.Template)
		assert.Equal(t, "Шаблон для связей", objectWithRelations.Template.Name)
	})

	t.Run("Плановое удаление объекта", func(t *testing.T) {
		// Создаем минимальные связанные сущности
		billingPlan := BillingPlan{
			Name:          "Тариф для удаления",
			Price:         100.0,
			Currency:      "RUB",
			BillingPeriod: "monthly",
			IsActive:      true,
		}
		err := db.Create(&billingPlan).Error
		require.NoError(t, err)

		contract := Contract{
			Number:       "DELETE-001",
			Title:        "Договор для удаления",
			ClientName:   "ООО Удаление",
			StartDate:    time.Now(),
			EndDate:      time.Now().AddDate(1, 0, 0),
			TariffPlanID: billingPlan.ID,
			TotalAmount:  decimal.NewFromFloat(1200.0),
			Status:       "active",
			IsActive:     true,
		}
		err = db.Create(&contract).Error
		require.NoError(t, err)

		// Создаем объект с плановой датой удаления
		scheduledDelete := time.Now().AddDate(0, 1, 0) // Через месяц
		object := Object{
			Name:              "Объект для удаления",
			Type:              "asset",
			IMEI:              "delete123456789",
			ContractID:        contract.ID,
			Status:            "scheduled_delete",
			ScheduledDeleteAt: &scheduledDelete,
			IsActive:          false,
		}

		err = db.Create(&object).Error
		require.NoError(t, err)
		assert.NotNil(t, object.ScheduledDeleteAt)
		assert.Equal(t, "scheduled_delete", object.Status)
		assert.False(t, object.IsActive)
	})

	t.Run("Обновление времени последней активности", func(t *testing.T) {
		// Создаем минимальные связанные сущности
		billingPlan := BillingPlan{
			Name:          "Тариф активности",
			Price:         150.0,
			Currency:      "RUB",
			BillingPeriod: "monthly",
			IsActive:      true,
		}
		err := db.Create(&billingPlan).Error
		require.NoError(t, err)

		contract := Contract{
			Number:       "ACTIVITY-001",
			Title:        "Договор активности",
			ClientName:   "ООО Активность",
			StartDate:    time.Now(),
			EndDate:      time.Now().AddDate(1, 0, 0),
			TariffPlanID: billingPlan.ID,
			TotalAmount:  decimal.NewFromFloat(1800.0),
			Status:       "active",
			IsActive:     true,
		}
		err = db.Create(&contract).Error
		require.NoError(t, err)

		// Создаем объект
		object := Object{
			Name:       "Активный объект",
			Type:       "vehicle",
			IMEI:       "activity123456",
			ContractID: contract.ID,
			IsActive:   true,
		}
		err = db.Create(&object).Error
		require.NoError(t, err)

		// Обновляем время последней активности
		lastActivity := time.Now()
		object.LastActivityAt = &lastActivity

		err = db.Save(&object).Error
		require.NoError(t, err)

		// Проверяем обновление
		var updatedObject Object
		err = db.First(&updatedObject, object.ID).Error
		require.NoError(t, err)
		assert.NotNil(t, updatedObject.LastActivityAt)
	})
}

// TestContractModel тестирует модель Contract
func TestContractModel(t *testing.T) {
	db := setupTestDB(t)

	t.Run("Создание договора", func(t *testing.T) {
		// Создаем тарифный план
		billingPlan := BillingPlan{
			Name:          "Стандартный план",
			Description:   "Стандартный тарифный план",
			Price:         1500.0,
			Currency:      "RUB",
			BillingPeriod: "monthly",
			IsActive:      true,
		}
		err := db.Create(&billingPlan).Error
		require.NoError(t, err)

		// Создаем договор
		startDate := time.Now()
		endDate := startDate.AddDate(1, 0, 0)
		signedAt := time.Now().Add(-time.Hour)

		contract := Contract{
			Number:        "CONTRACT-001",
			Title:         "Договор на оказание услуг мониторинга",
			Description:   "Полное описание услуг договора",
			ClientName:    "ООО Рога и Копыта",
			ClientINN:     "1234567890",
			ClientKPP:     "123456789",
			ClientEmail:   "client@example.com",
			ClientPhone:   "+7-495-123-45-67",
			ClientAddress: "г. Москва, ул. Тверская, д. 1",
			StartDate:     startDate,
			EndDate:       endDate,
			SignedAt:      &signedAt,
			TariffPlanID:  billingPlan.ID,
			TotalAmount:   decimal.NewFromFloat(18000.0),
			Currency:      "RUB",
			Status:        "active",
			IsActive:      true,
			NotifyBefore:  30,
			Notes:         "Особые условия договора",
			ExternalID:    "1C-12345",
		}

		err = db.Create(&contract).Error
		require.NoError(t, err)
		assert.NotZero(t, contract.ID)
		assert.Equal(t, "CONTRACT-001", contract.Number)
		assert.Equal(t, "ООО Рога и Копыта", contract.ClientName)
		assert.Equal(t, "RUB", contract.Currency)
	})

	t.Run("Уникальность номера договора", func(t *testing.T) {
		// Создаем тарифный план
		billingPlan := BillingPlan{
			Name:          "План для уникальности",
			Price:         800.0,
			Currency:      "RUB",
			BillingPeriod: "monthly",
			IsActive:      true,
		}
		err := db.Create(&billingPlan).Error
		require.NoError(t, err)

		// Создаем первый договор
		contract1 := Contract{
			Number:       "UNIQUE-CONTRACT-001",
			Title:        "Первый договор",
			ClientName:   "ООО Первый",
			StartDate:    time.Now(),
			EndDate:      time.Now().AddDate(1, 0, 0),
			TariffPlanID: billingPlan.ID,
			TotalAmount:  decimal.NewFromFloat(9600.0),
			Status:       "active",
			IsActive:     true,
		}
		err = db.Create(&contract1).Error
		require.NoError(t, err)

		// Попытка создать договор с тем же номером
		contract2 := Contract{
			Number:       "UNIQUE-CONTRACT-001",
			Title:        "Второй договор",
			ClientName:   "ООО Второй",
			StartDate:    time.Now(),
			EndDate:      time.Now().AddDate(1, 0, 0),
			TariffPlanID: billingPlan.ID,
			TotalAmount:  decimal.NewFromFloat(9600.0),
			Status:       "active",
			IsActive:     true,
		}
		err = db.Create(&contract2).Error
		assert.Error(t, err, "Должна быть ошибка из-за дублирования номера договора")
	})

	t.Run("Методы проверки срока действия договора", func(t *testing.T) {
		// Создаем тарифный план
		billingPlan := BillingPlan{
			Name:          "План для сроков",
			Price:         1200.0,
			Currency:      "RUB",
			BillingPeriod: "monthly",
			IsActive:      true,
		}
		err := db.Create(&billingPlan).Error
		require.NoError(t, err)

		// Создаем истекший договор
		expiredContract := Contract{
			Number:       "EXPIRED-001",
			Title:        "Истекший договор",
			ClientName:   "ООО Истекший",
			StartDate:    time.Now().AddDate(-2, 0, 0),
			EndDate:      time.Now().AddDate(-1, 0, 0),
			TariffPlanID: billingPlan.ID,
			TotalAmount:  decimal.NewFromFloat(14400.0),
			Status:       "expired",
			IsActive:     false,
			NotifyBefore: 30,
		}
		err = db.Create(&expiredContract).Error
		require.NoError(t, err)

		// Тестируем метод IsExpired
		assert.True(t, expiredContract.IsExpired())
		assert.False(t, expiredContract.IsExpiringSoon())
		assert.Equal(t, 0, expiredContract.GetDaysUntilExpiry())

		// Создаем договор, который скоро истечет
		expiringContract := Contract{
			Number:       "EXPIRING-001",
			Title:        "Истекающий договор",
			ClientName:   "ООО Истекающий",
			StartDate:    time.Now().AddDate(-11, 0, 0),
			EndDate:      time.Now().AddDate(0, 0, 15), // Истекает через 15 дней
			TariffPlanID: billingPlan.ID,
			TotalAmount:  decimal.NewFromFloat(14400.0),
			Status:       "active",
			IsActive:     true,
			NotifyBefore: 30,
		}
		err = db.Create(&expiringContract).Error
		require.NoError(t, err)

		// Тестируем методы
		assert.False(t, expiringContract.IsExpired())
		assert.True(t, expiringContract.IsExpiringSoon())
		assert.Equal(t, 15, expiringContract.GetDaysUntilExpiry())

		// Создаем активный договор
		activeContract := Contract{
			Number:       "ACTIVE-001",
			Title:        "Активный договор",
			ClientName:   "ООО Активный",
			StartDate:    time.Now().AddDate(-6, 0, 0),
			EndDate:      time.Now().AddDate(0, 6, 0), // Истекает через 6 месяцев
			TariffPlanID: billingPlan.ID,
			TotalAmount:  decimal.NewFromFloat(14400.0),
			Status:       "active",
			IsActive:     true,
			NotifyBefore: 30,
		}
		err = db.Create(&activeContract).Error
		require.NoError(t, err)

		// Тестируем методы
		assert.False(t, activeContract.IsExpired())
		assert.False(t, activeContract.IsExpiringSoon())
		assert.True(t, activeContract.GetDaysUntilExpiry() > 100)
	})

	t.Run("Связь договора с тарифным планом", func(t *testing.T) {
		// Создаем тарифный план
		billingPlan := BillingPlan{
			Name:          "Премиум план",
			Description:   "Расширенный тарифный план",
			Price:         2500.0,
			Currency:      "RUB",
			BillingPeriod: "monthly",
			HasAnalytics:  true,
			HasAPI:        true,
			IsActive:      true,
		}
		err := db.Create(&billingPlan).Error
		require.NoError(t, err)

		// Создаем договор с тарифным планом
		contract := Contract{
			Number:       "PREMIUM-001",
			Title:        "Премиум договор",
			ClientName:   "ООО Премиум",
			StartDate:    time.Now(),
			EndDate:      time.Now().AddDate(1, 0, 0),
			TariffPlanID: billingPlan.ID,
			TotalAmount:  decimal.NewFromFloat(30000.0),
			Status:       "active",
			IsActive:     true,
		}
		err = db.Create(&contract).Error
		require.NoError(t, err)

		// Загружаем договор с тарифным планом
		var contractWithPlan Contract
		err = db.Preload("TariffPlan").First(&contractWithPlan, contract.ID).Error
		require.NoError(t, err)
		assert.Equal(t, "Премиум план", contractWithPlan.TariffPlan.Name)
		assert.True(t, contractWithPlan.TariffPlan.HasAnalytics)
		assert.True(t, contractWithPlan.TariffPlan.HasAPI)
	})

	t.Run("Приложения к договору", func(t *testing.T) {
		// Создаем тарифный план
		billingPlan := BillingPlan{
			Name:          "План с приложениями",
			Price:         1000.0,
			Currency:      "RUB",
			BillingPeriod: "monthly",
			IsActive:      true,
		}
		err := db.Create(&billingPlan).Error
		require.NoError(t, err)

		// Создаем договор
		contract := Contract{
			Number:       "APPENDIX-001",
			Title:        "Договор с приложениями",
			ClientName:   "ООО Приложения",
			StartDate:    time.Now(),
			EndDate:      time.Now().AddDate(1, 0, 0),
			TariffPlanID: billingPlan.ID,
			TotalAmount:  decimal.NewFromFloat(12000.0),
			Status:       "active",
			IsActive:     true,
		}
		err = db.Create(&contract).Error
		require.NoError(t, err)

		// Создаем приложения к договору
		appendix1 := ContractAppendix{
			ContractID:  contract.ID,
			Number:      "APP-001",
			Title:       "Приложение №1",
			Description: "Дополнительные услуги",
			StartDate:   time.Now(),
			EndDate:     time.Now().AddDate(0, 6, 0),
			Amount:      decimal.NewFromFloat(5000.0),
			Currency:    "RUB",
			Status:      "active",
			IsActive:    true,
		}

		appendix2 := ContractAppendix{
			ContractID:  contract.ID,
			Number:      "APP-002",
			Title:       "Приложение №2",
			Description: "Еще больше услуг",
			StartDate:   time.Now(),
			EndDate:     time.Now().AddDate(0, 3, 0),
			Amount:      decimal.NewFromFloat(3000.0),
			Currency:    "RUB",
			Status:      "active",
			IsActive:    true,
		}

		err = db.Create(&appendix1).Error
		require.NoError(t, err)
		err = db.Create(&appendix2).Error
		require.NoError(t, err)

		// Загружаем договор с приложениями
		var contractWithAppendices Contract
		err = db.Preload("Appendices").First(&contractWithAppendices, contract.ID).Error
		require.NoError(t, err)
		assert.Len(t, contractWithAppendices.Appendices, 2)
	})
}

// TestContractAppendixModel тестирует модель ContractAppendix
func TestContractAppendixModel(t *testing.T) {
	db := setupTestDB(t)

	t.Run("Создание приложения к договору", func(t *testing.T) {
		// Создаем тарифный план и договор
		billingPlan := BillingPlan{
			Name:          "План для приложений",
			Price:         1500.0,
			Currency:      "RUB",
			BillingPeriod: "monthly",
			IsActive:      true,
		}
		err := db.Create(&billingPlan).Error
		require.NoError(t, err)

		contract := Contract{
			Number:       "MAIN-CONTRACT-001",
			Title:        "Основной договор",
			ClientName:   "ООО Основной",
			StartDate:    time.Now(),
			EndDate:      time.Now().AddDate(1, 0, 0),
			TariffPlanID: billingPlan.ID,
			TotalAmount:  decimal.NewFromFloat(18000.0),
			Status:       "active",
			IsActive:     true,
		}
		err = db.Create(&contract).Error
		require.NoError(t, err)

		// Создаем приложение
		signedAt := time.Now().Add(-time.Hour * 24)
		appendix := ContractAppendix{
			ContractID:  contract.ID,
			Number:      "APPENDIX-001",
			Title:       "Дополнительное соглашение №1",
			Description: "Расширение функционала системы",
			StartDate:   time.Now(),
			EndDate:     time.Now().AddDate(0, 6, 0),
			SignedAt:    &signedAt,
			Amount:      decimal.NewFromFloat(7500.0),
			Currency:    "RUB",
			Status:      "active",
			IsActive:    true,
			Notes:       "Специальные условия приложения",
			ExternalID:  "1C-APP-001",
		}

		err = db.Create(&appendix).Error
		require.NoError(t, err)
		assert.NotZero(t, appendix.ID)
		assert.Equal(t, "APPENDIX-001", appendix.Number)
		assert.Equal(t, contract.ID, appendix.ContractID)
	})

	t.Run("Метод IsExpired для приложения", func(t *testing.T) {
		// Создаем тарифный план и договор
		billingPlan := BillingPlan{
			Name:          "План истечения",
			Price:         1000.0,
			Currency:      "RUB",
			BillingPeriod: "monthly",
			IsActive:      true,
		}
		err := db.Create(&billingPlan).Error
		require.NoError(t, err)

		contract := Contract{
			Number:       "EXPIRY-CONTRACT-001",
			Title:        "Договор для истечения",
			ClientName:   "ООО Истечение",
			StartDate:    time.Now(),
			EndDate:      time.Now().AddDate(1, 0, 0),
			TariffPlanID: billingPlan.ID,
			TotalAmount:  decimal.NewFromFloat(12000.0),
			Status:       "active",
			IsActive:     true,
		}
		err = db.Create(&contract).Error
		require.NoError(t, err)

		// Создаем истекшее приложение
		expiredAppendix := ContractAppendix{
			ContractID:  contract.ID,
			Number:      "EXPIRED-APP-001",
			Title:       "Истекшее приложение",
			Description: "Это приложение уже истекло",
			StartDate:   time.Now().AddDate(-3, 0, 0),
			EndDate:     time.Now().AddDate(-1, 0, 0), // Истекло вчера
			Amount:      decimal.NewFromFloat(2000.0),
			Currency:    "RUB",
			Status:      "expired",
			IsActive:    false,
		}
		err = db.Create(&expiredAppendix).Error
		require.NoError(t, err)

		// Тестируем метод IsExpired
		assert.True(t, expiredAppendix.IsExpired())

		// Создаем активное приложение
		activeAppendix := ContractAppendix{
			ContractID:  contract.ID,
			Number:      "ACTIVE-APP-001",
			Title:       "Активное приложение",
			Description: "Это приложение еще активно",
			StartDate:   time.Now().AddDate(-1, 0, 0),
			EndDate:     time.Now().AddDate(0, 3, 0), // Истекает через 3 месяца
			Amount:      decimal.NewFromFloat(3000.0),
			Currency:    "RUB",
			Status:      "active",
			IsActive:    true,
		}
		err = db.Create(&activeAppendix).Error
		require.NoError(t, err)

		// Тестируем метод IsExpired
		assert.False(t, activeAppendix.IsExpired())
	})

	t.Run("Связь приложения с договором", func(t *testing.T) {
		// Создаем тарифный план и договор
		billingPlan := BillingPlan{
			Name:          "План связи",
			Price:         800.0,
			Currency:      "RUB",
			BillingPeriod: "monthly",
			IsActive:      true,
		}
		err := db.Create(&billingPlan).Error
		require.NoError(t, err)

		contract := Contract{
			Number:       "RELATION-CONTRACT-001",
			Title:        "Договор для связи",
			ClientName:   "ООО Связь",
			StartDate:    time.Now(),
			EndDate:      time.Now().AddDate(1, 0, 0),
			TariffPlanID: billingPlan.ID,
			TotalAmount:  decimal.NewFromFloat(9600.0),
			Status:       "active",
			IsActive:     true,
		}
		err = db.Create(&contract).Error
		require.NoError(t, err)

		// Создаем приложение
		appendix := ContractAppendix{
			ContractID:  contract.ID,
			Number:      "RELATION-APP-001",
			Title:       "Приложение для связи",
			Description: "Тестирование связи с договором",
			StartDate:   time.Now(),
			EndDate:     time.Now().AddDate(0, 6, 0),
			Amount:      decimal.NewFromFloat(4800.0),
			Currency:    "RUB",
			Status:      "active",
			IsActive:    true,
		}
		err = db.Create(&appendix).Error
		require.NoError(t, err)

		// Загружаем приложение с договором
		var appendixWithContract ContractAppendix
		err = db.Preload("Contract").First(&appendixWithContract, appendix.ID).Error
		require.NoError(t, err)
		assert.NotNil(t, appendixWithContract.Contract)
		assert.Equal(t, "RELATION-CONTRACT-001", appendixWithContract.Contract.Number)
		assert.Equal(t, "ООО Связь", appendixWithContract.Contract.ClientName)
	})
}

// Вспомогательная функция для создания указателя на float64
func floatPtr(f float64) *float64 {
	return &f
}
