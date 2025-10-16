package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"flag"
	"fmt"
	"log"
	"os"
)

func main() {
	// Флаги командной строки
	globalOnly := flag.Bool("global-only", false, "Выполнить только глобальные миграции")
	force := flag.Bool("force", false, "Принудительно выполнить миграции")
	help := flag.Bool("help", false, "Показать справку")

	flag.Parse()

	if *help {
		fmt.Println("Утилита для выполнения миграций базы данных Axenta CRM")
		fmt.Println("")
		fmt.Println("Использование:")
		fmt.Println("  go run cmd/migrate/main.go [флаги]")
		fmt.Println("")
		fmt.Println("Флаги:")
		fmt.Println("  -global-only    Выполнить только глобальные миграции (схема public)")
		fmt.Println("  -force          Принудительно выполнить миграции")
		fmt.Println("  -help           Показать эту справку")
		fmt.Println("")
		fmt.Println("Примеры:")
		fmt.Println("  go run cmd/migrate/main.go                    # Выполнить все миграции")
		fmt.Println("  go run cmd/migrate/main.go -global-only       # Только глобальные таблицы")
		fmt.Println("  go run cmd/migrate/main.go -force             # Принудительно")
		return
	}

	log.Println("🚀 Запуск утилиты миграций Axenta CRM")

	if *force {
		log.Println("⚠️ ПРИНУДИТЕЛЬНЫЙ РЕЖИМ: Миграции будут выполнены принудительно")
	}

	if *globalOnly {
		log.Println("📋 Режим: только глобальные миграции")
	} else {
		log.Println("📋 Режим: все миграции (глобальные + тенантные)")
	}

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

	// Выполняем миграции
	log.Println("")
	log.Println("🔄 Начинаем выполнение миграций...")

	err = database.RunAllMigrations(*globalOnly)
	if err != nil {
		log.Printf("❌ Ошибка выполнения миграций: %v", err)
		if !*force {
			os.Exit(1)
		} else {
			log.Println("⚠️ Игнорируем ошибку в принудительном режиме")
		}
	}

	log.Println("")
	log.Println("🎉 Миграции завершены успешно!")

	// Показываем информацию о созданных таблицах
	log.Println("")
	log.Println("📊 Проверяем созданные таблицы...")

	// Проверяем глобальные таблицы
	globalTables := []string{
		"companies", "billing_plans", "subscriptions",
		"integrations", "integration_errors",
		"local_users", "refresh_tokens", "user_tokens",
	}

	db := database.GetDB()

	log.Println("📋 Глобальные таблицы (схема public):")
	for _, table := range globalTables {
		if db.Migrator().HasTable(table) {
			log.Printf("  ✅ %s", table)
		} else {
			log.Printf("  ❌ %s (отсутствует)", table)
		}
	}

	if !*globalOnly {
		// Получаем список компаний для проверки тенантных таблиц
		var companies []struct {
			ID             uint   `json:"id"`
			Name           string `json:"name"`
			DatabaseSchema string `json:"database_schema"`
			IsActive       bool   `json:"is_active"`
		}

		if err := db.Table("companies").Find(&companies).Error; err != nil {
			log.Printf("⚠️ Не удалось получить список компаний: %v", err)
		} else {
			log.Printf("📋 Найдено компаний: %d", len(companies))

			for _, company := range companies {
				if !company.IsActive {
					log.Printf("  ⏭️ %s (схема: %s) - деактивирована", company.Name, company.DatabaseSchema)
					continue
				}

				log.Printf("  🏢 %s (схема: %s)", company.Name, company.DatabaseSchema)

				// Переключаемся на схему компании для проверки
				tenantDB := db.Exec(fmt.Sprintf("SET search_path TO %s", company.DatabaseSchema))
				if tenantDB.Error != nil {
					log.Printf("    ❌ Ошибка переключения на схему: %v", tenantDB.Error)
					continue
				}

				// Проверяем основные тенантные таблицы
				tenantTables := []string{"users", "roles", "permissions", "objects", "contracts", "user_tokens"}
				for _, table := range tenantTables {
					if db.Migrator().HasTable(table) {
						log.Printf("    ✅ %s", table)
					} else {
						log.Printf("    ❌ %s (отсутствует)", table)
					}
				}
			}

			// Возвращаемся к схеме public
			db.Exec("SET search_path TO public")
		}
	}

	log.Println("")
	log.Println("✨ Готово! Теперь можно запускать приложение.")
}
