package database

import (
	"fmt"
	"log"
	"reflect"
	"strings"
	"time"

	"backend_axenta/models"

	"gorm.io/gorm"
)

// MigrationInfo содержит информацию о миграции
type MigrationInfo struct {
	TableName   string
	Model       interface{}
	Description string
	IsGlobal    bool // Глобальная таблица (в схеме public) или тенантная
}

// TableInfo содержит информацию о таблице в БД
type TableInfo struct {
	TableName string
	Columns   []ColumnInfo
	Indexes   []IndexInfo
}

// ColumnInfo содержит информацию о колонке
type ColumnInfo struct {
	Name         string
	Type         string
	IsNullable   bool
	DefaultValue *string
	IsPrimaryKey bool
}

// IndexInfo содержит информацию об индексе
type IndexInfo struct {
	Name     string
	Columns  []string
	IsUnique bool
}

// MigrationResult содержит результат миграции
type MigrationResult struct {
	TableName string
	Action    string // created, updated, skipped, error
	Changes   []string
	Error     error
	Duration  time.Duration
}

// GetAllMigrations возвращает список всех миграций
func GetAllMigrations() []MigrationInfo {
	return []MigrationInfo{
		// Глобальные таблицы (в схеме public)
		{
			TableName:   "companies",
			Model:       &models.Company{},
			Description: "Таблица компаний (мультитенантность)",
			IsGlobal:    true,
		},
		{
			TableName:   "billing_plans",
			Model:       &models.BillingPlan{},
			Description: "Тарифные планы",
			IsGlobal:    true,
		},
		{
			TableName:   "subscriptions",
			Model:       &models.Subscription{},
			Description: "Подписки компаний",
			IsGlobal:    true,
		},
		{
			TableName:   "integrations",
			Model:       &models.Integration{},
			Description: "Интеграции с внешними системами",
			IsGlobal:    true,
		},
		{
			TableName:   "integration_errors",
			Model:       &models.IntegrationError{},
			Description: "Ошибки интеграций",
			IsGlobal:    true,
		},
		{
			TableName:   "local_users",
			Model:       &models.LocalUser{},
			Description: "Локальные пользователи для альтернативной авторизации",
			IsGlobal:    true,
		},
		{
			TableName:   "refresh_tokens",
			Model:       &models.RefreshToken{},
			Description: "Refresh токены для локальной авторизации",
			IsGlobal:    true,
		},

		// Тенантные таблицы (в схемах компаний)
		{
			TableName:   "users",
			Model:       &models.User{},
			Description: "Пользователи",
			IsGlobal:    false,
		},
		{
			TableName:   "roles",
			Model:       &models.Role{},
			Description: "Роли пользователей",
			IsGlobal:    false,
		},
		{
			TableName:   "permissions",
			Model:       &models.Permission{},
			Description: "Разрешения",
			IsGlobal:    false,
		},
		{
			TableName:   "role_permissions",
			Model:       nil, // Many-to-many таблица
			Description: "Связь ролей и разрешений",
			IsGlobal:    false,
		},
		{
			TableName:   "user_templates",
			Model:       &models.UserTemplate{},
			Description: "Шаблоны пользователей",
			IsGlobal:    false,
		},
		{
			TableName:   "objects",
			Model:       &models.Object{},
			Description: "Объекты мониторинга",
			IsGlobal:    false,
		},
		{
			TableName:   "object_templates",
			Model:       &models.ObjectTemplate{},
			Description: "Шаблоны объектов",
			IsGlobal:    false,
		},
		{
			TableName:   "contracts",
			Model:       &models.Contract{},
			Description: "Договоры",
			IsGlobal:    false,
		},
		{
			TableName:   "contract_appendices",
			Model:       &models.ContractAppendix{},
			Description: "Приложения к договорам",
			IsGlobal:    false,
		},
		{
			TableName:   "tariff_plans",
			Model:       &models.TariffPlan{},
			Description: "Тарифные планы компаний",
			IsGlobal:    false,
		},
		{
			TableName:   "equipment",
			Model:       &models.Equipment{},
			Description: "Оборудование на складе",
			IsGlobal:    false,
		},
		{
			TableName:   "equipment_categories",
			Model:       &models.EquipmentCategory{},
			Description: "Категории оборудования",
			IsGlobal:    false,
		},
		{
			TableName:   "locations",
			Model:       &models.Location{},
			Description: "Локации (города)",
			IsGlobal:    false,
		},
		{
			TableName:   "installers",
			Model:       &models.Installer{},
			Description: "Монтажники",
			IsGlobal:    false,
		},
		{
			TableName:   "installer_locations",
			Model:       nil, // Many-to-many таблица
			Description: "Связь монтажников и локаций",
			IsGlobal:    false,
		},
		{
			TableName:   "installations",
			Model:       &models.Installation{},
			Description: "Монтажи и обслуживание",
			IsGlobal:    false,
		},
		{
			TableName:   "installation_equipment",
			Model:       nil, // Many-to-many таблица
			Description: "Связь монтажей и оборудования",
			IsGlobal:    false,
		},
		{
			TableName:   "warehouse_operations",
			Model:       &models.WarehouseOperation{},
			Description: "Операции на складе",
			IsGlobal:    false,
		},
		{
			TableName:   "stock_alerts",
			Model:       &models.StockAlert{},
			Description: "Уведомления о складских остатках",
			IsGlobal:    false,
		},
		{
			TableName:   "monitoring_templates",
			Model:       &models.MonitoringTemplate{},
			Description: "Шаблоны мониторинга",
			IsGlobal:    false,
		},
		{
			TableName:   "monitoring_notification_templates",
			Model:       &models.MonitoringNotificationTemplate{},
			Description: "Шаблоны уведомлений мониторинга",
			IsGlobal:    false,
		},
	}
}

// CheckTableExists проверяет существование таблицы
func CheckTableExists(db *gorm.DB, tableName string) (bool, error) {
	var count int64

	// Проверяем в information_schema
	err := db.Raw(`
		SELECT COUNT(*) 
		FROM information_schema.tables 
		WHERE table_schema = current_schema() 
		AND table_name = ?
	`, tableName).Scan(&count).Error

	if err != nil {
		return false, fmt.Errorf("ошибка проверки существования таблицы %s: %v", tableName, err)
	}

	return count > 0, nil
}

// GetTableInfo получает информацию о структуре таблицы
func GetTableInfo(db *gorm.DB, tableName string) (*TableInfo, error) {
	tableInfo := &TableInfo{
		TableName: tableName,
		Columns:   []ColumnInfo{},
		Indexes:   []IndexInfo{},
	}

	// Получаем информацию о колонках
	rows, err := db.Raw(`
		SELECT 
			c.column_name,
			c.data_type,
			CASE WHEN c.is_nullable = 'YES' THEN true ELSE false END as is_nullable,
			c.column_default,
			CASE WHEN tc.constraint_type = 'PRIMARY KEY' THEN true ELSE false END as is_primary_key
		FROM information_schema.columns c
		LEFT JOIN information_schema.key_column_usage kcu ON 
			c.table_name = kcu.table_name AND c.column_name = kcu.column_name
		LEFT JOIN information_schema.table_constraints tc ON 
			kcu.constraint_name = tc.constraint_name
		WHERE c.table_schema = current_schema() 
		AND c.table_name = ?
		ORDER BY c.ordinal_position
	`, tableName).Rows()

	if err != nil {
		return nil, fmt.Errorf("ошибка получения информации о колонках таблицы %s: %v", tableName, err)
	}
	defer rows.Close()

	for rows.Next() {
		var col ColumnInfo
		var defaultValue *string

		err := rows.Scan(&col.Name, &col.Type, &col.IsNullable, &defaultValue, &col.IsPrimaryKey)
		if err != nil {
			return nil, fmt.Errorf("ошибка сканирования колонки: %v", err)
		}

		col.DefaultValue = defaultValue
		tableInfo.Columns = append(tableInfo.Columns, col)
	}

	// Получаем информацию об индексах (упрощенный запрос)
	indexRows, err := db.Raw(`
		SELECT 
			indexname as name,
			indexdef LIKE '%UNIQUE%' as is_unique
		FROM pg_indexes 
		WHERE schemaname = current_schema()
		AND tablename = ?
	`, tableName).Rows()

	if err != nil {
		return nil, fmt.Errorf("ошибка получения информации об индексах таблицы %s: %v", tableName, err)
	}
	defer indexRows.Close()

	for indexRows.Next() {
		var idx IndexInfo

		err := indexRows.Scan(&idx.Name, &idx.IsUnique)
		if err != nil {
			return nil, fmt.Errorf("ошибка сканирования индекса: %v", err)
		}

		// Для упрощенной версии не извлекаем колонки
		idx.Columns = []string{} // Пустой список колонок

		tableInfo.Indexes = append(tableInfo.Indexes, idx)
	}

	return tableInfo, nil
}

// CompareTableStructure сравнивает структуру таблицы с моделью
func CompareTableStructure(db *gorm.DB, migration MigrationInfo) ([]string, error) {
	if migration.Model == nil {
		return []string{}, nil // Для many-to-many таблиц
	}

	exists, err := CheckTableExists(db, migration.TableName)
	if err != nil {
		return nil, err
	}

	if !exists {
		return []string{"Таблица не существует"}, nil
	}

	tableInfo, err := GetTableInfo(db, migration.TableName)
	if err != nil {
		return nil, err
	}

	var differences []string

	// Получаем ожидаемую структуру из модели
	expectedColumns := getExpectedColumns(migration.Model)

	// Проверяем отсутствующие колонки
	for _, expectedCol := range expectedColumns {
		found := false
		for _, actualCol := range tableInfo.Columns {
			if actualCol.Name == expectedCol {
				found = true
				break
			}
		}
		if !found {
			differences = append(differences, fmt.Sprintf("Отсутствует колонка: %s", expectedCol))
		}
	}

	// Проверяем лишние колонки
	for _, actualCol := range tableInfo.Columns {
		found := false
		for _, expectedCol := range expectedColumns {
			if actualCol.Name == expectedCol {
				found = true
				break
			}
		}
		if !found {
			differences = append(differences, fmt.Sprintf("Лишняя колонка: %s", actualCol.Name))
		}
	}

	return differences, nil
}

// getExpectedColumns извлекает ожидаемые колонки из модели GORM
func getExpectedColumns(model interface{}) []string {
	var columns []string

	// Используем рефлексию для получения полей структуры
	modelType := reflect.TypeOf(model)
	if modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}

	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)

		// Пропускаем встроенные структуры и связи
		if field.Anonymous || field.Type.Kind() == reflect.Slice || field.Type.Kind() == reflect.Ptr {
			// Проверяем, не является ли это встроенной структурой с полями БД
			if field.Type.Kind() == reflect.Ptr && field.Type.Elem().Kind() == reflect.Struct {
				continue // Это связь, пропускаем
			}
			if field.Type.Kind() == reflect.Slice {
				continue // Это связь один-ко-многим, пропускаем
			}
		}

		// Получаем имя колонки из тега gorm
		gormTag := field.Tag.Get("gorm")
		if strings.Contains(gormTag, "-") {
			continue // Поле исключено из БД
		}

		columnName := getColumnNameFromTag(gormTag, field.Name)
		if columnName != "" {
			columns = append(columns, columnName)
		}
	}

	// Добавляем стандартные поля GORM
	columns = append(columns, "id", "created_at", "updated_at", "deleted_at")

	return columns
}

// getColumnNameFromTag извлекает имя колонки из GORM тега
func getColumnNameFromTag(gormTag, fieldName string) string {
	if gormTag == "" {
		return strings.ToLower(fieldName)
	}

	// Ищем column: в теге
	parts := strings.Split(gormTag, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "column:") {
			return strings.TrimPrefix(part, "column:")
		}
	}

	// Если column не указан, используем snake_case имени поля
	return toSnakeCase(fieldName)
}

// toSnakeCase конвертирует CamelCase в snake_case
func toSnakeCase(str string) string {
	var result []rune
	for i, r := range str {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '_')
		}
		result = append(result, r)
	}
	return strings.ToLower(string(result))
}

// RunMigration выполняет миграцию для конкретной таблицы
func RunMigration(db *gorm.DB, migration MigrationInfo) MigrationResult {
	startTime := time.Now()
	result := MigrationResult{
		TableName: migration.TableName,
		Changes:   []string{},
	}

	log.Printf("🔄 Миграция таблицы: %s (%s)", migration.TableName, migration.Description)

	// Для many-to-many таблиц пропускаем
	if migration.Model == nil {
		result.Action = "skipped"
		result.Changes = append(result.Changes, "Many-to-many таблица будет создана автоматически")
		result.Duration = time.Since(startTime)
		return result
	}

	// Проверяем существование таблицы
	exists, err := CheckTableExists(db, migration.TableName)
	if err != nil {
		log.Printf("⚠️ Не удалось проверить существование таблицы %s: %v", migration.TableName, err)
		// Продолжаем с AutoMigrate
	}

	// Выполняем AutoMigrate (создаст таблицу или обновит структуру)
	err = db.AutoMigrate(migration.Model)
	if err != nil {
		result.Error = fmt.Errorf("ошибка миграции: %v", err)
		result.Action = "error"
		log.Printf("❌ Ошибка миграции %s: %v", migration.TableName, err)
	} else {
		if exists {
			result.Action = "updated"
			result.Changes = append(result.Changes, "Структура таблицы проверена/обновлена")
			log.Printf("✅ Таблица %s обновлена", migration.TableName)
		} else {
			result.Action = "created"
			result.Changes = append(result.Changes, "Таблица создана")
			log.Printf("✅ Таблица %s создана", migration.TableName)
		}
	}

	result.Duration = time.Since(startTime)
	return result
}

// RunAllMigrations выполняет все миграции
func RunAllMigrations(globalOnly bool) error {
	if DB == nil {
		return fmt.Errorf("база данных не инициализирована")
	}

	migrations := GetAllMigrations()
	var results []MigrationResult

	log.Println("🚀 Начинаем процесс миграции базы данных")

	// Выполняем глобальные миграции
	log.Println("📋 Выполняем миграции глобальных таблиц (схема public)")

	// Убеждаемся, что мы в схеме public для глобальных миграций
	if err := DB.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️ Не удалось переключиться на схему public: %v", err)
	}

	for _, migration := range migrations {
		if migration.IsGlobal {
			result := RunMigration(DB, migration)
			results = append(results, result)

			if result.Error != nil {
				log.Printf("❌ Ошибка миграции %s: %v", migration.TableName, result.Error)
				return fmt.Errorf("ошибка миграции глобальной таблицы %s: %v", migration.TableName, result.Error)
			}
		}
	}

	// Если нужны только глобальные миграции, завершаем
	if globalOnly {
		log.Println("✅ Глобальные миграции завершены")
		printMigrationSummary(results)
		return nil
	}

	// Выполняем тенантные миграции для всех компаний
	log.Println("📋 Выполняем миграции тенантных таблиц")

	var companies []models.Company
	err := DB.Find(&companies).Error
	if err != nil {
		log.Printf("⚠️ Не удалось получить список компаний: %v", err)
		log.Println("Пропускаем тенантные миграции")
	} else {
		for _, company := range companies {
			if !company.IsActive {
				continue
			}

			log.Printf("🏢 Выполняем миграции для компании: %s (схема: %s)", company.Name, company.DatabaseSchema)

			// Переключаемся на схему компании
			tenantDB := DB.Exec(fmt.Sprintf("SET search_path TO %s", company.DatabaseSchema))
			if tenantDB.Error != nil {
				log.Printf("❌ Ошибка переключения на схему %s: %v", company.DatabaseSchema, tenantDB.Error)
				continue
			}

			// Выполняем тенантные миграции
			for _, migration := range migrations {
				if !migration.IsGlobal {
					result := RunMigration(DB, migration)
					results = append(results, result)

					if result.Error != nil {
						log.Printf("❌ Ошибка миграции %s для компании %s: %v", migration.TableName, company.Name, result.Error)
						// Продолжаем с другими таблицами
					}
				}
			}
		}

		// Возвращаемся к схеме public
		DB.Exec("SET search_path TO public")
	}

	log.Println("✅ Все миграции завершены")
	printMigrationSummary(results)

	return nil
}

// printMigrationSummary выводит сводку по миграциям
func printMigrationSummary(results []MigrationResult) {
	created := 0
	updated := 0
	skipped := 0
	errors := 0

	for _, result := range results {
		switch result.Action {
		case "created":
			created++
		case "updated":
			updated++
		case "skipped":
			skipped++
		case "error":
			errors++
		}
	}

	log.Println("📊 Сводка по миграциям:")
	log.Printf("   ✅ Создано таблиц: %d", created)
	log.Printf("   🔄 Обновлено таблиц: %d", updated)
	log.Printf("   ⏭️  Пропущено таблиц: %d", skipped)
	log.Printf("   ❌ Ошибок: %d", errors)

	if errors > 0 {
		log.Println("⚠️ Обнаружены ошибки в процессе миграции. Проверьте логи выше.")
	}
}

// CreateTenantSchema создает схему для новой компании
func CreateTenantSchema(companyID uint, schemaName string) error {
	if DB == nil {
		return fmt.Errorf("база данных не инициализирована")
	}

	log.Printf("🏢 Создаем схему для компании ID %d: %s", companyID, schemaName)

	// Создаем схему
	err := DB.Exec(fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", schemaName)).Error
	if err != nil {
		return fmt.Errorf("ошибка создания схемы %s: %v", schemaName, err)
	}

	// Переключаемся на новую схему
	err = DB.Exec(fmt.Sprintf("SET search_path TO %s", schemaName)).Error
	if err != nil {
		return fmt.Errorf("ошибка переключения на схему %s: %v", schemaName, err)
	}

	// Выполняем миграции только для тенантных таблиц
	migrations := GetAllMigrations()
	for _, migration := range migrations {
		if !migration.IsGlobal {
			result := RunMigration(DB, migration)
			if result.Error != nil {
				log.Printf("❌ Ошибка создания таблицы %s в схеме %s: %v", migration.TableName, schemaName, result.Error)
				// Продолжаем создание других таблиц
			} else {
				log.Printf("✅ Таблица %s создана в схеме %s", migration.TableName, schemaName)
			}
		}
	}

	// Возвращаемся к схеме public
	if err := DB.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️ Не удалось вернуться к схеме public: %v", err)
	}

	log.Printf("✅ Схема %s создана и настроена", schemaName)
	return nil
}
