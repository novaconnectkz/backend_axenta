package services

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"time"
)

// OneCClient клиент для работы с 1С API
type OneCClient struct {
	BaseURL    string
	HTTPClient *http.Client
	Logger     *log.Logger
}

// OneCCredentials учетные данные для 1С API
type OneCCredentials struct {
	BaseURL    string // Базовый URL 1С веб-сервисов
	Username   string // Имя пользователя
	Password   string // Пароль
	Database   string // Имя информационной базы
	APIVersion string // Версия API (обычно "v1")
}

// OneCCounterparty контрагент в 1С
type OneCCounterparty struct {
	ID            string `json:"Ref_Key"`       // Уникальный ключ ссылки
	Code          string `json:"Code"`          // Код контрагента
	Description   string `json:"Description"`   // Наименование
	FullName      string `json:"FullName"`      // Полное наименование
	INN           string `json:"INN"`           // ИНН
	KPP           string `json:"KPP"`           // КПП
	OGRN          string `json:"OGRN"`          // ОГРН
	LegalAddress  string `json:"LegalAddress"`  // Юридический адрес
	ActualAddress string `json:"ActualAddress"` // Фактический адрес
	Phone         string `json:"Phone"`         // Телефон
	Email         string `json:"Email"`         // Email
	IsFolder      bool   `json:"IsFolder"`      // Является ли папкой
	Parent        string `json:"Parent_Key"`    // Ссылка на родителя
	IsActive      bool   `json:"IsActive"`      // Активность
}

// OneCPayment платеж в 1С
type OneCPayment struct {
	ID            string    `json:"Ref_Key"`           // Уникальный ключ ссылки
	Number        string    `json:"Number"`            // Номер документа
	Date          time.Time `json:"Date"`              // Дата документа
	Posted        bool      `json:"Posted"`            // Проведен ли документ
	DeletionMark  bool      `json:"DeletionMark"`      // Пометка удаления
	Organization  string    `json:"Organization_Key"`  // Организация
	Counterparty  string    `json:"Counterparty_Key"`  // Контрагент
	Contract      string    `json:"Contract_Key"`      // Договор
	CashAccount   string    `json:"CashAccount_Key"`   // Банковский счет
	Currency      string    `json:"Currency_Key"`      // Валюта
	Amount        float64   `json:"Amount"`            // Сумма
	Purpose       string    `json:"Purpose"`           // Назначение платежа
	PaymentMethod string    `json:"PaymentMethod"`     // Способ оплаты
	OperationType string    `json:"OperationType"`     // Тип операции
	BasisDocument string    `json:"BasisDocument_Key"` // Документ-основание
	Comment       string    `json:"Comment"`           // Комментарий
	ExternalID    string    `json:"ExternalID"`        // Внешний идентификатор
}

// OneCContract договор в 1С
type OneCContract struct {
	ID           string    `json:"Ref_Key"`          // Уникальный ключ ссылки
	Code         string    `json:"Code"`             // Код договора
	Description  string    `json:"Description"`      // Наименование
	Organization string    `json:"Organization_Key"` // Организация
	Counterparty string    `json:"Counterparty_Key"` // Контрагент
	Currency     string    `json:"Currency_Key"`     // Валюта
	ContractType string    `json:"ContractType"`     // Тип договора
	StartDate    time.Time `json:"StartDate"`        // Дата начала
	EndDate      time.Time `json:"EndDate"`          // Дата окончания
	Amount       float64   `json:"Amount"`           // Сумма договора
	IsActive     bool      `json:"IsActive"`         // Активность
	Comment      string    `json:"Comment"`          // Комментарий
	ExternalID   string    `json:"ExternalID"`       // Внешний идентификатор
}

// OneCPaymentRegistry реестр платежей для экспорта в 1С
type OneCPaymentRegistry struct {
	RegistryNumber string        `json:"RegistryNumber"` // Номер реестра
	RegistryDate   time.Time     `json:"RegistryDate"`   // Дата реестра
	Organization   string        `json:"Organization"`   // Организация
	BankAccount    string        `json:"BankAccount"`    // Банковский счет
	TotalAmount    float64       `json:"TotalAmount"`    // Общая сумма
	PaymentsCount  int           `json:"PaymentsCount"`  // Количество платежей
	Payments       []OneCPayment `json:"Payments"`       // Список платежей
	Period         OneCPeriod    `json:"Period"`         // Период
	Status         string        `json:"Status"`         // Статус реестра
}

// OneCPeriod период для отчетов
type OneCPeriod struct {
	StartDate time.Time `json:"StartDate"` // Дата начала периода
	EndDate   time.Time `json:"EndDate"`   // Дата окончания периода
}

// OneCResponse стандартный ответ от 1С API
type OneCResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
	Error   struct {
		Code        string `json:"code"`
		Message     string `json:"message"`
		Description string `json:"description"`
	} `json:"error"`
	Metadata struct {
		Total  int `json:"total"`
		Offset int `json:"offset"`
		Limit  int `json:"limit"`
	} `json:"metadata"`
}

// OneCListResponse ответ со списком записей
type OneCListResponse struct {
	Success bool                     `json:"success"`
	Data    []map[string]interface{} `json:"data"`
	Error   struct {
		Code        string `json:"code"`
		Message     string `json:"message"`
		Description string `json:"description"`
	} `json:"error"`
	Metadata struct {
		Total  int `json:"total"`
		Offset int `json:"offset"`
		Limit  int `json:"limit"`
	} `json:"metadata"`
}

// NewOneCClient создает новый клиент для 1С API
func NewOneCClient(logger *log.Logger) *OneCClient {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}

	client := &http.Client{
		Timeout: 60 * time.Second, // 1С может быть медленнее
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false,
			},
			MaxIdleConns:        50,
			MaxIdleConnsPerHost: 50,
			IdleConnTimeout:     120 * time.Second,
		},
	}

	return &OneCClient{
		HTTPClient: client,
		Logger:     logger,
	}
}

// CallMethod выполняет вызов метода 1С API
func (c *OneCClient) CallMethod(ctx context.Context, credentials *OneCCredentials, method string, params map[string]interface{}) (*OneCResponse, error) {
	// Формируем URL для API вызова
	apiVersion := credentials.APIVersion
	if apiVersion == "" {
		apiVersion = "v1"
	}

	apiURL := fmt.Sprintf("%s/%s/hs/api/%s/%s",
		strings.TrimRight(credentials.BaseURL, "/"),
		credentials.Database,
		apiVersion,
		method)

	// Подготавливаем данные запроса
	var requestBody io.Reader
	var contentType string

	if len(params) > 0 {
		jsonData, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("ошибка сериализации параметров: %w", err)
		}
		requestBody = strings.NewReader(string(jsonData))
		contentType = "application/json"
	}

	// Создаем HTTP запрос
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, requestBody)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}

	// Устанавливаем заголовки
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "AxentaCRM/1.0")
	req.Header.Set("Accept", "application/json")

	// Добавляем базовую аутентификацию
	req.SetBasicAuth(credentials.Username, credentials.Password)

	// Выполняем запрос с retry
	config := RetryConfig{
		MaxRetries:      3,
		InitialDelay:    2 * time.Second,
		MaxDelay:        30 * time.Second,
		BackoffFactor:   2.0,
		RetryableErrors: []int{500, 502, 503, 504, 408, 429},
	}

	resp, err := c.CallWithRetry(req, config)
	if err != nil {
		return nil, fmt.Errorf("ошибка выполнения запроса: %w", err)
	}
	defer resp.Body.Close()

	// Читаем ответ
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	// Проверяем HTTP статус
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP ошибка %d: %s", resp.StatusCode, string(body))
	}

	// Парсим JSON ответ
	var oneCResp OneCResponse
	if err := json.Unmarshal(body, &oneCResp); err != nil {
		return nil, fmt.Errorf("ошибка парсинга ответа: %w, тело ответа: %s", err, string(body))
	}

	// Проверяем наличие ошибки в ответе
	if !oneCResp.Success && oneCResp.Error.Code != "" {
		return nil, fmt.Errorf("ошибка 1С API [%s]: %s - %s",
			oneCResp.Error.Code,
			oneCResp.Error.Message,
			oneCResp.Error.Description)
	}

	c.Logger.Printf("Успешный вызов метода 1С: %s", method)
	return &oneCResp, nil
}

// GetCounterparties получает список контрагентов из 1С
func (c *OneCClient) GetCounterparties(ctx context.Context, credentials *OneCCredentials, limit int, offset int) ([]OneCCounterparty, int, error) {
	params := map[string]interface{}{
		"limit":  limit,
		"offset": offset,
		"filter": map[string]interface{}{
			"IsFolder": false, // Исключаем папки
			"IsActive": true,  // Только активные
		},
	}

	resp, err := c.CallMethod(ctx, credentials, "counterparties", params)
	if err != nil {
		return nil, 0, fmt.Errorf("ошибка получения списка контрагентов: %w", err)
	}

	// Парсим список контрагентов
	var counterparties []OneCCounterparty
	if dataList, ok := resp.Data.([]interface{}); ok {
		for _, item := range dataList {
			if itemData, ok := item.(map[string]interface{}); ok {
				counterparty := OneCCounterparty{}

				// Маппинг полей
				if id, ok := itemData["Ref_Key"].(string); ok {
					counterparty.ID = id
				}
				if code, ok := itemData["Code"].(string); ok {
					counterparty.Code = code
				}
				if desc, ok := itemData["Description"].(string); ok {
					counterparty.Description = desc
				}
				if fullName, ok := itemData["FullName"].(string); ok {
					counterparty.FullName = fullName
				}
				if inn, ok := itemData["INN"].(string); ok {
					counterparty.INN = inn
				}
				if kpp, ok := itemData["KPP"].(string); ok {
					counterparty.KPP = kpp
				}
				if ogrn, ok := itemData["OGRN"].(string); ok {
					counterparty.OGRN = ogrn
				}
				if phone, ok := itemData["Phone"].(string); ok {
					counterparty.Phone = phone
				}
				if email, ok := itemData["Email"].(string); ok {
					counterparty.Email = email
				}
				if isActive, ok := itemData["IsActive"].(bool); ok {
					counterparty.IsActive = isActive
				}

				counterparties = append(counterparties, counterparty)
			}
		}
	}

	total := resp.Metadata.Total
	return counterparties, total, nil
}

// CreateCounterparty создает контрагента в 1С
func (c *OneCClient) CreateCounterparty(ctx context.Context, credentials *OneCCredentials, counterparty *OneCCounterparty) (string, error) {
	params := map[string]interface{}{
		"Code":        counterparty.Code,
		"Description": counterparty.Description,
		"FullName":    counterparty.FullName,
		"INN":         counterparty.INN,
		"KPP":         counterparty.KPP,
		"OGRN":        counterparty.OGRN,
		"Phone":       counterparty.Phone,
		"Email":       counterparty.Email,
		"IsActive":    counterparty.IsActive,
		"ExternalID":  counterparty.ID, // Используем наш ID как внешний
	}

	resp, err := c.CallMethod(ctx, credentials, "counterparties/create", params)
	if err != nil {
		return "", fmt.Errorf("ошибка создания контрагента: %w", err)
	}

	// Извлекаем ID созданного контрагента
	if resultData, ok := resp.Data.(map[string]interface{}); ok {
		if refKey, ok := resultData["Ref_Key"].(string); ok {
			c.Logger.Printf("Контрагент успешно создан в 1С: %s", refKey)
			return refKey, nil
		}
	}

	return "", fmt.Errorf("неожиданный формат ответа при создании контрагента")
}

// ExportPaymentRegistry экспортирует реестр платежей в 1С
func (c *OneCClient) ExportPaymentRegistry(ctx context.Context, credentials *OneCCredentials, registry *OneCPaymentRegistry) error {
	params := map[string]interface{}{
		"RegistryNumber": registry.RegistryNumber,
		"RegistryDate":   registry.RegistryDate.Format("2006-01-02T15:04:05"),
		"Organization":   registry.Organization,
		"BankAccount":    registry.BankAccount,
		"TotalAmount":    registry.TotalAmount,
		"PaymentsCount":  registry.PaymentsCount,
		"Payments":       registry.Payments,
		"Period": map[string]interface{}{
			"StartDate": registry.Period.StartDate.Format("2006-01-02T15:04:05"),
			"EndDate":   registry.Period.EndDate.Format("2006-01-02T15:04:05"),
		},
	}

	_, err := c.CallMethod(ctx, credentials, "payment-registry/import", params)
	if err != nil {
		return fmt.Errorf("ошибка экспорта реестра платежей: %w", err)
	}

	c.Logger.Printf("Реестр платежей успешно экспортирован в 1С: %s", registry.RegistryNumber)
	return nil
}

// UpdatePaymentStatus обновляет статус платежа в 1С
func (c *OneCClient) UpdatePaymentStatus(ctx context.Context, credentials *OneCCredentials, paymentID string, status string) error {
	params := map[string]interface{}{
		"PaymentID": paymentID,
		"Status":    status,
		"UpdatedAt": time.Now().Format("2006-01-02T15:04:05"),
	}

	_, err := c.CallMethod(ctx, credentials, "payments/update-status", params)
	if err != nil {
		return fmt.Errorf("ошибка обновления статуса платежа: %w", err)
	}

	c.Logger.Printf("Статус платежа обновлен в 1С: %s -> %s", paymentID, status)
	return nil
}

// GetPaymentStatus получает статус платежа из 1С
func (c *OneCClient) GetPaymentStatus(ctx context.Context, credentials *OneCCredentials, externalID string) (*OneCPayment, error) {
	params := map[string]interface{}{
		"ExternalID": externalID,
	}

	resp, err := c.CallMethod(ctx, credentials, "payments/get-by-external-id", params)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения статуса платежа: %w", err)
	}

	// Парсим данные платежа
	if resultData, ok := resp.Data.(map[string]interface{}); ok {
		payment := &OneCPayment{}

		if id, ok := resultData["Ref_Key"].(string); ok {
			payment.ID = id
		}
		if number, ok := resultData["Number"].(string); ok {
			payment.Number = number
		}
		if posted, ok := resultData["Posted"].(bool); ok {
			payment.Posted = posted
		}
		if amount, ok := resultData["Amount"].(float64); ok {
			payment.Amount = amount
		}
		if purpose, ok := resultData["Purpose"].(string); ok {
			payment.Purpose = purpose
		}
		if externalID, ok := resultData["ExternalID"].(string); ok {
			payment.ExternalID = externalID
		}

		// Парсим дату
		if dateStr, ok := resultData["Date"].(string); ok {
			if parsed, err := time.Parse("2006-01-02T15:04:05", dateStr); err == nil {
				payment.Date = parsed
			}
		}

		return payment, nil
	}

	return nil, fmt.Errorf("платеж не найден")
}

// IsHealthy проверяет доступность 1С API
func (c *OneCClient) IsHealthy(ctx context.Context, credentials *OneCCredentials) error {
	// Проверяем доступность API простым запросом информации о базе
	_, err := c.CallMethod(ctx, credentials, "info", nil)
	if err != nil {
		return fmt.Errorf("API 1С недоступно: %w", err)
	}

	return nil
}

// CallWithRetry выполняет HTTP запрос с retry механизмом
func (c *OneCClient) CallWithRetry(req *http.Request, config RetryConfig) (*http.Response, error) {
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

		c.Logger.Printf("Повтор запроса 1С %s через %v (попытка %d/%d), причина: %v",
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
func (c *OneCClient) shouldRetry(statusCode int, retryableErrors []int) bool {
	for _, code := range retryableErrors {
		if statusCode == code {
			return true
		}
	}
	return false
}

// calculateDelay вычисляет задержку для retry с экспоненциальным backoff
func (c *OneCClient) calculateDelay(attempt int, config RetryConfig) time.Duration {
	delay := float64(config.InitialDelay) * math.Pow(config.BackoffFactor, float64(attempt))

	if delay > float64(config.MaxDelay) {
		delay = float64(config.MaxDelay)
	}

	return time.Duration(delay)
}
