package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"log"
)

func main() {
	log.Println("🏢 Создание компании по умолчанию")
	log.Println("=================================")

	// Загружаем конфигурацию
	_, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("❌ Не удалось загрузить конфигурацию: %v", err)
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
	}

	// Проверяем, существует ли таблица companies
	var tableExists bool
	checkQuery := "SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'companies')"
	if err := db.Raw(checkQuery).Scan(&tableExists).Error; err != nil {
		log.Printf("⚠️ Ошибка проверки таблицы companies: %v", err)
	}

	if !tableExists {
		log.Println("🔄 Создаем таблицу companies...")

		createCompaniesSQL := `
			CREATE TABLE IF NOT EXISTS companies (
				id SERIAL PRIMARY KEY,
				created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
				deleted_at TIMESTAMP WITH TIME ZONE,
				name VARCHAR(255) NOT NULL,
				database_schema VARCHAR(100) UNIQUE NOT NULL,
				domain VARCHAR(255),
				axetna_login VARCHAR(255) NOT NULL,
				axetna_password VARCHAR(255) NOT NULL,
				bitrix24_webhook_url VARCHAR(500),
				bitrix24_client_id VARCHAR(255),
				bitrix24_client_secret VARCHAR(255),
				contact_email VARCHAR(255),
				contact_phone VARCHAR(50),
				contact_person VARCHAR(255),
				address TEXT,
				city VARCHAR(100),
				country VARCHAR(100) DEFAULT 'Russia',
				is_active BOOLEAN DEFAULT true,
				max_users INTEGER DEFAULT 10,
				max_objects INTEGER DEFAULT 100,
				storage_quota INTEGER DEFAULT 1024,
				language VARCHAR(5) DEFAULT 'ru',
				timezone VARCHAR(50) DEFAULT 'Europe/Moscow',
				currency VARCHAR(3) DEFAULT 'RUB',
				subscription_id VARCHAR(255)
			)`

		if err := db.Exec(createCompaniesSQL).Error; err != nil {
			log.Printf("❌ Ошибка создания таблицы companies: %v", err)
		} else {
			log.Println("✅ Таблица companies создана")
		}
	} else {
		log.Println("✅ Таблица companies уже существует")
	}

	// Проверяем, есть ли компания по умолчанию
	var companyCount int64
	if err := db.Raw("SELECT COUNT(*) FROM companies WHERE database_schema = 'tenant_default'").Scan(&companyCount).Error; err != nil {
		log.Printf("⚠️ Ошибка проверки компании по умолчанию: %v", err)
	}

	if companyCount == 0 {
		log.Println("🔄 Создаем компанию по умолчанию...")

		insertCompanySQL := `
			INSERT INTO companies (
				name, database_schema, axetna_login, axetna_password, 
				contact_email, is_active, created_at, updated_at
			) VALUES (
				'Компания по умолчанию', 'tenant_default', 'default', 'encrypted_password',
				'admin@example.com', true, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
			)`

		if err := db.Exec(insertCompanySQL).Error; err != nil {
			log.Printf("❌ Ошибка создания компании по умолчанию: %v", err)
		} else {
			log.Println("✅ Компания по умолчанию создана")
		}
	} else {
		log.Printf("✅ Компания по умолчанию уже существует (найдено записей: %d)", companyCount)
	}

	// Проверяем финальный результат
	log.Println("")
	log.Println("🔍 Финальная проверка:")

	var finalCount int64
	if err := db.Raw("SELECT COUNT(*) FROM companies WHERE is_active = true").Scan(&finalCount).Error; err != nil {
		log.Printf("❌ Ошибка финальной проверки: %v", err)
	} else {
		log.Printf("📊 Активных компаний в БД: %d", finalCount)
	}

	// Показываем список компаний
	rows, err := db.Raw("SELECT id, name, database_schema, is_active FROM companies LIMIT 5").Rows()
	if err != nil {
		log.Printf("⚠️ Не удалось получить список компаний: %v", err)
	} else {
		defer rows.Close()

		log.Println("📋 Список компаний:")
		for rows.Next() {
			var id int
			var name, schema string
			var isActive bool

			if err := rows.Scan(&id, &name, &schema, &isActive); err != nil {
				log.Printf("  ⚠️ Ошибка чтения записи: %v", err)
				continue
			}

			status := "✅"
			if !isActive {
				status = "❌"
			}

			log.Printf("  %s ID: %d, Имя: %s, Схема: %s", status, id, name, schema)
		}
	}

	log.Println("")
	log.Println("🎉 Настройка завершена!")
	log.Println("💡 Теперь middleware сможет найти компанию по умолчанию и использовать схему tenant_default")
}
