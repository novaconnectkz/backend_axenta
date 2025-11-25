package main

import (
	"backend_axenta/api"
	"backend_axenta/audit"
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/handlers"
	"backend_axenta/middleware"
	"backend_axenta/services"

	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func main() {
	log.Println("Starting Axenta Backend Server...")
	log.Println("🔧 DEBUG: Main function started")

	// Загружаем конфигурацию
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Выводим конфигурацию в лог
	cfg.LogConfig()

	// Создаем базу данных если её нет
	if err := database.CreateDatabaseIfNotExists(); err != nil {
		log.Fatalf("Failed to create database: %v", err)
	}

	// Подключаемся к базе данных
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Инициализируем аудит-логирование
	log.Println("🔧 Initializing audit logging system...")
	auditCfg := &audit.Config{
		Enabled:     cfg.Audit.Enabled,
		LogFilePath: cfg.Audit.LogFilePath,
		LogToStdout: cfg.Audit.LogToStdout,
		LogToFile:   cfg.Audit.LogToFile,
		MaxFileSize: cfg.Audit.MaxFileSize,
		MaxBackups:  cfg.Audit.MaxBackups,
	}
	
	if cfg.Audit.LogToDB {
		// Создаем таблицу для аудит-логов
		if err := database.DB.AutoMigrate(&audit.AuditLog{}); err != nil {
			log.Printf("Warning: Failed to create audit_logs table: %v", err)
		} else {
			log.Println("✅ Audit logs table created/verified")
		}
		
		// Инициализируем логгер с поддержкой БД
		dbLogger, err := audit.NewDBLogger(auditCfg, database.DB)
		if err != nil {
			log.Printf("Warning: Failed to initialize audit DB logger: %v", err)
		} else {
			audit.SetGlobalDBLogger(dbLogger)
			log.Println("✅ Audit logging with database support initialized")
		}
	} else {
		// Инициализируем базовый файловый логгер
		if err := audit.Init(auditCfg); err != nil {
			log.Printf("Warning: Failed to initialize audit logger: %v", err)
		} else {
			log.Println("✅ Audit logging initialized (file only)")
		}
	}

	// Инициализируем Redis
	if err := database.InitRedis(); err != nil {
		log.Printf("Warning: Failed to connect to Redis: %v", err)
		log.Println("Continuing without Redis caching...")
	}

	// Временно отключаем Integration Service для запуска сервера
	// integrationService, err := services.NewIntegrationService(axetnaBaseURL, nil)
	// if err != nil {
	// 	log.Fatalf("Failed to initialize Integration Service: %v", err)
	// }
	// services.SetIntegrationService(integrationService)
	log.Println("⚠️ Integration Service temporarily disabled")

	// Инициализируем сервис интеграции с Битрикс24
	// api.InitBitrix24Service() // Временно отключено из-за ошибок компиляции
	log.Println("⚠️ Bitrix24 Integration Service temporarily disabled")

	// Инициализируем сервис интеграции с 1С
	api.InitOneCService()
	log.Println("✅ 1C Integration Service initialized successfully")
	
	// Инициализируем сервис интеграции с Telegram
	api.InitTelegramService()
	log.Println("✅ Telegram Integration Service initialized successfully")
	
	// Инициализируем сервис интеграции с MAX
	api.InitMaxService()
	log.Println("✅ MAX Integration Service initialized successfully")

	// Инициализируем сервис синхронизации Axenta
	axentaSyncService := services.NewAxentaSyncService(database.DB)
	axentaSyncScheduler := services.NewAxentaSyncScheduler(axentaSyncService)
	if err := axentaSyncScheduler.Start(); err != nil {
		log.Printf("⚠️ Axenta Sync Scheduler failed to start: %v", err)
	} else {
		services.SetAxentaSyncScheduler(axentaSyncScheduler)
		defer axentaSyncScheduler.Stop()
	}

	// Инициализируем систему уведомлений - временно отключено
	// cache := services.NewCacheService(database.RedisClient, log.New(log.Writer(), "CACHE: ", log.LstdFlags))
	// notificationService := services.NewNotificationService(database.DB, cache)
	// _ = services.NewNotificationFallbackService(database.DB, notificationService) // fallbackService для будущего использования
	// notificationAPI := api.NewNotificationAPI(notificationService)
	log.Println("⚠️ Notification System temporarily disabled")

	// Выполняем миграции для основных таблиц (не мультитенантных)
	// Миграции выполняются в database.ConnectDatabase() через RunAllMigrations()
	// LocalUser и RefreshToken теперь включены в систему миграций как глобальные таблицы

	// Создаем сервисы
	jwtService := services.NewJWTService(database.DB)

	// Инициализируем роли по умолчанию для Axenta пользователей в схеме public
	// Так как /api/auth endpoints работают без мультитенантности
	log.Println("🔧 Initializing default Axenta roles in public schema...")
	axentaUserService := services.NewAxentaUserService(database.DB)
	if err := axentaUserService.EnsureDefaultRoles(); err != nil {
		log.Printf("Warning: Failed to ensure default Axenta roles: %v", err)
	} else {
		log.Println("✅ Default Axenta roles initialized successfully in public schema")
	}

	// Создаем middleware для мультитенантности
	tenantMiddleware := middleware.NewTenantMiddleware(database.DB)

	// Создаем middleware для аутентификации
	authMiddleware := middleware.NewAuthMiddleware()
	// localAuthMiddleware := middleware.NewLocalAuthMiddleware(jwtService) // Создается в API

	// Создаем middleware для проверки API токенов (не используется в текущей реализации)
	// apiTokensMiddleware := middleware.NewAxentaAPITokensMiddleware()

	r := gin.Default()

	// Отключаем автоматические редиректы для trailing slash
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false

	// Добавляем audit middleware для логирования всех запросов
	if cfg.Audit.Enabled {
		r.Use(audit.Middleware())
		log.Println("✅ Audit middleware enabled for all routes")
	}

	// Настройка CORS
	corsConfig := middleware.CustomCORSConfig{
		AllowOrigins: []string{
			"http://localhost:3000",
			"http://127.0.0.1:3000",
			"http://localhost:3001",
			"http://127.0.0.1:3001",
			"http://localhost:3002",
			"http://127.0.0.1:3002",
			"http://localhost:3003",
			"http://127.0.0.1:3003",
			"https://axenta.glonass-saratov.ru",
			"http://axenta.glonass-saratov.ru",
			"https://api.axenta.glonass-saratov.ru",
			"http://api.axenta.glonass-saratov.ru",
		},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders: []string{
			"Origin",
			"Content-Length",
			"Content-Type",
			"Authorization",
			"authorization",
			"X-Tenant-ID",
			"Cache-Control",
			"Pragma",
			"Accept",
			"X-Requested-With",
		},
		ExposeHeaders: []string{
			"Content-Length",
			"Access-Control-Allow-Origin",
			"Access-Control-Allow-Headers",
			"Cache-Control",
			"Content-Language",
			"Content-Type",
			"Expires",
			"Last-Modified",
			"Pragma",
		},
		AllowCredentials: true,
		MaxAge:           12 * 3600, // 12 часов кеширования preflight запросов
	}
	r.Use(middleware.CustomCORS(corsConfig))

	// Публичные маршруты (без проверки tenant)
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "success", "message": "pong"})
	})
	r.POST("/api/auth/login", api.Login)
	
	// Документация по интеграциям (публичный доступ)
	r.GET("/docs/TELEGRAM_INTEGRATION.md", api.GetTelegramIntegrationDocs)
	r.GET("/api/docs/telegram", api.GetTelegramIntegrationDocs) // Альтернативный маршрут
	r.GET("/docs/EMAIL_INTEGRATION.md", api.GetEmailIntegrationDocs)
	r.GET("/api/docs/email", api.GetEmailIntegrationDocs) // Альтернативный маршрут

	// Публичные тестовые endpoints для отладки ролей
	r.GET("/api/debug/roles", api.TestRolesCreation)
	r.GET("/api/debug/user-role", api.TestUserWithRole)

	// Публичные endpoints для ролей и шаблонов пользователей (без аутентификации)
	r.GET("/api/public/roles", api.GetRolesPublic)
	r.GET("/api/public/user-templates", api.GetUserTemplatesPublic)

	// Тестовый endpoint для создания пользователя в Axenta Cloud (без проверки токена)
	r.POST("/api/test/cms/users", api.TestCreateUserInAxenta)
	// Endpoint для создания пользователя напрямую в локальной базе (без проверки токена)
	r.POST("/api/create/user", api.CreateUserDirectly)
	// Простой тестовый endpoint
	r.GET("/api/test", api.TestEndpoint)
	// Еще один тестовый endpoint
	r.GET("/api/test2", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "success", "message": "Test endpoint 2 is working"})
	})
	// ВРЕМЕННЫЙ тестовый endpoint для создания нумератора без авторизации (только для тестирования)
	r.POST("/api/test/contract-numerators", api.CreateContractNumerator)
	// Тестовый endpoint для создания пользователя без проверки токена
	r.POST("/api/test-cms-users", api.CreateCmsUserWithCurrentToken)
	// Основной endpoint для создания пользователей CMS
	r.POST("/api/create-cms-user", api.CreateCmsUserWithCurrentToken)
	// Endpoint для создания пользователей CMS без проверки Axenta токенов
	r.POST("/api/cms/create-user", api.CreateCmsUserWithCurrentToken)
	// Endpoint для создания пользователей CMS с проверкой сохраненного токена
	r.POST("/api/cms/create-user-with-saved-token", api.CreateCmsUserWithCurrentToken)

	// === ЛОКАЛЬНАЯ АВТОРИЗАЦИЯ ===
	localAuthAPI := api.NewLocalAuthAPI(database.DB, jwtService)
	localAuthAPI.RegisterRoutes(r.Group("/api"))

	// === WEBSOCKET С АВТОРИЗАЦИЕЙ ===
	wsAPI := api.NewWebSocketAuthAPI(jwtService)
	wsAPI.RegisterRoutes(r.Group("/ws"))

	// Тестовый маршрут для проверки
	r.GET("/api/test-objects-stats", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "success",
			"message": "Test route works!",
			"data": gin.H{
				"total":                0,
				"active":               0,
				"inactive":             0,
				"scheduled_for_delete": 0,
			},
		})
	})

	// Временное in-memory хранилище для объектов
	var mockObjects []gin.H
	var mockObjectID int = 1

	// Демо данные очищены - теперь список объектов пустой

	// Вспомогательная функция для парсинга ID
	parseID := func(idStr string) int {
		if id, err := strconv.Atoi(idStr); err == nil {
			return id
		}
		return 0
	}

	// Временные публичные маршруты для объектов (для тестирования фронтенда)
	getObjectsHandler := func(c *gin.Context) {
		// Добавляем заголовки для предотвращения кеширования
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")

		// Проксируем к Axenta Cloud API напрямую
		api.GetObjectsFromAxentaCloud(c)
	}
	r.GET("/api/objects", getObjectsHandler)
	r.GET("/api/objects/", getObjectsHandler)

	// УДАЛЕНО: Публичные CMS эндпоинты для безопасности
	// Теперь все запросы объектов требуют аутентификации
	// r.GET("/api/cms/objects", getObjectsHandler)
	// r.GET("/api/cms/objects/", getObjectsHandler)

	// Временный публичный маршрут для получения одного объекта (для тестирования фронтенда)
	getObjectHandler := func(c *gin.Context) {
		// Получаем ID объекта
		objectID := c.Param("id")
		id := parseID(objectID)

		// Добавляем заголовки для предотвращения кеширования
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")

		// Ищем объект в нашем временном хранилище
		for _, obj := range mockObjects {
			if objID, ok := obj["id"].(int); ok && objID == id {
				c.JSON(200, gin.H{
					"status": "success",
					"data":   obj,
				})
				return
			}
		}

		c.JSON(404, gin.H{"status": "error", "error": "Объект не найден"})
	}
	r.GET("/api/objects/:id", getObjectHandler)
	r.GET("/api/objects/:id/", getObjectHandler)

	getObjectsStatsHandler := func(c *gin.Context) {
		// Добавляем заголовки для предотвращения кеширования
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")

		// Проксируем к Axenta Cloud API для статистики
		api.GetObjectsStatsFromAxentaCloud(c)
	}
	r.GET("/api/objects/stats", getObjectsStatsHandler)
	r.GET("/api/objects/stats/", getObjectsStatsHandler)

	// УДАЛЕНО: Публичные CMS эндпоинты статистики для безопасности
	// Теперь вся статистика объектов требует аутентификации
	// r.GET("/api/cms/objects/stats", getObjectsStatsHandler)
	// r.GET("/api/cms/objects/stats/", getObjectsStatsHandler)

	getObjectTemplatesHandler := func(c *gin.Context) {
		// Добавляем заголовки для предотвращения кеширования
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.JSON(200, gin.H{
			"status": "success",
			"data": gin.H{
				"items":    []gin.H{},
				"total":    0,
				"page":     1,
				"per_page": 50,
			},
		})
	}
	r.GET("/api/object-templates", getObjectTemplatesHandler)
	r.GET("/api/object-templates/", getObjectTemplatesHandler)

	// Временный публичный маршрут для создания объектов (для тестирования фронтенда)
	createObjectHandler := func(c *gin.Context) {
		// Парсим данные из запроса
		var requestData gin.H
		if err := c.ShouldBindJSON(&requestData); err != nil {
			c.JSON(400, gin.H{"status": "error", "error": "Некорректные данные: " + err.Error()})
			return
		}

		// Создаем новый объект
		newObject := gin.H{
			"id":         mockObjectID,
			"name":       requestData["name"],
			"type":       requestData["type"],
			"status":     "active",
			"created_at": "2025-09-24T07:40:00Z",
		}

		// Добавляем в хранилище
		mockObjects = append(mockObjects, newObject)
		mockObjectID++

		c.JSON(200, gin.H{
			"status":  "success",
			"data":    newObject,
			"message": "Объект успешно создан",
		})
	}
	r.POST("/api/objects", createObjectHandler)
	r.POST("/api/objects/", createObjectHandler)

	// Временный публичный маршрут для создания шаблона из объекта
	createTemplateHandler := func(c *gin.Context) {
		// Получаем ID объекта
		objectID := c.Param("id")

		// Парсим данные для нового шаблона
		var templateData gin.H
		if err := c.ShouldBindJSON(&templateData); err != nil {
			c.JSON(400, gin.H{"status": "error", "error": "Некорректные данные: " + err.Error()})
			return
		}

		// Ищем объект в нашем временном хранилище
		var foundObject gin.H
		for _, obj := range mockObjects {
			if objID, ok := obj["id"].(int); ok && objID == parseID(objectID) {
				foundObject = obj
				break
			}
		}

		if foundObject == nil {
			c.JSON(404, gin.H{"status": "error", "error": "Объект не найден"})
			return
		}

		// Создаем шаблон на основе объекта
		template := gin.H{
			"id":          len(mockObjects) + 100, // Уникальный ID для шаблона
			"name":        templateData["name"],
			"description": templateData["description"],
			"category":    templateData["category"],
			"icon":        templateData["icon"],
			"color":       templateData["color"],
			"type":        foundObject["type"],
			"is_active":   true,
			"is_system":   false,
			"usage_count": 0,
			"created_at":  "2025-09-24T07:40:00Z",
			"updated_at":  "2025-09-24T07:40:00Z",
		}

		c.JSON(201, gin.H{
			"status":  "success",
			"message": "Шаблон успешно создан на основе объекта (демо режим)",
			"data":    template,
		})
	}
	r.POST("/api/objects/:id/create-template", createTemplateHandler)
	r.POST("/api/objects/:id/create-template/", createTemplateHandler)

	// Временный публичный маршрут для обновления объектов (для тестирования фронтенда)
	updateObjectHandler := func(c *gin.Context) {
		// Получаем ID объекта
		objectID := c.Param("id")
		id := parseID(objectID)

		// Парсим данные из запроса
		var requestData gin.H
		if err := c.ShouldBindJSON(&requestData); err != nil {
			c.JSON(400, gin.H{"status": "error", "error": "Некорректные данные: " + err.Error()})
			return
		}

		// Ищем объект в нашем временном хранилище
		var foundIndex = -1
		for i, obj := range mockObjects {
			if objID, ok := obj["id"].(int); ok && objID == id {
				foundIndex = i
				break
			}
		}

		if foundIndex == -1 {
			c.JSON(404, gin.H{"status": "error", "error": "Объект не найден"})
			return
		}

		// Обновляем объект
		updatedObject := mockObjects[foundIndex]
		for key, value := range requestData {
			updatedObject[key] = value
		}
		updatedObject["updated_at"] = "2025-09-24T07:40:00Z"
		mockObjects[foundIndex] = updatedObject

		c.JSON(200, gin.H{
			"status":  "success",
			"data":    updatedObject,
			"message": "Объект успешно обновлен",
		})
	}
	r.PUT("/api/objects/:id", updateObjectHandler)
	r.PUT("/api/objects/:id/", updateObjectHandler)

	// Dashboard endpoints (перемещены в apiGroup для поддержки мультитенантности)
	// Dashboard для биллинга согласно roadmap (Этап 4.2) - остается публичным
	r.GET("/api/dashboard", api.GetBillingDashboard)

	// OpenAPI документация (Этап 9)
	r.GET("/api/docs", api.GetSwaggerUI)
	r.GET("/api/docs/openapi.yaml", api.GetOpenAPISpec)
	r.GET("/api/docs/billing-openapi.yaml", api.GetBillingOpenAPISpec)
	// Алиас для совместимости
	r.GET("/docs", api.GetSwaggerUI)

	// Простые billing endpoints для отладки (без мультитенантности)
	r.GET("/api/billing-plans-simple", api.GetBillingPlansSimple)
	r.GET("/api/subscriptions-simple", api.GetSubscriptionsSimple)
	r.GET("/api/billing-settings-simple", api.GetBillingSettingsSimple)

	// Административные маршруты (с авторизацией)
	adminGroup := r.Group("/api/admin")
	adminGroup.Use(authMiddleware.RequireAuth()) // Требуем авторизацию для админ маршрутов
	{
		// Управление учетными записями (компаниями)
		companiesAPI := api.NewCompaniesAPI(database.DB, tenantMiddleware)
		companiesAPI.RegisterCompaniesRoutes(adminGroup)
	}

	// Временные endpoints без мультитенантности для тестирования
	testGroup := r.Group("/api/test")
	{
		installationAPI := api.NewInstallationAPI(database.DB)
		testGroup.GET("/installations", installationAPI.GetInstallations)
		testGroup.GET("/installations/statistics", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"total":           0,
				"today":           0,
				"overdue":         0,
				"completion_rate": 0.0,
			})
		})

		installerAPI := api.NewInstallerAPI(database.DB)
		testGroup.GET("/installers", installerAPI.GetInstallers)

		locationAPI := api.NewLocationAPI(database.DB)
		testGroup.GET("/locations", locationAPI.GetLocations)

		equipmentAPI := api.NewEquipmentAPI(database.DB)
		testGroup.GET("/equipment", equipmentAPI.GetEquipment)
	}

	// Группа API с аутентификацией и мультитенантностью
	log.Println("🔧 Registering authenticated API endpoints...")
	apiGroup := r.Group("/api/auth")

	// Создаем middleware для локальной авторизации (используется в других местах)
	_ = middleware.NewLocalAuthMiddleware(jwtService)

	// Подключаем Axenta авторизацию и мультитенантность до регистрации маршрутов
	apiGroup.Use(
		authMiddleware.RequireAuth(),
		tenantMiddleware.SetTenant(),
	)
	log.Println("✅ Axenta Cloud authentication enabled for /api/auth endpoints (with multitenancy)")

	// Отдельная группа для CMS endpoints без проверки Axenta токенов
	log.Println("🔧 Registering CMS endpoints without Axenta authentication...")
	cmsGroup := r.Group("/api/cms")
	// Не используем authMiddleware.RequireAuth() для CMS endpoints
	cmsGroup.POST("/users", api.CreateCmsUserWithCurrentToken)
	cmsGroup.POST("/users/", api.CreateCmsUserWithCurrentToken)
	cmsGroup.POST("/users/create", api.CreateCmsUserWithAdminToken)
	cmsGroup.POST("/users/create/", api.CreateCmsUserWithAdminToken)
	cmsGroup.POST("/users/login_as", api.LoginAs)
	cmsGroup.POST("/users/login_as/", api.LoginAs)
	cmsGroup.POST("/users/:id/activate/", api.ActivateUser)
	cmsGroup.GET("/test", api.TestEndpoint)

	// Добавляем обработчик для перемещения учетных записей
	accountsHandler := handlers.NewAccountsHandler()
	cmsGroup.POST("/accounts/change_account", accountsHandler.MoveAccount)
	cmsGroup.POST("/accounts/change_account/", accountsHandler.MoveAccount)

	log.Println("✅ CMS endpoints registered without Axenta authentication")

	// Объекты (с аутентификацией) - проксирование к Axenta Cloud
	log.Println("🔧 Registering Axenta Cloud proxy endpoints...")
	apiGroup.GET("/objects", api.GetObjectsFromAxentaCloud)
	apiGroup.GET("/objects/", api.GetObjectsFromAxentaCloud)
	apiGroup.GET("/objects/export", api.ExportObjectsToXLSX)
	log.Printf("✅ Зарегистрирован GET /api/auth/objects/export -> ExportObjectsToXLSX")
	apiGroup.GET("/objects/export/", api.ExportObjectsToXLSX)
	apiGroup.GET("/objects/stats", api.GetObjectsStatsFromAxentaCloud)
	apiGroup.GET("/objects/stats/", api.GetObjectsStatsFromAxentaCloud)
	apiGroup.GET("/objects/stats/optimized", api.GetObjectsStatsOptimizedFromAxentaCloud)
	apiGroup.GET("/objects/stats/optimized/", api.GetObjectsStatsOptimizedFromAxentaCloud)

	// Добавляем поддержку CMS эндпоинтов для совместимости с фронтендом
	apiGroup.GET("/cms/objects", api.GetObjectsFromAxentaCloud)
	apiGroup.GET("/cms/objects/", api.GetObjectsFromAxentaCloud)
	apiGroup.GET("/cms/objects/export", api.ExportObjectsToXLSX)
	log.Printf("✅ Зарегистрирован GET /api/auth/cms/objects/export -> ExportObjectsToXLSX")
	apiGroup.GET("/cms/objects/export/", api.ExportObjectsToXLSX)
	apiGroup.GET("/cms/objects/stats", api.GetObjectsStatsFromAxentaCloud)
	apiGroup.GET("/cms/objects/stats/", api.GetObjectsStatsFromAxentaCloud)
	apiGroup.GET("/objects/:id", api.GetObject)
	apiGroup.GET("/objects/:id/", api.GetObject)
	apiGroup.POST("/objects", api.CreateObject)
	apiGroup.POST("/objects/", api.CreateObject)
	apiGroup.PUT("/objects/:id", api.UpdateObject)
	apiGroup.PUT("/objects/:id/", api.UpdateObject)
	apiGroup.DELETE("/objects/:id", api.DeleteObject)
	apiGroup.DELETE("/objects/:id/", api.DeleteObject)

	// Плановое удаление объектов
	apiGroup.PUT("/objects/:id/schedule-delete", api.ScheduleObjectDelete)
	apiGroup.PUT("/objects/:id/schedule-delete/", api.ScheduleObjectDelete)
	apiGroup.PUT("/objects/:id/cancel-delete", api.CancelScheduledDelete)
	apiGroup.PUT("/objects/:id/cancel-delete/", api.CancelScheduledDelete)

	// Корзина для объектов
	apiGroup.GET("/objects-trash", api.GetDeletedObjects)
	apiGroup.GET("/objects-trash/", api.GetDeletedObjects)
	apiGroup.PUT("/objects/:id/restore", api.RestoreObject)
	apiGroup.PUT("/objects/:id/restore/", api.RestoreObject)
	apiGroup.DELETE("/objects/:id/permanent", api.PermanentDeleteObject)
	apiGroup.DELETE("/objects/:id/permanent/", api.PermanentDeleteObject)

	// CMS эндпоинты для корзины (совместимость с фронтендом) - проксирование к Axenta Cloud
	apiGroup.GET("/cms/trash", api.GetDeletedObjectsFromAxentaCloud)
	apiGroup.GET("/cms/trash/", api.GetDeletedObjectsFromAxentaCloud)

	// Шаблоны объектов - временно отключено
	// apiGroup.GET("/object-templates", api.GetObjectTemplates)
	// apiGroup.GET("/object-templates/:id", api.GetObjectTemplate)
	// apiGroup.POST("/object-templates", api.CreateObjectTemplate)
	// apiGroup.PUT("/object-templates/:id", api.UpdateObjectTemplate)
	// apiGroup.DELETE("/object-templates/:id", api.DeleteObjectTemplate)
	// apiGroup.POST("/objects/:id/create-template", api.CreateTemplateFromObject)

	// Временный эндпоинт для шаблонов объектов (возвращает пустой список)
	apiGroup.GET("/object-templates", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "success",
			"data": gin.H{
				"items":       []gin.H{},
				"total":       0,
				"page":        1,
				"per_page":    50,
				"total_pages": 0,
			},
		})
	})
	apiGroup.GET("/object-templates/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "success",
			"data": gin.H{
				"items":       []gin.H{},
				"total":       0,
				"page":        1,
				"per_page":    50,
				"total_pages": 0,
			},
		})
	})

	// Пользователи (прокси к Axenta Cloud API)
	log.Println("🔧 Registering users proxy endpoints...")
	apiGroup.GET("/users", api.GetUsersFromAxentaCloud)
	apiGroup.GET("/users/", api.GetUsersFromAxentaCloud)
	apiGroup.POST("/users", api.CreateUserInAxentaCloud)
	apiGroup.POST("/users/", api.CreateUserInAxentaCloud)

	// Публичные маршруты для создания пользователей (без проверки auth)
	log.Println("🔧 Registering public users endpoints...")
	r.POST("/api/users", api.CreateUserInAxentaCloud)
	r.POST("/api/users/", api.CreateUserInAxentaCloud)
	apiGroup.GET("/users/stats", api.GetUsersStatsFromAxentaCloud)
	apiGroup.GET("/users/stats/", api.GetUsersStatsFromAxentaCloud)
	apiGroup.GET("/users/stats/optimized", api.GetUsersStatsOptimizedFromAxentaCloud)
	apiGroup.GET("/users/stats/optimized/", api.GetUsersStatsOptimizedFromAxentaCloud)

	// CMS endpoints для пользователей
	apiGroup.GET("/cms/users", api.GetUsersFromAxentaCloud)
	apiGroup.GET("/cms/users/", api.GetUsersFromAxentaCloud)
	apiGroup.GET("/cms/users/stats", api.GetUsersStatsFromAxentaCloud)
	apiGroup.GET("/cms/users/stats/", api.GetUsersStatsFromAxentaCloud)
	apiGroup.GET("/cms/users/:id", api.GetUser)
	apiGroup.GET("/cms/users/:id/", api.GetUser)
	apiGroup.POST("/cms/update_user_password/", api.UpdateUserPassword)

	// CMS endpoints для создания пользователей (закомментированы, используем публичные)
	// apiGroup.POST("/cms/users", api.CreateCmsUserWithCurrentToken)
	// apiGroup.POST("/cms/users/", api.CreateCmsUserWithCurrentToken)

	// Локальные endpoints для управления пользователями (создание, редактирование)
	apiGroup.GET("/local/users/:id", api.GetUser)
	apiGroup.POST("/local/users", api.CreateUser)
	apiGroup.POST("/local/users/check-username", api.CheckUsername)

	// Учетные записи (прокси к Axenta API)
	log.Println("🔧 Registering accounts proxy endpoints...")
	accountsHandler = handlers.NewAccountsHandler()
	apiGroup.GET("/accounts", accountsHandler.GetAccounts)
	apiGroup.GET("/accounts/", accountsHandler.GetAccounts)
	apiGroup.POST("/accounts", accountsHandler.CreateAccount)
	apiGroup.POST("/accounts/", accountsHandler.CreateAccount)
	apiGroup.GET("/accounts/:id", accountsHandler.GetAccount)
	apiGroup.GET("/accounts/:id/", accountsHandler.GetAccount)

	// Административные эндпоинты для аккаунтов (совместимость с фронтендом)
	apiGroup.GET("/admin/accounts/list", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "success",
			"data": []gin.H{
				{
					"id":        1,
					"name":      "Демо компания",
					"is_active": true,
				},
			},
		})
	})
	apiGroup.PUT("/users/:id", api.UpdateUser)
	apiGroup.DELETE("/users/:id", api.DeleteUser)
	apiGroup.POST("/users/bulk-delete", api.BulkDeleteUsers)

	// Роли
	apiGroup.GET("/roles", api.GetRoles)
	apiGroup.GET("/roles/:id", api.GetRole)
	apiGroup.POST("/roles", api.CreateRole)
	apiGroup.PUT("/roles/:id", api.UpdateRole)
	apiGroup.DELETE("/roles/:id", api.DeleteRole)
	apiGroup.PUT("/roles/:id/permissions", api.UpdateRolePermissions)

	// Разрешения
	apiGroup.GET("/permissions", api.GetPermissions)
	apiGroup.POST("/permissions", api.CreatePermission)

	// Шаблоны пользователей
	apiGroup.GET("/user-templates", api.GetUserTemplates)
	apiGroup.GET("/user-templates/:id", api.GetUserTemplate)
	apiGroup.POST("/user-templates", api.CreateUserTemplate)
	apiGroup.PUT("/user-templates/:id", api.UpdateUserTemplate)
	apiGroup.DELETE("/user-templates/:id", api.DeleteUserTemplate)

	// === Примечание: эндпоинты уже зарегистрированы выше ===
	// Удалены дубликаты для /users, /roles, /permissions, /user-templates
	// так как apiGroup уже имеет префикс /api/auth и роуты уже зарегистрированы

	// Управление ролями Axenta пользователей
	log.Println("🔧 Registering Axenta users management endpoints...")
	apiGroup.GET("/axenta-users", api.GetAxentaUsers)                  // Получить пользователей по типу (?type=partner|client|local|all)
	apiGroup.GET("/axenta-users/stats", api.GetUsersByAxentaType)      // Статистика по типам пользователей
	apiGroup.POST("/axenta-users/local", api.CreateLocalUser)          // Создать локального пользователя
	apiGroup.PUT("/axenta-users/:id/role", api.UpdateUserAxentaRole)   // Обновить роль Axenta пользователя
	apiGroup.POST("/axenta-users/sync", api.SyncUserWithAxenta)        // Синхронизировать пользователя с Axenta
	apiGroup.POST("/axenta-users/ensure-roles", api.EnsureAxentaRoles) // Создать роли по умолчанию

	// Синхронизация всех пользователей из Axenta
	apiGroup.POST("/axenta-users/sync-all", api.SyncAllAxentaUsers) // Синхронизировать всех пользователей из Axenta
	apiGroup.GET("/users/synced", api.GetSyncedUsersFromLocal)      // Получить синхронизированных пользователей из локальной БД

	// Тестовые endpoints для отладки ролей
	apiGroup.GET("/test/roles", api.TestRolesCreation)    // Тест создания ролей в tenant схеме
	apiGroup.GET("/test/user-role", api.TestUserWithRole) // Тест пользователя с назначенной ролью

	// DaData API для поиска организаций по ИНН/ОГРН
	log.Println("🔧 Registering DaData API endpoints...")
	apiGroup.POST("/dadata/organization", api.FindOrganizationByINN)
	apiGroup.POST("/dadata/organization/", api.FindOrganizationByINN)

	// DaData API для поиска банков по БИК
	apiGroup.POST("/dadata/bank", api.FindBankByBIK)
	apiGroup.POST("/dadata/bank/", api.FindBankByBIK)

	// Договоры
	// Важно: более специфичные роуты (с дополнительными параметрами) должны быть зарегистрированы ПЕРЕД общими
	// Например: /contracts/:id/objects должен быть ПЕРЕД /contracts/:id
	log.Println("🔧 Регистрация роутов для договоров...")
	apiGroup.GET("/contracts/expiring", api.GetExpiringContracts)

	// Роуты для работы с объектами договора (регистрируем ПЕРЕД общими роутами)
	apiGroup.POST("/contracts/:id/objects", api.AttachObjectsToContract)
	log.Printf("✅ Зарегистрирован POST /api/auth/contracts/:id/objects -> AttachObjectsToContract")

	apiGroup.DELETE("/contracts/:id/objects/:object_id", api.DetachObjectFromContract)
	log.Printf("✅ Зарегистрирован DELETE /api/auth/contracts/:id/objects/:object_id -> DetachObjectFromContract")

	// Синхронизация договора с подпиской
	apiGroup.POST("/contracts/:id/sync-from-subscription", api.SyncContractFromSubscription)
	log.Printf("✅ Зарегистрирован POST /api/auth/contracts/:id/sync-from-subscription -> SyncContractFromSubscription")

	// Общие роуты для договоров
	apiGroup.GET("/contracts", api.GetContracts)
	apiGroup.GET("/contracts/:id", api.GetContract)
	apiGroup.POST("/contracts", api.CreateContract)
	apiGroup.PUT("/contracts/:id", api.UpdateContract)
	apiGroup.DELETE("/contracts/:id", api.DeleteContract)
	log.Println("✅ Все роуты для договоров зарегистрированы")
	// apiGroup.GET("/contracts/:contract_id/cost", api.CalculateContractCost) // Временно отключено

	// Приложения к договорам - временно отключено
	// apiGroup.GET("/contracts/:contract_id/appendices", api.GetContractAppendices)
	// apiGroup.POST("/contracts/:contract_id/appendices", api.CreateContractAppendix)
	// apiGroup.PUT("/contract-appendices/:id", api.UpdateContractAppendix)
	// apiGroup.DELETE("/contract-appendices/:id", api.DeleteContractAppendix)

	// Нумераторы договоров
	apiGroup.POST("/contract-numerators/:numerator_id/generate", api.GenerateContractNumber)
	apiGroup.GET("/contract-numerators/:id", api.GetContractNumerator)
	apiGroup.PUT("/contract-numerators/:id", api.UpdateContractNumerator)
	apiGroup.DELETE("/contract-numerators/:id", api.DeleteContractNumerator)
	apiGroup.GET("/contract-numerators", api.GetContractNumerators)
	apiGroup.POST("/contract-numerators", api.CreateContractNumerator)

	// Тарифные планы и биллинг (уже были)
	apiGroup.GET("/billing/plans", api.GetBillingPlans)
	apiGroup.GET("/billing/plans/:id", api.GetBillingPlan)
	apiGroup.POST("/billing/plans", api.CreateBillingPlan)
	apiGroup.PUT("/billing/plans/:id", api.UpdateBillingPlan)
	apiGroup.DELETE("/billing/plans/:id", api.DeleteBillingPlan)

	// Подписки
	apiGroup.GET("/billing/subscriptions", api.GetSubscriptions)
	apiGroup.POST("/billing/subscriptions", api.CreateSubscription)
	apiGroup.PUT("/billing/subscriptions/:id", api.UpdateSubscription)
	apiGroup.DELETE("/billing/subscriptions/:id", api.DeleteSubscription)

	// Алиасы для совместимости с frontend
	apiGroup.GET("/subscriptions", api.GetSubscriptions)
	apiGroup.GET("/billing-plans", api.GetBillingPlans)

	// Новые эндпоинты системы биллинга
	// Расчеты и счета
	apiGroup.GET("/billing/contracts/:contract_id/calculate", api.CalculateBilling)
	apiGroup.POST("/billing/contracts/:contract_id/invoice", api.GenerateInvoice)
	
	// Счета - ВАЖНО: специфичные роуты (/overdue) ПЕРЕД параметризованными (/:id)
	apiGroup.GET("/billing/invoices", api.GetInvoices)
	apiGroup.GET("/billing/invoices/overdue", api.GetOverdueInvoices) // Переместили сюда!
	apiGroup.GET("/billing/invoices/:id", api.GetInvoice)
	apiGroup.POST("/billing/invoices/:id/send", api.SendInvoice)      // Отправка счета клиенту
	apiGroup.POST("/billing/invoices/:id/payment", api.ProcessPayment)
	apiGroup.POST("/billing/invoices/:id/cancel", api.CancelInvoice)
	apiGroup.DELETE("/billing/invoices/:id", api.DeleteInvoice)
	apiGroup.GET("/billing/invoice-numerators", api.GetInvoiceNumerators)
	apiGroup.POST("/billing/invoice-numerators", api.CreateInvoiceNumerator)
	apiGroup.PUT("/billing/invoice-numerators/:id", api.UpdateInvoiceNumerator)
	apiGroup.DELETE("/billing/invoice-numerators/:id", api.DeleteInvoiceNumerator)

	// Эндпоинты согласно roadmap (Этап 4.4)
	apiGroup.POST("/invoices/run", api.RunInvoicesGeneration) // POST /api/invoices/run
	apiGroup.POST("/invoices/:id/send", api.SendInvoice)      // POST /api/invoices/:id/send
	apiGroup.POST("/invoices/:id/pay", api.ProcessPayment)    // POST /api/invoices/:id/pay (алиас)

	// История и отчеты
	apiGroup.GET("/billing/history", api.GetBillingHistory)
	// apiGroup.GET("/billing/invoices/overdue", api.GetOverdueInvoices) // Перемещено выше, к остальным invoice роутам

	// Настройки биллинга
	apiGroup.GET("/billing/settings", api.GetBillingSettings)
	apiGroup.PUT("/billing/settings", api.UpdateBillingSettings)

	// Примечание: все биллинг эндпоинты уже зарегистрированы выше
	// Удалена дублирующая секция для /api/auth/billing/*

	// Dashboard endpoints (с мультитенантностью)
	apiGroup.GET("/dashboard/stats", api.GetDashboardStatsSimple)
	apiGroup.GET("/dashboard/activity", api.GetDashboardActivitySimple)
	apiGroup.GET("/dashboard/layouts", api.GetDashboardLayouts)
	apiGroup.GET("/dashboard/layouts/default", api.GetDefaultDashboardLayout)
	apiGroup.GET("/notifications", api.GetDashboardNotificationsSimple)

	// Системные настройки
	apiGroup.GET("/system/settings", api.GetSystemSettings)
	apiGroup.PUT("/system/settings", api.UpdateSystemSettings)

	// Аудит-логи (только для авторизованных пользователей)
	auditAPI := api.NewAuditAPI(database.DB)
	auditAPI.RegisterRoutes(apiGroup)
	log.Println("✅ Audit API endpoints registered at /api/auth/audit/*")

	// Примечание: автоматизация биллинга уже зарегистрирована выше

	// Интеграции - временно отключено
	// apiGroup.GET("/integration/health", api.GetIntegrationHealth)
	// apiGroup.GET("/integration/errors", api.GetIntegrationErrors)
	// apiGroup.GET("/integration/errors/stats", api.GetIntegrationErrorStats)
	// apiGroup.POST("/integration/errors/:id/retry", api.RetryIntegrationError)
	// apiGroup.POST("/integration/errors/:id/resolve", api.ResolveIntegrationError)
	// apiGroup.POST("/integration/credentials", api.SetupCompanyCredentials)
	// apiGroup.DELETE("/integration/cache", api.ClearIntegrationCache)

	// Интеграция с Битрикс24 - временно отключено
	// apiGroup.POST("/integration/bitrix24/setup", api.SetupBitrix24Integration)
	// apiGroup.GET("/integration/bitrix24/health", api.CheckBitrix24Health)
	// apiGroup.POST("/integration/bitrix24/sync/to", api.SyncToBitrix24)
	// apiGroup.POST("/integration/bitrix24/sync/from", api.SyncFromBitrix24)
	// apiGroup.GET("/integration/bitrix24/mappings", api.GetBitrix24Mappings)
	// apiGroup.GET("/integration/bitrix24/stats", api.GetBitrix24Stats)
	// apiGroup.DELETE("/integration/bitrix24/cache", api.ClearBitrix24Cache)

	// Система планирования монтажей - временные mock маршруты без middleware

	r.GET("/test/installations", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"items": []interface{}{},
				"total": 0,
			},
		})
	})

	r.GET("/api/installations/statistics", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"total":           0,
				"today":           0,
				"overdue":         0,
				"completion_rate": 100.0,
			},
		})
	})

	r.GET("/api/installers", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"items": []interface{}{},
				"total": 0,
			},
		})
	})

	r.GET("/api/equipment", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"items": []interface{}{},
				"total": 0,
			},
		})
	})

	r.GET("/api/locations", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"items": []interface{}{},
				"total": 0,
			},
		})
	})

	// Остальные маршруты installations временно отключены (в рамках основной apiGroup)
	// Остальные маршруты installations временно отключены
	/*
		apiGroup.GET("/installations/:id", installationAPI.GetInstallation)
		apiGroup.POST("/installations", installationAPI.CreateInstallation)
		apiGroup.PUT("/installations/:id", installationAPI.UpdateInstallation)
		apiGroup.DELETE("/installations/:id", installationAPI.DeleteInstallation)
		apiGroup.PUT("/installations/:id/start", installationAPI.StartInstallation)
		apiGroup.PUT("/installations/:id/complete", installationAPI.CompleteInstallation)
		apiGroup.PUT("/installations/:id/cancel", installationAPI.CancelInstallation)
	*/

	// Монтажники - временно отключено
	/*
		installerAPI := api.NewInstallerAPI(database.DB)
		apiGroup.GET("/installers", installerAPI.GetInstallers)
		apiGroup.GET("/installers/:id", installerAPI.GetInstaller)
		apiGroup.POST("/installers", installerAPI.CreateInstaller)
		apiGroup.PUT("/installers/:id", installerAPI.UpdateInstaller)
		apiGroup.DELETE("/installers/:id", installerAPI.DeleteInstaller)
		apiGroup.PUT("/installers/:id/activate", installerAPI.ActivateInstaller)
		apiGroup.PUT("/installers/:id/deactivate", installerAPI.DeactivateInstaller)
		apiGroup.GET("/installers/:id/schedule", installationAPI.GetInstallerSchedule)
		apiGroup.GET("/installers/:id/workload", installerAPI.GetInstallerWorkload)
		apiGroup.GET("/installers/available", installerAPI.GetAvailableInstallers)
		apiGroup.GET("/installers/statistics", installerAPI.GetInstallerStatistics)
	*/

	// Локации - временно отключено
	/*
		locationAPI := api.NewLocationAPI(database.DB)
		apiGroup.GET("/locations", locationAPI.GetLocations)
		apiGroup.GET("/locations/:id", locationAPI.GetLocation)
	*/
	/*
		apiGroup.POST("/locations", locationAPI.CreateLocation)
		apiGroup.PUT("/locations/:id", locationAPI.UpdateLocation)
		apiGroup.DELETE("/locations/:id", locationAPI.DeleteLocation)
		apiGroup.PUT("/locations/:id/activate", locationAPI.ActivateLocation)
		apiGroup.PUT("/locations/:id/deactivate", locationAPI.DeactivateLocation)
	*/
	/*
		apiGroup.GET("/locations/statistics", locationAPI.GetLocationStatistics)
		apiGroup.GET("/locations/by-region", locationAPI.GetLocationsByRegion)
		apiGroup.GET("/locations/search", locationAPI.SearchLocations)
	*/

	// Оборудование - временно отключено
	/*
		equipmentAPI := api.NewEquipmentAPI(database.DB)
		apiGroup.GET("/equipment", equipmentAPI.GetEquipment)
		apiGroup.GET("/equipment/:id", equipmentAPI.GetEquipmentItem)
		apiGroup.POST("/equipment", equipmentAPI.CreateEquipment)
		apiGroup.PUT("/equipment/:id", equipmentAPI.UpdateEquipment)
		apiGroup.DELETE("/equipment/:id", equipmentAPI.DeleteEquipment)
		apiGroup.PUT("/equipment/:id/install", equipmentAPI.InstallEquipment)
		apiGroup.PUT("/equipment/:id/uninstall", equipmentAPI.UninstallEquipment)
		apiGroup.GET("/equipment/statistics", equipmentAPI.GetEquipmentStatistics)
		apiGroup.GET("/equipment/low-stock", equipmentAPI.GetLowStockEquipment)
		apiGroup.GET("/equipment/qr/:qr_code", equipmentAPI.SearchEquipmentByQR)
	*/

	// Система управления складом
	warehouseAPI := api.NewWarehouseAPI(database.DB)

	// Складские операции
	apiGroup.POST("/warehouse/operations", warehouseAPI.CreateWarehouseOperation)
	apiGroup.GET("/warehouse/operations", warehouseAPI.GetWarehouseOperations)
	apiGroup.POST("/warehouse/transfer", warehouseAPI.TransferEquipment)

	// Категории оборудования - временно отключено
	/*
		apiGroup.GET("/equipment/categories", warehouseAPI.GetEquipmentCategories)
		apiGroup.POST("/equipment/categories", warehouseAPI.CreateEquipmentCategory)
		apiGroup.PUT("/equipment/categories/:id", warehouseAPI.UpdateEquipmentCategory)
		apiGroup.DELETE("/equipment/categories/:id", warehouseAPI.DeleteEquipmentCategory)
	*/

	// Складские уведомления
	apiGroup.GET("/warehouse/alerts", warehouseAPI.GetStockAlerts)
	apiGroup.POST("/warehouse/alerts", warehouseAPI.CreateStockAlert)
	apiGroup.PUT("/warehouse/alerts/:id/acknowledge", warehouseAPI.AcknowledgeStockAlert)
	apiGroup.PUT("/warehouse/alerts/:id/resolve", warehouseAPI.ResolveStockAlert)

	// Статистика склада
	apiGroup.GET("/warehouse/statistics", warehouseAPI.GetWarehouseStatistics)

	// Группа для интеграций (с мультитенантностью для изоляции данных между компаниями)
	integrationsGroup := r.Group("/api")
	integrationsGroup.Use(authMiddleware.RequireAuth())
	integrationsGroup.Use(tenantMiddleware.SetTenant()) // Добавляем мультитенантность для изоляции данных

	// Интеграция с 1С
	oneCAPI := api.NewOneCIntegrationAPI()
	oneCAPI.RegisterRoutes(integrationsGroup)

	// Интеграция с Axenta Cloud
	axentaAPI := api.NewAxentaIntegrationAPI(database.DB)
	axentaAPI.RegisterRoutes(integrationsGroup)

	// Интеграция с NovaConnect
	novaconnectAPI := api.NewNovaConnectIntegrationAPI(database.DB)
	novaconnectAPI.RegisterRoutes(integrationsGroup)

	// Интеграция с Telegram
	telegramAPI := api.NewTelegramIntegrationAPI()
	telegramAPI.RegisterRoutes(integrationsGroup)

	// Интеграция с MAX
	maxAPI := api.NewMaxIntegrationAPI()
	maxAPI.RegisterRoutes(integrationsGroup)

	// Общий эндпоинт для списка интеграций
	integrationsListAPI := api.NewIntegrationsAPI(database.DB)
	integrationsListAPI.RegisterRoutes(integrationsGroup)

	// Email SMTP интеграция (требует tenant middleware для company_id)
	// КРИТИЧНО: Хотя NotificationSettings в public схеме, данные фильтруются по company_id
	emailAuthGroup := r.Group("/api/auth/email")
	emailAuthGroup.Use(authMiddleware.RequireAuth())      // Auth middleware для проверки токена
	emailAuthGroup.Use(tenantMiddleware.SetTenant())      // Tenant middleware для установки company_id
	emailAuthGroup.POST("/setup", api.SetupEmailIntegration)
	emailAuthGroup.PUT("/setup", api.UpdateEmailIntegration)
	emailAuthGroup.GET("/config", api.GetEmailConfig)
	emailAuthGroup.POST("/test-connection", api.TestEmailConnection)

	// Система отчетности
	reportService := services.NewReportService(database.DB)
	reportSchedulerService := services.NewReportSchedulerService(database.DB, reportService, nil) // notificationService временно отключен
	reportsAPI := api.NewReportsAPI(database.DB, reportService, reportSchedulerService)
	// Регистрируем маршруты отчетов в группе /api (не /api/auth)
	reportsAPIGroup := r.Group("/api")
	reportsAPIGroup.Use(
		authMiddleware.RequireAuth(),
		tenantMiddleware.SetTenant(),
	)
	reportsAPI.RegisterRoutes(reportsAPIGroup)

	// Запускаем планировщик отчетов
	go func() {
		if err := reportSchedulerService.Start(); err != nil {
			log.Printf("Failed to start report scheduler: %v", err)
		}
	}()

	// Система уведомлений - временно отключено
	// apiGroup.GET("/notifications/logs", notificationAPI.GetNotificationLogs)
	// apiGroup.GET("/notifications/statistics", notificationAPI.GetNotificationStatistics)
	// apiGroup.GET("/notifications/templates", notificationAPI.GetNotificationTemplates)
	// apiGroup.POST("/notifications/templates", notificationAPI.CreateNotificationTemplate)
	// apiGroup.PUT("/notifications/templates/:id", notificationAPI.UpdateNotificationTemplate)
	// apiGroup.DELETE("/notifications/templates/:id", notificationAPI.DeleteNotificationTemplate)
	// apiGroup.POST("/notifications/templates/defaults", notificationAPI.CreateDefaultTemplates)
	// apiGroup.GET("/notifications/settings", notificationAPI.GetNotificationSettings)
	// apiGroup.PUT("/notifications/settings", notificationAPI.UpdateNotificationSettings)
	// apiGroup.GET("/notifications/preferences", notificationAPI.GetUserNotificationPreferences)
	// apiGroup.PUT("/notifications/preferences", notificationAPI.UpdateUserNotificationPreferences)
	// apiGroup.POST("/notifications/test", notificationAPI.TestNotification)

	// Публичный webhook для Telegram (без авторизации) - временно отключено
	// r.POST("/api/notifications/telegram/webhook/:company_id", notificationAPI.ProcessTelegramWebhook)

	log.Printf("Server starting on port %s...", cfg.App.Port)
	r.Run(":" + cfg.App.Port)
}
