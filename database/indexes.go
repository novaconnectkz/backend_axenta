package database

import (
	"fmt"
	"log"

	"gorm.io/gorm"
)

// DatabaseIndex представляет индекс базы данных
type DatabaseIndex struct {
	Name    string
	Table   string
	Columns []string
	Unique  bool
	Type    string // btree, hash, gin, gist
}

// PerformanceIndexes индексы для оптимизации производительности
var PerformanceIndexes = []DatabaseIndex{
	// Индексы для таблицы objects
	{
		Name:    "idx_objects_tenant_status",
		Table:   "objects",
		Columns: []string{"tenant_id", "status"},
		Type:    "btree",
	},
	{
		Name:    "idx_objects_tenant_type",
		Table:   "objects",
		Columns: []string{"tenant_id", "object_type"},
		Type:    "btree",
	},
	{
		Name:    "idx_objects_tenant_active",
		Table:   "objects",
		Columns: []string{"tenant_id", "is_active"},
		Type:    "btree",
	},
	{
		Name:    "idx_objects_tenant_deleted",
		Table:   "objects",
		Columns: []string{"tenant_id", "deleted_at"},
		Type:    "btree",
	},
	{
		Name:    "idx_objects_imei",
		Table:   "objects",
		Columns: []string{"imei"},
		Type:    "btree",
	},
	{
		Name:    "idx_objects_phone",
		Table:   "objects",
		Columns: []string{"phone_number"},
		Type:    "btree",
	},
	{
		Name:    "idx_objects_contract",
		Table:   "objects",
		Columns: []string{"contract_id"},
		Type:    "btree",
	},
	{
		Name:    "idx_objects_scheduled_delete",
		Table:   "objects",
		Columns: []string{"scheduled_delete_at"},
		Type:    "btree",
	},

	// Индексы для таблицы users
	{
		Name:    "idx_users_tenant_active",
		Table:   "users",
		Columns: []string{"tenant_id", "is_active"},
		Type:    "btree",
	},
	{
		Name:    "idx_users_tenant_role",
		Table:   "users",
		Columns: []string{"tenant_id", "role_id"},
		Type:    "btree",
	},
	{
		Name:    "idx_users_email",
		Table:   "users",
		Columns: []string{"email"},
		Unique:  true,
		Type:    "btree",
	},
	{
		Name:    "idx_users_login",
		Table:   "users",
		Columns: []string{"login"},
		Unique:  true,
		Type:    "btree",
	},
	{
		Name:    "idx_users_last_activity",
		Table:   "users",
		Columns: []string{"last_activity_at"},
		Type:    "btree",
	},

	// Индексы для таблицы contracts
	{
		Name:    "idx_contracts_tenant_status",
		Table:   "contracts",
		Columns: []string{"tenant_id", "status"},
		Type:    "btree",
	},
	{
		Name:    "idx_contracts_tenant_active",
		Table:   "contracts",
		Columns: []string{"tenant_id", "is_active"},
		Type:    "btree",
	},
	{
		Name:    "idx_contracts_end_date",
		Table:   "contracts",
		Columns: []string{"end_date"},
		Type:    "btree",
	},
	{
		Name:    "idx_contracts_client_inn",
		Table:   "contracts",
		Columns: []string{"client_inn"},
		Type:    "btree",
	},

	// Индексы для таблицы installations
	{
		Name:    "idx_installations_tenant_status",
		Table:   "installations",
		Columns: []string{"tenant_id", "status"},
		Type:    "btree",
	},
	{
		Name:    "idx_installations_tenant_date",
		Table:   "installations",
		Columns: []string{"tenant_id", "scheduled_date"},
		Type:    "btree",
	},
	{
		Name:    "idx_installations_installer",
		Table:   "installations",
		Columns: []string{"installer_id"},
		Type:    "btree",
	},
	{
		Name:    "idx_installations_object",
		Table:   "installations",
		Columns: []string{"object_id"},
		Type:    "btree",
	},

	// Индексы для таблицы equipment
	{
		Name:    "idx_equipment_tenant_status",
		Table:   "equipment",
		Columns: []string{"tenant_id", "status"},
		Type:    "btree",
	},
	{
		Name:    "idx_equipment_tenant_type",
		Table:   "equipment",
		Columns: []string{"tenant_id", "equipment_type"},
		Type:    "btree",
	},
	{
		Name:    "idx_equipment_serial",
		Table:   "equipment",
		Columns: []string{"serial_number"},
		Unique:  true,
		Type:    "btree",
	},
	{
		Name:    "idx_equipment_qr_code",
		Table:   "equipment",
		Columns: []string{"qr_code"},
		Unique:  true,
		Type:    "btree",
	},
	{
		Name:    "idx_equipment_location",
		Table:   "equipment",
		Columns: []string{"location_id"},
		Type:    "btree",
	},

	// Индексы для таблицы invoices
	{
		Name:    "idx_invoices_tenant_status",
		Table:   "invoices",
		Columns: []string{"tenant_id", "status"},
		Type:    "btree",
	},
	{
		Name:    "idx_invoices_tenant_date",
		Table:   "invoices",
		Columns: []string{"tenant_id", "issue_date"},
		Type:    "btree",
	},
	{
		Name:    "idx_invoices_due_date",
		Table:   "invoices",
		Columns: []string{"due_date"},
		Type:    "btree",
	},
	{
		Name:    "idx_invoices_contract",
		Table:   "invoices",
		Columns: []string{"contract_id"},
		Type:    "btree",
	},

	// Индексы для таблицы reports
	{
		Name:    "idx_reports_tenant_type",
		Table:   "reports",
		Columns: []string{"tenant_id", "report_type"},
		Type:    "btree",
	},
	{
		Name:    "idx_reports_tenant_created",
		Table:   "reports",
		Columns: []string{"tenant_id", "created_at"},
		Type:    "btree",
	},
	{
		Name:    "idx_reports_status",
		Table:   "reports",
		Columns: []string{"status"},
		Type:    "btree",
	},

	// Композитные индексы для сложных запросов
	{
		Name:    "idx_objects_complex_search",
		Table:   "objects",
		Columns: []string{"tenant_id", "status", "object_type", "is_active"},
		Type:    "btree",
	},
	{
		Name:    "idx_users_complex_search",
		Table:   "users",
		Columns: []string{"tenant_id", "is_active", "role_id", "last_activity_at"},
		Type:    "btree",
	},
	{
		Name:    "idx_installations_complex_search",
		Table:   "installations",
		Columns: []string{"tenant_id", "status", "scheduled_date", "installer_id"},
		Type:    "btree",
	},

	// Индексы для полнотекстового поиска (GIN)
	{
		Name:    "idx_objects_fulltext",
		Table:   "objects",
		Columns: []string{"name", "description"},
		Type:    "gin",
	},
	{
		Name:    "idx_users_fulltext",
		Table:   "users",
		Columns: []string{"first_name", "last_name", "email"},
		Type:    "gin",
	},
}

// CreatePerformanceIndexes создает индексы для оптимизации производительности
func CreatePerformanceIndexes(db *gorm.DB) error {
	log.Printf("Creating performance indexes...")

	for _, index := range PerformanceIndexes {
		if err := CreateIndex(db, index); err != nil {
			log.Printf("Failed to create index %s: %v", index.Name, err)
			// Продолжаем создание других индексов даже если один упал
			continue
		}
		log.Printf("Created index: %s", index.Name)
	}

	log.Printf("Performance indexes creation completed")
	return nil
}

// CreateIndex создает отдельный индекс
func CreateIndex(db *gorm.DB, index DatabaseIndex) error {
	var sql string

	switch index.Type {
	case "gin":
		// Для полнотекстового поиска
		if len(index.Columns) == 2 {
			sql = fmt.Sprintf(
				"CREATE INDEX IF NOT EXISTS %s ON %s USING GIN (to_tsvector('russian', COALESCE(%s, '') || ' ' || COALESCE(%s, '')))",
				index.Name, index.Table, index.Columns[0], index.Columns[1],
			)
		} else {
			sql = fmt.Sprintf(
				"CREATE INDEX IF NOT EXISTS %s ON %s USING GIN (to_tsvector('russian', %s))",
				index.Name, index.Table, index.Columns[0],
			)
		}
	default:
		// Обычные B-tree индексы
		uniqueStr := ""
		if index.Unique {
			uniqueStr = "UNIQUE "
		}

		columns := ""
		for i, col := range index.Columns {
			if i > 0 {
				columns += ", "
			}
			columns += col
		}

		sql = fmt.Sprintf(
			"CREATE %sINDEX IF NOT EXISTS %s ON %s (%s)",
			uniqueStr, index.Name, index.Table, columns,
		)
	}

	return db.Exec(sql).Error
}

// DropIndex удаляет индекс
func DropIndex(db *gorm.DB, indexName string) error {
	sql := fmt.Sprintf("DROP INDEX IF EXISTS %s", indexName)
	return db.Exec(sql).Error
}

// GetIndexInfo получает информацию об индексах таблицы
func GetIndexInfo(db *gorm.DB, tableName string) ([]map[string]interface{}, error) {
	var results []map[string]interface{}

	sql := `
		SELECT 
			indexname as name,
			tablename as table_name,
			indexdef as definition
		FROM pg_indexes 
		WHERE tablename = ? 
		AND schemaname = current_schema()
		ORDER BY indexname
	`

	rows, err := db.Raw(sql, tableName).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var name, table, definition string
		if err := rows.Scan(&name, &table, &definition); err != nil {
			return nil, err
		}

		results = append(results, map[string]interface{}{
			"name":       name,
			"table":      table,
			"definition": definition,
		})
	}

	return results, nil
}

// AnalyzeIndexUsage анализирует использование индексов
func AnalyzeIndexUsage(db *gorm.DB) ([]map[string]interface{}, error) {
	var results []map[string]interface{}

	sql := `
		SELECT 
			schemaname,
			tablename,
			indexname,
			idx_tup_read,
			idx_tup_fetch,
			idx_scan
		FROM pg_stat_user_indexes 
		WHERE schemaname = current_schema()
		ORDER BY idx_scan DESC, idx_tup_read DESC
	`

	rows, err := db.Raw(sql).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var schema, table, index string
		var tupRead, tupFetch, scan int64

		if err := rows.Scan(&schema, &table, &index, &tupRead, &tupFetch, &scan); err != nil {
			return nil, err
		}

		results = append(results, map[string]interface{}{
			"schema":     schema,
			"table":      table,
			"index":      index,
			"tup_read":   tupRead,
			"tup_fetch":  tupFetch,
			"scan_count": scan,
		})
	}

	return results, nil
}

// OptimizeDatabase выполняет оптимизацию базы данных
func OptimizeDatabase(db *gorm.DB) error {
	log.Printf("Starting database optimization...")

	// Обновляем статистику
	if err := db.Exec("ANALYZE").Error; err != nil {
		return fmt.Errorf("failed to analyze database: %v", err)
	}

	// Очищаем мертвые строки
	if err := db.Exec("VACUUM").Error; err != nil {
		return fmt.Errorf("failed to vacuum database: %v", err)
	}

	log.Printf("Database optimization completed")
	return nil
}

// GetTableStats получает статистику таблиц
func GetTableStats(db *gorm.DB) ([]map[string]interface{}, error) {
	var results []map[string]interface{}

	sql := `
		SELECT 
			schemaname,
			tablename,
			n_tup_ins as inserts,
			n_tup_upd as updates,
			n_tup_del as deletes,
			n_live_tup as live_tuples,
			n_dead_tup as dead_tuples,
			last_vacuum,
			last_analyze
		FROM pg_stat_user_tables 
		WHERE schemaname = current_schema()
		ORDER BY n_live_tup DESC
	`

	rows, err := db.Raw(sql).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var schema, table string
		var inserts, updates, deletes, liveTuples, deadTuples int64
		var lastVacuum, lastAnalyze *string

		if err := rows.Scan(&schema, &table, &inserts, &updates, &deletes,
			&liveTuples, &deadTuples, &lastVacuum, &lastAnalyze); err != nil {
			return nil, err
		}

		results = append(results, map[string]interface{}{
			"schema":       schema,
			"table":        table,
			"inserts":      inserts,
			"updates":      updates,
			"deletes":      deletes,
			"live_tuples":  liveTuples,
			"dead_tuples":  deadTuples,
			"last_vacuum":  lastVacuum,
			"last_analyze": lastAnalyze,
		})
	}

	return results, nil
}
