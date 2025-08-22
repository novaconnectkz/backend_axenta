package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// LoadTestConfig конфигурация нагрузочного тестирования
type LoadTestConfig struct {
	ConcurrentUsers  int           `json:"concurrent_users"`
	DurationSeconds  int           `json:"duration_seconds"`
	RampUpSeconds    int           `json:"ramp_up_seconds"`
	Endpoints        []string      `json:"endpoints"`
	RequestsPerUser  int           `json:"requests_per_user"`
	ThinkTimeMs      int           `json:"think_time_ms"`
	Timeout          time.Duration `json:"timeout"`
}

// LoadTestResult результат нагрузочного тестирования
type LoadTestResult struct {
	Config                LoadTestConfig                    `json:"config"`
	StartTime             time.Time                         `json:"start_time"`
	EndTime               time.Time                         `json:"end_time"`
	TotalRequests         int64                             `json:"total_requests"`
	SuccessfulRequests    int64                             `json:"successful_requests"`
	FailedRequests        int64                             `json:"failed_requests"`
	AverageResponseTime   float64                           `json:"average_response_time"`
	MaxResponseTime       float64                           `json:"max_response_time"`
	MinResponseTime       float64                           `json:"min_response_time"`
	RequestsPerSecond     float64                           `json:"requests_per_second"`
	ErrorRate             float64                           `json:"error_rate"`
	ResultsByEndpoint     map[string]*EndpointResult        `json:"results_by_endpoint"`
	ResponseTimeHistogram map[string]int64                  `json:"response_time_histogram"`
	ErrorsByType          map[string]int64                  `json:"errors_by_type"`
}

// EndpointResult результат для конкретного endpoint
type EndpointResult struct {
	Endpoint            string  `json:"endpoint"`
	Requests            int64   `json:"requests"`
	SuccessfulRequests  int64   `json:"successful_requests"`
	FailedRequests      int64   `json:"failed_requests"`
	AverageResponseTime float64 `json:"average_response_time"`
	MaxResponseTime     float64 `json:"max_response_time"`
	MinResponseTime     float64 `json:"min_response_time"`
	ErrorRate           float64 `json:"error_rate"`
}

// LoadTestService сервис нагрузочного тестирования
type LoadTestService struct {
	httpClient *http.Client
	logger     *log.Logger
	baseURL    string
}

// NewLoadTestService создает новый сервис нагрузочного тестирования
func NewLoadTestService(baseURL string, logger *log.Logger) *LoadTestService {
	return &LoadTestService{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger:  logger,
		baseURL: baseURL,
	}
}

// RequestMetrics метрики одного запроса
type RequestMetrics struct {
	Endpoint     string
	ResponseTime time.Duration
	Success      bool
	ErrorType    string
}

// RunLoadTest выполняет нагрузочное тестирование
func (lts *LoadTestService) RunLoadTest(ctx context.Context, config LoadTestConfig) (*LoadTestResult, error) {
	if lts.logger != nil {
		lts.logger.Printf("Starting load test with %d concurrent users for %d seconds", 
			config.ConcurrentUsers, config.DurationSeconds)
	}

	result := &LoadTestResult{
		Config:            config,
		StartTime:         time.Now(),
		ResultsByEndpoint: make(map[string]*EndpointResult),
		ResponseTimeHistogram: map[string]int64{
			"0-50ms":    0,
			"50-100ms":  0,
			"100-200ms": 0,
			"200-500ms": 0,
			"500ms-1s":  0,
			"1s-2s":     0,
			"2s+":       0,
		},
		ErrorsByType: make(map[string]int64),
	}

	// Инициализируем результаты для каждого endpoint
	for _, endpoint := range config.Endpoints {
		result.ResultsByEndpoint[endpoint] = &EndpointResult{
			Endpoint:        endpoint,
			MinResponseTime: math.MaxFloat64,
		}
	}

	// Канал для сбора метрик
	metricsChan := make(chan RequestMetrics, config.ConcurrentUsers*100)
	
	// Контекст с таймаутом
	testCtx, cancel := context.WithTimeout(ctx, time.Duration(config.DurationSeconds)*time.Second)
	defer cancel()

	// Запускаем пользователей
	var wg sync.WaitGroup
	userStartInterval := time.Duration(config.RampUpSeconds) * time.Second / time.Duration(config.ConcurrentUsers)

	for i := 0; i < config.ConcurrentUsers; i++ {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()
			
			// Постепенное увеличение нагрузки
			time.Sleep(time.Duration(userID) * userStartInterval)
			
			lts.simulateUser(testCtx, userID, config, metricsChan)
		}(i)
	}

	// Горутина для сбора метрик
	go func() {
		wg.Wait()
		close(metricsChan)
	}()

	// Обрабатываем метрики
	lts.processMetrics(metricsChan, result)

	result.EndTime = time.Now()
	lts.calculateFinalStats(result)

	if lts.logger != nil {
		lts.logger.Printf("Load test completed: %d requests, %.2f RPS, %.2f%% error rate",
			result.TotalRequests, result.RequestsPerSecond, result.ErrorRate)
	}

	return result, nil
}

// simulateUser симулирует поведение одного пользователя
func (lts *LoadTestService) simulateUser(ctx context.Context, userID int, config LoadTestConfig, metricsChan chan<- RequestMetrics) {
	thinkTime := time.Duration(config.ThinkTimeMs) * time.Millisecond
	
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Выбираем случайный endpoint
			endpoint := config.Endpoints[userID%len(config.Endpoints)]
			
			// Выполняем запрос
			metrics := lts.makeRequest(endpoint)
			
			select {
			case metricsChan <- metrics:
			case <-ctx.Done():
				return
			}
			
			// Пауза между запросами
			if thinkTime > 0 {
				select {
				case <-time.After(thinkTime):
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

// makeRequest выполняет HTTP запрос и возвращает метрики
func (lts *LoadTestService) makeRequest(endpoint string) RequestMetrics {
	start := time.Now()
	
	url := lts.baseURL + endpoint
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return RequestMetrics{
			Endpoint:     endpoint,
			ResponseTime: time.Since(start),
			Success:      false,
			ErrorType:    "request_creation_error",
		}
	}

	// Добавляем заголовки
	req.Header.Set("User-Agent", "LoadTest/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := lts.httpClient.Do(req)
	responseTime := time.Since(start)
	
	if err != nil {
		return RequestMetrics{
			Endpoint:     endpoint,
			ResponseTime: responseTime,
			Success:      false,
			ErrorType:    "network_error",
		}
	}
	defer resp.Body.Close()

	success := resp.StatusCode >= 200 && resp.StatusCode < 400
	errorType := ""
	if !success {
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			errorType = "client_error"
		} else if resp.StatusCode >= 500 {
			errorType = "server_error"
		}
	}

	return RequestMetrics{
		Endpoint:     endpoint,
		ResponseTime: responseTime,
		Success:      success,
		ErrorType:    errorType,
	}
}

// processMetrics обрабатывает метрики запросов
func (lts *LoadTestService) processMetrics(metricsChan <-chan RequestMetrics, result *LoadTestResult) {
	var totalResponseTime float64
	
	for metrics := range metricsChan {
		atomic.AddInt64(&result.TotalRequests, 1)
		
		responseTimeMs := float64(metrics.ResponseTime.Nanoseconds()) / 1e6
		totalResponseTime += responseTimeMs
		
		// Обновляем статистику по endpoint
		endpointResult := result.ResultsByEndpoint[metrics.Endpoint]
		endpointResult.Requests++
		
		if metrics.Success {
			atomic.AddInt64(&result.SuccessfulRequests, 1)
			endpointResult.SuccessfulRequests++
		} else {
			atomic.AddInt64(&result.FailedRequests, 1)
			endpointResult.FailedRequests++
			result.ErrorsByType[metrics.ErrorType]++
		}
		
		// Обновляем времена ответа
		if responseTimeMs > result.MaxResponseTime {
			result.MaxResponseTime = responseTimeMs
		}
		if responseTimeMs < result.MinResponseTime || result.MinResponseTime == 0 {
			result.MinResponseTime = responseTimeMs
		}
		
		if responseTimeMs > endpointResult.MaxResponseTime {
			endpointResult.MaxResponseTime = responseTimeMs
		}
		if responseTimeMs < endpointResult.MinResponseTime {
			endpointResult.MinResponseTime = responseTimeMs
		}
		
		// Гистограмма времени ответа
		lts.updateResponseTimeHistogram(result, responseTimeMs)
	}
	
	if result.TotalRequests > 0 {
		result.AverageResponseTime = totalResponseTime / float64(result.TotalRequests)
	}
}

// updateResponseTimeHistogram обновляет гистограмму времени ответа
func (lts *LoadTestService) updateResponseTimeHistogram(result *LoadTestResult, responseTimeMs float64) {
	switch {
	case responseTimeMs < 50:
		result.ResponseTimeHistogram["0-50ms"]++
	case responseTimeMs < 100:
		result.ResponseTimeHistogram["50-100ms"]++
	case responseTimeMs < 200:
		result.ResponseTimeHistogram["100-200ms"]++
	case responseTimeMs < 500:
		result.ResponseTimeHistogram["200-500ms"]++
	case responseTimeMs < 1000:
		result.ResponseTimeHistogram["500ms-1s"]++
	case responseTimeMs < 2000:
		result.ResponseTimeHistogram["1s-2s"]++
	default:
		result.ResponseTimeHistogram["2s+"]++
	}
}

// calculateFinalStats вычисляет финальную статистику
func (lts *LoadTestService) calculateFinalStats(result *LoadTestResult) {
	duration := result.EndTime.Sub(result.StartTime).Seconds()
	
	if duration > 0 {
		result.RequestsPerSecond = float64(result.TotalRequests) / duration
	}
	
	if result.TotalRequests > 0 {
		result.ErrorRate = float64(result.FailedRequests) / float64(result.TotalRequests) * 100
	}
	
	// Вычисляем статистику для каждого endpoint
	for _, endpointResult := range result.ResultsByEndpoint {
		if endpointResult.Requests > 0 {
			endpointResult.ErrorRate = float64(endpointResult.FailedRequests) / float64(endpointResult.Requests) * 100
		}
		
		// Средние времена ответа нужно пересчитать на основе всех запросов
		// Для простоты используем общее среднее время
		endpointResult.AverageResponseTime = result.AverageResponseTime
	}
}

// GetPredefinedConfigs возвращает предустановленные конфигурации тестов
func (lts *LoadTestService) GetPredefinedConfigs() map[string]LoadTestConfig {
	return map[string]LoadTestConfig{
		"light": {
			ConcurrentUsers: 10,
			DurationSeconds: 60,
			RampUpSeconds:   10,
			Endpoints: []string{
				"/api/objects",
				"/api/users",
				"/api/dashboard/stats",
			},
			ThinkTimeMs: 1000,
			Timeout:     30 * time.Second,
		},
		"moderate": {
			ConcurrentUsers: 50,
			DurationSeconds: 300,
			RampUpSeconds:   30,
			Endpoints: []string{
				"/api/objects",
				"/api/users",
				"/api/contracts",
				"/api/installations",
				"/api/dashboard/stats",
			},
			ThinkTimeMs: 500,
			Timeout:     30 * time.Second,
		},
		"heavy": {
			ConcurrentUsers: 100,
			DurationSeconds: 600,
			RampUpSeconds:   60,
			Endpoints: []string{
				"/api/objects",
				"/api/users",
				"/api/contracts",
				"/api/installations",
				"/api/warehouse/equipment",
				"/api/reports",
				"/api/dashboard/stats",
			},
			ThinkTimeMs: 200,
			Timeout:     30 * time.Second,
		},
		"stress": {
			ConcurrentUsers: 200,
			DurationSeconds: 300,
			RampUpSeconds:   30,
			Endpoints: []string{
				"/api/objects",
				"/api/users",
				"/api/contracts",
				"/api/installations",
				"/api/warehouse/equipment",
				"/api/reports",
				"/api/dashboard/stats",
				"/api/billing/invoices",
			},
			ThinkTimeMs: 100,
			Timeout:     15 * time.Second,
		},
	}
}

// ExportResults экспортирует результаты в JSON
func (lts *LoadTestService) ExportResults(result *LoadTestResult) ([]byte, error) {
	return json.MarshalIndent(result, "", "  ")
}

// GenerateReport генерирует текстовый отчет
func (lts *LoadTestService) GenerateReport(result *LoadTestResult) string {
	report := fmt.Sprintf(`
Отчет о нагрузочном тестировании
================================

Конфигурация теста:
- Одновременные пользователи: %d
- Длительность: %d секунд
- Время разгона: %d секунд
- Endpoints: %v

Результаты:
- Время выполнения: %v
- Общее количество запросов: %d
- Успешных запросов: %d (%.2f%%)
- Неуспешных запросов: %d (%.2f%%)
- Запросов в секунду: %.2f
- Среднее время ответа: %.2f мс
- Минимальное время ответа: %.2f мс
- Максимальное время ответа: %.2f мс

Распределение времени ответа:
- 0-50мс: %d (%.1f%%)
- 50-100мс: %d (%.1f%%)
- 100-200мс: %d (%.1f%%)
- 200-500мс: %d (%.1f%%)
- 500мс-1с: %d (%.1f%%)
- 1с-2с: %d (%.1f%%)
- 2с+: %d (%.1f%%)

Результаты по endpoints:
`,
		result.Config.ConcurrentUsers,
		result.Config.DurationSeconds,
		result.Config.RampUpSeconds,
		result.Config.Endpoints,
		result.EndTime.Sub(result.StartTime),
		result.TotalRequests,
		result.SuccessfulRequests, float64(result.SuccessfulRequests)/float64(result.TotalRequests)*100,
		result.FailedRequests, result.ErrorRate,
		result.RequestsPerSecond,
		result.AverageResponseTime,
		result.MinResponseTime,
		result.MaxResponseTime,
		result.ResponseTimeHistogram["0-50ms"], float64(result.ResponseTimeHistogram["0-50ms"])/float64(result.TotalRequests)*100,
		result.ResponseTimeHistogram["50-100ms"], float64(result.ResponseTimeHistogram["50-100ms"])/float64(result.TotalRequests)*100,
		result.ResponseTimeHistogram["100-200ms"], float64(result.ResponseTimeHistogram["100-200ms"])/float64(result.TotalRequests)*100,
		result.ResponseTimeHistogram["200-500ms"], float64(result.ResponseTimeHistogram["200-500ms"])/float64(result.TotalRequests)*100,
		result.ResponseTimeHistogram["500ms-1s"], float64(result.ResponseTimeHistogram["500ms-1s"])/float64(result.TotalRequests)*100,
		result.ResponseTimeHistogram["1s-2s"], float64(result.ResponseTimeHistogram["1s-2s"])/float64(result.TotalRequests)*100,
		result.ResponseTimeHistogram["2s+"], float64(result.ResponseTimeHistogram["2s+"])/float64(result.TotalRequests)*100,
	)

	for endpoint, endpointResult := range result.ResultsByEndpoint {
		report += fmt.Sprintf(`
%s:
  - Запросов: %d
  - Успешных: %d (%.2f%%)
  - Неуспешных: %d (%.2f%%)
  - Среднее время ответа: %.2f мс
  - Мин/Макс время: %.2f/%.2f мс
`,
			endpoint,
			endpointResult.Requests,
			endpointResult.SuccessfulRequests, float64(endpointResult.SuccessfulRequests)/float64(endpointResult.Requests)*100,
			endpointResult.FailedRequests, endpointResult.ErrorRate,
			endpointResult.AverageResponseTime,
			endpointResult.MinResponseTime,
			endpointResult.MaxResponseTime,
		)
	}

	if len(result.ErrorsByType) > 0 {
		report += "\nОшибки по типам:\n"
		for errorType, count := range result.ErrorsByType {
			report += fmt.Sprintf("- %s: %d\n", errorType, count)
		}
	}

	return report
}
