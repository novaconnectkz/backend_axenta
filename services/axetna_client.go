package services

import (
	"backend_axenta/models"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"time"
)

// AxetnaClient клиент для работы с Axetna.cloud API
type AxetnaClient struct {
	BaseURL    string
	HTTPClient *http.Client
	Logger     *log.Logger
}

// TenantCredentials учетные данные компании для API
type TenantCredentials struct {
	Login     string
	Password  string
	Token     string // JWT токен для авторизации
	ExpiresAt time.Time
}

// RetryConfig конфигурация для retry механизма
type RetryConfig struct {
	MaxRetries      int
	InitialDelay    time.Duration
	MaxDelay        time.Duration
	BackoffFactor   float64
	RetryableErrors []int // HTTP статус коды для повтора
}

// AxetnaObjectRequest запрос для создания/обновления объекта в Axetna.cloud
type AxetnaObjectRequest struct {
	Name         string                 `json:"name"`
	Type         string                 `json:"type"`
	Description  string                 `json:"description,omitempty"`
	IMEI         string                 `json:"imei,omitempty"`
	PhoneNumber  string                 `json:"phone_number,omitempty"`
	SerialNumber string                 `json:"serial_number,omitempty"`
	Settings     map[string]interface{} `json:"settings,omitempty"`
	Latitude     *float64               `json:"latitude,omitempty"`
	Longitude    *float64               `json:"longitude,omitempty"`
	Address      string                 `json:"address,omitempty"`
}

// AxetnaObjectResponse ответ от Axetna.cloud API
type AxetnaObjectResponse struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Type      string                 `json:"type"`
	Status    string                 `json:"status"`
	Settings  map[string]interface{} `json:"settings"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
	Error     string                 `json:"error,omitempty"`
}

// AxetnaAuthResponse ответ авторизации от Axetna.cloud
type AxetnaAuthResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	Error     string    `json:"error,omitempty"`
}

// NewAxetnaClient создает новый клиент для Axetna.cloud API
func NewAxetnaClient(baseURL string, logger *log.Logger) *AxetnaClient {
	if logger == nil {
		logger = log.New(io.Discard, "", 0) // Пустой логгер если не передан
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false, // В продакшене должно быть false
			},
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	return &AxetnaClient{
		BaseURL:    baseURL,
		HTTPClient: client,
		Logger:     logger,
	}
}

// GetDefaultRetryConfig возвращает стандартную конфигурацию retry
func GetDefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:    3,
		InitialDelay:  1 * time.Second,
		MaxDelay:      30 * time.Second,
		BackoffFactor: 2.0,
		RetryableErrors: []int{
			http.StatusInternalServerError,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout,
			http.StatusTooManyRequests,
		},
	}
}

// Authenticate авторизуется в Axetna.cloud API
func (c *AxetnaClient) Authenticate(ctx context.Context, login, password string) (*TenantCredentials, error) {
	authData := map[string]string{
		"login":    login,
		"password": password,
	}

	jsonData, err := json.Marshal(authData)
	if err != nil {
		return nil, fmt.Errorf("ошибка сериализации данных авторизации: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/auth/login", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса авторизации: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "AxentaCRM/1.0")

	resp, err := c.CallWithRetry(req, GetDefaultRetryConfig())
	if err != nil {
		return nil, fmt.Errorf("ошибка выполнения запроса авторизации: %w", err)
	}
	defer resp.Body.Close()

	var authResp AxetnaAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return nil, fmt.Errorf("ошибка декодирования ответа авторизации: %w", err)
	}

	if authResp.Error != "" {
		return nil, fmt.Errorf("ошибка авторизации: %s", authResp.Error)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("неуспешная авторизация, статус: %d", resp.StatusCode)
	}

	credentials := &TenantCredentials{
		Login:     login,
		Password:  password,
		Token:     authResp.Token,
		ExpiresAt: authResp.ExpiresAt,
	}

	c.Logger.Printf("Успешная авторизация для пользователя: %s", login)
	return credentials, nil
}

// CreateObject создает объект в Axetna.cloud
func (c *AxetnaClient) CreateObject(ctx context.Context, credentials *TenantCredentials, object *models.Object) (*AxetnaObjectResponse, error) {
	// Проверяем валидность токена
	if time.Now().After(credentials.ExpiresAt) {
		newCreds, err := c.Authenticate(ctx, credentials.Login, credentials.Password)
		if err != nil {
			return nil, fmt.Errorf("ошибка обновления токена: %w", err)
		}
		*credentials = *newCreds
	}

	// Подготавливаем данные объекта
	objectReq := &AxetnaObjectRequest{
		Name:         object.Name,
		Type:         object.Type,
		Description:  object.Description,
		IMEI:         object.IMEI,
		PhoneNumber:  object.PhoneNumber,
		SerialNumber: object.SerialNumber,
		Latitude:     object.Latitude,
		Longitude:    object.Longitude,
		Address:      object.Address,
	}

	// Парсим настройки из JSON строки
	if object.Settings != "" {
		var settings map[string]interface{}
		if err := json.Unmarshal([]byte(object.Settings), &settings); err == nil {
			objectReq.Settings = settings
		}
	}

	jsonData, err := json.Marshal(objectReq)
	if err != nil {
		return nil, fmt.Errorf("ошибка сериализации данных объекта: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/objects", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+credentials.Token)
	req.Header.Set("User-Agent", "AxentaCRM/1.0")

	resp, err := c.CallWithRetry(req, GetDefaultRetryConfig())
	if err != nil {
		return nil, fmt.Errorf("ошибка выполнения запроса создания объекта: %w", err)
	}
	defer resp.Body.Close()

	var objectResp AxetnaObjectResponse
	if err := json.NewDecoder(resp.Body).Decode(&objectResp); err != nil {
		return nil, fmt.Errorf("ошибка декодирования ответа: %w", err)
	}

	if objectResp.Error != "" {
		return nil, fmt.Errorf("ошибка создания объекта в Axetna.cloud: %s", objectResp.Error)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("неуспешное создание объекта, статус: %d", resp.StatusCode)
	}

	c.Logger.Printf("Объект успешно создан в Axetna.cloud: %s (ID: %s)", objectResp.Name, objectResp.ID)
	return &objectResp, nil
}

// UpdateObject обновляет объект в Axetna.cloud
func (c *AxetnaClient) UpdateObject(ctx context.Context, credentials *TenantCredentials, object *models.Object) (*AxetnaObjectResponse, error) {
	// Проверяем валидность токена
	if time.Now().After(credentials.ExpiresAt) {
		newCreds, err := c.Authenticate(ctx, credentials.Login, credentials.Password)
		if err != nil {
			return nil, fmt.Errorf("ошибка обновления токена: %w", err)
		}
		*credentials = *newCreds
	}

	if object.ExternalID == "" {
		return nil, fmt.Errorf("отсутствует ExternalID для обновления объекта")
	}

	// Подготавливаем данные объекта
	objectReq := &AxetnaObjectRequest{
		Name:         object.Name,
		Type:         object.Type,
		Description:  object.Description,
		IMEI:         object.IMEI,
		PhoneNumber:  object.PhoneNumber,
		SerialNumber: object.SerialNumber,
		Latitude:     object.Latitude,
		Longitude:    object.Longitude,
		Address:      object.Address,
	}

	// Парсим настройки из JSON строки
	if object.Settings != "" {
		var settings map[string]interface{}
		if err := json.Unmarshal([]byte(object.Settings), &settings); err == nil {
			objectReq.Settings = settings
		}
	}

	jsonData, err := json.Marshal(objectReq)
	if err != nil {
		return nil, fmt.Errorf("ошибка сериализации данных объекта: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", c.BaseURL+"/objects/"+object.ExternalID, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+credentials.Token)
	req.Header.Set("User-Agent", "AxentaCRM/1.0")

	resp, err := c.CallWithRetry(req, GetDefaultRetryConfig())
	if err != nil {
		return nil, fmt.Errorf("ошибка выполнения запроса обновления объекта: %w", err)
	}
	defer resp.Body.Close()

	var objectResp AxetnaObjectResponse
	if err := json.NewDecoder(resp.Body).Decode(&objectResp); err != nil {
		return nil, fmt.Errorf("ошибка декодирования ответа: %w", err)
	}

	if objectResp.Error != "" {
		return nil, fmt.Errorf("ошибка обновления объекта в Axetna.cloud: %s", objectResp.Error)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("неуспешное обновление объекта, статус: %d", resp.StatusCode)
	}

	c.Logger.Printf("Объект успешно обновлен в Axetna.cloud: %s (ID: %s)", objectResp.Name, objectResp.ID)
	return &objectResp, nil
}

// DeleteObject удаляет объект в Axetna.cloud
func (c *AxetnaClient) DeleteObject(ctx context.Context, credentials *TenantCredentials, externalID string) error {
	// Проверяем валидность токена
	if time.Now().After(credentials.ExpiresAt) {
		newCreds, err := c.Authenticate(ctx, credentials.Login, credentials.Password)
		if err != nil {
			return fmt.Errorf("ошибка обновления токена: %w", err)
		}
		*credentials = *newCreds
	}

	req, err := http.NewRequestWithContext(ctx, "DELETE", c.BaseURL+"/objects/"+externalID, nil)
	if err != nil {
		return fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+credentials.Token)
	req.Header.Set("User-Agent", "AxentaCRM/1.0")

	resp, err := c.CallWithRetry(req, GetDefaultRetryConfig())
	if err != nil {
		return fmt.Errorf("ошибка выполнения запроса удаления объекта: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("неуспешное удаление объекта, статус: %d", resp.StatusCode)
	}

	c.Logger.Printf("Объект успешно удален из Axetna.cloud: %s", externalID)
	return nil
}

// CallWithRetry выполняет HTTP запрос с retry механизмом
func (c *AxetnaClient) CallWithRetry(req *http.Request, config RetryConfig) (*http.Response, error) {
	var lastErr error
	var resp *http.Response

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Клонируем запрос для повторного использования
		reqClone := req.Clone(req.Context())

		// Восстанавливаем тело запроса если оно есть
		if req.Body != nil {
			if req.GetBody != nil {
				body, err := req.GetBody()
				if err != nil {
					return nil, fmt.Errorf("ошибка восстановления тела запроса: %w", err)
				}
				reqClone.Body = body
			}
		}

		resp, lastErr = c.HTTPClient.Do(reqClone)

		// Если запрос успешен или это последняя попытка
		if lastErr == nil && !c.shouldRetry(resp.StatusCode, config.RetryableErrors) {
			return resp, nil
		}

		// Закрываем тело ответа перед повтором
		if resp != nil {
			resp.Body.Close()
		}

		// Если это последняя попытка, возвращаем ошибку
		if attempt == config.MaxRetries {
			break
		}

		// Вычисляем задержку с экспоненциальным backoff
		delay := c.calculateDelay(attempt, config)

		c.Logger.Printf("Повтор запроса %s через %v (попытка %d/%d), причина: %v",
			req.URL.String(), delay, attempt+1, config.MaxRetries+1, lastErr)

		// Ждем перед повтором
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(delay):
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("все попытки исчерпаны, последняя ошибка: %w", lastErr)
	}

	return resp, nil
}

// shouldRetry определяет, нужно ли повторить запрос
func (c *AxetnaClient) shouldRetry(statusCode int, retryableErrors []int) bool {
	for _, code := range retryableErrors {
		if statusCode == code {
			return true
		}
	}
	return false
}

// calculateDelay вычисляет задержку для retry с экспоненциальным backoff
func (c *AxetnaClient) calculateDelay(attempt int, config RetryConfig) time.Duration {
	delay := float64(config.InitialDelay) * math.Pow(config.BackoffFactor, float64(attempt))

	if delay > float64(config.MaxDelay) {
		delay = float64(config.MaxDelay)
	}

	return time.Duration(delay)
}

// IsHealthy проверяет доступность Axetna.cloud API
func (c *AxetnaClient) IsHealthy(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("ошибка создания запроса проверки здоровья: %w", err)
	}

	req.Header.Set("User-Agent", "AxentaCRM/1.0")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка выполнения запроса проверки здоровья: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API недоступно, статус: %d", resp.StatusCode)
	}

	return nil
}
