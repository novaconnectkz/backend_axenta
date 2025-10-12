package main

import (
	"flag"
	"fmt"
	"log"

	"backend_axenta/config"
	"backend_axenta/database"
)

func main() {
	// Определяем флаги командной строки
	var (
		globalOnly   = flag.Bool("global", false, "Выполнить только глобальные миграции (таблицы в схеме public)")
		createSchema = flag.String("create-schema", "", "Создать схему для новой компании (укажите имя схемы)")
		companyID    = flag.Uint("company-id", 0, "ID компании для создания схемы (используется с -create-schema)")
		dryRun       = flag.Bool("dry-run", false, "Показать, какие миграции будут выполнены, но не выполнять их")
		help         = flag.Bool("help", false, "Показать справку")
	)

	flag.Parse()

	if *help {
		printHelp()
		return
	}

	// Инициализируем конфигурацию
	config.LoadConfig()

	// Создаем базу данных если не существует
	if err := database.CreateDatabaseIfNotExists(); err != nil {
		log.Fatalf("❌ Ошибка создания базы данных: %v", err)
	}

	// Подключаемся к базе данных
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("❌ Ошибка подключения к базе данных: %v", err)
	}

	// Выполняем запрошенные операции
	if *createSchema != "" {
		if *companyID == 0 {
			log.Fatal("❌ Для создания схемы необходимо указать ID компании с флагом -company-id")
		}

		if err := database.CreateTenantSchema(*companyID, *createSchema); err != nil {
			log.Fatalf("❌ Ошибка создания схемы: %v", err)
		}

		log.Printf("✅ Схема %s для компании ID %d создана успешно", *createSchema, *companyID)
		return
	}

	if *dryRun {
		log.Println("🔍 Режим dry-run: показываем, какие миграции будут выполнены")
		if err := showMigrationPlan(*globalOnly); err != nil {
			log.Fatalf("❌ Ошибка анализа миграций: %v", err)
		}
		return
	}

	// Выполняем миграции
	log.Println("🚀 Запуск миграций базы данных")

	if err := database.RunAllMigrations(*globalOnly); err != nil {
		log.Fatalf("❌ Ошибка выполнения миграций: %v", err)
	}

	log.Println("🎉 Миграции завершены успешно!")
}

// showMigrationPlan показывает план миграций без их выполнения
func showMigrationPlan(globalOnly bool) error {
	migrations := database.GetAllMigrations()

	log.Println("📋 План миграций:")

	// Показываем глобальные миграции
	log.Println("\n🌍 Глобальные таблицы (схема public):")
	for _, migration := range migrations {
		if migration.IsGlobal {
			exists, err := database.CheckTableExists(database.GetDB(), migration.TableName)
			if err != nil {
				return fmt.Errorf("ошибка проверки таблицы %s: %v", migration.TableName, err)
			}

			status := "✅ существует"
			if !exists {
				status = "🆕 будет создана"
			} else {
				// Проверяем структуру
				differences, err := database.CompareTableStructure(database.GetDB(), migration)
				if err != nil {
					return fmt.Errorf("ошибка проверки структуры таблицы %s: %v", migration.TableName, err)
				}

				if len(differences) > 0 {
					status = fmt.Sprintf("🔄 будет обновлена (%d изменений)", len(differences))
				}
			}

			log.Printf("   - %s: %s - %s", migration.TableName, migration.Description, status)
		}
	}

	if !globalOnly {
		// Показываем тенантные миграции
		log.Println("\n🏢 Тенантные таблицы:")
		for _, migration := range migrations {
			if !migration.IsGlobal {
				log.Printf("   - %s: %s", migration.TableName, migration.Description)
			}
		}

		log.Println("\n📝 Примечание: Тенантные таблицы будут проверены/созданы для каждой активной компании")
	}

	return nil
}

// printHelp выводит справку по использованию
func printHelp() {
	fmt.Println("Утилита миграции базы данных Axenta CRM")
	fmt.Println()
	fmt.Println("Использование:")
	fmt.Println("  migrate [флаги]")
	fmt.Println()
	fmt.Println("Флаги:")
	fmt.Println("  -global              Выполнить только глобальные миграции (таблицы в схеме public)")
	fmt.Println("  -create-schema NAME  Создать схему для новой компании")
	fmt.Println("  -company-id ID       ID компании для создания схемы (используется с -create-schema)")
	fmt.Println("  -dry-run             Показать план миграций без их выполнения")
	fmt.Println("  -help                Показать эту справку")
	fmt.Println()
	fmt.Println("Примеры:")
	fmt.Println("  migrate                                    # Выполнить все миграции")
	fmt.Println("  migrate -global                            # Только глобальные миграции")
	fmt.Println("  migrate -dry-run                           # Показать план миграций")
	fmt.Println("  migrate -create-schema tenant_123 -company-id 123  # Создать схему для компании")
	fmt.Println()
	fmt.Println("Переменные окружения:")
	fmt.Println("  DB_HOST      Хост базы данных (по умолчанию: localhost)")
	fmt.Println("  DB_PORT      Порт базы данных (по умолчанию: 5432)")
	fmt.Println("  DB_USER      Пользователь базы данных")
	fmt.Println("  DB_PASSWORD  Пароль базы данных")
	fmt.Println("  DB_NAME      Имя базы данных")
	fmt.Println("  DB_SSLMODE   Режим SSL (по умолчанию: disable)")
}
