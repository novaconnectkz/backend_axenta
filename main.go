package main

import (
	"backend_axenta/api"
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/services"

	// "backend_axenta/models" // Temporarily commented out
	"log"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	log.Println("Starting Axenta Backend Server...")

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
	api.InitBitrix24Service()
	log.Println("✅ Bitrix24 Integration Service initialized successfully")

	// Инициализируем сервис интеграции с 1С
	api.InitOneCService()
	log.Println("✅ 1C Integration Service initialized successfully")

	// Инициализируем систему уведомлений
	cache := services.NewCacheService(database.RedisClient, log.New(log.Writer(), "CACHE: ", log.LstdFlags))
	notificationService := services.NewNotificationService(database.DB, cache)
	_ = services.NewNotificationFallbackService(database.DB, notificationService) // fallbackService для будущего использования
	notificationAPI := api.NewNotificationAPI(notificationService)
	log.Println("✅ Notification System initialized successfully")

	// Выполняем миграции для основных таблиц (не мультитенантных)
	// Временно отключаем миграции для отладки
	/*
		if err := database.DB.AutoMigrate(
			&models.Company{},
		); err != nil {
			log.Fatalf("Failed to migrate main database: %v", err)
		}
	*/

	// Создаем middleware для мультитенантности
	tenantMiddleware := middleware.NewTenantMiddleware(database.DB)

	r := gin.Default()

	// Настройка CORS
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = []string{"http://localhost:3000", "http://127.0.0.1:3000"}
	corsConfig.AllowHeaders = append(corsConfig.AllowHeaders, "Authorization", "authorization", "X-Tenant-ID")
	corsConfig.AllowCredentials = true
	r.Use(cors.New(corsConfig))

	// Публичные маршруты (без проверки tenant)
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "success", "message": "pong"})
	})
	r.POST("/api/auth/login", api.Login)

	// Dashboard endpoints без мультитенантности (пока)
	r.GET("/api/dashboard/stats", api.GetDashboardStatsSimple)
	r.GET("/api/dashboard/activity", api.GetDashboardActivitySimple)
	r.GET("/api/dashboard/layouts", api.GetDashboardLayouts)
	r.GET("/api/dashboard/layouts/default", api.GetDefaultDashboardLayout)
	r.GET("/api/notifications", api.GetDashboardNotificationsSimple)

	// Billing endpoints убраны, так как уже есть в apiGroup

	// Группа API с мультитенантностью
	apiGroup := r.Group("/api")
	apiGroup.Use(tenantMiddleware.SetTenant())
	{
		// Объекты
		apiGroup.GET("/objects", api.GetObjects)
		apiGroup.GET("/objects/:id", api.GetObject)
		apiGroup.POST("/objects", api.CreateObject)
		apiGroup.PUT("/objects/:id", api.UpdateObject)
		apiGroup.DELETE("/objects/:id", api.DeleteObject)

		// Плановое удаление объектов
		apiGroup.PUT("/objects/:id/schedule-delete", api.ScheduleObjectDelete)
		apiGroup.PUT("/objects/:id/cancel-delete", api.CancelScheduledDelete)

		// Корзина для объектов
		apiGroup.GET("/objects-trash", api.GetDeletedObjects)
		apiGroup.PUT("/objects/:id/restore", api.RestoreObject)
		apiGroup.DELETE("/objects/:id/permanent", api.PermanentDeleteObject)

		// Шаблоны объектов
		apiGroup.GET("/object-templates", api.GetObjectTemplates)
		apiGroup.GET("/object-templates/:id", api.GetObjectTemplate)
		apiGroup.POST("/object-templates", api.CreateObjectTemplate)
		apiGroup.PUT("/object-templates/:id", api.UpdateObjectTemplate)
		apiGroup.DELETE("/object-templates/:id", api.DeleteObjectTemplate)

		// Пользователи
		apiGroup.GET("/users", api.GetUsers)
		apiGroup.GET("/users/:id", api.GetUser)
		apiGroup.POST("/users", api.CreateUser)
		apiGroup.PUT("/users/:id", api.UpdateUser)
		apiGroup.DELETE("/users/:id", api.DeleteUser)

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

		// Договоры
		apiGroup.GET("/contracts", api.GetContracts)
		apiGroup.GET("/contracts/:id", api.GetContract)
		apiGroup.POST("/contracts", api.CreateContract)
		apiGroup.PUT("/contracts/:id", api.UpdateContract)
		apiGroup.DELETE("/contracts/:id", api.DeleteContract)
		apiGroup.GET("/contracts/expiring", api.GetExpiringContracts)
		// apiGroup.GET("/contracts/:contract_id/cost", api.CalculateContractCost) // Временно отключено

		// Приложения к договорам - временно отключено
		// apiGroup.GET("/contracts/:contract_id/appendices", api.GetContractAppendices)
		// apiGroup.POST("/contracts/:contract_id/appendices", api.CreateContractAppendix)
		// apiGroup.PUT("/contract-appendices/:id", api.UpdateContractAppendix)
		// apiGroup.DELETE("/contract-appendices/:id", api.DeleteContractAppendix)

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

		// Новые эндпоинты системы биллинга
		// Расчеты и счета
		apiGroup.GET("/billing/contracts/:contract_id/calculate", api.CalculateBilling)
		apiGroup.POST("/billing/contracts/:contract_id/invoice", api.GenerateInvoice)
		apiGroup.GET("/billing/invoices", api.GetInvoices)
		apiGroup.GET("/billing/invoices/:id", api.GetInvoice)
		apiGroup.POST("/billing/invoices/:id/payment", api.ProcessPayment)
		apiGroup.POST("/billing/invoices/:id/cancel", api.CancelInvoice)

		// История и отчеты
		apiGroup.GET("/billing/history", api.GetBillingHistory)
		apiGroup.GET("/billing/invoices/overdue", api.GetOverdueInvoices)

		// Настройки биллинга
		apiGroup.GET("/billing/settings", api.GetBillingSettings)
		apiGroup.PUT("/billing/settings", api.UpdateBillingSettings)

		// Автоматизация биллинга
		apiGroup.POST("/billing/auto-generate", api.AutoGenerateInvoices)
		apiGroup.POST("/billing/process-deletions", api.ProcessScheduledDeletions)
		apiGroup.GET("/billing/statistics", api.GetBillingStatistics)
		apiGroup.GET("/billing/invoices/period", api.GetInvoicesByPeriod)

		// Интеграции
		apiGroup.GET("/integration/health", api.GetIntegrationHealth)
		apiGroup.GET("/integration/errors", api.GetIntegrationErrors)
		apiGroup.GET("/integration/errors/stats", api.GetIntegrationErrorStats)
		apiGroup.POST("/integration/errors/:id/retry", api.RetryIntegrationError)
		apiGroup.POST("/integration/errors/:id/resolve", api.ResolveIntegrationError)
		apiGroup.POST("/integration/credentials", api.SetupCompanyCredentials)
		apiGroup.DELETE("/integration/cache", api.ClearIntegrationCache)

		// Интеграция с Битрикс24
		apiGroup.POST("/integration/bitrix24/setup", api.SetupBitrix24Integration)
		apiGroup.GET("/integration/bitrix24/health", api.CheckBitrix24Health)
		apiGroup.POST("/integration/bitrix24/sync/to", api.SyncToBitrix24)
		apiGroup.POST("/integration/bitrix24/sync/from", api.SyncFromBitrix24)
		apiGroup.GET("/integration/bitrix24/mappings", api.GetBitrix24Mappings)
		apiGroup.GET("/integration/bitrix24/stats", api.GetBitrix24Stats)
		apiGroup.DELETE("/integration/bitrix24/cache", api.ClearBitrix24Cache)

		// Система планирования монтажей
		// Монтажи
		installationAPI := api.NewInstallationAPI(database.DB)
		apiGroup.GET("/installations", installationAPI.GetInstallations)
		apiGroup.GET("/installations/:id", installationAPI.GetInstallation)
		apiGroup.POST("/installations", installationAPI.CreateInstallation)
		apiGroup.PUT("/installations/:id", installationAPI.UpdateInstallation)
		apiGroup.DELETE("/installations/:id", installationAPI.DeleteInstallation)
		apiGroup.PUT("/installations/:id/start", installationAPI.StartInstallation)
		apiGroup.PUT("/installations/:id/complete", installationAPI.CompleteInstallation)
		apiGroup.PUT("/installations/:id/cancel", installationAPI.CancelInstallation)
		apiGroup.GET("/installations/statistics", installationAPI.GetInstallationStatistics)

		// Монтажники
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

		// Локации
		locationAPI := api.NewLocationAPI(database.DB)
		apiGroup.GET("/locations", locationAPI.GetLocations)
		apiGroup.GET("/locations/:id", locationAPI.GetLocation)
		apiGroup.POST("/locations", locationAPI.CreateLocation)
		apiGroup.PUT("/locations/:id", locationAPI.UpdateLocation)
		apiGroup.DELETE("/locations/:id", locationAPI.DeleteLocation)
		apiGroup.PUT("/locations/:id/activate", locationAPI.ActivateLocation)
		apiGroup.PUT("/locations/:id/deactivate", locationAPI.DeactivateLocation)
		apiGroup.GET("/locations/statistics", locationAPI.GetLocationStatistics)
		apiGroup.GET("/locations/by-region", locationAPI.GetLocationsByRegion)
		apiGroup.GET("/locations/search", locationAPI.SearchLocations)

		// Оборудование
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

		// Система управления складом
		warehouseAPI := api.NewWarehouseAPI(database.DB)

		// Складские операции
		apiGroup.POST("/warehouse/operations", warehouseAPI.CreateWarehouseOperation)
		apiGroup.GET("/warehouse/operations", warehouseAPI.GetWarehouseOperations)
		apiGroup.POST("/warehouse/transfer", warehouseAPI.TransferEquipment)

		// Категории оборудования
		apiGroup.GET("/equipment/categories", warehouseAPI.GetEquipmentCategories)
		apiGroup.POST("/equipment/categories", warehouseAPI.CreateEquipmentCategory)
		apiGroup.PUT("/equipment/categories/:id", warehouseAPI.UpdateEquipmentCategory)
		apiGroup.DELETE("/equipment/categories/:id", warehouseAPI.DeleteEquipmentCategory)

		// Складские уведомления
		apiGroup.GET("/warehouse/alerts", warehouseAPI.GetStockAlerts)
		apiGroup.POST("/warehouse/alerts", warehouseAPI.CreateStockAlert)
		apiGroup.PUT("/warehouse/alerts/:id/acknowledge", warehouseAPI.AcknowledgeStockAlert)
		apiGroup.PUT("/warehouse/alerts/:id/resolve", warehouseAPI.ResolveStockAlert)

		// Статистика склада
		apiGroup.GET("/warehouse/statistics", warehouseAPI.GetWarehouseStatistics)

		// Интеграция с 1С
		oneCAPI := api.NewOneCIntegrationAPI()
		oneCAPI.RegisterRoutes(apiGroup)

		// Система отчетности
		reportService := services.NewReportService(database.DB)
		reportSchedulerService := services.NewReportSchedulerService(database.DB, reportService, notificationService)
		reportsAPI := api.NewReportsAPI(database.DB, reportService, reportSchedulerService)
		reportsAPI.RegisterRoutes(apiGroup)

		// Запускаем планировщик отчетов
		go func() {
			if err := reportSchedulerService.Start(); err != nil {
				log.Printf("Failed to start report scheduler: %v", err)
			}
		}()

		// Система уведомлений
		apiGroup.GET("/notifications/logs", notificationAPI.GetNotificationLogs)
		apiGroup.GET("/notifications/statistics", notificationAPI.GetNotificationStatistics)
		apiGroup.GET("/notifications/templates", notificationAPI.GetNotificationTemplates)
		apiGroup.POST("/notifications/templates", notificationAPI.CreateNotificationTemplate)
		apiGroup.PUT("/notifications/templates/:id", notificationAPI.UpdateNotificationTemplate)
		apiGroup.DELETE("/notifications/templates/:id", notificationAPI.DeleteNotificationTemplate)
		apiGroup.POST("/notifications/templates/defaults", notificationAPI.CreateDefaultTemplates)
		apiGroup.GET("/notifications/settings", notificationAPI.GetNotificationSettings)
		apiGroup.PUT("/notifications/settings", notificationAPI.UpdateNotificationSettings)
		apiGroup.GET("/notifications/preferences", notificationAPI.GetUserNotificationPreferences)
		apiGroup.PUT("/notifications/preferences", notificationAPI.UpdateUserNotificationPreferences)
		apiGroup.POST("/notifications/test", notificationAPI.TestNotification)
	}

	// Публичный webhook для Telegram (без авторизации)
	r.POST("/api/notifications/telegram/webhook/:company_id", notificationAPI.ProcessTelegramWebhook)

	log.Printf("Server starting on port %s...", cfg.App.Port)
	r.Run(":" + cfg.App.Port)
}
