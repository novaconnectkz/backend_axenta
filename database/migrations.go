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
			TableName:   "wialon_connections",
			Model:       &models.WialonConnection{},
			Description: "Подключения к Wialon (Hosting и Local)",
			IsGlobal:    true,
		},
		{
			TableName:   "wialon_object_stats",
			Model:       &models.WialonObjectStat{},
			Description: "Кэш статистики объектов Wialon (заполняется фоновым cron каждые 15 мин)",
			IsGlobal:    true,
		},
		{
			TableName:   "wialon_units",
			Model:       &models.WialonUnit{},
			Description: "Реестр Wialon-юнитов с UNIQUE(connection_id, unit_id) — точный COUNT DISTINCT для dashboard",
			IsGlobal:    true,
		},
		{
			TableName:   "wialon_billing_plans",
			Model:       &models.WialonBillingPlan{},
			Description: "Кэш тарифных планов Wialon (фоновый sync раз в час)",
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
		{
			TableName:   "notification_settings",
			Model:       &models.NotificationSettings{},
			Description: "Настройки уведомлений компаний (Email, Telegram, MAX)",
			IsGlobal:    true,
		},
		// Новые таблицы для продвинутого биллинга (roadmap)
		{
			TableName:   "countries",
			Model:       &models.Country{},
			Description: "Справочник стран для налогов",
			IsGlobal:    true,
		},
		{
			TableName:   "tax_rates",
			Model:       &models.TaxRate{},
			Description: "Ставки НДС для стран",
			IsGlobal:    true,
		},
		{
			TableName:   "tax_rules",
			Model:       &models.TaxRule{},
			Description: "Правила применения НДС между странами",
			IsGlobal:    true,
		},
		{
			TableName:   "tariff_components",
			Model:       &models.TariffComponent{},
			Description: "Компоненты тарифов",
			IsGlobal:    true,
		},
		{
			TableName:   "assignments",
			Model:       &models.Assignment{},
			Description: "Привязки объектов к подпискам",
			IsGlobal:    true,
		},
		{
			TableName:   "freezes",
			Model:       &models.Freeze{},
			Description: "Заморозки объектов",
			IsGlobal:    true,
		},
		{
			TableName:   "usages",
			Model:       &models.Usage{},
			Description: "Использование объектов по дням",
			IsGlobal:    true,
		},
		{
			TableName:   "discounts",
			Model:       &models.Discount{},
			Description: "Скидки на различных уровнях",
			IsGlobal:    true,
		},
		{
			TableName:   "axenta_account_snapshots",
			Model:       &models.AxentaAccountSnapshot{},
			Description: "Снимки учетных записей Axenta",
			IsGlobal:    false, // Изолированы по тенантам
		},
		{
			TableName:   "axenta_object_snapshots",
			Model:       &models.AxentaObjectSnapshot{},
			Description: "Снимки объектов Axenta",
			IsGlobal:    false, // Изолированы по тенантам
		},
		{
			TableName:   "axenta_user_snapshots",
			Model:       &models.AxentaUserSnapshot{},
			Description: "Снимки пользователей Axenta (read-path для /unified/users)",
			IsGlobal:    false, // Изолированы по тенантам
		},
		{
			TableName:   "invoice_headers",
			Model:       &models.InvoiceHeader{},
			Description: "Заголовки счетов (advanced billing)",
			IsGlobal:    true,
		},
		{
			TableName:   "invoice_lines",
			Model:       &models.InvoiceLine{},
			Description: "Строки счетов (advanced billing)",
			IsGlobal:    true,
		},
		{
			TableName:   "invoice_sequences",
			Model:       &models.InvoiceSequence{},
			Description: "Последовательности нумерации счетов по странам",
			IsGlobal:    true,
		},
		{
			TableName:   "billing_daily_snapshots",
			Model:       &models.BillingDailySnapshot{},
			Description: "Агрегированные ежедневные снимки количества объектов для биллинга",
			IsGlobal:    true, // Глобальная таблица в схеме public
		},
		{
			TableName:   "partner_daily_snapshots",
			Model:       &models.PartnerDailySnapshot{},
			Description: "Ежедневные снимки объектов партнеров для тарификации",
			IsGlobal:    false, // Тенантная таблица - каждая компания имеет свои снимки
		},
		{
			TableName:   "snapshot_settings",
			Model:       &models.SnapshotSettings{},
			Description: "Настройки автоматического создания снимков (токен для Axenta API)",
			IsGlobal:    false, // Тенантная таблица - хранится в схеме суперадмина (ID=1)
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
			TableName:   "user_tabs",
			Model:       &models.UserTab{},
			Description: "Вкладки пользователей",
			IsGlobal:    false,
		},
		{
			TableName:   "user_accesses",
			Model:       &models.UserAccess{},
			Description: "Доступы пользователей",
			IsGlobal:    false,
		},
		{
			TableName:   "user_tokens",
			Model:       &models.UserToken{},
			Description: "Токены пользователей для Axenta Cloud",
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
			TableName:   "contract_numerators",
			Model:       &models.ContractNumerator{},
			Description: "Нумераторы договоров",
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
		{
			TableName:   "deleted_items",
			Model:       &models.DeletedItem{},
			Description: "Корзина удаленных элементов (аудит удалений)",
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

	// Специальная обработка для таблиц, которые могут содержать NULL значения в NOT NULL колонках
	if exists && migration.TableName == "contract_numerators" {
		log.Printf("🔧 Проверка и исправление NULL значений в таблице %s...", migration.TableName)
		// Обновляем все NULL значения в admin_account_id на дефолтное значение 1
		// Это безопасно, так как admin_account_id должен быть установлен для всех записей
		if err := db.Exec("UPDATE contract_numerators SET admin_account_id = 1 WHERE admin_account_id IS NULL").Error; err != nil {
			log.Printf("⚠️ Не удалось обновить NULL значения в admin_account_id: %v", err)
		} else {
			var updatedCount int64
			db.Raw("SELECT COUNT(*) FROM contract_numerators WHERE admin_account_id IS NULL").Scan(&updatedCount)
			if updatedCount == 0 {
				log.Printf("✅ NULL значения в admin_account_id обновлены")
			}
		}
	}

	// Выполняем AutoMigrate (создаст таблицу или обновит структуру)
	if migration.Model != nil {
		err = db.AutoMigrate(migration.Model)
	} else {
		result.Error = fmt.Errorf("модель для таблицы %s не определена", migration.TableName)
		result.Action = "error"
		log.Printf("❌ Модель для таблицы %s не определена", migration.TableName)
		return result
	}
	if err != nil {
		// Игнорируем ошибки о несуществующих ограничениях (constraint does not exist)
		// Это может происходить, когда GORM пытается удалить старое ограничение,
		// но оно уже было удалено или имеет другое имя
		if strings.Contains(err.Error(), "does not exist") && strings.Contains(err.Error(), "constraint") {
			log.Printf("⚠️ Предупреждение при миграции %s (ограничение не существует, игнорируем): %v", migration.TableName, err)
			// Продолжаем выполнение, так как это не критическая ошибка
			if exists {
				result.Action = "updated"
				result.Changes = append(result.Changes, "Структура таблицы проверена/обновлена (некоторые ограничения могли быть пропущены)")
				log.Printf("✅ Таблица %s обновлена (с предупреждениями)", migration.TableName)
			} else {
				result.Action = "created"
				result.Changes = append(result.Changes, "Таблица создана")
				log.Printf("✅ Таблица %s создана", migration.TableName)
			}
		} else if strings.Contains(err.Error(), "contains null values") || strings.Contains(err.Error(), "column") && strings.Contains(err.Error(), "null") {
			// Ошибка о NULL значениях в NOT NULL колонке
			log.Printf("⚠️ Обнаружены NULL значения в NOT NULL колонке таблицы %s: %v", migration.TableName, err)
			log.Printf("🔧 Попытка исправить NULL значения...")

			// Пытаемся определить имя колонки из ошибки
			// Формат ошибки: "column "admin_account_id" of relation "contract_numerators" contains null values"
			var columnName string
			if strings.Contains(err.Error(), "admin_account_id") {
				columnName = "admin_account_id"
			} else if strings.Contains(err.Error(), "company_id") {
				columnName = "company_id"
			}

			if columnName != "" {
				// Обновляем NULL значения на дефолтное значение
				updateSQL := fmt.Sprintf("UPDATE %s SET %s = 1 WHERE %s IS NULL", migration.TableName, columnName, columnName)
				if updateErr := db.Exec(updateSQL).Error; updateErr != nil {
					log.Printf("❌ Не удалось обновить NULL значения: %v", updateErr)
					result.Error = fmt.Errorf("ошибка миграции (NULL значения): %v", err)
					result.Action = "error"
				} else {
					log.Printf("✅ NULL значения обновлены, повторная попытка миграции...")
					// Повторяем попытку миграции
					if retryErr := db.AutoMigrate(migration.Model); retryErr != nil {
						result.Error = fmt.Errorf("ошибка миграции после исправления NULL: %v", retryErr)
						result.Action = "error"
						log.Printf("❌ Ошибка миграции %s после исправления NULL: %v", migration.TableName, retryErr)
					} else {
						result.Action = "updated"
						result.Changes = append(result.Changes, fmt.Sprintf("NULL значения в %s исправлены, таблица обновлена", columnName))
						log.Printf("✅ Таблица %s успешно обновлена после исправления NULL значений", migration.TableName)
					}
				}
			} else {
				result.Error = fmt.Errorf("ошибка миграции (NULL значения): %v", err)
				result.Action = "error"
				log.Printf("❌ Ошибка миграции %s (не удалось определить колонку): %v", migration.TableName, err)
			}
		} else {
			result.Error = fmt.Errorf("ошибка миграции: %v", err)
			result.Action = "error"
			log.Printf("❌ Ошибка миграции %s: %v", migration.TableName, err)
		}
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

	var migrationErrors []string
	for _, migration := range migrations {
		if migration.IsGlobal {
			result := RunMigration(DB, migration)
			results = append(results, result)

			if result.Error != nil {
				log.Printf("❌ Ошибка миграции %s: %v", migration.TableName, result.Error)
				migrationErrors = append(migrationErrors, fmt.Sprintf("%s: %v", migration.TableName, result.Error))
				// Продолжаем выполнение остальных миграций вместо прерывания
			}
		}
	}

	// Создаем материализованные представления для биллинга
	log.Println("")
	log.Println("🔄 Создание материализованных представлений для биллинга...")
	exists, errCheck := CheckMaterializedViewsExists(DB)
	if errCheck != nil {
		log.Printf("⚠️ Ошибка проверки существования представлений: %v", errCheck)
	} else if !exists {
		if errCreate := CreateMaterializedViews(DB); errCreate != nil {
			log.Printf("⚠️ Ошибка создания материализованных представлений: %v", errCreate)
			log.Println("ℹ️ Продолжаем выполнение миграций...")
		}
	} else {
		log.Println("✅ Материализованные представления уже существуют")
		log.Println("ℹ️ Используйте RefreshMaterializedViews() для обновления")
	}

	// Если нужны только глобальные миграции, завершаем
	if globalOnly {
		if len(migrationErrors) > 0 {
			log.Printf("⚠️ Глобальные миграции завершены с ошибками (%d): %v", len(migrationErrors), migrationErrors)
		} else {
			log.Println("✅ Глобальные миграции завершены успешно")
		}
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

	if len(migrationErrors) > 0 {
		log.Printf("⚠️ Все миграции завершены с ошибками (%d): %v", len(migrationErrors), migrationErrors)
	} else {
		log.Println("✅ Все миграции завершены успешно")
	}
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

	tenantDB, err := ConnectToTenant(schemaName)
	if err != nil {
		return fmt.Errorf("ошибка подключения к схеме %s: %v", schemaName, err)
	}

	// Выполняем миграции только для тенантных таблиц
	migrations := GetAllMigrations()
	for _, migration := range migrations {
		if !migration.IsGlobal {
			result := RunMigration(tenantDB, migration)
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

// CreateMissingGlobalTables создает недостающие глобальные таблицы напрямую через AutoMigrate
func CreateMissingGlobalTables() error {
	if DB == nil {
		return fmt.Errorf("база данных не инициализирована")
	}

	// Убеждаемся, что мы в схеме public
	if err := DB.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️ Не удалось переключиться на схему public: %v", err)
		return err
	}

	log.Println("🔧 Создание недостающих глобальных таблиц...")

	// Список моделей для создания в схеме public (в правильном порядке)
	// Сначала создаем таблицы без зависимостей
	globalModels := []interface{}{
		&models.BillingSettings{},
		&models.ContractNumerator{},
		&models.InvoiceNumerator{},
		&models.InvoiceItem{},
		&models.Invoice{},
		&models.BillingHistory{},
		// AxentaAccountSnapshot и AxentaObjectSnapshot теперь тенант-специфичны (IsGlobal: false)
	}

	for _, model := range globalModels {
		log.Printf("🔄 Создание таблицы для модели %T...", model)

		// Специальная обработка для ContractNumerator - исправляем NULL значения перед миграцией
		if _, ok := model.(*models.ContractNumerator); ok {
			// Проверяем, существует ли таблица
			var tableExists bool
			if err := DB.Raw("SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'contract_numerators')").Scan(&tableExists).Error; err == nil && tableExists {
				log.Printf("🔧 Подготовка таблицы contract_numerators перед миграцией...")

				// Проверяем, существует ли колонка admin_account_id
				var columnExists bool
				if err := DB.Raw("SELECT EXISTS (SELECT FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'contract_numerators' AND column_name = 'admin_account_id')").Scan(&columnExists).Error; err == nil {
					if columnExists {
						// Колонка существует - обновляем NULL значения
						log.Printf("🔧 Обновление NULL значений в admin_account_id...")
						if err := DB.Exec("UPDATE contract_numerators SET admin_account_id = 1 WHERE admin_account_id IS NULL").Error; err != nil {
							log.Printf("⚠️ Не удалось обновить NULL значения в admin_account_id: %v", err)
						} else {
							log.Printf("✅ NULL значения в admin_account_id обновлены")
						}
					} else {
						// Колонка не существует - добавляем её как nullable сначала, затем обновляем NULL, затем устанавливаем NOT NULL
						log.Printf("🔧 Колонка admin_account_id не существует, добавляем её...")

						// Шаг 1: Добавляем колонку как nullable
						if err := DB.Exec("ALTER TABLE contract_numerators ADD COLUMN IF NOT EXISTS admin_account_id BIGINT").Error; err != nil {
							log.Printf("⚠️ Не удалось добавить колонку admin_account_id: %v", err)
						} else {
							log.Printf("✅ Колонка admin_account_id добавлена (nullable)")

							// Шаг 2: Обновляем NULL значения
							if err := DB.Exec("UPDATE contract_numerators SET admin_account_id = 1 WHERE admin_account_id IS NULL").Error; err != nil {
								log.Printf("⚠️ Не удалось обновить NULL значения: %v", err)
							} else {
								log.Printf("✅ NULL значения обновлены")

								// Шаг 3: Устанавливаем NOT NULL (AutoMigrate сделает это автоматически)
								log.Printf("✅ Колонка готова для установки NOT NULL через AutoMigrate")
							}
						}
					}
				}
			}
		}

		// Используем обычный AutoMigrate - если foreign key constraints не могут быть созданы,
		// таблица все равно будет создана, но без constraints
		if err := DB.AutoMigrate(model); err != nil {
			// Обрабатываем ошибки о NULL значениях в NOT NULL колонках
			if strings.Contains(err.Error(), "contains null values") || (strings.Contains(err.Error(), "column") && strings.Contains(err.Error(), "null")) {
				log.Printf("⚠️ Обнаружены NULL значения в NOT NULL колонке для %T: %v", model, err)
				log.Printf("🔧 Попытка исправить NULL значения...")

				// Определяем имя таблицы и колонки
				var tableName string
				var columnName string
				if strings.Contains(err.Error(), "contract_numerators") {
					tableName = "contract_numerators"
					if strings.Contains(err.Error(), "admin_account_id") {
						columnName = "admin_account_id"
					} else if strings.Contains(err.Error(), "company_id") {
						columnName = "company_id"
					}
				}

				if tableName != "" && columnName != "" {
					// Проверяем, существует ли колонка
					var columnExists bool
					checkColumnSQL := fmt.Sprintf("SELECT EXISTS (SELECT FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = '%s' AND column_name = '%s')", tableName, columnName)
					if checkErr := DB.Raw(checkColumnSQL).Scan(&columnExists).Error; checkErr == nil {
						if !columnExists {
							// Колонка не существует - добавляем её как nullable сначала
							log.Printf("🔧 Колонка %s не существует, добавляем её...", columnName)
							addColumnSQL := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s BIGINT", tableName, columnName)
							if addErr := DB.Exec(addColumnSQL).Error; addErr != nil {
								log.Printf("❌ Не удалось добавить колонку %s: %v", columnName, addErr)
								log.Printf("❌ Ошибка создания таблицы для %T: %v", model, err)
								continue
							}
							log.Printf("✅ Колонка %s добавлена (nullable)", columnName)
						}

						// Теперь обновляем NULL значения
						updateSQL := fmt.Sprintf("UPDATE %s SET %s = 1 WHERE %s IS NULL", tableName, columnName, columnName)
						if updateErr := DB.Exec(updateSQL).Error; updateErr != nil {
							log.Printf("❌ Не удалось обновить NULL значения: %v", updateErr)
							log.Printf("❌ Ошибка создания таблицы для %T: %v", model, err)
						} else {
							log.Printf("✅ NULL значения обновлены, повторная попытка миграции...")
							// Повторяем попытку миграции
							if retryErr := DB.AutoMigrate(model); retryErr != nil {
								log.Printf("❌ Ошибка создания таблицы для %T после исправления NULL: %v", model, retryErr)
							} else {
								log.Printf("✅ Таблица для %T создана/обновлена после исправления NULL значений", model)
								continue
							}
						}
					}
				} else {
					log.Printf("❌ Ошибка создания таблицы для %T (не удалось определить колонку): %v", model, err)
				}
			} else if !strings.Contains(err.Error(), "foreign") && !strings.Contains(err.Error(), "constraint") {
				log.Printf("❌ Ошибка создания таблицы для %T: %v", model, err)
				// Продолжаем создавать другие таблицы даже при ошибке
			} else {
				log.Printf("⚠️ Предупреждение при создании таблицы для %T (возможно foreign key): %v", model, err)
			}
			continue
		}
		log.Printf("✅ Таблица для %T создана/обновлена", model)
	}

	// Если AutoMigrate не сработал для Invoice, InvoiceItem и BillingHistory, создаем их через SQL
	if err := createInvoiceTablesViaSQL(); err != nil {
		log.Printf("⚠️ Ошибка создания таблиц счетов через SQL: %v", err)
	}

	log.Println("✅ Попытка создания недостающих глобальных таблиц завершена")

	if err := ensureBillingSchemaIntegrity(); err != nil {
		log.Printf("⚠️ Ошибка проверки структуры таблиц биллинга: %v", err)
	}

	return nil
}

// createInvoiceTablesViaSQL создает таблицы invoices, invoice_items и billing_history через SQL напрямую
func createInvoiceTablesViaSQL() error {
	// Проверяем, существует ли таблица invoices
	var count int64
	if err := DB.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'invoices'").Scan(&count).Error; err == nil && count > 0 {
		log.Println("ℹ️ Таблица invoices уже существует")
		return nil
	}

	log.Println("🔧 Создание таблиц счетов через SQL...")

	// Создаем таблицу invoices
	invoicesSQL := `
		CREATE TABLE IF NOT EXISTS invoices (
			id BIGSERIAL PRIMARY KEY,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			deleted_at TIMESTAMP WITH TIME ZONE,
			admin_account_id BIGINT NOT NULL,
			number VARCHAR(50) NOT NULL,
			title VARCHAR(200) NOT NULL,
			description TEXT,
			invoice_date TIMESTAMP WITH TIME ZONE NOT NULL,
			due_date TIMESTAMP WITH TIME ZONE NOT NULL,
			company_id BIGINT NOT NULL,
			contract_id BIGINT,
			tariff_plan_id BIGINT,
			billing_period_start TIMESTAMP WITH TIME ZONE NOT NULL,
			billing_period_end TIMESTAMP WITH TIME ZONE NOT NULL,
			subtotal_amount DECIMAL(15,2) NOT NULL,
			tax_rate DECIMAL(5,2) DEFAULT 0,
			tax_amount DECIMAL(15,2) DEFAULT 0,
			total_amount DECIMAL(15,2) NOT NULL,
			currency VARCHAR(3) DEFAULT 'RUB',
			status VARCHAR(20) DEFAULT 'draft',
			paid_at TIMESTAMP WITH TIME ZONE,
			paid_amount DECIMAL(15,2) DEFAULT 0,
			notes TEXT,
			external_id VARCHAR(100)
		);
		CREATE UNIQUE INDEX IF NOT EXISTS invoices_number_key ON invoices(number) WHERE deleted_at IS NULL;
		CREATE INDEX IF NOT EXISTS invoices_admin_account_id_idx ON invoices(admin_account_id);
		CREATE INDEX IF NOT EXISTS invoices_company_id_idx ON invoices(company_id);
		CREATE INDEX IF NOT EXISTS invoices_contract_id_idx ON invoices(contract_id);
		CREATE INDEX IF NOT EXISTS invoices_deleted_at_idx ON invoices(deleted_at);
	`

	if err := DB.Exec(invoicesSQL).Error; err != nil {
		log.Printf("❌ Ошибка создания таблицы invoices: %v", err)
		return err
	}
	log.Println("✅ Таблица invoices создана")

	// Создаем таблицу invoice_items
	invoiceItemsSQL := `
		CREATE TABLE IF NOT EXISTS invoice_items (
			id BIGSERIAL PRIMARY KEY,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			deleted_at TIMESTAMP WITH TIME ZONE,
			invoice_id BIGINT NOT NULL,
			name VARCHAR(200) NOT NULL,
			description TEXT,
			item_type VARCHAR(50) NOT NULL,
			object_id BIGINT,
			quantity DECIMAL(10,3) NOT NULL,
			unit_price DECIMAL(15,2) NOT NULL,
			amount DECIMAL(15,2) NOT NULL,
			period_start TIMESTAMP WITH TIME ZONE,
			period_end TIMESTAMP WITH TIME ZONE,
			notes TEXT
		);
		CREATE INDEX IF NOT EXISTS invoice_items_invoice_id_idx ON invoice_items(invoice_id);
		CREATE INDEX IF NOT EXISTS invoice_items_object_id_idx ON invoice_items(object_id);
		CREATE INDEX IF NOT EXISTS invoice_items_deleted_at_idx ON invoice_items(deleted_at);
	`

	if err := DB.Exec(invoiceItemsSQL).Error; err != nil {
		log.Printf("❌ Ошибка создания таблицы invoice_items: %v", err)
		return err
	}
	log.Println("✅ Таблица invoice_items создана")

	// Создаем таблицу billing_history
	billingHistorySQL := `
		CREATE TABLE IF NOT EXISTS billing_history (
			id BIGSERIAL PRIMARY KEY,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			deleted_at TIMESTAMP WITH TIME ZONE,
			admin_account_id BIGINT NOT NULL,
			company_id BIGINT NOT NULL,
			invoice_id BIGINT,
			contract_id BIGINT,
			operation VARCHAR(50) NOT NULL,
			amount DECIMAL(15,2),
			currency VARCHAR(3) DEFAULT 'RUB',
			description TEXT,
			period_start TIMESTAMP WITH TIME ZONE,
			period_end TIMESTAMP WITH TIME ZONE,
			metadata JSONB,
			status VARCHAR(20) DEFAULT 'completed'
		);
		CREATE INDEX IF NOT EXISTS billing_history_admin_account_id_idx ON billing_history(admin_account_id);
		CREATE INDEX IF NOT EXISTS billing_history_company_id_idx ON billing_history(company_id);
		CREATE INDEX IF NOT EXISTS billing_history_invoice_id_idx ON billing_history(invoice_id);
		CREATE INDEX IF NOT EXISTS billing_history_contract_id_idx ON billing_history(contract_id);
		CREATE INDEX IF NOT EXISTS billing_history_deleted_at_idx ON billing_history(deleted_at);
	`

	if err := DB.Exec(billingHistorySQL).Error; err != nil {
		log.Printf("❌ Ошибка создания таблицы billing_history: %v", err)
		return err
	}
	log.Println("✅ Таблица billing_history создана")

	return nil
}

func ensureBillingSchemaIntegrity() error {
	var issues []string

	columnChecks := []struct {
		table          string
		column         string
		definition     string
		requireNotNull bool
	}{
		{"billing_plans", "admin_account_id", "BIGINT", false},
		{"billing_plans", "company_id", "BIGINT", false},
		{"subscriptions", "admin_account_id", "BIGINT", false},
		{"subscriptions", "company_id", "BIGINT", false},
		{"invoices", "admin_account_id", "BIGINT", true},
		{"billing_history", "admin_account_id", "BIGINT", true},
		{"billing_settings", "admin_account_id", "BIGINT", true},
		{"billing_settings", "company_id", "BIGINT", true},
		{"billing_settings", "min_days_for_full_month", "INTEGER DEFAULT 5", false},
	}

	for _, check := range columnChecks {
		if err := ensureColumnExists(check.table, check.column, check.definition); err != nil {
			issues = append(issues, fmt.Sprintf("таблица %s: %v", check.table, err))
			continue
		}
		if check.requireNotNull {
			if err := ensureNotNullIfPossible(check.table, check.column); err != nil {
				issues = append(issues, fmt.Sprintf("таблица %s: %v", check.table, err))
			}
		}
	}

	if err := dropIndexIfExists("idx_billing_settings_admin_company"); err != nil {
		issues = append(issues, fmt.Sprintf("индекс idx_billing_settings_admin_company: %v", err))
	}

	indexChecks := []struct {
		name       string
		table      string
		definition string
		unique     bool
	}{
		{"idx_billing_plan_admin_name", "billing_plans", "(admin_account_id, name)", true},
		{"idx_subscriptions_admin_company", "subscriptions", "(admin_account_id, company_id)", false},
		{"billing_settings_company_id_idx", "billing_settings", "(company_id)", false},
		{"billing_settings_admin_account_id_idx", "billing_settings", "(admin_account_id)", false},
		{"invoices_admin_account_id_idx", "invoices", "(admin_account_id)", false},
		{"billing_history_admin_account_id_idx", "billing_history", "(admin_account_id)", false},
	}

	for _, check := range indexChecks {
		if err := ensureIndexExists(check.name, check.table, check.definition, check.unique); err != nil {
			issues = append(issues, fmt.Sprintf("индекс %s: %v", check.name, err))
		}
	}

	if len(issues) > 0 {
		return fmt.Errorf("%s", strings.Join(issues, "; "))
	}

	return nil
}

func ensureColumnExists(table, column, dataType string) error {
	query := fmt.Sprintf(
		"ALTER TABLE IF EXISTS public.%s ADD COLUMN IF NOT EXISTS %s %s",
		table,
		column,
		dataType,
	)
	return DB.Exec(query).Error
}

func ensureNotNullIfPossible(table, column string) error {
	var total int64
	if err := DB.Raw(
		fmt.Sprintf("SELECT COUNT(*) FROM public.%s", table),
	).Scan(&total).Error; err != nil {
		return fmt.Errorf("не удалось подсчитать строки: %w", err)
	}

	if total == 0 {
		if err := DB.Exec(
			fmt.Sprintf("ALTER TABLE public.%s ALTER COLUMN %s SET NOT NULL", table, column),
		).Error; err != nil {
			return fmt.Errorf("не удалось установить NOT NULL: %w", err)
		}
		return nil
	}

	var nulls int64
	if err := DB.Raw(
		fmt.Sprintf("SELECT COUNT(*) FROM public.%s WHERE %s IS NULL", table, column),
	).Scan(&nulls).Error; err != nil {
		return fmt.Errorf("не удалось подсчитать NULL значения: %w", err)
	}

	if nulls == 0 {
		if err := DB.Exec(
			fmt.Sprintf("ALTER TABLE public.%s ALTER COLUMN %s SET NOT NULL", table, column),
		).Error; err != nil {
			return fmt.Errorf("не удалось установить NOT NULL: %w", err)
		}
	}

	return nil
}

func ensureIndexExists(name, table, definition string, unique bool) error {
	indexType := "INDEX"
	if unique {
		indexType = "UNIQUE INDEX"
	}

	query := fmt.Sprintf(
		"CREATE %s IF NOT EXISTS %s ON public.%s %s",
		indexType,
		name,
		table,
		definition,
	)

	return DB.Exec(query).Error
}

func dropIndexIfExists(name string) error {
	return DB.Exec(fmt.Sprintf("DROP INDEX IF EXISTS %s", name)).Error
}
