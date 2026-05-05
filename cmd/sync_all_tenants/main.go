package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/services"
	"log"
)

func main() {
	log.Println("🔄 Запуск синхронизации для всех tenant схем...")

	// Загружаем конфиг
	config.LoadConfig()

	// Инициализируем БД
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("Ошибка подключения к БД: %v", err)
	}

	// Создаем сервис синхронизации
	syncService := services.NewAxentaSyncService(database.DB)

	// Запускаем синхронизацию для всех компаний
	log.Println("📥 Начинаем синхронизацию всех компаний...")
	syncService.SyncAllAdmins()

	log.Println("✅ Синхронизация завершена")
}
