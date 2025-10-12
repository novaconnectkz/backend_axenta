package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"log"
)

func main() {
	log.Println("🚀 Миграция таблиц локальной авторизации")
	log.Println("==========================================")

	// Загружаем конфигурацию
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("❌ Не удалось загрузить конфигурацию: %v", err)
	}

	log.Printf("🔧 Подключение к базе данных: %s@%s:%s/%s",
		cfg.Database.User, cfg.Database.Host, cfg.Database.Port, cfg.Database.Name)

	// Создаем базу данных если её нет
	if err := database.CreateDatabaseIfNotExists(); err != nil {
		log.Fatalf("❌ Не удалось создать базу данных: %v", err)
	}

	// Подключаемся к базе данных
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("❌ Не удалось подключиться к базе данных: %v", err)
	}

	log.Println("✅ Подключение к базе данных установлено")

	// Получаем подключение к БД
	db := database.GetDB()

	// Убеждаемся, что мы в схеме public
	if err := db.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️ Не удалось переключиться на схему public: %v", err)
	} else {
		log.Println("✅ Переключились на схему public")
	}

	// Миграция LocalUser
	log.Println("🔄 Миграция таблицы local_users...")
	if err := db.AutoMigrate(&models.LocalUser{}); err != nil {
		log.Printf("❌ Ошибка миграции local_users: %v", err)
	} else {
		log.Println("✅ Таблица local_users успешно создана/обновлена")
	}

	// Миграция RefreshToken
	log.Println("🔄 Миграция таблицы refresh_tokens...")
	if err := db.AutoMigrate(&models.RefreshToken{}); err != nil {
		log.Printf("❌ Ошибка миграции refresh_tokens: %v", err)
	} else {
		log.Println("✅ Таблица refresh_tokens успешно создана/обновлена")
	}

	// Проверяем созданные таблицы
	log.Println("")
	log.Println("📊 Проверка созданных таблиц:")

	if db.Migrator().HasTable(&models.LocalUser{}) {
		log.Println("  ✅ local_users - существует")
	} else {
		log.Println("  ❌ local_users - отсутствует")
	}

	if db.Migrator().HasTable(&models.RefreshToken{}) {
		log.Println("  ✅ refresh_tokens - существует")
	} else {
		log.Println("  ❌ refresh_tokens - отсутствует")
	}

	// Проверяем количество записей
	var localUserCount, refreshTokenCount int64
	db.Model(&models.LocalUser{}).Count(&localUserCount)
	db.Model(&models.RefreshToken{}).Count(&refreshTokenCount)

	log.Printf("📈 Статистика:")
	log.Printf("  👥 Локальных пользователей: %d", localUserCount)
	log.Printf("  🔑 Refresh токенов: %d", refreshTokenCount)

	log.Println("")
	log.Println("🎉 Миграция локальной авторизации завершена!")
	log.Println("")
	log.Println("💡 Теперь можно тестировать локальную авторизацию:")
	log.Println("   POST /api/local/login")
	log.Println("   POST /api/local/register")
	log.Println("   POST /api/local/refresh")
}
