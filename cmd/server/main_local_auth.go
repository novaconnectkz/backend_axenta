package main

import (
	"backend_axenta/api"
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"
	"backend_axenta/services"
	"log"
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// Альтернативный main.go с локальной авторизацией
// Используйте этот файл если хотите запустить сервер с локальной авторизацией
// Переименуйте в main.go для использования

func main() {
	log.Println("Starting Axenta Backend Server with Local Auth...")

	// Загружаем конфигурацию
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

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

	// Выполняем миграции для локальной авторизации
	log.Println("🔧 Setting up local auth tables...")
	if err := database.DB.AutoMigrate(
		&models.LocalUser{},
		&models.RefreshToken{},
	); err != nil {
		log.Printf("Warning: Failed to migrate local auth models: %v", err)
		log.Println("Tables may already exist or will be created manually")
	}

	// Создаем сервисы
	jwtService := services.NewJWTService(database.DB)

	// Создаем middleware
	tenantMiddleware := middleware.NewTenantMiddleware(database.DB)
	localAuthMiddleware := middleware.NewLocalAuthMiddleware(jwtService)

	// Создаем Gin router
	r := gin.Default()

	// Настройка CORS
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = []string{
		"http://localhost:3000",
		"http://localhost:5173",
		"http://localhost:8081",
		"https://axenta.glonass-saratov.ru",
		"https://api.axenta.glonass-saratov.ru",
	}
	corsConfig.AllowHeaders = append(corsConfig.AllowHeaders,
		"Authorization", "authorization", "X-Tenant-ID", "Cache-Control", "Pragma", "Content-Type", "Accept")
	corsConfig.AllowCredentials = true
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"}
	r.Use(cors.New(corsConfig))

	// Публичные маршруты
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "success", "message": "pong"})
	})

	// API группа
	apiGroup := r.Group("/api")

	// === ЛОКАЛЬНАЯ АВТОРИЗАЦИЯ ===
	localAuthAPI := api.NewLocalAuthAPI(database.DB, jwtService)
	localAuthAPI.RegisterRoutes(apiGroup)

	// === ОРИГИНАЛЬНАЯ AXENTA CLOUD АВТОРИЗАЦИЯ ===
	apiGroup.POST("/auth/login", api.Login)

	// === WEBSOCKET С АВТОРИЗАЦИЕЙ ===
	wsAPI := api.NewWebSocketAuthAPI(jwtService)
	wsGroup := r.Group("/ws")
	wsAPI.RegisterRoutes(wsGroup)

	// === ЗАЩИЩЕННЫЕ МАРШРУТЫ ===

	// Группа с локальной авторизацией
	localProtected := apiGroup.Group("/local")
	localProtected.Use(localAuthMiddleware.RequireAuth())
	localProtected.Use(tenantMiddleware.SetTenant())
	{
		// Здесь можно добавить защищенные маршруты для локальной авторизации
		localProtected.GET("/test", func(c *gin.Context) {
			userID, _ := middleware.GetCurrentUserID(c)
			companyID, _ := middleware.GetCurrentCompanyID(c)
			role, _ := middleware.GetCurrentUserRole(c)

			c.JSON(200, gin.H{
				"status":     "success",
				"message":    "Local auth test successful",
				"user_id":    userID,
				"company_id": companyID,
				"role":       role,
			})
		})
	}

	// Группа только для админов
	adminProtected := apiGroup.Group("/admin")
	adminProtected.Use(localAuthMiddleware.RequireAuth())
	adminProtected.Use(localAuthMiddleware.RequireRole(models.RoleAdmin))
	{
		adminProtected.GET("/test", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"status":  "success",
				"message": "Admin access granted",
			})
		})
	}

	// Запускаем сервер
	port := cfg.App.Port
	if port == "" {
		port = "8080"
	}

	log.Printf("🚀 Server starting on port %s", port)
	log.Printf("📝 Local Auth endpoints:")
	log.Printf("   POST /api/local/login - Local login")
	log.Printf("   POST /api/local/refresh - Refresh token")
	log.Printf("   GET  /api/local/current_user - Current user")
	log.Printf("   POST /api/local/logout - Logout")
	log.Printf("   POST /api/local/register - Register user (admin only)")
	log.Printf("🌐 WebSocket endpoint:")
	log.Printf("   WS   /ws/live-data/:company_id - Live data with auth")
	log.Printf("🔗 Original Axenta Cloud endpoint:")
	log.Printf("   POST /api/auth/login - Axenta Cloud login")

	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
