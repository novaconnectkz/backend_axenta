package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// MigrationVerificationResult содержит результат проверки миграций
type MigrationVerificationResult struct {
	Environment     string                    `json:"environment"`
	DatabaseInfo    DatabaseInfo              `json:"database_info"`
	GlobalTables    []TableVerificationResult `json:"global_tables"`
	TenantSchemas   []TenantSchemaResult      `json:"tenant_schemas"`
	Summary         VerificationSummary       `json:"summary"`
	Timestamp       time.Time                 `json:"timestamp"`
	Recommendations []string                  `json:"recommendations"`
}

// DatabaseInfo содержит информацию о базе данных
type DatabaseInfo struct {
	Host         string `json:"host"`
	Port         string `json:"port"`
	DatabaseName string `json:"database_name"`
	Version      string `json:"version"`
	Connected    bool   `json:"connected"`
	Error        string `json:"error,omitempty"`
}

// TableVerificationResult содержит результат проверки таблицы
type TableVerificationResult struct {
	TableName    string     `json:"table_name"`
	Description  string     `json:"description"`
	Exists       bool       `json:"exists"`
	ColumnCount  int        `json:"column_count"`
	IndexCount   int        `json:"index_count"`
	RecordCount  int64      `json:"record_count"`
	Issues       []string   `json:"issues"`
	Status       string     `json:"status"` // ok, warning, error
	LastModified *time.Time `json:"last_modified,omitempty"`
}

// TenantSchemaResult содержит результат проверки схемы компании
type TenantSchemaResult struct {
	CompanyID   uint                      `json:"company_id"`
	CompanyName string                    `json:"company_name"`
	SchemaName  string                    `json:"schema_name"`
	IsActive    bool                      `json:"is_active"`
	Tables      []TableVerificationResult `json:"tables"`
	Issues      []string                  `json:"issues"`
	Status      string                    `json:"status"`
}

// VerificationSummary содержит сводку по проверке
type VerificationSummary struct {
	TotalTables       int    `json:"total_tables"`
	TablesOK          int    `json:"tables_ok"`
	TablesWithWarning int    `json:"tables_with_warning"`
	TablesWithError   int    `json:"tables_with_error"`
	TotalCompanies    int    `json:"total_companies"`
	ActiveCompanies   int    `json:"active_companies"`
	OverallStatus     string `json:"overall_status"`
}

func main() {
	fmt.Println("🔍 Проверка миграций базы данных Axenta CRM")
	fmt.Println("==========================================")

	// Проверяем аргументы командной строки
	var environment string
	var outputFile string
	var checkProduction bool

	for i, arg := range os.Args[1:] {
		switch arg {
		case "--env", "-e":
			if i+1 < len(os.Args[1:]) {
				environment = os.Args[i+2]
			}
		case "--output", "-o":
			if i+1 < len(os.Args[1:]) {
				outputFile = os.Args[i+2]
			}
		case "--production", "-p":
			checkProduction = true
		case "--help", "-h":
			printHelp()
			return
		}
	}

	// Устанавливаем окружение
	if environment != "" {
		os.Setenv("APP_ENV", environment)
	}
	if checkProduction {
		os.Setenv("APP_ENV", "production")
		environment = "production"
	}

	// Загружаем конфигурацию
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("❌ Ошибка загрузки конфигурации: %v", err)
	}

	fmt.Printf("🌍 Окружение: %s\n", cfg.App.Env)
	fmt.Printf("🗄️  База данных: %s@%s:%s/%s\n", cfg.Database.User, cfg.Database.Host, cfg.Database.Port, cfg.Database.Name)

	// Подключаемся к базе данных
	db, dbInfo := connectToDatabase(cfg)
	if db == nil {
		log.Fatalf("❌ Не удалось подключиться к базе данных: %s", dbInfo.Error)
	}

	// Устанавливаем глобальную переменную для совместимости
	database.DB = db

	// Выполняем проверку
	result := verifyMigrations(db, cfg, dbInfo)

	// Выводим результаты
	printResults(result)

	// Сохраняем в файл если указан
	if outputFile != "" {
		saveResults(result, outputFile)
	}

	// Определяем код выхода
	exitCode := 0
	if result.Summary.OverallStatus == "error" {
		exitCode = 1
	} else if result.Summary.OverallStatus == "warning" {
		exitCode = 2
	}

	fmt.Printf("\n🎯 Проверка завершена со статусом: %s\n", result.Summary.OverallStatus)
	os.Exit(exitCode)
}

func printHelp() {
	fmt.Println("Использование: go run cmd/verify_migration/main.go [опции]")
	fmt.Println("")
	fmt.Println("Опции:")
	fmt.Println("  --env, -e <env>        Окружение (development, production)")
	fmt.Println("  --production, -p       Проверить продакшен (эквивалент --env production)")
	fmt.Println("  --output, -o <file>    Сохранить результат в JSON файл")
	fmt.Println("  --help, -h             Показать эту справку")
	fmt.Println("")
	fmt.Println("Примеры:")
	fmt.Println("  go run cmd/verify_migration/main.go --env development")
	fmt.Println("  go run cmd/verify_migration/main.go --production --output migration_check.json")
}

func connectToDatabase(cfg *config.Config) (*gorm.DB, DatabaseInfo) {
	dbInfo := DatabaseInfo{
		Host:         cfg.Database.Host,
		Port:         cfg.Database.Port,
		DatabaseName: cfg.Database.Name,
		Connected:    false,
	}

	dsn := cfg.GetDatabaseDSN()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})

	if err != nil {
		dbInfo.Error = err.Error()
		return nil, dbInfo
	}

	// Проверяем соединение
	sqlDB, err := db.DB()
	if err != nil {
		dbInfo.Error = err.Error()
		return nil, dbInfo
	}

	if err := sqlDB.Ping(); err != nil {
		dbInfo.Error = err.Error()
		return nil, dbInfo
	}

	// Получаем версию PostgreSQL
	var version string
	db.Raw("SELECT version()").Scan(&version)
	dbInfo.Version = version
	dbInfo.Connected = true

	return db, dbInfo
}

func verifyMigrations(db *gorm.DB, cfg *config.Config, dbInfo DatabaseInfo) MigrationVerificationResult {
	result := MigrationVerificationResult{
		Environment:     cfg.App.Env,
		DatabaseInfo:    dbInfo,
		GlobalTables:    []TableVerificationResult{},
		TenantSchemas:   []TenantSchemaResult{},
		Timestamp:       time.Now(),
		Recommendations: []string{},
	}

	// Переключаемся на схему public для проверки глобальных таблиц
	db.Exec("SET search_path TO public")

	// Проверяем глобальные таблицы
	fmt.Println("\n📋 Проверка глобальных таблиц (схема public):")
	migrations := database.GetAllMigrations()

	for _, migration := range migrations {
		if migration.IsGlobal {
			tableResult := verifyTable(db, migration)
			result.GlobalTables = append(result.GlobalTables, tableResult)

			status := "✅"
			if tableResult.Status == "warning" {
				status = "⚠️"
			} else if tableResult.Status == "error" {
				status = "❌"
			}

			fmt.Printf("  %s %s (%d записей)\n", status, tableResult.TableName, tableResult.RecordCount)
			if len(tableResult.Issues) > 0 {
				for _, issue := range tableResult.Issues {
					fmt.Printf("    - %s\n", issue)
				}
			}
		}
	}

	// Проверяем схемы компаний
	fmt.Println("\n🏢 Проверка схем компаний:")
	companies := getCompanies(db)

	for _, company := range companies {
		tenantResult := verifyTenantSchema(db, company, migrations)
		result.TenantSchemas = append(result.TenantSchemas, tenantResult)

		status := "✅"
		if tenantResult.Status == "warning" {
			status = "⚠️"
		} else if tenantResult.Status == "error" {
			status = "❌"
		}

		activeStatus := ""
		if !company.IsActive {
			activeStatus = " (неактивная)"
		}

		fmt.Printf("  %s %s (%s)%s - %d таблиц\n",
			status, company.Name, tenantResult.SchemaName, activeStatus, len(tenantResult.Tables))

		if len(tenantResult.Issues) > 0 {
			for _, issue := range tenantResult.Issues {
				fmt.Printf("    - %s\n", issue)
			}
		}
	}

	// Возвращаемся к схеме public
	db.Exec("SET search_path TO public")

	// Подсчитываем сводку
	result.Summary = calculateSummary(result)

	// Генерируем рекомендации
	result.Recommendations = generateRecommendations(result)

	return result
}

func verifyTable(db *gorm.DB, migration database.MigrationInfo) TableVerificationResult {
	result := TableVerificationResult{
		TableName:   migration.TableName,
		Description: migration.Description,
		Issues:      []string{},
		Status:      "ok",
	}

	// Проверяем существование таблицы
	exists, err := database.CheckTableExists(db, migration.TableName)
	if err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("Ошибка проверки существования: %v", err))
		result.Status = "error"
		return result
	}

	result.Exists = exists
	if !exists {
		result.Issues = append(result.Issues, "Таблица не существует")
		result.Status = "error"
		return result
	}

	// Получаем информацию о таблице
	tableInfo, err := database.GetTableInfo(db, migration.TableName)
	if err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("Ошибка получения информации о таблице: %v", err))
		result.Status = "error"
		return result
	}

	result.ColumnCount = len(tableInfo.Columns)
	result.IndexCount = len(tableInfo.Indexes)

	// Подсчитываем количество записей
	var count int64
	if err := db.Table(migration.TableName).Count(&count).Error; err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("Ошибка подсчета записей: %v", err))
		result.Status = "warning"
	} else {
		result.RecordCount = count
	}

	// Базовая проверка структуры (без детального сравнения колонок)
	if migration.Model != nil && result.ColumnCount < 3 {
		result.Issues = append(result.Issues, "Подозрительно мало колонок в таблице")
		result.Status = "warning"
	}

	// Получаем время последнего изменения (если возможно)
	var lastModified time.Time
	err = db.Raw(`
		SELECT GREATEST(
			COALESCE(MAX(created_at), '1970-01-01'::timestamp),
			COALESCE(MAX(updated_at), '1970-01-01'::timestamp)
		) as last_modified
		FROM ` + migration.TableName + `
		WHERE created_at IS NOT NULL OR updated_at IS NOT NULL
	`).Scan(&lastModified).Error

	if err == nil && !lastModified.IsZero() {
		result.LastModified = &lastModified
	}

	return result
}

func verifyTenantSchema(db *gorm.DB, company models.Company, migrations []database.MigrationInfo) TenantSchemaResult {
	result := TenantSchemaResult{
		CompanyID:   company.ID,
		CompanyName: company.Name,
		SchemaName:  company.DatabaseSchema,
		IsActive:    company.IsActive,
		Tables:      []TableVerificationResult{},
		Issues:      []string{},
		Status:      "ok",
	}

	// Проверяем существование схемы
	var schemaExists bool
	err := db.Raw(`
		SELECT EXISTS(
			SELECT 1 FROM information_schema.schemata 
			WHERE schema_name = ?
		)
	`, company.DatabaseSchema).Scan(&schemaExists).Error

	if err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("Ошибка проверки схемы: %v", err))
		result.Status = "error"
		return result
	}

	if !schemaExists {
		result.Issues = append(result.Issues, "Схема не существует")
		result.Status = "error"
		return result
	}

	// Переключаемся на схему компании
	err = db.Exec(fmt.Sprintf("SET search_path TO %s", company.DatabaseSchema)).Error
	if err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("Ошибка переключения на схему: %v", err))
		result.Status = "error"
		return result
	}

	// Проверяем тенантные таблицы
	for _, migration := range migrations {
		if !migration.IsGlobal {
			tableResult := verifyTable(db, migration)
			result.Tables = append(result.Tables, tableResult)

			if tableResult.Status == "error" {
				result.Status = "error"
			} else if tableResult.Status == "warning" && result.Status == "ok" {
				result.Status = "warning"
			}
		}
	}

	return result
}

func getCompanies(db *gorm.DB) []models.Company {
	var companies []models.Company
	db.Find(&companies)
	return companies
}

func calculateSummary(result MigrationVerificationResult) VerificationSummary {
	summary := VerificationSummary{}

	// Подсчитываем глобальные таблицы
	for _, table := range result.GlobalTables {
		summary.TotalTables++
		switch table.Status {
		case "ok":
			summary.TablesOK++
		case "warning":
			summary.TablesWithWarning++
		case "error":
			summary.TablesWithError++
		}
	}

	// Подсчитываем тенантные таблицы
	for _, tenant := range result.TenantSchemas {
		summary.TotalCompanies++
		if tenant.IsActive {
			summary.ActiveCompanies++
		}

		for _, table := range tenant.Tables {
			summary.TotalTables++
			switch table.Status {
			case "ok":
				summary.TablesOK++
			case "warning":
				summary.TablesWithWarning++
			case "error":
				summary.TablesWithError++
			}
		}
	}

	// Определяем общий статус
	if summary.TablesWithError > 0 {
		summary.OverallStatus = "error"
	} else if summary.TablesWithWarning > 0 {
		summary.OverallStatus = "warning"
	} else {
		summary.OverallStatus = "ok"
	}

	return summary
}

func generateRecommendations(result MigrationVerificationResult) []string {
	var recommendations []string

	// Проверяем глобальные таблицы
	for _, table := range result.GlobalTables {
		if !table.Exists {
			recommendations = append(recommendations,
				fmt.Sprintf("Выполните миграцию для создания таблицы %s", table.TableName))
		}
		if len(table.Issues) > 0 && table.Status == "warning" {
			recommendations = append(recommendations,
				fmt.Sprintf("Проверьте структуру таблицы %s: %s", table.TableName, strings.Join(table.Issues, ", ")))
		}
	}

	// Проверяем схемы компаний
	missingSchemas := 0
	for _, tenant := range result.TenantSchemas {
		if tenant.Status == "error" {
			missingSchemas++
		}
	}

	if missingSchemas > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("Создайте схемы для %d компаний с помощью CreateTenantSchema", missingSchemas))
	}

	// Общие рекомендации
	if result.Summary.TablesWithError > 0 {
		recommendations = append(recommendations, "Выполните полную миграцию: go run cmd/migrate/main.go")
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Все миграции выполнены корректно")
	}

	return recommendations
}

func printResults(result MigrationVerificationResult) {
	fmt.Println("\n📊 Сводка по проверке миграций:")
	fmt.Println("================================")
	fmt.Printf("🌍 Окружение: %s\n", result.Environment)
	fmt.Printf("🗄️  База данных: %s (%s)\n", result.DatabaseInfo.DatabaseName, result.DatabaseInfo.Version)
	fmt.Printf("📅 Время проверки: %s\n", result.Timestamp.Format("2006-01-02 15:04:05"))
	fmt.Println()

	fmt.Printf("📋 Всего таблиц: %d\n", result.Summary.TotalTables)
	fmt.Printf("✅ Корректных: %d\n", result.Summary.TablesOK)
	fmt.Printf("⚠️  С предупреждениями: %d\n", result.Summary.TablesWithWarning)
	fmt.Printf("❌ С ошибками: %d\n", result.Summary.TablesWithError)
	fmt.Println()

	fmt.Printf("🏢 Всего компаний: %d\n", result.Summary.TotalCompanies)
	fmt.Printf("🟢 Активных: %d\n", result.Summary.ActiveCompanies)
	fmt.Println()

	fmt.Printf("🎯 Общий статус: %s\n", strings.ToUpper(result.Summary.OverallStatus))
	fmt.Println()

	if len(result.Recommendations) > 0 {
		fmt.Println("💡 Рекомендации:")
		for i, rec := range result.Recommendations {
			fmt.Printf("  %d. %s\n", i+1, rec)
		}
	}
}

func saveResults(result MigrationVerificationResult, filename string) {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Printf("❌ Ошибка сериализации результатов: %v", err)
		return
	}

	err = os.WriteFile(filename, data, 0644)
	if err != nil {
		log.Printf("❌ Ошибка сохранения файла %s: %v", filename, err)
		return
	}

	fmt.Printf("💾 Результаты сохранены в файл: %s\n", filename)
}
