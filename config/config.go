package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config содержит всю конфигурацию приложения
type Config struct {
	// Основные настройки приложения
	App AppConfigStruct `json:"app"`

	// База данных
	Database DatabaseConfig `json:"database"`

	// Redis
	Redis RedisConfig `json:"redis"`

	// JWT
	JWT JWTConfig `json:"jwt"`

	// Axenta Cloud
	Axenta AxentaConfig `json:"axenta"`

	// CORS
	CORS CORSConfig `json:"cors"`

	// Безопасность
	Security SecurityConfig `json:"security"`

	// Логирование
	Logging LoggingConfig `json:"logging"`

	// Внешние сервисы
	External ExternalConfig `json:"external"`
}

type AppConfigStruct struct {
	Env     string `json:"env"`
	Port    string `json:"port"`
	Host    string `json:"host"`
	BaseURL string `json:"base_url"`
	Version string `json:"version"`
	Debug   bool   `json:"debug"`
}

type DatabaseConfig struct {
	Host            string        `json:"host"`
	Port            string        `json:"port"`
	User            string        `json:"user"`
	Password        string        `json:"password"`
	Name            string        `json:"name"`
	SSLMode         string        `json:"ssl_mode"`
	MaxOpenConns    int           `json:"max_open_conns"`
	MaxIdleConns    int           `json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime"`
}

type RedisConfig struct {
	Host     string        `json:"host"`
	Port     string        `json:"port"`
	Password string        `json:"password"`
	DB       int           `json:"db"`
	URL      string        `json:"url"`
	Timeout  time.Duration `json:"timeout"`
	MaxConns int           `json:"max_connections"`
}

type JWTConfig struct {
	Secret           string        `json:"secret"`
	ExpiresIn        time.Duration `json:"expires_in"`
	RefreshExpiresIn time.Duration `json:"refresh_expires_in"`
	Issuer           string        `json:"issuer"`
}

type AxentaConfig struct {
	APIURL        string        `json:"api_url"`
	Timeout       time.Duration `json:"timeout"`
	MaxRetries    int           `json:"max_retries"`
	EncryptionKey string        `json:"encryption_key"`
}

type CORSConfig struct {
	AllowedOrigins   []string `json:"allowed_origins"`
	AllowedMethods   []string `json:"allowed_methods"`
	AllowedHeaders   []string `json:"allowed_headers"`
	AllowCredentials bool     `json:"allow_credentials"`
	MaxAge           int      `json:"max_age"`
}

type SecurityConfig struct {
	RateLimitRequests int           `json:"rate_limit_requests"`
	RateLimitWindow   time.Duration `json:"rate_limit_window"`
	MaxRequestSize    string        `json:"max_request_size"`
	RequestTimeout    time.Duration `json:"request_timeout"`
	ResponseTimeout   time.Duration `json:"response_timeout"`
	MaxConnections    int           `json:"max_connections"`
}

type LoggingConfig struct {
	Level      string `json:"level"`
	Format     string `json:"format"`
	File       string `json:"file"`
	MaxSize    int    `json:"max_size"`
	MaxBackups int    `json:"max_backups"`
	MaxAge     int    `json:"max_age"`
}

type ExternalConfig struct {
	// Email
	SMTP SMTPConfig `json:"smtp"`

	// Telegram
	TelegramBotToken string `json:"telegram_bot_token"`
	TelegramChatID   string `json:"telegram_chat_id"`

	// Webhook
	WebhookURL    string `json:"webhook_url"`
	WebhookSecret string `json:"webhook_secret"`

	// Google Maps
	GoogleMapsAPIKey string `json:"google_maps_api_key"`
}

type SMTPConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	From     string `json:"from"`
	TLS      bool   `json:"tls"`
}

var GlobalConfig *Config

// LoadConfig загружает конфигурацию из переменных окружения
func LoadConfig() (*Config, error) {
	// Загружаем .env файл если он существует
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found or could not be loaded: %v", err)
	}

	config := &Config{
		App: AppConfigStruct{
			Env:     getEnv("APP_ENV", "development"),
			Port:    getEnv("APP_PORT", "8080"),
			Host:    getEnv("APP_HOST", "0.0.0.0"),
			BaseURL: getEnv("BACKEND_URL", "http://localhost:8080"),
			Version: getEnv("API_VERSION", "v1"),
			Debug:   getEnvBool("DEBUG_MODE", false),
		},
		Database: DatabaseConfig{
			Host:            getEnv("DB_HOST", "localhost"),
			Port:            getEnv("DB_PORT", "5432"),
			User:            getEnv("DB_USER", "postgres"),
			Password:        getEnv("DB_PASSWORD", ""),
			Name:            getEnv("DB_NAME", "axenta_db"),
			SSLMode:         getEnv("DB_SSLMODE", "disable"),
			MaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getEnvDuration("DB_CONN_MAX_LIFETIME", 300*time.Second),
		},
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnv("REDIS_PORT", "6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
			URL:      getEnv("REDIS_URL", ""),
			Timeout:  getEnvDuration("REDIS_TIMEOUT", 5*time.Second),
			MaxConns: getEnvInt("REDIS_MAX_CONNECTIONS", 10),
		},
		JWT: JWTConfig{
			Secret:           getEnv("JWT_SECRET", ""),
			ExpiresIn:        getEnvDuration("JWT_EXPIRES_IN", 24*time.Hour),
			RefreshExpiresIn: getEnvDuration("JWT_REFRESH_EXPIRES_IN", 168*time.Hour),
			Issuer:           getEnv("JWT_ISSUER", "axenta-crm"),
		},
		Axenta: AxentaConfig{
			APIURL:        getEnv("AXENTA_API_URL", "https://api.axetna.cloud"),
			Timeout:       getEnvDuration("AXENTA_TIMEOUT", 30*time.Second),
			MaxRetries:    getEnvInt("AXENTA_MAX_RETRIES", 3),
			EncryptionKey: getEnv("ENCRYPTION_KEY", ""),
		},
		CORS: CORSConfig{
			AllowedOrigins:   getEnvSlice("CORS_ALLOWED_ORIGINS", []string{"*"}),
			AllowedMethods:   getEnvSlice("CORS_ALLOWED_METHODS", []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
			AllowedHeaders:   getEnvSlice("CORS_ALLOWED_HEADERS", []string{"Content-Type", "Authorization", "X-Requested-With", "Accept", "Origin"}),
			AllowCredentials: getEnvBool("CORS_ALLOW_CREDENTIALS", true),
			MaxAge:           getEnvInt("CORS_MAX_AGE", 86400),
		},
		Security: SecurityConfig{
			RateLimitRequests: getEnvInt("RATE_LIMIT_REQUESTS", 100),
			RateLimitWindow:   getEnvDuration("RATE_LIMIT_WINDOW", 1*time.Minute),
			MaxRequestSize:    getEnv("MAX_REQUEST_SIZE", "10MB"),
			RequestTimeout:    getEnvDuration("REQUEST_TIMEOUT", 30*time.Second),
			ResponseTimeout:   getEnvDuration("RESPONSE_TIMEOUT", 30*time.Second),
			MaxConnections:    getEnvInt("MAX_CONNECTIONS", 1000),
		},
		Logging: LoggingConfig{
			Level:      getEnv("LOG_LEVEL", "info"),
			Format:     getEnv("LOG_FORMAT", "json"),
			File:       getEnv("LOG_FILE", ""),
			MaxSize:    getEnvInt("LOG_MAX_SIZE", 100),
			MaxBackups: getEnvInt("LOG_MAX_BACKUPS", 10),
			MaxAge:     getEnvInt("LOG_MAX_AGE", 30),
		},
		External: ExternalConfig{
			SMTP: SMTPConfig{
				Host:     getEnv("SMTP_HOST", ""),
				Port:     getEnvInt("SMTP_PORT", 587),
				User:     getEnv("SMTP_USER", ""),
				Password: getEnv("SMTP_PASSWORD", ""),
				From:     getEnv("SMTP_FROM", ""),
				TLS:      getEnvBool("SMTP_TLS", true),
			},
			TelegramBotToken: getEnv("TELEGRAM_BOT_TOKEN", ""),
			TelegramChatID:   getEnv("TELEGRAM_CHAT_ID", ""),
			WebhookURL:       getEnv("WEBHOOK_URL", ""),
			WebhookSecret:    getEnv("WEBHOOK_SECRET", ""),
			GoogleMapsAPIKey: getEnv("GOOGLE_MAPS_API_KEY", ""),
		},
	}

	// Валидация критически важных настроек
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	GlobalConfig = config
	return config, nil
}

// Validate проверяет корректность конфигурации
func (c *Config) Validate() error {
	// Проверяем обязательные поля для продакшена
	if c.App.Env == "production" {
		if c.JWT.Secret == "" {
			return fmt.Errorf("JWT_SECRET is required in production")
		}
		if len(c.JWT.Secret) < 32 {
			return fmt.Errorf("JWT_SECRET must be at least 32 characters long")
		}
		if c.Database.Password == "" {
			return fmt.Errorf("DB_PASSWORD is required in production")
		}
		if c.Axenta.EncryptionKey == "" {
			return fmt.Errorf("ENCRYPTION_KEY is required in production")
		}
		if len(c.Axenta.EncryptionKey) < 32 {
			return fmt.Errorf("ENCRYPTION_KEY must be at least 32 characters long")
		}
	}

	// Проверяем в любом окружении
	if c.Database.Name == "" {
		return fmt.Errorf("DB_NAME cannot be empty")
	}
	if c.Database.User == "" {
		return fmt.Errorf("DB_USER cannot be empty")
	}

	return nil
}

// GetConfig возвращает текущую конфигурацию
func GetConfig() *Config {
	if GlobalConfig == nil {
		log.Fatal("Config not loaded. Call LoadConfig() first.")
	}
	return GlobalConfig
}

// Вспомогательные функции для получения переменных окружения

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
		log.Printf("Warning: Invalid integer value for %s: %s, using default: %d", key, value, defaultValue)
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
		log.Printf("Warning: Invalid boolean value for %s: %s, using default: %t", key, value, defaultValue)
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
		log.Printf("Warning: Invalid duration value for %s: %s, using default: %v", key, value, defaultValue)
	}
	return defaultValue
}

func getEnvSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		return strings.Split(value, ",")
	}
	return defaultValue
}

// IsDevelopment проверяет, запущено ли приложение в режиме разработки
func (c *Config) IsDevelopment() bool {
	return c.App.Env == "development"
}

// IsProduction проверяет, запущено ли приложение в продакшене
func (c *Config) IsProduction() bool {
	return c.App.Env == "production"
}

// GetDatabaseDSN возвращает строку подключения к БД
func (c *Config) GetDatabaseDSN() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Database.Host, c.Database.Port, c.Database.User,
		c.Database.Password, c.Database.Name, c.Database.SSLMode)
}

// GetRedisAddr возвращает адрес Redis
func (c *Config) GetRedisAddr() string {
	if c.Redis.URL != "" {
		return c.Redis.URL
	}
	return fmt.Sprintf("%s:%s", c.Redis.Host, c.Redis.Port)
}

// LogConfig выводит конфигурацию в лог (без секретных данных)
func (c *Config) LogConfig() {
	log.Printf("=== Application Configuration ===")
	log.Printf("Environment: %s", c.App.Env)
	log.Printf("Port: %s", c.App.Port)
	log.Printf("Database Host: %s:%s", c.Database.Host, c.Database.Port)
	log.Printf("Database Name: %s", c.Database.Name)
	log.Printf("Redis Host: %s:%s", c.Redis.Host, c.Redis.Port)
	log.Printf("Axenta API URL: %s", c.Axenta.APIURL)
	log.Printf("JWT Issuer: %s", c.JWT.Issuer)
	log.Printf("Log Level: %s", c.Logging.Level)
	log.Printf("Debug Mode: %t", c.App.Debug)
	log.Printf("================================")
}
