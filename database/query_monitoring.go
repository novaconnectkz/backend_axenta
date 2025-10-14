package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// SlowQueryLogger настраивает логирование медленных запросов
type SlowQueryLogger struct {
	SlowThreshold time.Duration
	Logger        logger.Interface
}

// NewSlowQueryLogger создает новый логгер для медленных запросов
func NewSlowQueryLogger(slowThreshold time.Duration) *SlowQueryLogger {
	return &SlowQueryLogger{
		SlowThreshold: slowThreshold,
		Logger:        logger.Default.LogMode(logger.Warn),
	}
}

// LogMode возвращает логгер в указанном режиме
func (l *SlowQueryLogger) LogMode(level logger.LogLevel) logger.Interface {
	return l
}

// Info логирует информационные сообщения
func (l *SlowQueryLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if len(data) > 0 {
		log.Printf("INFO: "+msg, data...)
	} else {
		log.Printf("INFO: %s", msg)
	}
}

// Warn логирует предупреждения
func (l *SlowQueryLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if len(data) > 0 {
		log.Printf("WARN: "+msg, data...)
	} else {
		log.Printf("WARN: %s", msg)
	}
}

// Error логирует ошибки
func (l *SlowQueryLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if len(data) > 0 {
		log.Printf("ERROR: "+msg, data...)
	} else {
		log.Printf("ERROR: %s", msg)
	}
}

// Trace логирует трассировку SQL запросов
func (l *SlowQueryLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	elapsed := time.Since(begin)

	// Логируем только медленные запросы
	if elapsed >= l.SlowThreshold {
		sql, rows := fc()

		if err != nil {
			log.Printf("SLOW QUERY [%.3fms] [rows:%v] ERROR: %v\nSQL: %s",
				float64(elapsed.Nanoseconds())/1e6, rows, err, sql)
		} else {
			log.Printf("SLOW QUERY [%.3fms] [rows:%v]\nSQL: %s",
				float64(elapsed.Nanoseconds())/1e6, rows, sql)
		}
	}
}

// QueryStats содержит статистику по запросам
type QueryStats struct {
	Query         string    `json:"query"`
	AvgDuration   float64   `json:"avg_duration_ms"`
	MaxDuration   float64   `json:"max_duration_ms"`
	MinDuration   float64   `json:"min_duration_ms"`
	CallCount     int64     `json:"call_count"`
	TotalDuration float64   `json:"total_duration_ms"`
	LastExecuted  time.Time `json:"last_executed"`
}

// GetSlowQueries получает статистику медленных запросов
func GetSlowQueries(db *gorm.DB, limit int) ([]QueryStats, error) {
	var stats []QueryStats

	sql := `
		SELECT 
			query,
			ROUND(mean_time::numeric, 2) as avg_duration_ms,
			ROUND(max_time::numeric, 2) as max_duration_ms,
			ROUND(min_time::numeric, 2) as min_duration_ms,
			calls as call_count,
			ROUND(total_time::numeric, 2) as total_duration_ms,
			last_exec as last_executed
		FROM pg_stat_statements 
		WHERE mean_time > 1000 
		ORDER BY mean_time DESC 
		LIMIT $1
	`

	rows, err := db.Raw(sql, limit).Rows()
	if err != nil {
		return nil, fmt.Errorf("ошибка получения статистики запросов: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var stat QueryStats
		err := rows.Scan(
			&stat.Query,
			&stat.AvgDuration,
			&stat.MaxDuration,
			&stat.MinDuration,
			&stat.CallCount,
			&stat.TotalDuration,
			&stat.LastExecuted,
		)
		if err != nil {
			log.Printf("Ошибка сканирования строки статистики: %v", err)
			continue
		}
		stats = append(stats, stat)
	}

	return stats, nil
}

// GetIndexUsageStats получает статистику использования индексов
func GetIndexUsageStats(db *gorm.DB) ([]map[string]interface{}, error) {
	var stats []map[string]interface{}

	sql := `
		SELECT 
			schemaname,
			tablename,
			indexname,
			idx_tup_read,
			idx_tup_fetch,
			idx_scan,
			ROUND((idx_tup_fetch::float / NULLIF(idx_scan, 0))::numeric, 2) as avg_tuples_per_scan
		FROM pg_stat_user_indexes 
		WHERE schemaname = current_schema()
		ORDER BY idx_scan DESC, idx_tup_read DESC
	`

	rows, err := db.Raw(sql).Rows()
	if err != nil {
		return nil, fmt.Errorf("ошибка получения статистики индексов: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var schema, table, index string
		var tupRead, tupFetch, scan int64
		var avgTuples interface{}

		err := rows.Scan(&schema, &table, &index, &tupRead, &tupFetch, &scan, &avgTuples)
		if err != nil {
			log.Printf("Ошибка сканирования строки статистики индексов: %v", err)
			continue
		}

		avgTuplesFloat := 0.0
		if avgTuples != nil {
			if f, ok := avgTuples.(float64); ok {
				avgTuplesFloat = f
			}
		}

		stat := map[string]interface{}{
			"schema":     schema,
			"table":      table,
			"index":      index,
			"tup_read":   tupRead,
			"tup_fetch":  tupFetch,
			"scan_count": scan,
			"avg_tuples": avgTuplesFloat,
		}
		stats = append(stats, stat)
	}

	return stats, nil
}

// OptimizeDatabaseAdvanced выполняет расширенную оптимизацию базы данных
func OptimizeDatabaseAdvanced(db *gorm.DB) error {
	log.Printf("🔧 Начинаем расширенную оптимизацию базы данных...")

	// Обновляем статистику
	if err := db.Exec("ANALYZE").Error; err != nil {
		return fmt.Errorf("ошибка обновления статистики: %v", err)
	}
	log.Printf("✅ Статистика базы данных обновлена")

	// Выполняем VACUUM для очистки мертвых строк
	if err := db.Exec("VACUUM").Error; err != nil {
		log.Printf("⚠️ Ошибка VACUUM: %v", err)
	} else {
		log.Printf("✅ Очистка мертвых строк выполнена")
	}

	// Переиндексируем часто используемые индексы
	reindexSQL := `
		REINDEX INDEX CONCURRENTLY idx_objects_tenant_status;
		REINDEX INDEX CONCURRENTLY idx_users_tenant_active;
		REINDEX INDEX CONCURRENTLY idx_contracts_tenant_status;
	`

	if err := db.Exec(reindexSQL).Error; err != nil {
		log.Printf("⚠️ Ошибка переиндексации: %v", err)
	} else {
		log.Printf("✅ Переиндексация выполнена")
	}

	log.Printf("✅ Расширенная оптимизация базы данных завершена")
	return nil
}
