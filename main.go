package main

import (
	"backend_axenta/api"
	"backend_axenta/audit"
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/services"

	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// Подставляются через -ldflags при сборке (см. Makefile prod-build).
// В dev-режиме (`go run`) остаются дефолтами.
var (
	gitCommitCount = "0"
	gitCommitHash  = "dev"
)

// GetGitCommitCount возвращает count для api.RegisterVersionRoute
func GetGitCommitCount() string { return gitCommitCount }

// GetGitCommitHash возвращает hash для api.RegisterVersionRoute
func GetGitCommitHash() string { return gitCommitHash }

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
	axentaSyncIntervalMin := cfg.Axenta.SyncInterval
	if envInterval := os.Getenv("AXENTA_SYNC_INTERVAL_MIN"); envInterval != "" {
		if v, err := strconv.Atoi(envInterval); err == nil && v > 0 {
			axentaSyncIntervalMin = v
		}
	}
	if axentaSyncIntervalMin <= 0 {
		axentaSyncIntervalMin = 10
	}
	axentaSyncScheduler := services.NewAxentaSyncScheduler(axentaSyncService, axentaSyncIntervalMin)
	services.SetAxentaSyncScheduler(axentaSyncScheduler)

	// Инвалидатор snapshot'ов для всех 3 систем (Axenta, Wialon Hosting, Wialon Local).
	// После mutation API-handler вызывает InvalidateAxenta(adminID) или InvalidateWialon(connID),
	// воркер с debounce делает SyncAdmin / CollectForConnectionID.
	wialonStatsService := services.NewWialonStatsService()
	snapshotInvalidator := services.InitSnapshotInvalidator(axentaSyncService, wialonStatsService)
	defer snapshotInvalidator.Stop()

	disableAxentaSync := os.Getenv("DISABLE_AXENTA_SYNC_SCHEDULER") == "true"
	if disableAxentaSync {
		log.Printf("🔧 AxentaSync: планировщик ОТКЛЮЧЕН через DISABLE_AXENTA_SYNC_SCHEDULER=true")
		log.Printf("   🔄 Ручной запуск: POST /api/auth/axenta-sync/trigger")
	} else {
		log.Printf("🔧 AxentaSync: запуск планировщика, интервал %d мин", axentaSyncIntervalMin)
		if err := axentaSyncScheduler.Start(); err != nil {
			log.Printf("⚠️ Axenta Sync Scheduler failed to start: %v", err)
		} else {
			defer axentaSyncScheduler.Stop()
		}
	}

	// Инициализируем планировщик ежедневных снимков партнерских договоров
	// Проверяем, включен ли планировщик через переменную окружения
	enableSnapshotScheduler := os.Getenv("ENABLE_SNAPSHOT_SCHEDULER")
	if enableSnapshotScheduler == "" {
		// По умолчанию включаем планировщик только в development режиме
		// В production нужно явно установить ENABLE_SNAPSHOT_SCHEDULER=true
		enableSnapshotScheduler = "false"
		if cfg.IsDevelopment() {
			enableSnapshotScheduler = "true"
			log.Println("ℹ️ Планировщик снимков включен по умолчанию в development режиме")
		} else {
			log.Println("ℹ️ Планировщик снимков отключен по умолчанию в production. Установите ENABLE_SNAPSHOT_SCHEDULER=true для включения")
		}
	}

	if enableSnapshotScheduler == "true" || enableSnapshotScheduler == "1" {
		// Планировщик партнерских снимков (00:30 UTC / 03:30 MSK)
		partnerSnapshotScheduler := services.NewPartnerSnapshotScheduler()
		if err := partnerSnapshotScheduler.Start(); err != nil {
			log.Printf("⚠️ Partner Snapshot Scheduler failed to start: %v", err)
		} else {
			log.Println("✅ Partner Snapshot Scheduler started (daily at 00:30 UTC / 03:30 MSK)")
		}

		// Планировщик billing snapshots (01:00 UTC / 04:00 MSK)
		billingSnapshotScheduler := services.NewBillingSnapshotScheduler()
		if err := billingSnapshotScheduler.Start(); err != nil {
			log.Printf("⚠️ Billing Snapshot Scheduler failed to start: %v", err)
		} else {
			log.Println("✅ Billing Snapshot Scheduler started (daily at 01:00 UTC / 04:00 MSK)")
		}
	} else {
		log.Println("⚠️ Планировщики снимков отключены (ENABLE_SNAPSHOT_SCHEDULER != true)")
	}

	// Wialon stats scheduler — раз в N минут собирает usage объектов в public.wialon_object_stats.
	// Endpoint /api/wialon/connections/:id/objects-stats читает из этой таблицы (live-запрос для
	// WH с 3412 ресурсов занимает 6.5 минут). Включён всегда — это критично для UI.
	wialonStatsInterval := 15
	if v := os.Getenv("WIALON_STATS_INTERVAL_MIN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			wialonStatsInterval = n
		}
	}
	if os.Getenv("DISABLE_WIALON_STATS_SCHEDULER") != "true" {
		wialonStatsScheduler := services.NewWialonStatsScheduler(wialonStatsInterval)
		if err := wialonStatsScheduler.Start(); err != nil {
			log.Printf("⚠️ WialonStatsScheduler failed to start: %v", err)
		}
	} else {
		log.Println("⚠️ WialonStatsScheduler отключён (DISABLE_WIALON_STATS_SCHEDULER=true)")
	}

	// Wialon billing plans scheduler — раз в час обходит connections и обновляет тарифы.
	// Раньше тарифы дёргались с Wialon на каждое открытие формы создания (1-2с overhead),
	// теперь — мгновенный SELECT из public.wialon_billing_plans.
	wialonPlansInterval := 60
	if v := os.Getenv("WIALON_PLANS_INTERVAL_MIN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			wialonPlansInterval = n
		}
	}
	if os.Getenv("DISABLE_WIALON_PLANS_SCHEDULER") != "true" {
		wialonPlansScheduler := services.NewWialonBillingPlansScheduler(wialonPlansInterval)
		if err := wialonPlansScheduler.Start(); err != nil {
			log.Printf("⚠️ WialonBillingPlansScheduler failed to start: %v", err)
		}
	} else {
		log.Println("⚠️ WialonBillingPlansScheduler отключён (DISABLE_WIALON_PLANS_SCHEDULER=true)")
	}

	// Wialon all-accounts scheduler — каждые N мин дёргает /wialon/all-accounts для каждой company,
	// чтобы Redis cache (TTL 15 мин) всегда был свежий. F5 пользователя = cache-hit (~50ms) вместо live (18s).
	wialonAccountsInterval := 5
	if v := os.Getenv("WIALON_ACCOUNTS_REFRESH_INTERVAL_MIN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			wialonAccountsInterval = n
		}
	}
	if os.Getenv("DISABLE_WIALON_ACCOUNTS_SCHEDULER") != "true" {
		wialonAccountsRefresh := func(companyID uint) error {
			_, err := api.BuildAndCacheAllAccountsForCompany(companyID, database.DB)
			return err
		}
		wialonAccountsScheduler := services.NewWialonAllAccountsScheduler(wialonAccountsInterval, wialonAccountsRefresh)
		if err := wialonAccountsScheduler.Start(); err != nil {
			log.Printf("⚠️ WialonAllAccountsScheduler failed to start: %v", err)
		}
	} else {
		log.Println("⚠️ WialonAllAccountsScheduler отключён (DISABLE_WIALON_ACCOUNTS_SCHEDULER=true)")
	}

	// Инициализируем систему уведомлений (Phase 1+2: email/telegram/max каналы).
	// Telegram и MAX-сервисы созданы выше через api.InitTelegramService/InitMaxService.
	notifCache := services.NewCacheService(database.RedisClient, log.New(log.Writer(), "[Notif_Cache] ", log.LstdFlags))
	notificationService := services.NewNotificationService(
		database.DB,
		notifCache,
		api.GetTelegramService(),
		api.GetMaxService(),
	)
	_ = notificationService // используется в InstallationService и API ниже когда будут подключены
	log.Println("✅ Notification System initialized (email/telegram/max channels)")

	// Запускаем фоновый retry-worker для повторных попыток отправки
	// failed/pending записей. Интервал из NOTIFICATION_RETRY_INTERVAL_SECONDS
	// или 60с по умолчанию. Останавливается с процессом сервера.
	{
		retryInterval := time.Minute
		if env := os.Getenv("NOTIFICATION_RETRY_INTERVAL_SECONDS"); env != "" {
			if secs, err := strconv.Atoi(env); err == nil && secs > 0 {
				retryInterval = time.Duration(secs) * time.Second
			}
		}
		notificationService.StartRetryWorker(context.Background(), retryInterval)
	}

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

	// Отключаем автоматические редиректы для trailing slash —
	// нормализацию URL делаем на уровне http.Handler-обёртки ниже
	// (см. вызов r.Run / http.ListenAndServe в конце main).
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false

	// Добавляем gzip сжатие для оптимизации при плохом интернете
	r.Use(middleware.GzipMiddleware())
	log.Println("✅ Gzip compression middleware enabled")

	// Добавляем audit middleware для логирования всех запросов
	if cfg.Audit.Enabled {
		r.Use(audit.Middleware())
		log.Println("✅ Audit middleware enabled for all routes")
	}

	// Настройка CORS — список origins берётся из CORS_ALLOWED_ORIGINS env
	// (запятыми) с прод-дефолтом в config/config.go при пустом env.
	log.Printf("🔧 CORS: %d allowed origins loaded", len(cfg.CORS.AllowedOrigins))
	corsConfig := middleware.CustomCORSConfig{
		AllowOrigins: cfg.CORS.AllowedOrigins,
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

	// === ВЕРСИЯ BACKEND === (public, без auth — нужно фронту до логина для footer)
	api.SetVersionInfoProvider(func() api.VersionInfo {
		return api.VersionInfo{CommitCount: gitCommitCount, CommitHash: gitCommitHash}
	})
	r.GET("/api/version", api.GetAppVersion)

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
		installationAPI := api.NewInstallationAPI(database.DB, notificationService)
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
	accountsHandler := api.NewAccountsHandler()
	cmsGroup.POST("/accounts/change_account", accountsHandler.MoveAccount)
	cmsGroup.POST("/accounts/change_account/", accountsHandler.MoveAccount)

	log.Println("✅ CMS endpoints registered without Axenta authentication")

	// Notification admin API (templates, test send, logs, stats)
	notificationAPI := api.NewNotificationAPI(notificationService)
	notificationAPI.RegisterRoutes(apiGroup)
	log.Println("✅ Notification API registered at /api/auth/notifications/*")

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

	// Унифицированный API для пользователей (объединяет Axenta + Wialon)
	api.RegisterUnifiedUsersRoutes(apiGroup)

	// Унифицированный API для объектов (объединяет Axenta + Wialon)
	api.RegisterUnifiedObjectsRoutes(apiGroup)

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
	accountsHandler = api.NewAccountsHandler()
	apiGroup.GET("/accounts", accountsHandler.GetAccounts)
	apiGroup.GET("/accounts/", accountsHandler.GetAccounts)
	// stats: total/active/blocked/clients/partners одним запросом из snapshot
	apiGroup.GET("/accounts/stats", accountsHandler.GetAccountsStats)
	apiGroup.GET("/accounts/stats/", accountsHandler.GetAccountsStats)
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

	// Временный эндпоинт для отладки данных из Axenta Cloud
	apiGroup.GET("/contracts/debug-axenta-objects", api.DebugAxentaPartnerObjects)
	log.Println("✅ Зарегистрирован GET /api/auth/contracts/debug-axenta-objects -> DebugAxentaPartnerObjects")

	// Эндпоинт для очистки кэша партнерских объектов
	apiGroup.POST("/contracts/clear-partner-cache", api.ClearPartnerObjectsCache)
	log.Println("✅ Зарегистрирован POST /api/auth/contracts/clear-partner-cache -> ClearPartnerObjectsCache")

	apiGroup.GET("/contracts/expiring", api.GetExpiringContracts)

	// Роуты для работы с объектами договора (регистрируем ПЕРЕД общими роутами)
	apiGroup.POST("/contracts/:contract_id/objects", api.AttachObjectsToContract)
	log.Printf("✅ Зарегистрирован POST /api/auth/contracts/:contract_id/objects -> AttachObjectsToContract")

	apiGroup.DELETE("/contracts/:contract_id/objects/:object_id", api.DetachObjectFromContract)
	log.Printf("✅ Зарегистрирован DELETE /api/auth/contracts/:contract_id/objects/:object_id -> DetachObjectFromContract")

	// Синхронизация договора с подпиской
	apiGroup.POST("/contracts/:contract_id/sync-from-subscription", api.SyncContractFromSubscription)
	log.Printf("✅ Зарегистрирован POST /api/auth/contracts/:contract_id/sync-from-subscription -> SyncContractFromSubscription")

	// Утилита для исправления статусов договоров без подписок
	apiGroup.POST("/contracts/fix-statuses", api.FixContractStatuses)
	log.Printf("✅ Зарегистрирован POST /api/auth/contracts/fix-statuses -> FixContractStatuses")

	// Общие роуты для договоров
	// ВАЖНО: Специфичные роуты должны быть ПЕРЕД общими, чтобы не перехватывались
	// Приложения к договорам (специфичный маршрут - ПЕРЕД общими)
	apiGroup.GET("/contracts/:contract_id/appendices", api.GetContractAppendices)
	log.Println("✅ Зарегистрирован GET /api/auth/contracts/:contract_id/appendices -> GetContractAppendices")

	// Специфичные роуты для договоров
	apiGroup.GET("/contracts/:contract_id/stats", api.GetContractStats) // Progressive Loading
	apiGroup.GET("/contracts/:contract_id/partner-snapshots", api.GetPartnerContractSnapshots)
	apiGroup.POST("/contracts/partner-snapshots/create", api.CreatePartnerSnapshots)
	apiGroup.POST("/contracts/:contract_id/partner-snapshots/generate", api.GeneratePartnerSnapshotsForPeriod)
	apiGroup.POST("/contracts/partner-snapshots/generate-all", api.GenerateAllPartnerSnapshotsForPeriod)
	log.Println("✅ Зарегистрирован POST /api/auth/contracts/:contract_id/partner-snapshots/generate -> GeneratePartnerSnapshotsForPeriod")
	log.Println("✅ Зарегистрирован POST /api/auth/contracts/partner-snapshots/generate-all -> GenerateAllPartnerSnapshotsForPeriod")

	// Расчет стоимости договора (специфичный маршрут - ПЕРЕД общими)
	apiGroup.GET("/contracts/:contract_id/calculate", api.CalculateContractCost)
	log.Println("✅ Зарегистрирован GET /api/auth/contracts/:contract_id/calculate -> CalculateContractCost")

	// Общие роуты для договоров (ПОСЛЕ специфичных)
	apiGroup.GET("/contracts", api.GetContracts)
	apiGroup.GET("/contracts/:contract_id", api.GetContract)
	apiGroup.POST("/contracts", api.CreateContract)
	apiGroup.PUT("/contracts/:contract_id", api.UpdateContract)
	apiGroup.DELETE("/contracts/:contract_id", api.DeleteContract)

	// История задач создания снимков
	apiGroup.GET("/snapshot-jobs", api.GetSnapshotJobs)
	apiGroup.GET("/snapshot-jobs/stats", api.GetSnapshotJobStats)
	apiGroup.GET("/snapshot-jobs/latest", api.GetLatestSnapshotJob)
	apiGroup.GET("/snapshot-jobs/:id", api.GetSnapshotJob)
	apiGroup.DELETE("/snapshot-jobs/cleanup", api.DeleteOldSnapshotJobs)
	apiGroup.DELETE("/snapshot-jobs/clear-all", api.ClearAllSnapshotHistory)
	apiGroup.POST("/snapshot-jobs/trigger", api.TriggerManualSnapshot)

	// Тестовый endpoint без авторизации (TODO: удалить в продакшене)
	r.POST("/api/test/snapshot-jobs/trigger", api.TriggerManualSnapshot)
	log.Println("⚠️ Зарегистрирован ТЕСТОВЫЙ endpoint /api/test/snapshot-jobs/trigger (без авторизации)")

	log.Println("✅ Зарегистрированы роуты для истории создания снимков (snapshot-jobs)")

	// Настройки снимков
	apiGroup.GET("/snapshot-settings", api.GetSnapshotSettings)
	apiGroup.POST("/snapshot-settings", api.UpdateSnapshotSettings)
	log.Println("✅ Зарегистрированы роуты для настроек снимков (snapshot-settings)")
	apiGroup.POST("/billing-snapshots/rebuild", api.RebuildBillingSnapshotsHandler)
	apiGroup.POST("/billing-snapshots/run-daily", api.RunDailyBillingSnapshotHandler)
	log.Println("✅ Зарегистрированы роуты для billing daily snapshots")

	// Новые endpoints для накопительного подхода к загрузке снимков
	apiGroup.POST("/snapshots/load-all-current", api.LoadAllCurrentObjects)
	apiGroup.GET("/snapshots/load-progress", api.GetLoadProgress)
	apiGroup.GET("/snapshots/billing-start-date", api.GetBillingStartDate)
	apiGroup.GET("/snapshots/check-billing-start-in-history", api.CheckBillingStartDateInHistory)
	apiGroup.POST("/snapshots/daily-accumulation", api.ProcessDailyAccumulation)
	apiGroup.GET("/snapshots/objects-count", api.GetObjectsCountForDate)
	log.Println("✅ Зарегистрированы роуты для накопительной загрузки снимков")

	// Эндпоинт для ручного запуска синхронизации Axenta
	apiGroup.POST("/axenta-sync/trigger", api.TriggerAxentaSync)
	apiGroup.POST("/axenta-sync/trigger/", api.TriggerAxentaSync)
	log.Println("✅ Зарегистрирован POST /api/auth/axenta-sync/trigger -> TriggerAxentaSync")

	// Тестовый endpoint без авторизации
	r.POST("/api/test/axenta-sync/trigger", api.TriggerAxentaSync)
	r.POST("/api/test/axenta-sync/trigger/", api.TriggerAxentaSync)
	log.Println("⚠️ Зарегистрирован ТЕСТОВЫЙ endpoint /api/test/axenta-sync/trigger (без авторизации)")

	// Тестовый endpoint для Wialon (без авторизации, для отладки)
	wialonTestGroup := r.Group("/api/test")
	wialonTestAPI := api.NewWialonIntegrationAPI(database.DB)
	wialonTestAPI.RegisterTestRoutes(wialonTestGroup)

	log.Println("✅ Все роуты для договоров зарегистрированы")
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
	apiGroup.GET("/billing/contracts/:contract_id/breakdown", api.GetContractBillingBreakdown)
	apiGroup.GET("/billing/contracts/by-number/:number/analysis", api.GetContractBillingAnalysis) // Отладочный эндпоинт для анализа договора
	apiGroup.POST("/billing/contracts/:contract_id/invoice", api.GenerateInvoice)

	// Счета - ВАЖНО: специфичные роуты (/overdue) ПЕРЕД параметризованными (/:id)
	log.Println("🔧 Регистрация роутов для счетов (invoices)...")
	apiGroup.GET("/billing/invoices", api.GetInvoices)
	apiGroup.GET("/billing/invoices/overdue", api.GetOverdueInvoices) // Переместили сюда!
	apiGroup.GET("/billing/invoices/:id", api.GetInvoice)
	apiGroup.POST("/billing/invoices/:id/send", api.SendInvoice) // Отправка счета клиенту
	apiGroup.POST("/billing/invoices/:id/payment", api.ProcessPayment)
	apiGroup.POST("/billing/invoices/:id/manual-payment", api.AddManualPayment) // Ручной платёж
	log.Println("✅ Зарегистрирован POST /api/auth/billing/invoices/:id/manual-payment -> AddManualPayment")
	apiGroup.POST("/billing/invoices/:id/cancel", api.CancelInvoice)
	apiGroup.DELETE("/billing/invoices/:id", api.DeleteInvoice)
	log.Println("✅ Все роуты для счетов зарегистрированы")
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
	apiGroup.GET("/dashboard/alerts", api.GetDashboardAlerts)
	apiGroup.GET("/dashboard/alerts/", api.GetDashboardAlerts)
	apiGroup.GET("/dashboard/kpi", api.GetDashboardKPI)
	apiGroup.GET("/dashboard/kpi/", api.GetDashboardKPI)
	apiGroup.GET("/dashboard/today-installations", api.GetTodayInstallations)
	apiGroup.GET("/dashboard/today-installations/", api.GetTodayInstallations)
	apiGroup.GET("/dashboard/sources-stats", api.GetDashboardSourcesStats)
	apiGroup.GET("/dashboard/sources-stats/", api.GetDashboardSourcesStats)
	apiGroup.GET("/dashboard/recent-invoices", api.GetRecentInvoices)
	apiGroup.GET("/dashboard/recent-invoices/", api.GetRecentInvoices)
	apiGroup.GET("/search", api.GetGlobalSearch)
	apiGroup.GET("/search/", api.GetGlobalSearch)
	apiGroup.GET("/notifications", api.GetDashboardNotificationsSimple)

	// Системные настройки
	apiGroup.GET("/system/settings", api.GetSystemSettings)
	apiGroup.PUT("/system/settings", api.UpdateSystemSettings)

	// Настройки синхронизации AxentaSync
	apiGroup.GET("/system/axenta-sync-settings", api.GetAxentaSyncSettings)
	apiGroup.PUT("/system/axenta-sync-settings", api.UpdateAxentaSyncSettings)

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

	// Installations API (apiGroup = /api/auth, под auth + tenant middleware).
	// Хендлеры берут БД из контекста через api.getDB(c) (tenant_db от
	// tenantMiddleware), с fallback на api.DB для тестов / dev.
	installationAPI := api.NewInstallationAPI(database.DB, notificationService)
	apiGroup.GET("/installations", installationAPI.GetInstallations)
	apiGroup.GET("/installations/", installationAPI.GetInstallations)
	apiGroup.GET("/installations/statistics", installationAPI.GetInstallationStatistics)
	apiGroup.GET("/installations/statistics/", installationAPI.GetInstallationStatistics)
	apiGroup.GET("/installations/:id", installationAPI.GetInstallation)
	apiGroup.POST("/installations", installationAPI.CreateInstallation)
	apiGroup.POST("/installations/", installationAPI.CreateInstallation)
	apiGroup.PUT("/installations/:id", installationAPI.UpdateInstallation)
	apiGroup.DELETE("/installations/:id", installationAPI.DeleteInstallation)
	apiGroup.PUT("/installations/:id/start", installationAPI.StartInstallation)
	apiGroup.PUT("/installations/:id/complete", installationAPI.CompleteInstallation)
	apiGroup.PUT("/installations/:id/cancel", installationAPI.CancelInstallation)

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

	// Корзина (удаленные элементы)
	log.Println("🔧 Регистрация роутов для корзины...")
	apiGroup.GET("/trash/items", api.GetTrashItems)                          // Список удаленных элементов
	apiGroup.GET("/trash/stats", api.GetTrashStats)                          // Статистика корзины
	apiGroup.POST("/trash/items/:id/restore", api.RestoreItem)               // Восстановить элемент
	apiGroup.DELETE("/trash/items/:id/permanent", api.PermanentlyDeleteItem) // Окончательно удалить элемент

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

	// Интеграция с Wialon
	wialonAPI := api.NewWialonIntegrationAPI(database.DB)
	wialonAPI.RegisterRoutes(integrationsGroup)

	// Подключения Wialon (мульти-хост)
	wialonConnectionsAPI := api.NewWialonConnectionAPI(database.DB)
	wialonConnectionsAPI.RegisterRoutes(integrationsGroup)

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
	emailAuthGroup.Use(authMiddleware.RequireAuth()) // Auth middleware для проверки токена
	emailAuthGroup.Use(tenantMiddleware.SetTenant()) // Tenant middleware для установки company_id
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

	// Оборачиваем gin engine в нормализатор URL: режем trailing slash
	// перед роутингом — чтобы новые маршруты регистрировались только
	// как `/path` без `/path/` дубля. Старые дубли остаются как
	// безвредные no-op (после нормализации они никогда не матчатся).
	if err := http.ListenAndServe(":"+cfg.App.Port, &trailingSlashNormalizer{inner: r}); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// trailingSlashNormalizer — http.Handler-обёртка которая удаляет
// trailing slash из URL.Path перед делегированием в gin engine.
// Корневой "/" не трогаем.
type trailingSlashNormalizer struct {
	inner http.Handler
}

func (h *trailingSlashNormalizer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if len(req.URL.Path) > 1 && req.URL.Path[len(req.URL.Path)-1] == '/' {
		req.URL.Path = req.URL.Path[:len(req.URL.Path)-1]
	}
	h.inner.ServeHTTP(w, req)
}
