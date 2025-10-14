package database

import (
	"fmt"
	"log"

	"gorm.io/gorm"
)

// PostgreSQLConfig содержит рекомендуемые настройки PostgreSQL для производительности
type PostgreSQLConfig struct {
	SharedBuffers              string `json:"shared_buffers"`
	EffectiveCacheSize         string `json:"effective_cache_size"`
	WorkMem                    string `json:"work_mem"`
	MaintenanceWorkMem         string `json:"maintenance_work_mem"`
	RandomPageCost             string `json:"random_page_cost"`
	SeqPageCost                string `json:"seq_page_cost"`
	CPUIndexTupleCost          string `json:"cpu_index_tuple_cost"`
	CPUTupleCost               string `json:"cpu_tuple_cost"`
	CPURandomPageCost          string `json:"cpu_random_page_cost"`
	CPUSequentialPageCost      string `json:"cpu_sequential_page_cost"`
	LogMinDurationStatement    string `json:"log_min_duration_statement"`
	LogStatement               string `json:"log_statement"`
	LogLinePrefix              string `json:"log_line_prefix"`
	LogCheckpoints             string `json:"log_checkpoints"`
	LogConnections             string `json:"log_connections"`
	LogDisconnections          string `json:"log_disconnections"`
	LogLockWaits               string `json:"log_lock_waits"`
	LogTempFiles               string `json:"log_temp_files"`
	CheckpointCompletionTarget string `json:"checkpoint_completion_target"`
	WalBuffers                 string `json:"wal_buffers"`
	DefaultStatisticsTarget    string `json:"default_statistics_target"`
	Autovacuum                 string `json:"autovacuum"`
	AutovacuumMaxWorkers       string `json:"autovacuum_max_workers"`
	AutovacuumNaptime          string `json:"autovacuum_naptime"`
	MaxConnections             string `json:"max_connections"`
}

// GetRecommendedConfig возвращает рекомендуемую конфигурацию PostgreSQL
func GetRecommendedConfig() PostgreSQLConfig {
	return PostgreSQLConfig{
		// Основные настройки памяти
		SharedBuffers:      "256MB", // 25% от RAM для продакшена
		EffectiveCacheSize: "1GB",   // 75% от RAM для продакшена
		WorkMem:            "4MB",   // Память для сортировки и хэширования
		MaintenanceWorkMem: "64MB",  // Память для VACUUM, CREATE INDEX

		// Настройки стоимости запросов (оптимизация для SSD)
		RandomPageCost:        "1.1",     // Стоимость случайного доступа к странице
		SeqPageCost:           "1.0",     // Стоимость последовательного доступа
		CPUIndexTupleCost:     "0.005",   // Стоимость обработки индекса
		CPUTupleCost:          "0.01",    // Стоимость обработки строки
		CPURandomPageCost:     "0.0005",  // Стоимость CPU для случайного доступа
		CPUSequentialPageCost: "0.00025", // Стоимость CPU для последовательного доступа

		// Настройки логирования
		LogMinDurationStatement: "1000", // Логировать запросы > 1 секунды
		LogStatement:            "mod",  // Логировать DDL и DML
		LogLinePrefix:           "%t [%p]: [%l-1] user=%u,db=%d,app=%a,client=%h ",
		LogCheckpoints:          "on", // Логировать контрольные точки
		LogConnections:          "on", // Логировать подключения
		LogDisconnections:       "on", // Логировать отключения
		LogLockWaits:            "on", // Логировать ожидания блокировок
		LogTempFiles:            "0",  // Логировать временные файлы

		// Настройки WAL и контрольных точек
		CheckpointCompletionTarget: "0.9",  // Цель завершения контрольных точек
		WalBuffers:                 "16MB", // Буферы WAL

		// Настройки статистики
		DefaultStatisticsTarget: "100", // Цель статистики для планировщика

		// Настройки автовакуума
		Autovacuum:           "on",   // Включить автовакуум
		AutovacuumMaxWorkers: "3",    // Максимум процессов автовакуума
		AutovacuumNaptime:    "1min", // Интервал между запусками автовакуума

		// Настройки соединений
		MaxConnections: "200", // Максимум подключений
	}
}

// ApplyRecommendedConfig применяет рекомендуемые настройки к PostgreSQL
func ApplyRecommendedConfig(db *gorm.DB) error {
	log.Printf("🔧 Применяем рекомендуемые настройки PostgreSQL...")

	config := GetRecommendedConfig()

	// Применяем настройки памяти
	if err := applyConfig(db, "shared_buffers", config.SharedBuffers); err != nil {
		log.Printf("⚠️ Ошибка настройки shared_buffers: %v", err)
	}

	if err := applyConfig(db, "effective_cache_size", config.EffectiveCacheSize); err != nil {
		log.Printf("⚠️ Ошибка настройки effective_cache_size: %v", err)
	}

	if err := applyConfig(db, "work_mem", config.WorkMem); err != nil {
		log.Printf("⚠️ Ошибка настройки work_mem: %v", err)
	}

	if err := applyConfig(db, "maintenance_work_mem", config.MaintenanceWorkMem); err != nil {
		log.Printf("⚠️ Ошибка настройки maintenance_work_mem: %v", err)
	}

	// Применяем настройки стоимости запросов
	if err := applyConfig(db, "random_page_cost", config.RandomPageCost); err != nil {
		log.Printf("⚠️ Ошибка настройки random_page_cost: %v", err)
	}

	if err := applyConfig(db, "seq_page_cost", config.SeqPageCost); err != nil {
		log.Printf("⚠️ Ошибка настройки seq_page_cost: %v", err)
	}

	// Применяем настройки логирования
	if err := applyConfig(db, "log_min_duration_statement", config.LogMinDurationStatement); err != nil {
		log.Printf("⚠️ Ошибка настройки log_min_duration_statement: %v", err)
	}

	if err := applyConfig(db, "log_statement", config.LogStatement); err != nil {
		log.Printf("⚠️ Ошибка настройки log_statement: %v", err)
	}

	// Применяем настройки автовакуума
	if err := applyConfig(db, "autovacuum", config.Autovacuum); err != nil {
		log.Printf("⚠️ Ошибка настройки autovacuum: %v", err)
	}

	// Применяем настройки статистики
	if err := applyConfig(db, "default_statistics_target", config.DefaultStatisticsTarget); err != nil {
		log.Printf("⚠️ Ошибка настройки default_statistics_target: %v", err)
	}

	// Перезагружаем конфигурацию
	if err := db.Exec("SELECT pg_reload_conf()").Error; err != nil {
		log.Printf("⚠️ Ошибка перезагрузки конфигурации: %v", err)
		return err
	}

	log.Printf("✅ Рекомендуемые настройки PostgreSQL применены")
	return nil
}

// applyConfig применяет одну настройку PostgreSQL
func applyConfig(db *gorm.DB, setting, value string) error {
	sql := fmt.Sprintf("ALTER SYSTEM SET %s = '%s'", setting, value)
	return db.Exec(sql).Error
}

// GetCurrentConfig получает текущую конфигурацию PostgreSQL
func GetCurrentConfig(db *gorm.DB) (map[string]string, error) {
	config := make(map[string]string)

	sql := `
		SELECT name, setting, unit, context, short_desc
		FROM pg_settings 
		WHERE name IN (
			'shared_buffers', 'effective_cache_size', 'work_mem', 
			'maintenance_work_mem', 'random_page_cost', 'seq_page_cost',
			'log_min_duration_statement', 'log_statement', 'autovacuum',
			'default_statistics_target', 'max_connections'
		)
		ORDER BY name
	`

	rows, err := db.Raw(sql).Rows()
	if err != nil {
		return nil, fmt.Errorf("ошибка получения конфигурации: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name, setting, unit, context, description string
		if err := rows.Scan(&name, &setting, &unit, &context, &description); err != nil {
			log.Printf("Ошибка сканирования конфигурации: %v", err)
			continue
		}

		fullSetting := setting
		if unit != "" {
			fullSetting += " " + unit
		}

		config[name] = fullSetting
	}

	return config, nil
}

// GetPerformanceStats получает статистику производительности PostgreSQL
func GetPerformanceStats(db *gorm.DB) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Получаем статистику соединений
	var connectionStats struct {
		MaxConnections     int64 `json:"max_connections"`
		ActiveConnections  int64 `json:"active_connections"`
		IdleConnections    int64 `json:"idle_connections"`
		WaitingConnections int64 `json:"waiting_connections"`
	}

	connSQL := `
		SELECT 
			(SELECT setting::int FROM pg_settings WHERE name = 'max_connections') as max_connections,
			(SELECT count(*) FROM pg_stat_activity WHERE state = 'active') as active_connections,
			(SELECT count(*) FROM pg_stat_activity WHERE state = 'idle') as idle_connections,
			(SELECT count(*) FROM pg_stat_activity WHERE wait_event_type IS NOT NULL) as waiting_connections
	`

	if err := db.Raw(connSQL).Scan(&connectionStats).Error; err != nil {
		return nil, fmt.Errorf("ошибка получения статистики соединений: %w", err)
	}
	stats["connections"] = connectionStats

	// Получаем статистику кэша
	var cacheStats struct {
		CacheHitRatio float64 `json:"cache_hit_ratio"`
		IndexHitRatio float64 `json:"index_hit_ratio"`
	}

	cacheSQL := `
		SELECT 
			ROUND(
				(sum(heap_blks_hit) / (sum(heap_blks_hit) + sum(heap_blks_read))) * 100, 2
			) as cache_hit_ratio,
			ROUND(
				(sum(idx_blks_hit) / (sum(idx_blks_hit) + sum(idx_blks_read))) * 100, 2
			) as index_hit_ratio
		FROM pg_statio_user_tables
	`

	if err := db.Raw(cacheSQL).Scan(&cacheStats).Error; err != nil {
		log.Printf("Ошибка получения статистики кэша: %v", err)
	} else {
		stats["cache"] = cacheStats
	}

	// Получаем размер базы данных
	var dbSize struct {
		DatabaseSize string `json:"database_size"`
	}

	sizeSQL := `SELECT pg_size_pretty(pg_database_size(current_database())) as database_size`
	if err := db.Raw(sizeSQL).Scan(&dbSize).Error; err != nil {
		log.Printf("Ошибка получения размера БД: %v", err)
	} else {
		stats["database_size"] = dbSize.DatabaseSize
	}

	return stats, nil
}
