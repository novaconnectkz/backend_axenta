package services

import (
	"backend_axenta/database"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// PerformanceMonitor сервис для мониторинга производительности
type PerformanceMonitor struct {
	db              *gorm.DB
	redis           *redis.Client
	metrics         map[string]*PerformanceMetric
	metricsMutex    sync.RWMutex
	alertThresholds map[string]float64
}

// PerformanceMetric метрика производительности
type PerformanceMetric struct {
	Name        string            `json:"name"`
	Value       float64           `json:"value"`
	Unit        string            `json:"unit"`
	Timestamp   time.Time         `json:"timestamp"`
	Tags        map[string]string `json:"tags"`
	Threshold   float64           `json:"threshold"`
	Status      string            `json:"status"` // "ok", "warning", "critical"
	Description string            `json:"description"`
}

// PerformanceAlert алерт о производительности
type PerformanceAlert struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"`
	Severity   string            `json:"severity"` // "low", "medium", "high", "critical"
	Message    string            `json:"message"`
	Metric     string            `json:"metric"`
	Value      float64           `json:"value"`
	Threshold  float64           `json:"threshold"`
	Timestamp  time.Time         `json:"timestamp"`
	Resolved   bool              `json:"resolved"`
	ResolvedAt *time.Time        `json:"resolved_at,omitempty"`
	Tags       map[string]string `json:"tags"`
}

// PerformanceReport отчет о производительности
type PerformanceReport struct {
	GeneratedAt     time.Time              `json:"generated_at"`
	Period          string                 `json:"period"`
	OverallStatus   string                 `json:"overall_status"`
	Metrics         []PerformanceMetric    `json:"metrics"`
	Alerts          []PerformanceAlert     `json:"alerts"`
	Recommendations []string               `json:"recommendations"`
	Summary         map[string]interface{} `json:"summary"`
}

// NewPerformanceMonitor создает новый монитор производительности
func NewPerformanceMonitor(db *gorm.DB, redis *redis.Client) *PerformanceMonitor {
	pm := &PerformanceMonitor{
		db:      db,
		redis:   redis,
		metrics: make(map[string]*PerformanceMetric),
		alertThresholds: map[string]float64{
			"response_time_ms":     1000, // > 1 секунды
			"cache_hit_rate":       80,   // < 80%
			"error_rate":           5,    // > 5%
			"active_connections":   150,  // > 150
			"memory_usage_percent": 85,   // > 85%
			"cpu_usage_percent":    80,   // > 80%
		},
	}

	// Запускаем мониторинг в фоне
	go pm.startMonitoring()

	return pm
}

// startMonitoring запускает мониторинг производительности
func (pm *PerformanceMonitor) startMonitoring() {
	ticker := time.NewTicker(30 * time.Second) // Проверяем каждые 30 секунд
	defer ticker.Stop()

	log.Println("🔍 Мониторинг производительности запущен")

	for range ticker.C {
		pm.collectMetrics()
		pm.checkAlerts()
	}
}

// collectMetrics собирает метрики производительности
func (pm *PerformanceMonitor) collectMetrics() {
	pm.metricsMutex.Lock()
	defer pm.metricsMutex.Unlock()

	// Метрики базы данных
	pm.collectDatabaseMetrics()

	// Метрики кэша Redis
	pm.collectCacheMetrics()

	// Метрики системы
	pm.collectSystemMetrics()

	log.Printf("📊 Собрано %d метрик производительности", len(pm.metrics))
}

// collectDatabaseMetrics собирает метрики базы данных
func (pm *PerformanceMonitor) collectDatabaseMetrics() {
	// Активные соединения
	var activeConnections int64
	pm.db.Raw("SELECT count(*) FROM pg_stat_activity WHERE state = 'active'").Scan(&activeConnections)

	pm.setMetric("active_connections", float64(activeConnections), "connections", map[string]string{
		"component": "database",
		"type":      "connections",
	})

	// Размер базы данных
	var dbSize string
	pm.db.Raw("SELECT pg_size_pretty(pg_database_size(current_database()))").Scan(&dbSize)

	// Кэш попадания
	var cacheHitRatio float64
	pm.db.Raw(`
		SELECT ROUND(
			(sum(heap_blks_hit) / (sum(heap_blks_hit) + sum(heap_blks_read))) * 100, 2
		) as cache_hit_ratio
		FROM pg_statio_user_tables
	`).Scan(&cacheHitRatio)

	pm.setMetric("cache_hit_rate", cacheHitRatio, "percent", map[string]string{
		"component": "database",
		"type":      "cache",
	})

	// Медленные запросы
	var slowQueries int64
	pm.db.Raw("SELECT count(*) FROM pg_stat_statements WHERE mean_time > 1000").Scan(&slowQueries)

	pm.setMetric("slow_queries", float64(slowQueries), "count", map[string]string{
		"component": "database",
		"type":      "performance",
	})
}

// collectCacheMetrics собирает метрики кэша Redis
func (pm *PerformanceMonitor) collectCacheMetrics() {
	if pm.redis == nil {
		pm.setMetric("cache_status", 0, "status", map[string]string{
			"component": "redis",
			"type":      "status",
		})
		return
	}

	// Статистика Redis
	_, err := pm.redis.Info(database.Ctx, "stats").Result()
	if err != nil {
		log.Printf("Ошибка получения статистики Redis: %v", err)
		return
	}

	// Извлекаем ключевые метрики из info
	// Это упрощенная версия - в реальности нужно парсить info
	pm.setMetric("redis_connected_clients", 0, "connections", map[string]string{
		"component": "redis",
		"type":      "connections",
	})

	pm.setMetric("redis_used_memory", 0, "bytes", map[string]string{
		"component": "redis",
		"type":      "memory",
	})
}

// collectSystemMetrics собирает системные метрики
func (pm *PerformanceMonitor) collectSystemMetrics() {
	// В реальной системе здесь бы собирались метрики CPU, памяти и т.д.
	// Для демонстрации используем заглушки

	pm.setMetric("cpu_usage_percent", 45.0, "percent", map[string]string{
		"component": "system",
		"type":      "cpu",
	})

	pm.setMetric("memory_usage_percent", 65.0, "percent", map[string]string{
		"component": "system",
		"type":      "memory",
	})

	pm.setMetric("response_time_ms", 250.0, "milliseconds", map[string]string{
		"component": "api",
		"type":      "response_time",
	})
}

// setMetric устанавливает метрику
func (pm *PerformanceMonitor) setMetric(name string, value float64, unit string, tags map[string]string) {
	threshold := pm.alertThresholds[name]
	status := "ok"

	if threshold > 0 {
		if value > threshold {
			status = "critical"
		} else if value > threshold*0.8 {
			status = "warning"
		}
	}

	pm.metrics[name] = &PerformanceMetric{
		Name:        name,
		Value:       value,
		Unit:        unit,
		Timestamp:   time.Now(),
		Tags:        tags,
		Threshold:   threshold,
		Status:      status,
		Description: pm.getMetricDescription(name),
	}
}

// getMetricDescription возвращает описание метрики
func (pm *PerformanceMonitor) getMetricDescription(name string) string {
	descriptions := map[string]string{
		"active_connections":      "Количество активных подключений к БД",
		"cache_hit_rate":          "Процент попаданий в кэш БД",
		"slow_queries":            "Количество медленных запросов",
		"redis_connected_clients": "Количество подключений к Redis",
		"redis_used_memory":       "Использованная память Redis",
		"cpu_usage_percent":       "Использование CPU в процентах",
		"memory_usage_percent":    "Использование памяти в процентах",
		"response_time_ms":        "Среднее время ответа API в миллисекундах",
	}

	if desc, exists := descriptions[name]; exists {
		return desc
	}
	return "Метрика производительности"
}

// checkAlerts проверяет алерты
func (pm *PerformanceMonitor) checkAlerts() {
	pm.metricsMutex.RLock()
	defer pm.metricsMutex.RUnlock()

	for name, metric := range pm.metrics {
		if metric.Status != "ok" && metric.Threshold > 0 {
			pm.createAlert(name, metric)
		}
	}
}

// createAlert создает алерт
func (pm *PerformanceMonitor) createAlert(metricName string, metric *PerformanceMetric) {
	severity := "medium"
	if metric.Status == "critical" {
		severity = "high"
	}

	alert := PerformanceAlert{
		ID:       fmt.Sprintf("%s_%d", metricName, time.Now().Unix()),
		Type:     "performance",
		Severity: severity,
		Message: fmt.Sprintf("Метрика %s превысила пороговое значение: %.2f %s (порог: %.2f %s)",
			metricName, metric.Value, metric.Unit, metric.Threshold, metric.Unit),
		Metric:    metricName,
		Value:     metric.Value,
		Threshold: metric.Threshold,
		Timestamp: time.Now(),
		Resolved:  false,
		Tags:      metric.Tags,
	}

	// Сохраняем алерт в кэш
	pm.saveAlert(alert)

	log.Printf("🚨 АЛЕРТ: %s", alert.Message)
}

// saveAlert сохраняет алерт
func (pm *PerformanceMonitor) saveAlert(alert PerformanceAlert) {
	if pm.redis != nil {
		key := fmt.Sprintf("alert:%s", alert.ID)
		database.CacheSetJSON(key, alert, 24*time.Hour)
	}
}

// GetMetrics возвращает все метрики
func (pm *PerformanceMonitor) GetMetrics() []PerformanceMetric {
	pm.metricsMutex.RLock()
	defer pm.metricsMutex.RUnlock()

	metrics := make([]PerformanceMetric, 0, len(pm.metrics))
	for _, metric := range pm.metrics {
		metrics = append(metrics, *metric)
	}

	return metrics
}

// GetAlerts возвращает активные алерты
func (pm *PerformanceMonitor) GetAlerts() ([]PerformanceAlert, error) {
	if pm.redis == nil {
		return []PerformanceAlert{}, nil
	}

	keys, err := pm.redis.Keys(database.Ctx, "alert:*").Result()
	if err != nil {
		return nil, err
	}

	var alerts []PerformanceAlert
	for _, key := range keys {
		var alert PerformanceAlert
		if err := database.CacheGetJSON(key, &alert); err == nil {
			if !alert.Resolved {
				alerts = append(alerts, alert)
			}
		}
	}

	return alerts, nil
}

// GenerateReport генерирует отчет о производительности
func (pm *PerformanceMonitor) GenerateReport(period string) (*PerformanceReport, error) {
	metrics := pm.GetMetrics()
	alerts, err := pm.GetAlerts()
	if err != nil {
		return nil, err
	}

	// Определяем общий статус
	overallStatus := "ok"
	criticalCount := 0
	warningCount := 0

	for _, metric := range metrics {
		if metric.Status == "critical" {
			criticalCount++
		} else if metric.Status == "warning" {
			warningCount++
		}
	}

	if criticalCount > 0 {
		overallStatus = "critical"
	} else if warningCount > 0 {
		overallStatus = "warning"
	}

	// Генерируем рекомендации
	recommendations := pm.generateRecommendations(metrics, alerts)

	// Создаем сводку
	summary := map[string]interface{}{
		"total_metrics":     len(metrics),
		"critical_metrics":  criticalCount,
		"warning_metrics":   warningCount,
		"active_alerts":     len(alerts),
		"monitoring_uptime": "100%", // В реальности нужно отслеживать
	}

	return &PerformanceReport{
		GeneratedAt:     time.Now(),
		Period:          period,
		OverallStatus:   overallStatus,
		Metrics:         metrics,
		Alerts:          alerts,
		Recommendations: recommendations,
		Summary:         summary,
	}, nil
}

// generateRecommendations генерирует рекомендации по оптимизации
func (pm *PerformanceMonitor) generateRecommendations(metrics []PerformanceMetric, alerts []PerformanceAlert) []string {
	var recommendations []string

	for _, metric := range metrics {
		switch metric.Name {
		case "cache_hit_rate":
			if metric.Value < 80 {
				recommendations = append(recommendations,
					"Низкий процент попаданий в кэш БД. Рассмотрите увеличение shared_buffers или оптимизацию запросов.")
			}
		case "slow_queries":
			if metric.Value > 10 {
				recommendations = append(recommendations,
					"Много медленных запросов. Проверьте индексы и оптимизируйте запросы.")
			}
		case "active_connections":
			if metric.Value > 100 {
				recommendations = append(recommendations,
					"Высокое количество активных соединений. Рассмотрите увеличение пула соединений.")
			}
		case "response_time_ms":
			if metric.Value > 500 {
				recommendations = append(recommendations,
					"Высокое время ответа API. Проверьте производительность кода и базы данных.")
			}
		}
	}

	// Общие рекомендации
	if len(alerts) > 5 {
		recommendations = append(recommendations,
			"Много активных алертов. Рекомендуется провести аудит производительности.")
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Система работает в оптимальном режиме.")
	}

	return recommendations
}

// ResolveAlert разрешает алерт
func (pm *PerformanceMonitor) ResolveAlert(alertID string) error {
	if pm.redis == nil {
		return nil
	}

	key := fmt.Sprintf("alert:%s", alertID)
	var alert PerformanceAlert

	if err := database.CacheGetJSON(key, &alert); err != nil {
		return err
	}

	alert.Resolved = true
	now := time.Now()
	alert.ResolvedAt = &now

	return database.CacheSetJSON(key, alert, 24*time.Hour)
}
