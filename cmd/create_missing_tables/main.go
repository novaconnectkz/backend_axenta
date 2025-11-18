package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"log"
)

func main() {
	log.Println("🔧 Создание недостающих таблиц в tenant_default")
	log.Println("===============================================")

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

	// Создаем схему tenant_default если её нет
	if err := db.Exec("CREATE SCHEMA IF NOT EXISTS tenant_default").Error; err != nil {
		log.Printf("⚠️ Не удалось создать схему tenant_default: %v", err)
	}

	// Переключаемся на схему tenant_default
	if err := db.Exec("SET search_path TO tenant_default").Error; err != nil {
		log.Fatalf("❌ Не удалось переключиться на схему tenant_default: %v", err)
	}

	log.Println("✅ Переключились на схему tenant_default")

	// Создаем таблицы напрямую через SQL
	tables := map[string]string{
		"permissions": `
			CREATE TABLE IF NOT EXISTS permissions (
				id SERIAL PRIMARY KEY,
				created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
				deleted_at TIMESTAMP WITH TIME ZONE,
				name VARCHAR(100) UNIQUE NOT NULL,
				display_name VARCHAR(100) NOT NULL,
				description TEXT,
				resource VARCHAR(50) NOT NULL,
				action VARCHAR(50) NOT NULL,
				category VARCHAR(50),
				is_active BOOLEAN DEFAULT true
			)`,

		"roles": `
			CREATE TABLE IF NOT EXISTS roles (
				id SERIAL PRIMARY KEY,
				created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
				deleted_at TIMESTAMP WITH TIME ZONE,
				name VARCHAR(100) UNIQUE NOT NULL,
				display_name VARCHAR(100) NOT NULL,
				description TEXT,
				color VARCHAR(7),
				priority INTEGER DEFAULT 0,
				is_active BOOLEAN DEFAULT true,
				is_system BOOLEAN DEFAULT false
			)`,

		"role_permissions": `
			CREATE TABLE IF NOT EXISTS role_permissions (
				role_id INTEGER NOT NULL,
				permission_id INTEGER NOT NULL,
				PRIMARY KEY (role_id, permission_id),
				FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE,
				FOREIGN KEY (permission_id) REFERENCES permissions(id) ON DELETE CASCADE
			)`,

		"user_templates": `
			CREATE TABLE IF NOT EXISTS user_templates (
				id SERIAL PRIMARY KEY,
				created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
				deleted_at TIMESTAMP WITH TIME ZONE,
				name VARCHAR(255) NOT NULL,
				description TEXT,
				role_id INTEGER,
				settings JSONB,
				is_active BOOLEAN DEFAULT true
			)`,

		"users": `
			CREATE TABLE IF NOT EXISTS users (
				id SERIAL PRIMARY KEY,
				created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
				deleted_at TIMESTAMP WITH TIME ZONE,
				username VARCHAR(255) UNIQUE NOT NULL,
				email VARCHAR(255) UNIQUE NOT NULL,
				password VARCHAR(255) NOT NULL,
				first_name VARCHAR(255),
				last_name VARCHAR(255),
				name VARCHAR(200),
				phone VARCHAR(50),
				telegram_id VARCHAR(50),
				is_active BOOLEAN DEFAULT true,
				user_type VARCHAR(50) DEFAULT 'user',
				external_id VARCHAR(100),
				external_source VARCHAR(50),
				axenta_user_type VARCHAR(50),
				axenta_user_id VARCHAR(100),
				is_axenta_user BOOLEAN DEFAULT false,
				company_id INTEGER,
				role_id INTEGER,
				template_id INTEGER,
				last_login TIMESTAMP WITH TIME ZONE,
				login_count INTEGER DEFAULT 0
			)`,

		"objects": `
			CREATE TABLE IF NOT EXISTS objects (
				id SERIAL PRIMARY KEY,
				created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
				deleted_at TIMESTAMP WITH TIME ZONE,
				name VARCHAR(255) NOT NULL,
				description TEXT,
				type VARCHAR(100) NOT NULL,
				status VARCHAR(50) DEFAULT 'active',
				external_id VARCHAR(100),
				is_active BOOLEAN DEFAULT true,
				company_id INTEGER,
				template_id INTEGER,
				location VARCHAR(255),
				coordinates VARCHAR(255),
				metadata JSONB
			)`,

		"contracts": `
			CREATE TABLE IF NOT EXISTS contracts (
				id SERIAL PRIMARY KEY,
				created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
				deleted_at TIMESTAMP WITH TIME ZONE,
				number VARCHAR(100) UNIQUE NOT NULL,
				name VARCHAR(255) NOT NULL,
				description TEXT,
				client_name VARCHAR(255) NOT NULL,
				client_contact VARCHAR(255),
				start_date DATE,
				end_date DATE,
				status VARCHAR(50) DEFAULT 'active',
				total_amount DECIMAL(15,2) DEFAULT 0,
				currency VARCHAR(3) DEFAULT 'RUB',
				billing_period VARCHAR(50) DEFAULT 'monthly',
				is_active BOOLEAN DEFAULT true,
				company_id INTEGER
			)`,
	}

	log.Printf("🔄 Создаем %d критических таблиц...", len(tables))

	successCount := 0
	errorCount := 0

	for tableName, createSQL := range tables {
		log.Printf("  🔄 Создание таблицы %s...", tableName)

		if err := db.Exec(createSQL).Error; err != nil {
			log.Printf("  ❌ Ошибка создания %s: %v", tableName, err)
			errorCount++
		} else {
			log.Printf("  ✅ Таблица %s создана", tableName)
			successCount++
		}
	}

	// Создаем индексы
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users(deleted_at)",
		"CREATE INDEX IF NOT EXISTS idx_users_company_id ON users(company_id)",
		"CREATE INDEX IF NOT EXISTS idx_users_role_id ON users(role_id)",
		"CREATE INDEX IF NOT EXISTS idx_roles_deleted_at ON roles(deleted_at)",
		"CREATE INDEX IF NOT EXISTS idx_permissions_deleted_at ON permissions(deleted_at)",
		"CREATE INDEX IF NOT EXISTS idx_user_templates_deleted_at ON user_templates(deleted_at)",
		"CREATE INDEX IF NOT EXISTS idx_objects_deleted_at ON objects(deleted_at)",
		"CREATE INDEX IF NOT EXISTS idx_objects_company_id ON objects(company_id)",
		"CREATE INDEX IF NOT EXISTS idx_contracts_deleted_at ON contracts(deleted_at)",
		"CREATE INDEX IF NOT EXISTS idx_contracts_company_id ON contracts(company_id)",
	}

	log.Printf("🔄 Создаем %d индексов...", len(indexes))

	for _, indexSQL := range indexes {
		if err := db.Exec(indexSQL).Error; err != nil {
			log.Printf("  ⚠️ Ошибка создания индекса: %v", err)
		}
	}

	// Возвращаемся к схеме public
	if err := db.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️ Не удалось вернуться к схеме public: %v", err)
	}

	log.Println("")
	log.Println("📊 Итоговая статистика:")
	log.Printf("  ✅ Успешно создано таблиц: %d", successCount)
	log.Printf("  ❌ Ошибок: %d", errorCount)

	// Финальная проверка
	log.Println("")
	log.Println("🔍 Финальная проверка критических таблиц:")

	if err := db.Exec("SET search_path TO tenant_default").Error; err != nil {
		log.Printf("❌ Не удалось переключиться на tenant_default: %v", err)
	} else {
		criticalTables := []string{"permissions", "roles", "role_permissions", "user_templates", "users"}
		allExist := true

		for _, tableName := range criticalTables {
			var exists bool
			checkQuery := "SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = ?)"
			if err := db.Raw(checkQuery, tableName).Scan(&exists).Error; err != nil {
				log.Printf("  ❌ Ошибка проверки %s: %v", tableName, err)
				allExist = false
			} else if exists {
				log.Printf("  ✅ %s - существует", tableName)
			} else {
				log.Printf("  ❌ %s - отсутствует", tableName)
				allExist = false
			}
		}

		if allExist {
			log.Println("")
			log.Println("🎉 Все критические таблицы созданы успешно!")
			log.Println("✨ Теперь /api/auth/roles и /api/auth/user-templates должны работать!")
		} else {
			log.Println("")
			log.Println("⚠️ Некоторые таблицы отсутствуют. Проверьте ошибки выше.")
		}
	}

	// Возвращаемся к public
	db.Exec("SET search_path TO public")
}
