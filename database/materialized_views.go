package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gorm.io/gorm"
)

// CreateMaterializedViews создает материализованные представления для биллинга
func CreateMaterializedViews(db *gorm.DB) error {
	// Путь к файлу миграции
	migrationPath := filepath.Join("migrations", "0001_create_materialized_views.up.sql")

	// Проверяем существование файла
	if _, err := os.Stat(migrationPath); os.IsNotExist(err) {
		log.Printf("⚠️ Файл миграции не найден: %s", migrationPath)
		return fmt.Errorf("файл миграции не найден: %s", migrationPath)
	}

	// Читаем SQL из файла
	sqlContent, err := os.ReadFile(migrationPath)
	if err != nil {
		return fmt.Errorf("не удалось прочитать файл миграции: %w", err)
	}

	log.Println("🔄 Создание материализованных представлений для биллинга...")

	// Получаем *sql.DB из GORM для выполнения множественных операторов
	var sqlDB *sql.DB
	sqlDB, err = db.DB()
	if err != nil {
		return fmt.Errorf("не удалось получить *sql.DB: %w", err)
	}

	// Начинаем транзакцию
	var tx *sql.Tx
	tx, err = sqlDB.Begin()
	if err != nil {
		return fmt.Errorf("не удалось начать транзакцию: %w", err)
	}
	defer tx.Rollback()

	// Разбиваем SQL на отдельные операторы
	statements := splitSQLStatements(string(sqlContent))

	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" || strings.HasPrefix(stmt, "--") {
			continue // Пропускаем пустые строки и комментарии
		}

		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("ошибка выполнения SQL: %w\nSQL: %s", err, stmt[:min(100, len(stmt))])
		}
	}

	// Коммитим транзакцию
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ошибка коммита транзакции: %w", err)
	}

	log.Println("✅ Материализованные представления успешно созданы")
	return nil
}

// splitSQLStatements разбивает SQL на отдельные операторы
func splitSQLStatements(sql string) []string {
	// Простое разделение по точкам с запятой
	// Улучшенная версия должна учитывать точки с запятой внутри строк
	statements := strings.Split(sql, ";")
	var result []string
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt != "" && !strings.HasPrefix(stmt, "--") {
			result = append(result, stmt+";")
		}
	}
	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// RefreshMaterializedViews обновляет материализованные представления для биллинга
// Использует CONCURRENTLY для обновления без блокировки (требует уникальные индексы)
func RefreshMaterializedViews(db *gorm.DB, useConcurrent bool) error {
	views := []string{
		"mv_object_days",
		"mv_object_day_components",
	}

	log.Println("🔄 Обновление материализованных представлений...")

	for _, viewName := range views {
		var sql string
		if useConcurrent {
			sql = fmt.Sprintf("REFRESH MATERIALIZED VIEW CONCURRENTLY %s", viewName)
		} else {
			sql = fmt.Sprintf("REFRESH MATERIALIZED VIEW %s", viewName)
		}

		if err := db.Exec(sql).Error; err != nil {
			log.Printf("⚠️ Ошибка обновления представления %s: %v", viewName, err)
			// Продолжаем попытку обновить остальные представления
			continue
		}
		log.Printf("✅ Представление %s обновлено", viewName)
	}

	// Также вызываем функцию PostgreSQL для обновления всех представлений
	if err := db.Exec("SELECT refresh_billing_materialized_views()").Error; err != nil {
		log.Printf("⚠️ Не удалось вызвать функцию refresh_billing_materialized_views(): %v", err)
		log.Println("ℹ️ Продолжаем с ручным обновлением...")
	}

	log.Println("✅ Обновление материализованных представлений завершено")
	return nil
}

// CheckMaterializedViewsExists проверяет существование материализованных представлений
func CheckMaterializedViewsExists(db *gorm.DB) (bool, error) {
	var count int64
	err := db.Raw(`
		SELECT COUNT(*) 
		FROM pg_matviews 
		WHERE matviewname IN ('mv_object_days', 'mv_object_day_components')
	`).Scan(&count).Error

	if err != nil {
		return false, fmt.Errorf("ошибка проверки существования представлений: %w", err)
	}

	return count == 2, nil
}

// DropMaterializedViews удаляет материализованные представления (для отката миграции)
func DropMaterializedViews(db *gorm.DB) error {
	views := []string{
		"mv_object_day_components", // Удаляем зависимое сначала
		"mv_object_days",           // Затем базовое
	}

	log.Println("🔄 Удаление материализованных представлений...")

	for _, viewName := range views {
		sql := fmt.Sprintf("DROP MATERIALIZED VIEW IF EXISTS %s CASCADE", viewName)
		if err := db.Exec(sql).Error; err != nil {
			log.Printf("⚠️ Ошибка удаления представления %s: %v", viewName, err)
			return fmt.Errorf("ошибка удаления представления %s: %w", viewName, err)
		}
		log.Printf("✅ Представление %s удалено", viewName)
	}

	// Удаляем функцию обновления
	if err := db.Exec("DROP FUNCTION IF EXISTS refresh_billing_materialized_views()").Error; err != nil {
		log.Printf("⚠️ Ошибка удаления функции: %v", err)
	}

	log.Println("✅ Материализованные представления удалены")
	return nil
}
