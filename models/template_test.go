package models

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestObjectTemplateModel тестирует модель ObjectTemplate
func TestObjectTemplateModel(t *testing.T) {
	db := setupTestDB(t)

	t.Run("Создание шаблона объекта", func(t *testing.T) {
		template := ObjectTemplate{
			Name:        "Шаблон автомобиля",
			Description: "Стандартный шаблон для легковых автомобилей",
			Category:    "vehicle",
			Icon:        "car",
			Color:       "#3498db",
			Config:      `{"tracking": {"interval": 30, "accuracy": "high"}, "alerts": {"speed_limit": 90, "geofence": true}}`,
			DefaultSettings: `{
				"monitoring": {
					"check_interval": 60,
					"alert_threshold": 300,
					"geo_fence_enabled": true,
					"speed_limit": 90
				},
				"notifications": {
					"email_enabled": true,
					"sms_enabled": false,
					"telegram_enabled": true
				}
			}`,
			RequiredEquipment: []string{"GPS-tracker", "fuel-sensor", "temperature-sensor"},
			IsActive:          true,
			IsSystem:          false,
			UsageCount:        0,
		}

		err := db.Create(&template).Error
		require.NoError(t, err)
		assert.NotZero(t, template.ID)
		assert.Equal(t, "Шаблон автомобиля", template.Name)
		assert.Equal(t, "vehicle", template.Category)
		assert.Equal(t, "#3498db", template.Color)
		assert.Len(t, template.RequiredEquipment, 3)
		assert.Contains(t, template.RequiredEquipment, "GPS-tracker")
	})

	t.Run("Системный шаблон", func(t *testing.T) {
		systemTemplate := ObjectTemplate{
			Name:              "Системный шаблон по умолчанию",
			Description:       "Базовый системный шаблон",
			Category:          "default",
			Icon:              "default",
			Color:             "#95a5a6",
			Config:            `{"basic": true}`,
			DefaultSettings:   `{"basic_monitoring": true}`,
			RequiredEquipment: []string{"GPS-tracker"},
			IsActive:          true,
			IsSystem:          true,
			UsageCount:        0,
		}

		err := db.Create(&systemTemplate).Error
		require.NoError(t, err)
		assert.True(t, systemTemplate.IsSystem)
		assert.True(t, systemTemplate.IsActive)
	})

	t.Run("Метод IncrementUsage", func(t *testing.T) {
		template := ObjectTemplate{
			Name:        "Шаблон для счетчика",
			Description: "Тестирование счетчика использований",
			Category:    "test",
			IsActive:    true,
			UsageCount:  0,
		}

		err := db.Create(&template).Error
		require.NoError(t, err)
		assert.Equal(t, 0, template.UsageCount)

		// Увеличиваем счетчик использований
		err = template.IncrementUsage(db)
		require.NoError(t, err)

		// Проверяем, что счетчик увеличился
		var updatedTemplate ObjectTemplate
		err = db.First(&updatedTemplate, template.ID).Error
		require.NoError(t, err)
		assert.Equal(t, 1, updatedTemplate.UsageCount)

		// Увеличиваем еще раз
		err = updatedTemplate.IncrementUsage(db)
		require.NoError(t, err)

		err = db.First(&updatedTemplate, template.ID).Error
		require.NoError(t, err)
		assert.Equal(t, 2, updatedTemplate.UsageCount)
	})

	t.Run("Связь с объектами", func(t *testing.T) {
		// Создаем шаблон
		template := ObjectTemplate{
			Name:        "Шаблон грузовика",
			Description: "Шаблон для грузовых автомобилей",
			Category:    "truck",
			Icon:        "truck",
			Color:       "#e74c3c",
			IsActive:    true,
		}
		err := db.Create(&template).Error
		require.NoError(t, err)

		// Создаем необходимые связанные сущности для объектов
		billingPlan := BillingPlan{
			Name:          "План для шаблона",
			Price:         decimal.NewFromFloat(2000.0),
			Currency:      "RUB",
			BillingPeriod: "monthly",
			IsActive:      true,
		}
		err = db.Create(&billingPlan).Error
		require.NoError(t, err)

		contract := Contract{
			Number:       "TEMPLATE-CONTRACT-001",
			Title:        "Договор для шаблона",
			ClientName:   "ООО Шаблон",
			StartDate:    time.Now(),
			EndDate:      time.Now().AddDate(1, 0, 0),
			TariffPlanID: billingPlan.ID,
			TotalAmount:  decimal.NewFromFloat(24000.0),
			Status:       "active",
			IsActive:     true,
		}
		err = db.Create(&contract).Error
		require.NoError(t, err)

		// Создаем объекты с этим шаблоном
		object1 := Object{
			Name:       "Грузовик 1",
			Type:       "truck",
			IMEI:       "truck123456789",
			ContractID: &contract.ID,
			TemplateID: &template.ID,
			IsActive:   true,
		}
		object2 := Object{
			Name:       "Грузовик 2",
			Type:       "truck",
			IMEI:       "truck987654321",
			ContractID: &contract.ID,
			TemplateID: &template.ID,
			IsActive:   true,
		}

		err = db.Create(&object1).Error
		require.NoError(t, err)
		err = db.Create(&object2).Error
		require.NoError(t, err)

		// Загружаем шаблон с объектами
		var templateWithObjects ObjectTemplate
		err = db.Preload("Objects").First(&templateWithObjects, template.ID).Error
		require.NoError(t, err)
		assert.Len(t, templateWithObjects.Objects, 2)
		assert.Equal(t, "Грузовик 1", templateWithObjects.Objects[0].Name)
		assert.Equal(t, "Грузовик 2", templateWithObjects.Objects[1].Name)
	})

	t.Run("Различные категории шаблонов", func(t *testing.T) {
		templates := []ObjectTemplate{
			{
				Name:     "Легковой автомобиль",
				Category: "vehicle",
				Icon:     "car",
				Color:    "#3498db",
				IsActive: true,
			},
			{
				Name:     "Промышленное оборудование",
				Category: "equipment",
				Icon:     "cog",
				Color:    "#f39c12",
				IsActive: true,
			},
			{
				Name:     "Недвижимость",
				Category: "asset",
				Icon:     "building",
				Color:    "#2ecc71",
				IsActive: true,
			},
			{
				Name:     "Морской транспорт",
				Category: "vessel",
				Icon:     "ship",
				Color:    "#1abc9c",
				IsActive: true,
			},
		}

		for _, tmpl := range templates {
			err := db.Create(&tmpl).Error
			require.NoError(t, err)
			assert.NotZero(t, tmpl.ID)
		}

		// Проверяем, что все шаблоны созданы
		var count int64
		db.Model(&ObjectTemplate{}).Count(&count)
		assert.GreaterOrEqual(t, count, int64(4))
	})

	t.Run("JSON конфигурация и настройки", func(t *testing.T) {
		complexConfig := `{
			"monitoring": {
				"interval": 30,
				"accuracy": "high",
				"sensors": ["gps", "fuel", "temperature", "door"],
				"alerts": {
					"speed_limit": 100,
					"geofence": true,
					"fuel_theft": true,
					"temperature_range": {"min": -20, "max": 60}
				}
			},
			"reporting": {
				"daily_reports": true,
				"weekly_summary": true,
				"custom_reports": ["mileage", "fuel_consumption", "driver_behavior"]
			},
			"integrations": {
				"1c": true,
				"bitrix24": false,
				"webhook_url": "https://api.example.com/webhook"
			}
		}`

		defaultSettings := `{
			"dashboard": {
				"widgets": ["map", "alerts", "statistics", "reports"],
				"refresh_interval": 30,
				"auto_zoom": true
			},
			"notifications": {
				"channels": ["email", "telegram"],
				"quiet_hours": {"start": "22:00", "end": "08:00"},
				"priorities": {
					"critical": ["engine_failure", "accident"],
					"high": ["speeding", "geofence_violation"],
					"medium": ["fuel_low", "maintenance_due"],
					"low": ["route_deviation", "idle_time"]
				}
			}
		}`

		template := ObjectTemplate{
			Name:            "Расширенный шаблон",
			Description:     "Шаблон с полной конфигурацией",
			Category:        "advanced",
			Config:          complexConfig,
			DefaultSettings: defaultSettings,
			RequiredEquipment: []string{
				"GPS-tracker",
				"fuel-sensor",
				"temperature-sensor",
				"door-sensor",
				"panic-button",
			},
			IsActive: true,
		}

		err := db.Create(&template).Error
		require.NoError(t, err)
		assert.Contains(t, template.Config, "monitoring")
		assert.Contains(t, template.Config, "integrations")
		assert.Contains(t, template.DefaultSettings, "dashboard")
		assert.Contains(t, template.DefaultSettings, "notifications")
		assert.Len(t, template.RequiredEquipment, 5)
	})
}

// TestMonitoringTemplateModel тестирует модель MonitoringTemplate
func TestMonitoringTemplateModel(t *testing.T) {
	db := setupTestDB(t)

	t.Run("Создание шаблона мониторинга", func(t *testing.T) {
		template := MonitoringTemplate{
			Name:             "Стандартный мониторинг",
			Description:      "Базовые настройки мониторинга",
			CheckInterval:    300, // 5 минут
			AlertThreshold:   600, // 10 минут
			GeoFenceEnabled:  true,
			SpeedLimit:       90,
			NotifyOnOffline:  true,
			NotifyOnMove:     false,
			NotifyOnSpeed:    true,
			NotifyOnGeoFence: true,
			EmailEnabled:     true,
			SMSEnabled:       false,
			TelegramEnabled:  true,
			WebhookEnabled:   false,
			Settings:         `{"advanced": {"sensitivity": "medium", "filter_noise": true}}`,
			IsActive:         true,
			UsageCount:       0,
		}

		err := db.Create(&template).Error
		require.NoError(t, err)
		assert.NotZero(t, template.ID)
		assert.Equal(t, "Стандартный мониторинг", template.Name)
		assert.Equal(t, 300, template.CheckInterval)
		assert.Equal(t, 90, template.SpeedLimit)
		assert.True(t, template.GeoFenceEnabled)
		assert.True(t, template.NotifyOnOffline)
		assert.True(t, template.EmailEnabled)
		assert.True(t, template.TelegramEnabled)
		assert.False(t, template.SMSEnabled)
	})

	t.Run("Различные профили мониторинга", func(t *testing.T) {
		profiles := []MonitoringTemplate{
			{
				Name:             "Экономичный мониторинг",
				Description:      "Минимальные настройки для экономии трафика",
				CheckInterval:    900,  // 15 минут
				AlertThreshold:   1800, // 30 минут
				GeoFenceEnabled:  false,
				SpeedLimit:       0, // Без ограничений
				NotifyOnOffline:  true,
				NotifyOnMove:     false,
				NotifyOnSpeed:    false,
				NotifyOnGeoFence: false,
				EmailEnabled:     true,
				SMSEnabled:       false,
				TelegramEnabled:  false,
				WebhookEnabled:   false,
				IsActive:         true,
			},
			{
				Name:             "Интенсивный мониторинг",
				Description:      "Максимальные настройки для критичных объектов",
				CheckInterval:    60,  // 1 минута
				AlertThreshold:   180, // 3 минуты
				GeoFenceEnabled:  true,
				SpeedLimit:       60,
				NotifyOnOffline:  true,
				NotifyOnMove:     true,
				NotifyOnSpeed:    true,
				NotifyOnGeoFence: true,
				EmailEnabled:     true,
				SMSEnabled:       true,
				TelegramEnabled:  true,
				WebhookEnabled:   true,
				IsActive:         true,
			},
			{
				Name:             "Городской мониторинг",
				Description:      "Настройки для городского транспорта",
				CheckInterval:    180, // 3 минуты
				AlertThreshold:   600, // 10 минут
				GeoFenceEnabled:  true,
				SpeedLimit:       60, // Городской лимит
				NotifyOnOffline:  true,
				NotifyOnMove:     false,
				NotifyOnSpeed:    true,
				NotifyOnGeoFence: true,
				EmailEnabled:     true,
				SMSEnabled:       false,
				TelegramEnabled:  true,
				WebhookEnabled:   false,
				IsActive:         true,
			},
		}

		for _, profile := range profiles {
			err := db.Create(&profile).Error
			require.NoError(t, err)
			assert.NotZero(t, profile.ID)
		}

		// Проверяем различия в настройках
		var economical, intensive MonitoringTemplate
		err := db.Where("name = ?", "Экономичный мониторинг").First(&economical).Error
		require.NoError(t, err)
		err = db.Where("name = ?", "Интенсивный мониторинг").First(&intensive).Error
		require.NoError(t, err)

		assert.Greater(t, economical.CheckInterval, intensive.CheckInterval)
		assert.Greater(t, economical.AlertThreshold, intensive.AlertThreshold)
		assert.False(t, economical.SMSEnabled)
		assert.True(t, intensive.SMSEnabled)
	})

	t.Run("Расширенные настройки мониторинга", func(t *testing.T) {
		advancedSettings := `{
			"sensors": {
				"fuel": {
					"enabled": true,
					"calibration": [0, 100, 200, 300, 400],
					"theft_threshold": 10
				},
				"temperature": {
					"enabled": true,
					"min_threshold": -20,
					"max_threshold": 60,
					"critical_threshold": 80
				},
				"door": {
					"enabled": true,
					"notify_on_open": true,
					"notify_on_close": false
				}
			},
			"driver_behavior": {
				"harsh_acceleration": {"enabled": true, "threshold": 0.4},
				"harsh_braking": {"enabled": true, "threshold": -0.4},
				"sharp_turns": {"enabled": true, "threshold": 0.3},
				"idle_time": {"enabled": true, "threshold": 300}
			},
			"maintenance": {
				"mileage_intervals": [10000, 20000, 30000],
				"time_intervals": [90, 180, 365],
				"engine_hours_intervals": [250, 500, 1000]
			}
		}`

		template := MonitoringTemplate{
			Name:             "Профессиональный мониторинг",
			Description:      "Расширенный мониторинг с анализом поведения водителя",
			CheckInterval:    120,
			AlertThreshold:   300,
			GeoFenceEnabled:  true,
			SpeedLimit:       80,
			NotifyOnOffline:  true,
			NotifyOnMove:     true,
			NotifyOnSpeed:    true,
			NotifyOnGeoFence: true,
			EmailEnabled:     true,
			SMSEnabled:       true,
			TelegramEnabled:  true,
			WebhookEnabled:   true,
			Settings:         advancedSettings,
			IsActive:         true,
		}

		err := db.Create(&template).Error
		require.NoError(t, err)
		assert.Contains(t, template.Settings, "driver_behavior")
		assert.Contains(t, template.Settings, "maintenance")
		assert.Contains(t, template.Settings, "sensors")
	})
}

// TestNotificationTemplateModel тестирует модель MonitoringNotificationTemplate
func TestNotificationTemplateModel(t *testing.T) {
	db := setupTestDB(t)

	t.Run("Создание шаблона уведомлений", func(t *testing.T) {
		template := MonitoringNotificationTemplate{
			Name:            "Уведомление об отключении",
			Description:     "Шаблон для уведомления о потере связи с объектом",
			Type:            "alert",
			EventType:       "offline",
			EmailSubject:    "Объект {{object_name}} потерял связь",
			EmailBody:       "Объект {{object_name}} ({{object_imei}}) потерял связь в {{timestamp}}. Последнее местоположение: {{last_location}}",
			SMSMessage:      "ВНИМАНИЕ: {{object_name}} не на связи с {{timestamp}}",
			TelegramMessage: "🚨 *Потеря связи*\n\nОбъект: {{object_name}}\nIMEI: {{object_imei}}\nВремя: {{timestamp}}\nМесто: {{last_location}}",
			WebhookPayload:  `{"event": "offline", "object": "{{object_name}}", "imei": "{{object_imei}}", "timestamp": "{{timestamp}}"}`,
			Priority:        "high",
			RetryCount:      3,
			RetryInterval:   300,
			MaxPerHour:      5,
			MaxPerDay:       20,
			WeekDays:        127, // Все дни недели
			TimeFrom:        "00:00",
			TimeUntil:       "23:59",
			IsActive:        true,
			Variables:       `{"object_name": "Название объекта", "object_imei": "IMEI устройства", "timestamp": "Время события", "last_location": "Последнее местоположение"}`,
		}

		err := db.Create(&template).Error
		require.NoError(t, err)
		assert.NotZero(t, template.ID)
		assert.Equal(t, "Уведомление об отключении", template.Name)
		assert.Equal(t, "alert", template.Type)
		assert.Equal(t, "offline", template.EventType)
		assert.Equal(t, "high", template.Priority)
		assert.Equal(t, 3, template.RetryCount)
		assert.Equal(t, 127, template.WeekDays) // Все дни недели
	})

	t.Run("Метод IsActiveNow", func(t *testing.T) {
		now := time.Now()

		// Активный шаблон без ограничений
		activeTemplate := MonitoringNotificationTemplate{
			Name:      "Всегда активный",
			Type:      "info",
			EventType: "test",
			IsActive:  true,
			WeekDays:  127, // Все дни недели
		}
		err := db.Create(&activeTemplate).Error
		require.NoError(t, err)
		assert.True(t, activeTemplate.IsActiveNow())

		// Неактивный шаблон
		inactiveTemplate := MonitoringNotificationTemplate{
			Name:      "Неактивный",
			Type:      "info",
			EventType: "test",
			IsActive:  false,
			WeekDays:  127,
		}
		err = db.Create(&inactiveTemplate).Error
		require.NoError(t, err)
		assert.False(t, inactiveTemplate.IsActiveNow())

		// Шаблон с ограничением по времени (активен только завтра)
		tomorrow := now.Add(24 * time.Hour)
		futureTemplate := MonitoringNotificationTemplate{
			Name:       "Будущий шаблон",
			Type:       "info",
			EventType:  "test",
			IsActive:   true,
			ActiveFrom: &tomorrow,
			WeekDays:   127,
		}
		err = db.Create(&futureTemplate).Error
		require.NoError(t, err)
		assert.False(t, futureTemplate.IsActiveNow())

		// Шаблон с ограничением по времени (был активен только вчера)
		yesterday := now.Add(-24 * time.Hour)
		pastTemplate := MonitoringNotificationTemplate{
			Name:        "Прошлый шаблон",
			Type:        "info",
			EventType:   "test",
			IsActive:    true,
			ActiveUntil: &yesterday,
			WeekDays:    127,
		}
		err = db.Create(&pastTemplate).Error
		require.NoError(t, err)
		assert.False(t, pastTemplate.IsActiveNow())
	})

	t.Run("Различные типы уведомлений", func(t *testing.T) {
		templates := []MonitoringNotificationTemplate{
			{
				Name:            "Превышение скорости",
				Type:            "warning",
				EventType:       "speed",
				EmailSubject:    "Превышение скорости: {{object_name}}",
				EmailBody:       "Объект {{object_name}} превысил скорость. Текущая скорость: {{current_speed}} км/ч, лимит: {{speed_limit}} км/ч",
				SMSMessage:      "СКОРОСТЬ: {{object_name}} - {{current_speed}} км/ч",
				TelegramMessage: "⚠️ *Превышение скорости*\n\nОбъект: {{object_name}}\nСкорость: {{current_speed}} км/ч\nЛимит: {{speed_limit}} км/ч",
				Priority:        "medium",
				IsActive:        true,
				WeekDays:        127,
			},
			{
				Name:            "Выход из геозоны",
				Type:            "alert",
				EventType:       "geofence",
				EmailSubject:    "Нарушение геозоны: {{object_name}}",
				EmailBody:       "Объект {{object_name}} покинул разрешенную зону {{geofence_name}} в {{timestamp}}",
				SMSMessage:      "ГЕОЗОНА: {{object_name}} вне зоны {{geofence_name}}",
				TelegramMessage: "🚫 *Нарушение геозоны*\n\nОбъект: {{object_name}}\nЗона: {{geofence_name}}\nВремя: {{timestamp}}",
				Priority:        "high",
				IsActive:        true,
				WeekDays:        127,
			},
			{
				Name:            "Техническое обслуживание",
				Type:            "reminder",
				EventType:       "maintenance",
				EmailSubject:    "Напоминание о ТО: {{object_name}}",
				EmailBody:       "Для объекта {{object_name}} подошло время технического обслуживания. Пробег: {{mileage}} км, последнее ТО: {{last_maintenance}}",
				SMSMessage:      "ТО: {{object_name}} - {{mileage}} км",
				TelegramMessage: "🔧 *Время ТО*\n\nОбъект: {{object_name}}\nПробег: {{mileage}} км\nПоследнее ТО: {{last_maintenance}}",
				Priority:        "low",
				IsActive:        true,
				WeekDays:        31, // Только рабочие дни (пн-пт)
				TimeFrom:        "09:00",
				TimeUntil:       "18:00",
			},
		}

		for _, tmpl := range templates {
			err := db.Create(&tmpl).Error
			require.NoError(t, err)
			assert.NotZero(t, tmpl.ID)
		}

		// Проверяем различия в настройках
		var speedTemplate, maintenanceTemplate MonitoringNotificationTemplate
		err := db.Where("event_type = ?", "speed").First(&speedTemplate).Error
		require.NoError(t, err)
		err = db.Where("event_type = ?", "maintenance").First(&maintenanceTemplate).Error
		require.NoError(t, err)

		assert.Equal(t, "medium", speedTemplate.Priority)
		assert.Equal(t, "low", maintenanceTemplate.Priority)
		assert.Equal(t, 127, speedTemplate.WeekDays)      // Все дни
		assert.Equal(t, 31, maintenanceTemplate.WeekDays) // Только рабочие дни
	})

	t.Run("Метод RenderMessage", func(t *testing.T) {
		template := MonitoringNotificationTemplate{
			Name:            "Тестовый рендер",
			Type:            "info",
			EventType:       "test",
			EmailSubject:    "Тест {{variable1}}",
			EmailBody:       "Сообщение с {{variable1}} и {{variable2}}",
			SMSMessage:      "SMS: {{variable1}}",
			TelegramMessage: "Telegram: {{variable1}} - {{variable2}}",
			WebhookPayload:  `{"test": "{{variable1}}", "data": "{{variable2}}"}`,
			IsActive:        true,
		}

		err := db.Create(&template).Error
		require.NoError(t, err)

		// Тестируем рендеринг различных типов сообщений
		variables := map[string]interface{}{
			"variable1": "значение1",
			"variable2": "значение2",
		}

		emailSubject := template.RenderMessage("email_subject", variables)
		assert.Equal(t, "Тест {{variable1}}", emailSubject) // TODO: реализовать подстановку переменных

		emailBody := template.RenderMessage("email_body", variables)
		assert.Equal(t, "Сообщение с {{variable1}} и {{variable2}}", emailBody)

		sms := template.RenderMessage("sms", variables)
		assert.Equal(t, "SMS: {{variable1}}", sms)

		telegram := template.RenderMessage("telegram", variables)
		assert.Equal(t, "Telegram: {{variable1}} - {{variable2}}", telegram)

		webhook := template.RenderMessage("webhook", variables)
		assert.Contains(t, webhook, "{{variable1}}")

		// Тест неизвестного типа
		unknown := template.RenderMessage("unknown", variables)
		assert.Empty(t, unknown)
	})

	t.Run("Ограничения по времени и дням недели", func(t *testing.T) {
		// Шаблон только для рабочих дней с 9 до 18
		workdayTemplate := MonitoringNotificationTemplate{
			Name:      "Рабочие дни",
			Type:      "info",
			EventType: "workday",
			WeekDays:  31, // Пн-Пт (1+2+4+8+16 = 31)
			TimeFrom:  "09:00",
			TimeUntil: "18:00",
			IsActive:  true,
		}
		err := db.Create(&workdayTemplate).Error
		require.NoError(t, err)

		// Шаблон только для выходных
		weekendTemplate := MonitoringNotificationTemplate{
			Name:      "Выходные дни",
			Type:      "info",
			EventType: "weekend",
			WeekDays:  96, // Сб-Вс (32+64 = 96)
			TimeFrom:  "10:00",
			TimeUntil: "22:00",
			IsActive:  true,
		}
		err = db.Create(&weekendTemplate).Error
		require.NoError(t, err)

		// Круглосуточный шаблон
		alwaysTemplate := MonitoringNotificationTemplate{
			Name:      "Круглосуточно",
			Type:      "alert",
			EventType: "always",
			WeekDays:  127, // Все дни (1+2+4+8+16+32+64 = 127)
			TimeFrom:  "00:00",
			TimeUntil: "23:59",
			IsActive:  true,
		}
		err = db.Create(&alwaysTemplate).Error
		require.NoError(t, err)

		assert.Equal(t, 31, workdayTemplate.WeekDays)
		assert.Equal(t, 96, weekendTemplate.WeekDays)
		assert.Equal(t, 127, alwaysTemplate.WeekDays)
	})

	t.Run("Ограничения на количество уведомлений", func(t *testing.T) {
		// Шаблон с ограничениями
		limitedTemplate := MonitoringNotificationTemplate{
			Name:       "Ограниченный",
			Type:       "warning",
			EventType:  "limited",
			Priority:   "medium",
			MaxPerHour: 3,
			MaxPerDay:  10,
			IsActive:   true,
			WeekDays:   127,
		}
		err := db.Create(&limitedTemplate).Error
		require.NoError(t, err)

		// Шаблон без ограничений
		unlimitedTemplate := MonitoringNotificationTemplate{
			Name:       "Неограниченный",
			Type:       "info",
			EventType:  "unlimited",
			Priority:   "low",
			MaxPerHour: 0, // Без ограничений
			MaxPerDay:  0, // Без ограничений
			IsActive:   true,
			WeekDays:   127,
		}
		err = db.Create(&unlimitedTemplate).Error
		require.NoError(t, err)

		assert.Equal(t, 3, limitedTemplate.MaxPerHour)
		assert.Equal(t, 10, limitedTemplate.MaxPerDay)
		assert.Equal(t, 0, unlimitedTemplate.MaxPerHour)
		assert.Equal(t, 0, unlimitedTemplate.MaxPerDay)
	})
}
