package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// AuditLevel определяет уровень критичности события аудита
type AuditLevel string

const (
	LevelInfo    AuditLevel = "info"
	LevelWarning AuditLevel = "warning"
	LevelError   AuditLevel = "error"
	LevelSuccess AuditLevel = "success"
)

// AuditEntry представляет запись в аудит-логе
type AuditEntry struct {
	Timestamp  time.Time              `json:"timestamp"`
	UserID     string                 `json:"user_id,omitempty"`
	Username   string                 `json:"username,omitempty"`
	Role       string                 `json:"role,omitempty"`
	TenantID   string                 `json:"tenant_id,omitempty"`
	IP         string                 `json:"ip"`
	UserAgent  string                 `json:"user_agent,omitempty"`
	Action     string                 `json:"action"`
	Resource   string                 `json:"resource,omitempty"`
	Method     string                 `json:"method,omitempty"`
	Path       string                 `json:"path,omitempty"`
	StatusCode int                    `json:"status_code,omitempty"`
	Details    map[string]interface{} `json:"details,omitempty"`
	Success    bool                   `json:"success"`
	Level      AuditLevel             `json:"level"`
	Error      string                 `json:"error,omitempty"`
	Duration   int64                  `json:"duration_ms,omitempty"`
}

// Logger представляет логгер аудита.
// Запись неблокирующая: Log() кладёт entry в буферизированный канал, отдельная goroutine writeLoop
// сериализует и пишет в stdout/file. Раньше mutex.Lock на каждую Log сделал concurrent HTTP-запросы
// последовательными (8 параллельных запросов serialize-ились через mutex → ttfb 1с * 8 = 8с).
type Logger struct {
	file        *os.File
	logToStdout bool
	logToFile   bool
	encoder     *json.Encoder
	config      *Config

	ch      chan *AuditEntry // буфер для async-записи
	wg      sync.WaitGroup
	closeMu sync.Mutex
	closed  bool
	dropped uint64 // счётчик потерянных записей при переполнении канала
}

// Config конфигурация аудит-логгера
type Config struct {
	LogFilePath string
	LogToStdout bool
	LogToFile   bool
	MaxFileSize int64 // В байтах
	MaxBackups  int
	FlushPeriod time.Duration
	Enabled     bool
}

var (
	globalLogger *Logger
	once         sync.Once
)

// DefaultConfig возвращает конфигурацию по умолчанию
func DefaultConfig() *Config {
	return &Config{
		LogFilePath: "audit_logs.jsonl",
		LogToStdout: true,
		LogToFile:   true,
		MaxFileSize: 100 * 1024 * 1024, // 100 MB
		MaxBackups:  10,
		FlushPeriod: 1 * time.Second,
		Enabled:     true,
	}
}

// auditChannelSize — буфер async-канала. При переполнении (writeLoop тормозит) Log() сбрасывает
// записи и инкрементирует Logger.dropped. 4096 даёт ≈30с буфера при 100 RPS.
const auditChannelSize = 4096

// Init инициализирует глобальный аудит-логгер
func Init(cfg *Config) error {
	var initErr error
	once.Do(func() {
		if cfg == nil {
			cfg = DefaultConfig()
		}

		if !cfg.Enabled {
			log.Println("⚠️ Audit logging is disabled")
			return
		}

		logger := &Logger{
			logToStdout: cfg.LogToStdout,
			logToFile:   cfg.LogToFile,
			config:      cfg,
			ch:          make(chan *AuditEntry, auditChannelSize),
		}

		if cfg.LogToFile {
			// Создаем директорию для логов если её нет
			logDir := filepath.Dir(cfg.LogFilePath)
			if err := os.MkdirAll(logDir, 0755); err != nil {
				initErr = fmt.Errorf("failed to create audit log directory: %w", err)
				return
			}

			// Открываем файл для записи логов
			file, err := os.OpenFile(cfg.LogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				initErr = fmt.Errorf("failed to open audit log file: %w", err)
				return
			}

			logger.file = file
			logger.encoder = json.NewEncoder(io.MultiWriter(file))
		}

		// Запускаем единственную writer-goroutine. Все Log()-вызовы шлют в канал non-blocking.
		logger.wg.Add(1)
		go logger.writeLoop()

		globalLogger = logger
		log.Printf("✅ Audit logging initialized: file=%s, stdout=%t, file_logging=%t (async, buf=%d)",
			cfg.LogFilePath, cfg.LogToStdout, cfg.LogToFile, auditChannelSize)
	})

	return initErr
}

// GetLogger возвращает глобальный аудит-логгер
func GetLogger() *Logger {
	if globalLogger == nil {
		// Инициализируем с конфигурацией по умолчанию
		if err := Init(DefaultConfig()); err != nil {
			log.Printf("Failed to initialize audit logger: %v", err)
			return nil
		}
	}
	return globalLogger
}

// Close корректно завершает writer-goroutine и закрывает файл.
// Безопасно вызывать несколько раз — повторные вызовы no-op.
func (l *Logger) Close() error {
	l.closeMu.Lock()
	if l.closed {
		l.closeMu.Unlock()
		return nil
	}
	l.closed = true
	if l.ch != nil {
		close(l.ch)
	}
	l.closeMu.Unlock()

	l.wg.Wait() // ждём чтобы writeLoop сбросил буфер в файл

	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Log кладёт entry в async-канал (не блокирует HTTP-обработчик).
// Если канал переполнен (writeLoop тормозит) — entry дропается, dropped инкрементируется.
func (l *Logger) Log(entry *AuditEntry) {
	if l == nil || l.config == nil || !l.config.Enabled {
		return
	}

	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	if entry.Level == "" {
		if entry.Success {
			entry.Level = LevelSuccess
		} else if entry.Error != "" {
			entry.Level = LevelError
		} else {
			entry.Level = LevelInfo
		}
	}

	if l.ch == nil {
		// Lazy fallback: канал не инициализирован (например, до Init) — пишем синхронно.
		l.writeOne(entry)
		return
	}

	select {
	case l.ch <- entry:
	default:
		l.dropped++
		// Не блокируем HTTP — просто считаем потерянные записи.
		// Логируем переполнение редко, чтобы не засорять stdout (раз в 1000 дропов).
		if l.dropped%1000 == 1 {
			log.Printf("⚠️ audit channel переполнен, dropped=%d", l.dropped)
		}
	}
}

// writeLoop читает entry-shotgun из канала и пишет в stdout/file. Single goroutine = no mutex.
func (l *Logger) writeLoop() {
	defer l.wg.Done()
	for entry := range l.ch {
		l.writeOne(entry)
	}
}

// writeOne сериализует одну entry и пишет в stdout/file.
func (l *Logger) writeOne(entry *AuditEntry) {
	jsonData, err := json.Marshal(entry)
	if err != nil {
		log.Printf("Failed to marshal audit entry: %v", err)
		return
	}

	if l.logToStdout {
		fmt.Printf("[AUDIT] %s\n", string(jsonData))
	}

	if l.logToFile && l.file != nil {
		if _, err := l.file.Write(append(jsonData, '\n')); err != nil {
			log.Printf("Failed to write audit entry to file: %v", err)
		}
	}
}

// Log - глобальная функция для логирования
func Log(userID, action string, details interface{}) {
	logger := GetLogger()
	if logger == nil {
		return
	}

	var detailsMap map[string]interface{}
	if details != nil {
		// Пробуем преобразовать details в map
		switch v := details.(type) {
		case map[string]interface{}:
			detailsMap = v
		case string:
			detailsMap = map[string]interface{}{"message": v}
		default:
			// Сериализуем и десериализуем через JSON
			jsonData, err := json.Marshal(details)
			if err == nil {
				json.Unmarshal(jsonData, &detailsMap)
			}
		}
	}

	entry := &AuditEntry{
		Timestamp: time.Now(),
		UserID:    userID,
		Action:    action,
		Details:   detailsMap,
		Success:   true,
	}

	logger.Log(entry)
}

// LogFromContext записывает событие в аудит-лог используя контекст Gin
func LogFromContext(c *gin.Context, action string, details interface{}) {
	logger := GetLogger()
	if logger == nil {
		return
	}

	entry := extractEntryFromContext(c)
	entry.Action = action

	// Обрабатываем details
	var detailsMap map[string]interface{}
	if details != nil {
		switch v := details.(type) {
		case map[string]interface{}:
			detailsMap = v
		case string:
			detailsMap = map[string]interface{}{"message": v}
		default:
			jsonData, err := json.Marshal(details)
			if err == nil {
				json.Unmarshal(jsonData, &detailsMap)
			}
		}
	}
	entry.Details = detailsMap
	entry.Success = true

	logger.Log(entry)
}

// LogSuccess записывает успешное событие в аудит-лог
func LogSuccess(c *gin.Context, action string, details interface{}) {
	logger := GetLogger()
	if logger == nil {
		return
	}

	entry := extractEntryFromContext(c)
	entry.Action = action
	entry.Success = true
	entry.Level = LevelSuccess

	var detailsMap map[string]interface{}
	if details != nil {
		switch v := details.(type) {
		case map[string]interface{}:
			detailsMap = v
		case string:
			detailsMap = map[string]interface{}{"message": v}
		default:
			jsonData, err := json.Marshal(details)
			if err == nil {
				json.Unmarshal(jsonData, &detailsMap)
			}
		}
	}
	entry.Details = detailsMap

	logger.Log(entry)
}

// LogError записывает ошибку в аудит-лог
func LogError(c *gin.Context, action string, err error, details interface{}) {
	logger := GetLogger()
	if logger == nil {
		return
	}

	entry := extractEntryFromContext(c)
	entry.Action = action
	entry.Success = false
	entry.Level = LevelError
	if err != nil {
		entry.Error = err.Error()
	}

	var detailsMap map[string]interface{}
	if details != nil {
		switch v := details.(type) {
		case map[string]interface{}:
			detailsMap = v
		case string:
			detailsMap = map[string]interface{}{"message": v}
		default:
			jsonData, err := json.Marshal(details)
			if err == nil {
				json.Unmarshal(jsonData, &detailsMap)
			}
		}
	}
	entry.Details = detailsMap

	logger.Log(entry)
}

// LogWithContext записывает событие с дополнительным контекстом
func LogWithContext(ctx context.Context, userID, action string, success bool, details interface{}) {
	logger := GetLogger()
	if logger == nil {
		return
	}

	var detailsMap map[string]interface{}
	if details != nil {
		switch v := details.(type) {
		case map[string]interface{}:
			detailsMap = v
		case string:
			detailsMap = map[string]interface{}{"message": v}
		default:
			jsonData, err := json.Marshal(details)
			if err == nil {
				json.Unmarshal(jsonData, &detailsMap)
			}
		}
	}

	entry := &AuditEntry{
		Timestamp: time.Now(),
		UserID:    userID,
		Action:    action,
		Details:   detailsMap,
		Success:   success,
	}

	// Пытаемся извлечь дополнительную информацию из контекста
	if ginCtx, ok := ctx.(*gin.Context); ok {
		entry.IP = ginCtx.ClientIP()
		entry.UserAgent = ginCtx.GetHeader("User-Agent")
		entry.Method = ginCtx.Request.Method
		entry.Path = ginCtx.Request.URL.Path
	}

	logger.Log(entry)
}

// extractEntryFromContext извлекает информацию из gin.Context для создания AuditEntry
func extractEntryFromContext(c *gin.Context) *AuditEntry {
	entry := &AuditEntry{
		Timestamp: time.Now(),
		IP:        c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
		Method:    c.Request.Method,
		Path:      c.Request.URL.Path,
	}

	// Извлекаем информацию о пользователе
	if user, exists := c.Get("user"); exists {
		if userMap, ok := user.(map[string]interface{}); ok {
			// Извлекаем user_id
			if id, ok := userMap["id"]; ok {
				entry.UserID = fmt.Sprintf("%v", id)
			} else if id, ok := userMap["userId"]; ok {
				entry.UserID = fmt.Sprintf("%v", id)
			}

			// Извлекаем username
			if username, ok := userMap["username"].(string); ok {
				entry.Username = username
			} else if name, ok := userMap["name"].(string); ok {
				entry.Username = name
			}

			// Извлекаем роль
			if role, ok := userMap["role"].(string); ok {
				entry.Role = role
			} else if accountType, ok := userMap["accountType"].(string); ok {
				entry.Role = accountType
			}
		}
	}

	// Извлекаем tenant_id
	if tenantID, exists := c.Get("tenant_id"); exists {
		entry.TenantID = fmt.Sprintf("%v", tenantID)
	}

	// Извлекаем status code если есть
	if status, exists := c.Get("status_code"); exists {
		if statusInt, ok := status.(int); ok {
			entry.StatusCode = statusInt
		}
	}

	return entry
}

// Middleware создает middleware для автоматического логирования всех запросов
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		// Пропускаем логирование для некоторых путей
		skipPaths := []string{
			"/ping",
			"/health",
			"/metrics",
		}
		for _, skipPath := range skipPaths {
			if path == skipPath {
				c.Next()
				return
			}
		}

		// Обрабатываем запрос
		c.Next()

		// Логируем после обработки
		duration := time.Since(start)
		statusCode := c.Writer.Status()

		entry := extractEntryFromContext(c)
		entry.StatusCode = statusCode
		entry.Duration = duration.Milliseconds()
		entry.Action = fmt.Sprintf("%s %s", method, path)
		entry.Success = statusCode >= 200 && statusCode < 400

		if statusCode >= 400 {
			entry.Level = LevelError
			if errors := c.Errors.String(); errors != "" {
				entry.Error = errors
			}
		}

		logger := GetLogger()
		if logger != nil {
			logger.Log(entry)
		}
	}
}
