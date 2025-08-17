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
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Bitrix24Client клиент для работы с Битрикс24 API
type Bitrix24Client struct {
	BaseURL    string
	HTTPClient *http.Client
	Logger     *log.Logger
}

// Bitrix24Credentials учетные данные для Битрикс24 API
type Bitrix24Credentials struct {
	WebhookURL   string // URL вебхука Битрикс24
	ClientID     string // ID приложения (для OAuth)
	ClientSecret string // Секрет приложения (для OAuth)
	AccessToken  string // Токен доступа
	RefreshToken string // Токен обновления
	ExpiresAt    time.Time
}

// Bitrix24Contact контакт в Битрикс24
type Bitrix24Contact struct {
	ID       string `json:"ID"`
	Name     string `json:"NAME"`
	LastName string `json:"LAST_NAME"`
	Email    string `json:"EMAIL"`
	Phone    string `json:"PHONE"`
	Company  string `json:"COMPANY_TITLE"`
	Comments string `json:"COMMENTS"`
}

// Bitrix24Deal сделка в Битрикс24
type Bitrix24Deal struct {
	ID           string    `json:"ID"`
	Title        string    `json:"TITLE"`
	StageID      string    `json:"STAGE_ID"`
	Opportunity  float64   `json:"OPPORTUNITY"`
	CurrencyID   string    `json:"CURRENCY_ID"`
	ContactID    string    `json:"CONTACT_ID"`
	CompanyID    string    `json:"COMPANY_ID"`
	AssignedByID string    `json:"ASSIGNED_BY_ID"`
	DateCreate   time.Time `json:"DATE_CREATE"`
	DateModify   time.Time `json:"DATE_MODIFY"`
	BeginDate    time.Time `json:"BEGINDATE"`
	CloseDate    time.Time `json:"CLOSEDATE"`
	Comments     string    `json:"COMMENTS"`
}

// Bitrix24Company компания в Битрикс24
type Bitrix24Company struct {
	ID       string `json:"ID"`
	Title    string `json:"TITLE"`
	Email    string `json:"EMAIL_WORK"`
	Phone    string `json:"PHONE_WORK"`
	Address  string `json:"ADDRESS"`
	Comments string `json:"COMMENTS"`
}

// Bitrix24Response стандартный ответ от Битрикс24 API
type Bitrix24Response struct {
	Result interface{} `json:"result"`
	Error  struct {
		Code        string `json:"error"`
		Description string `json:"error_description"`
	} `json:"error"`
	Total int `json:"total"`
	Next  int `json:"next"`
}

// Bitrix24ListResponse ответ со списком записей
type Bitrix24ListResponse struct {
	Result []map[string]interface{} `json:"result"`
	Error  struct {
		Code        string `json:"error"`
		Description string `json:"error_description"`
	} `json:"error"`
	Total int `json:"total"`
	Next  int `json:"next"`
}

// NewBitrix24Client создает новый клиент для Битрикс24 API
func NewBitrix24Client(logger *log.Logger) *Bitrix24Client {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false,
			},
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	return &Bitrix24Client{
		HTTPClient: client,
		Logger:     logger,
	}
}

// CallMethod выполняет вызов метода Битрикс24 API
func (c *Bitrix24Client) CallMethod(ctx context.Context, credentials *Bitrix24Credentials, method string, params map[string]interface{}) (*Bitrix24Response, error) {
	// Формируем URL для API вызова
	var apiURL string
	if credentials.WebhookURL != "" {
		// Используем вебхук
		apiURL = credentials.WebhookURL + method
	} else {
		return nil, fmt.Errorf("не настроен WebhookURL для Битрикс24")
	}

	// Подготавливаем параметры
	values := url.Values{}
	for key, value := range params {
		switch v := value.(type) {
		case string:
			values.Set(key, v)
		case int, int64, float64:
			values.Set(key, fmt.Sprintf("%v", v))
		case map[string]interface{}:
			// Для сложных структур сериализуем в JSON
			jsonData, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("ошибка сериализации параметра %s: %w", key, err)
			}
			values.Set(key, string(jsonData))
		}
	}

	// Выполняем POST запрос
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "AxentaCRM/1.0")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка выполнения запроса: %w", err)
	}
	defer resp.Body.Close()

	// Читаем ответ
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	// Парсим JSON ответ
	var bitrixResp Bitrix24Response
	if err := json.Unmarshal(body, &bitrixResp); err != nil {
		return nil, fmt.Errorf("ошибка парсинга ответа: %w, тело ответа: %s", err, string(body))
	}

	// Проверяем наличие ошибки в ответе
	if bitrixResp.Error.Code != "" {
		return nil, fmt.Errorf("ошибка Битрикс24 API [%s]: %s", bitrixResp.Error.Code, bitrixResp.Error.Description)
	}

	c.Logger.Printf("Успешный вызов метода %s", method)
	return &bitrixResp, nil
}

// CreateContact создает контакт в Битрикс24
func (c *Bitrix24Client) CreateContact(ctx context.Context, credentials *Bitrix24Credentials, contact *Bitrix24Contact) (string, error) {
	fields := map[string]interface{}{
		"NAME":      contact.Name,
		"LAST_NAME": contact.LastName,
		"COMMENTS":  contact.Comments,
	}

	// Добавляем email если указан
	if contact.Email != "" {
		fields["EMAIL"] = []map[string]string{
			{
				"VALUE":      contact.Email,
				"VALUE_TYPE": "WORK",
			},
		}
	}

	// Добавляем телефон если указан
	if contact.Phone != "" {
		fields["PHONE"] = []map[string]string{
			{
				"VALUE":      contact.Phone,
				"VALUE_TYPE": "WORK",
			},
		}
	}

	params := map[string]interface{}{
		"fields": fields,
	}

	resp, err := c.CallMethod(ctx, credentials, "crm.contact.add", params)
	if err != nil {
		return "", fmt.Errorf("ошибка создания контакта: %w", err)
	}

	// Извлекаем ID созданного контакта
	contactID, ok := resp.Result.(float64)
	if !ok {
		return "", fmt.Errorf("неожиданный формат ответа при создании контакта")
	}

	contactIDStr := strconv.Itoa(int(contactID))
	c.Logger.Printf("Контакт успешно создан в Битрикс24: %s", contactIDStr)
	return contactIDStr, nil
}

// UpdateContact обновляет контакт в Битрикс24
func (c *Bitrix24Client) UpdateContact(ctx context.Context, credentials *Bitrix24Credentials, contactID string, contact *Bitrix24Contact) error {
	fields := map[string]interface{}{
		"NAME":      contact.Name,
		"LAST_NAME": contact.LastName,
		"COMMENTS":  contact.Comments,
	}

	// Добавляем email если указан
	if contact.Email != "" {
		fields["EMAIL"] = []map[string]string{
			{
				"VALUE":      contact.Email,
				"VALUE_TYPE": "WORK",
			},
		}
	}

	// Добавляем телефон если указан
	if contact.Phone != "" {
		fields["PHONE"] = []map[string]string{
			{
				"VALUE":      contact.Phone,
				"VALUE_TYPE": "WORK",
			},
		}
	}

	params := map[string]interface{}{
		"id":     contactID,
		"fields": fields,
	}

	_, err := c.CallMethod(ctx, credentials, "crm.contact.update", params)
	if err != nil {
		return fmt.Errorf("ошибка обновления контакта: %w", err)
	}

	c.Logger.Printf("Контакт успешно обновлен в Битрикс24: %s", contactID)
	return nil
}

// GetContact получает контакт из Битрикс24
func (c *Bitrix24Client) GetContact(ctx context.Context, credentials *Bitrix24Credentials, contactID string) (*Bitrix24Contact, error) {
	params := map[string]interface{}{
		"id": contactID,
	}

	resp, err := c.CallMethod(ctx, credentials, "crm.contact.get", params)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения контакта: %w", err)
	}

	// Парсим данные контакта
	resultData, ok := resp.Result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("неожиданный формат ответа при получении контакта")
	}

	contact := &Bitrix24Contact{
		ID: contactID,
	}

	if name, ok := resultData["NAME"].(string); ok {
		contact.Name = name
	}
	if lastName, ok := resultData["LAST_NAME"].(string); ok {
		contact.LastName = lastName
	}
	if comments, ok := resultData["COMMENTS"].(string); ok {
		contact.Comments = comments
	}

	// Извлекаем email и телефон из массивов
	if emails, ok := resultData["EMAIL"].([]interface{}); ok && len(emails) > 0 {
		if emailData, ok := emails[0].(map[string]interface{}); ok {
			if email, ok := emailData["VALUE"].(string); ok {
				contact.Email = email
			}
		}
	}

	if phones, ok := resultData["PHONE"].([]interface{}); ok && len(phones) > 0 {
		if phoneData, ok := phones[0].(map[string]interface{}); ok {
			if phone, ok := phoneData["VALUE"].(string); ok {
				contact.Phone = phone
			}
		}
	}

	return contact, nil
}

// CreateDeal создает сделку в Битрикс24
func (c *Bitrix24Client) CreateDeal(ctx context.Context, credentials *Bitrix24Credentials, deal *Bitrix24Deal) (string, error) {
	fields := map[string]interface{}{
		"TITLE":    deal.Title,
		"COMMENTS": deal.Comments,
	}

	// Добавляем контакт если указан
	if deal.ContactID != "" {
		fields["CONTACT_ID"] = deal.ContactID
	}

	// Добавляем компанию если указана
	if deal.CompanyID != "" {
		fields["COMPANY_ID"] = deal.CompanyID
	}

	// Добавляем сумму если указана
	if deal.Opportunity > 0 {
		fields["OPPORTUNITY"] = deal.Opportunity
	}

	// Добавляем валюту если указана
	if deal.CurrencyID != "" {
		fields["CURRENCY_ID"] = deal.CurrencyID
	} else {
		fields["CURRENCY_ID"] = "RUB" // По умолчанию рубли
	}

	// Добавляем даты если указаны
	if !deal.BeginDate.IsZero() {
		fields["BEGINDATE"] = deal.BeginDate.Format("2006-01-02T15:04:05+07:00")
	}
	if !deal.CloseDate.IsZero() {
		fields["CLOSEDATE"] = deal.CloseDate.Format("2006-01-02T15:04:05+07:00")
	}

	params := map[string]interface{}{
		"fields": fields,
	}

	resp, err := c.CallMethod(ctx, credentials, "crm.deal.add", params)
	if err != nil {
		return "", fmt.Errorf("ошибка создания сделки: %w", err)
	}

	// Извлекаем ID созданной сделки
	dealID, ok := resp.Result.(float64)
	if !ok {
		return "", fmt.Errorf("неожиданный формат ответа при создании сделки")
	}

	dealIDStr := strconv.Itoa(int(dealID))
	c.Logger.Printf("Сделка успешно создана в Битрикс24: %s", dealIDStr)
	return dealIDStr, nil
}

// UpdateDeal обновляет сделку в Битрикс24
func (c *Bitrix24Client) UpdateDeal(ctx context.Context, credentials *Bitrix24Credentials, dealID string, deal *Bitrix24Deal) error {
	fields := map[string]interface{}{
		"TITLE":    deal.Title,
		"COMMENTS": deal.Comments,
	}

	// Добавляем сумму если указана
	if deal.Opportunity > 0 {
		fields["OPPORTUNITY"] = deal.Opportunity
	}

	// Добавляем этап если указан
	if deal.StageID != "" {
		fields["STAGE_ID"] = deal.StageID
	}

	// Добавляем даты если указаны
	if !deal.BeginDate.IsZero() {
		fields["BEGINDATE"] = deal.BeginDate.Format("2006-01-02T15:04:05+07:00")
	}
	if !deal.CloseDate.IsZero() {
		fields["CLOSEDATE"] = deal.CloseDate.Format("2006-01-02T15:04:05+07:00")
	}

	params := map[string]interface{}{
		"id":     dealID,
		"fields": fields,
	}

	_, err := c.CallMethod(ctx, credentials, "crm.deal.update", params)
	if err != nil {
		return fmt.Errorf("ошибка обновления сделки: %w", err)
	}

	c.Logger.Printf("Сделка успешно обновлена в Битрикс24: %s", dealID)
	return nil
}

// GetDeal получает сделку из Битрикс24
func (c *Bitrix24Client) GetDeal(ctx context.Context, credentials *Bitrix24Credentials, dealID string) (*Bitrix24Deal, error) {
	params := map[string]interface{}{
		"id": dealID,
	}

	resp, err := c.CallMethod(ctx, credentials, "crm.deal.get", params)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения сделки: %w", err)
	}

	// Парсим данные сделки
	resultData, ok := resp.Result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("неожиданный формат ответа при получении сделки")
	}

	deal := &Bitrix24Deal{
		ID: dealID,
	}

	if title, ok := resultData["TITLE"].(string); ok {
		deal.Title = title
	}
	if stageID, ok := resultData["STAGE_ID"].(string); ok {
		deal.StageID = stageID
	}
	if opportunity, ok := resultData["OPPORTUNITY"].(string); ok {
		if opp, err := strconv.ParseFloat(opportunity, 64); err == nil {
			deal.Opportunity = opp
		}
	}
	if currencyID, ok := resultData["CURRENCY_ID"].(string); ok {
		deal.CurrencyID = currencyID
	}
	if contactID, ok := resultData["CONTACT_ID"].(string); ok {
		deal.ContactID = contactID
	}
	if companyID, ok := resultData["COMPANY_ID"].(string); ok {
		deal.CompanyID = companyID
	}
	if comments, ok := resultData["COMMENTS"].(string); ok {
		deal.Comments = comments
	}

	// Парсим даты
	if beginDate, ok := resultData["BEGINDATE"].(string); ok && beginDate != "" {
		if parsed, err := time.Parse("2006-01-02T15:04:05+07:00", beginDate); err == nil {
			deal.BeginDate = parsed
		}
	}
	if closeDate, ok := resultData["CLOSEDATE"].(string); ok && closeDate != "" {
		if parsed, err := time.Parse("2006-01-02T15:04:05+07:00", closeDate); err == nil {
			deal.CloseDate = parsed
		}
	}

	return deal, nil
}

// GetContacts получает список контактов из Битрикс24
func (c *Bitrix24Client) GetContacts(ctx context.Context, credentials *Bitrix24Credentials, limit int, start int) ([]Bitrix24Contact, int, error) {
	params := map[string]interface{}{
		"select": []string{"ID", "NAME", "LAST_NAME", "EMAIL", "PHONE", "COMMENTS"},
	}

	if limit > 0 {
		params["start"] = start
		// Битрикс24 имеет ограничение на количество записей за один запрос
		if limit > 50 {
			limit = 50
		}
	}

	resp, err := c.CallMethod(ctx, credentials, "crm.contact.list", params)
	if err != nil {
		return nil, 0, fmt.Errorf("ошибка получения списка контактов: %w", err)
	}

	// Парсим список контактов
	resultData, ok := resp.Result.([]interface{})
	if !ok {
		return nil, 0, fmt.Errorf("неожиданный формат ответа при получении списка контактов")
	}

	var contacts []Bitrix24Contact
	for _, item := range resultData {
		contactData, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		contact := Bitrix24Contact{}
		if id, ok := contactData["ID"].(string); ok {
			contact.ID = id
		}
		if name, ok := contactData["NAME"].(string); ok {
			contact.Name = name
		}
		if lastName, ok := contactData["LAST_NAME"].(string); ok {
			contact.LastName = lastName
		}
		if comments, ok := contactData["COMMENTS"].(string); ok {
			contact.Comments = comments
		}

		// Извлекаем email и телефон из массивов
		if emails, ok := contactData["EMAIL"].([]interface{}); ok && len(emails) > 0 {
			if emailData, ok := emails[0].(map[string]interface{}); ok {
				if email, ok := emailData["VALUE"].(string); ok {
					contact.Email = email
				}
			}
		}

		if phones, ok := contactData["PHONE"].([]interface{}); ok && len(phones) > 0 {
			if phoneData, ok := phones[0].(map[string]interface{}); ok {
				if phone, ok := phoneData["VALUE"].(string); ok {
					contact.Phone = phone
				}
			}
		}

		contacts = append(contacts, contact)
	}

	return contacts, resp.Total, nil
}

// GetDeals получает список сделок из Битрикс24
func (c *Bitrix24Client) GetDeals(ctx context.Context, credentials *Bitrix24Credentials, limit int, start int) ([]Bitrix24Deal, int, error) {
	params := map[string]interface{}{
		"select": []string{"ID", "TITLE", "STAGE_ID", "OPPORTUNITY", "CURRENCY_ID", "CONTACT_ID", "COMPANY_ID", "BEGINDATE", "CLOSEDATE", "COMMENTS"},
	}

	if limit > 0 {
		params["start"] = start
		// Битрикс24 имеет ограничение на количество записей за один запрос
		if limit > 50 {
			limit = 50
		}
	}

	resp, err := c.CallMethod(ctx, credentials, "crm.deal.list", params)
	if err != nil {
		return nil, 0, fmt.Errorf("ошибка получения списка сделок: %w", err)
	}

	// Парсим список сделок
	resultData, ok := resp.Result.([]interface{})
	if !ok {
		return nil, 0, fmt.Errorf("неожиданный формат ответа при получении списка сделок")
	}

	var deals []Bitrix24Deal
	for _, item := range resultData {
		dealData, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		deal := Bitrix24Deal{}
		if id, ok := dealData["ID"].(string); ok {
			deal.ID = id
		}
		if title, ok := dealData["TITLE"].(string); ok {
			deal.Title = title
		}
		if stageID, ok := dealData["STAGE_ID"].(string); ok {
			deal.StageID = stageID
		}
		if opportunity, ok := dealData["OPPORTUNITY"].(string); ok {
			if opp, err := strconv.ParseFloat(opportunity, 64); err == nil {
				deal.Opportunity = opp
			}
		}
		if currencyID, ok := dealData["CURRENCY_ID"].(string); ok {
			deal.CurrencyID = currencyID
		}
		if contactID, ok := dealData["CONTACT_ID"].(string); ok {
			deal.ContactID = contactID
		}
		if companyID, ok := dealData["COMPANY_ID"].(string); ok {
			deal.CompanyID = companyID
		}
		if comments, ok := dealData["COMMENTS"].(string); ok {
			deal.Comments = comments
		}

		// Парсим даты
		if beginDate, ok := dealData["BEGINDATE"].(string); ok && beginDate != "" {
			if parsed, err := time.Parse("2006-01-02T15:04:05+07:00", beginDate); err == nil {
				deal.BeginDate = parsed
			}
		}
		if closeDate, ok := dealData["CLOSEDATE"].(string); ok && closeDate != "" {
			if parsed, err := time.Parse("2006-01-02T15:04:05+07:00", closeDate); err == nil {
				deal.CloseDate = parsed
			}
		}

		deals = append(deals, deal)
	}

	return deals, resp.Total, nil
}

// IsHealthy проверяет доступность Битрикс24 API
func (c *Bitrix24Client) IsHealthy(ctx context.Context, credentials *Bitrix24Credentials) error {
	// Проверяем доступность API простым запросом профиля
	_, err := c.CallMethod(ctx, credentials, "profile", nil)
	if err != nil {
		return fmt.Errorf("API Битрикс24 недоступно: %w", err)
	}

	return nil
}

// CallWithRetry выполняет HTTP запрос с retry механизмом (аналогично AxetnaClient)
func (c *Bitrix24Client) CallWithRetry(req *http.Request, config RetryConfig) (*http.Response, error) {
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
func (c *Bitrix24Client) shouldRetry(statusCode int, retryableErrors []int) bool {
	for _, code := range retryableErrors {
		if statusCode == code {
			return true
		}
	}
	return false
}

// calculateDelay вычисляет задержку для retry с экспоненциальным backoff
func (c *Bitrix24Client) calculateDelay(attempt int, config RetryConfig) time.Duration {
	delay := float64(config.InitialDelay) * math.Pow(config.BackoffFactor, float64(attempt))

	if delay > float64(config.MaxDelay) {
		delay = float64(config.MaxDelay)
	}

	return time.Duration(delay)
}
