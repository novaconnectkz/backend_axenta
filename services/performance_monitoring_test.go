package services

import (
	"backend_axenta/database"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupPerformanceMonitorTestDB создает тестовую базу данных для performance monitor
func setupPerformanceMonitorTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Устанавливаем глобальную БД
	database.DB = db

	return db
}

// TestNewPerformanceMonitor тестирует создание нового монитора
func TestNewPerformanceMonitor(t *testing.T) {
	db := setupPerformanceMonitorTestDB(t)

	// Redis может быть nil для тестов
	monitor := NewPerformanceMonitor(db, nil)

	assert.NotNil(t, monitor)
	assert.NotNil(t, monitor.db)
	assert.NotNil(t, monitor.metrics)
	assert.NotNil(t, monitor.alertThresholds)
}

// TestPerformanceMonitor_SetMetric тестирует установку метрики
func TestPerformanceMonitor_SetMetric(t *testing.T) {
	db := setupPerformanceMonitorTestDB(t)
	monitor := NewPerformanceMonitor(db, nil)

	monitor.setMetric("test_metric", 100.0, "ms", map[string]string{
		"component": "test",
	})

	monitor.metricsMutex.RLock()
	metric, exists := monitor.metrics["test_metric"]
	monitor.metricsMutex.RUnlock()

	assert.True(t, exists)
	assert.NotNil(t, metric)
	assert.Equal(t, "test_metric", metric.Name)
	assert.Equal(t, 100.0, metric.Value)
	assert.Equal(t, "ms", metric.Unit)
}

// TestPerformanceMonitor_GetMetrics тестирует получение метрик
func TestPerformanceMonitor_GetMetrics(t *testing.T) {
	db := setupPerformanceMonitorTestDB(t)
	monitor := NewPerformanceMonitor(db, nil)

	// Устанавливаем метрику
	monitor.setMetric("test_metric", 100.0, "ms", map[string]string{
		"component": "test",
	})

	// Получаем все метрики
	metrics := monitor.GetMetrics()
	assert.GreaterOrEqual(t, len(metrics), 1)

	// Ищем нашу метрику
	found := false
	for _, metric := range metrics {
		if metric.Name == "test_metric" {
			found = true
			assert.Equal(t, 100.0, metric.Value)
			break
		}
	}
	assert.True(t, found)
}

// TestPerformanceMonitor_GetAlerts тестирует получение алертов
func TestPerformanceMonitor_GetAlerts(t *testing.T) {
	db := setupPerformanceMonitorTestDB(t)
	monitor := NewPerformanceMonitor(db, nil)

	alerts, err := monitor.GetAlerts()
	require.NoError(t, err)
	assert.NotNil(t, alerts)
}

// TestPerformanceMonitor_GenerateReport тестирует генерацию отчета
func TestPerformanceMonitor_GenerateReport(t *testing.T) {
	db := setupPerformanceMonitorTestDB(t)
	monitor := NewPerformanceMonitor(db, nil)

	report, err := monitor.GenerateReport("7d")
	require.NoError(t, err)
	assert.NotNil(t, report)
	assert.Equal(t, "7d", report.Period)
	assert.NotNil(t, report.Metrics)
	assert.NotNil(t, report.Alerts)
}
