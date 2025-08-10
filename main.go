package main

import (
	"log"
	"os"

	"backend_axenta/api"
	"backend_axenta/database"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

// initDB инициализирует подключение к базе данных
func initDB() {
	log.Println("🔧 Инициализация базы данных...")

	// Создаем базу данных, если она не существует
	if err := database.CreateDatabaseIfNotExists(); err != nil {
		log.Fatal("❌ Ошибка при создании базы данных:", err)
	}

	// Подключаемся к базе данных
	if err := database.ConnectDatabase(); err != nil {
		log.Fatal("❌ Ошибка подключения к базе данных:", err)
	}

	log.Println("✅ База данных успешно инициализирована")
}

func main() {
	// Загружаем переменные окружения из .env файла
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  Файл .env не найден, используются системные переменные окружения")
	}

	// Инициализируем базу данных
	initDB()

	// Настраиваем Gin router
	r := gin.Default()
	r.Use(cors.Default()) // Для избежания CORS-ошибок

	// Базовые роуты
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":   "success",
			"message":  "pong",
			"database": "connected",
		})
	})

	// API роуты
	r.GET("/api/objects", api.GetObjects)

	// Получаем порт из переменных окружения
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🚀 Сервер запущен на порту %s", port)
	r.Run(":" + port)
}
