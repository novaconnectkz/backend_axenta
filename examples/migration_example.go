package main

import (
	"fmt"
	"log"

	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
)

// Пример использования системы миграций
func main() {
	// Инициализируем конфигурацию
	config.LoadConfig()

	// Создаем базу данных если не существует
	if err := database.CreateDatabaseIfNotExists(); err != nil {
		log.Fatalf("Ошибка создания базы данных: %v", err)
	}

	// Подключаемся к базе данных
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("Ошибка подключения к базе данных: %v", err)
	}

	db := database.GetDB()

	fmt.Println("=== Пример использования системы миграций ===")

	// 1. Проверка существования таблицы
	fmt.Println("\n1. Проверка существования таблицы 'companies':")
	exists, err := database.CheckTableExists(db, "companies")
	if err != nil {
		log.Printf("Ошибка проверки: %v", err)
	} else {
		fmt.Printf("Таблица companies существует: %v\n", exists)
	}

	// 2. Получение информации о структуре таблицы
	if exists {
		fmt.Println("\n2. Информация о структуре таблицы 'companies':")
		tableInfo, err := database.GetTableInfo(db, "companies")
		if err != nil {
			log.Printf("Ошибка получения информации: %v", err)
		} else {
			fmt.Printf("Таблица: %s\n", tableInfo.TableName)
			fmt.Printf("Колонки (%d):\n", len(tableInfo.Columns))
			for _, col := range tableInfo.Columns {
				nullable := "NOT NULL"
				if col.IsNullable {
					nullable = "NULL"
				}
				fmt.Printf("  - %s: %s %s\n", col.Name, col.Type, nullable)
			}
			fmt.Printf("Индексы (%d):\n", len(tableInfo.Indexes))
			for _, idx := range tableInfo.Indexes {
				unique := ""
				if idx.IsUnique {
					unique = " (UNIQUE)"
				}
				fmt.Printf("  - %s: %v%s\n", idx.Name, idx.Columns, unique)
			}
		}
	}

	// 3. Сравнение структуры таблицы с моделью
	fmt.Println("\n3. Сравнение структуры таблицы с моделью:")
	migration := database.MigrationInfo{
		TableName:   "companies",
		Model:       &models.Company{},
		Description: "Таблица компаний",
		IsGlobal:    true,
	}

	differences, err := database.CompareTableStructure(db, migration)
	if err != nil {
		log.Printf("Ошибка сравнения: %v", err)
	} else {
		if len(differences) == 0 {
			fmt.Println("Структура таблицы соответствует модели")
		} else {
			fmt.Printf("Обнаружены различия (%d):\n", len(differences))
			for _, diff := range differences {
				fmt.Printf("  - %s\n", diff)
			}
		}
	}

	// 4. Выполнение одной миграции
	fmt.Println("\n4. Выполнение миграции для таблицы 'companies':")
	result := database.RunMigration(db, migration)
	fmt.Printf("Результат: %s\n", result.Action)
	fmt.Printf("Время выполнения: %v\n", result.Duration)
	if result.Error != nil {
		fmt.Printf("Ошибка: %v\n", result.Error)
	}
	if len(result.Changes) > 0 {
		fmt.Printf("Изменения:\n")
		for _, change := range result.Changes {
			fmt.Printf("  - %s\n", change)
		}
	}

	// 5. Получение списка всех миграций
	fmt.Println("\n5. Список всех доступных миграций:")
	migrations := database.GetAllMigrations()

	fmt.Println("Глобальные таблицы:")
	for _, mig := range migrations {
		if mig.IsGlobal {
			fmt.Printf("  - %s: %s\n", mig.TableName, mig.Description)
		}
	}

	fmt.Println("Тенантные таблицы:")
	for _, mig := range migrations {
		if !mig.IsGlobal {
			fmt.Printf("  - %s: %s\n", mig.TableName, mig.Description)
		}
	}

	// 6. Демонстрация создания схемы для новой компании
	fmt.Println("\n6. Пример создания схемы для новой компании:")

	// Создаем тестовую компанию
	testCompany := &models.Company{
		Name:           "Test Migration Company",
		DatabaseSchema: "tenant_migration_test",
		AxetnaLogin:    "test@example.com",
		AxetnaPassword: "encrypted_password",
		IsActive:       true,
	}

	// Сохраняем компанию в БД
	if err := db.Create(testCompany).Error; err != nil {
		log.Printf("Ошибка создания тестовой компании: %v", err)
	} else {
		fmt.Printf("Создана тестовая компания ID: %d\n", testCompany.ID)

		// Создаем схему для компании
		if err := database.CreateTenantSchema(testCompany.ID, testCompany.DatabaseSchema); err != nil {
			log.Printf("Ошибка создания схемы: %v", err)
		} else {
			fmt.Printf("Схема '%s' создана успешно\n", testCompany.DatabaseSchema)
		}

		// Удаляем тестовую компанию
		db.Delete(testCompany)
		fmt.Println("Тестовая компания удалена")
	}

	fmt.Println("\n=== Пример завершен ===")
}
