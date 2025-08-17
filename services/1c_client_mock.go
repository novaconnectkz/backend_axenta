package services

import (
	"context"
	"fmt"
	"log"
	"time"
)

// OneCClientMock мок-клиент для тестирования интеграции с 1С
type OneCClientMock struct {
	Logger                  *log.Logger
	ShouldFail              bool
	FailureMessage          string
	Counterparties          []OneCCounterparty
	Payments                map[string]*OneCPayment
	PaymentRegistries       []OneCPaymentRegistry
	ConnectionHealthy       bool
	CallMethodCalls         []MockCallMethodCall
	CreateCounterpartyCalls []OneCCounterparty
	ExportRegistryCalls     []OneCPaymentRegistry
}

// MockCallMethodCall запись о вызове CallMethod
type MockCallMethodCall struct {
	Method string
	Params map[string]interface{}
	Time   time.Time
}

// NewOneCClientMock создает новый мок-клиент для 1С
func NewOneCClientMock(logger *log.Logger) *OneCClientMock {
	return &OneCClientMock{
		Logger:                  logger,
		ShouldFail:              false,
		Counterparties:          []OneCCounterparty{},
		Payments:                make(map[string]*OneCPayment),
		PaymentRegistries:       []OneCPaymentRegistry{},
		ConnectionHealthy:       true,
		CallMethodCalls:         []MockCallMethodCall{},
		CreateCounterpartyCalls: []OneCCounterparty{},
		ExportRegistryCalls:     []OneCPaymentRegistry{},
	}
}

// SetupMockData настраивает тестовые данные
func (m *OneCClientMock) SetupMockData() {
	// Добавляем тестовых контрагентов
	m.Counterparties = []OneCCounterparty{
		{
			ID:          "counterparty-1",
			Code:        "CP001",
			Description: "ООО Тестовая компания 1",
			FullName:    "Общество с ограниченной ответственностью \"Тестовая компания 1\"",
			INN:         "1234567890",
			KPP:         "123456789",
			OGRN:        "1234567890123",
			Phone:       "+7 (495) 123-45-67",
			Email:       "info@testcompany1.ru",
			IsActive:    true,
		},
		{
			ID:          "counterparty-2",
			Code:        "CP002",
			Description: "ИП Иванов И.И.",
			FullName:    "Индивидуальный предприниматель Иванов Иван Иванович",
			INN:         "123456789012",
			Phone:       "+7 (495) 234-56-78",
			Email:       "ivanov@example.com",
			IsActive:    true,
		},
		{
			ID:          "counterparty-3",
			Code:        "CP003",
			Description: "ООО Неактивная компания",
			FullName:    "Общество с ограниченной ответственностью \"Неактивная компания\"",
			INN:         "9876543210",
			IsActive:    false,
		},
	}

	// Добавляем тестовые платежи
	m.Payments["invoice_1"] = &OneCPayment{
		ID:            "payment-1",
		Number:        "PAY-001",
		Date:          time.Now().AddDate(0, 0, -1),
		Posted:        true,
		Amount:        10000.00,
		Purpose:       "Оплата по счету INV-001",
		PaymentMethod: "bank_transfer",
		OperationType: "income",
		Currency:      "RUB",
		ExternalID:    "invoice_1",
	}

	m.Payments["invoice_2"] = &OneCPayment{
		ID:            "payment-2",
		Number:        "PAY-002",
		Date:          time.Now().AddDate(0, 0, -2),
		Posted:        false, // Не проведен
		Amount:        5000.00,
		Purpose:       "Оплата по счету INV-002",
		PaymentMethod: "bank_transfer",
		OperationType: "income",
		Currency:      "RUB",
		ExternalID:    "invoice_2",
	}
}

// SetFailure настраивает мок на возврат ошибки
func (m *OneCClientMock) SetFailure(shouldFail bool, message string) {
	m.ShouldFail = shouldFail
	m.FailureMessage = message
}

// SetConnectionHealth устанавливает состояние подключения
func (m *OneCClientMock) SetConnectionHealth(healthy bool) {
	m.ConnectionHealthy = healthy
}

// CallMethod выполняет мок вызова метода 1С API
func (m *OneCClientMock) CallMethod(ctx context.Context, credentials *OneCCredentials, method string, params map[string]interface{}) (*OneCResponse, error) {
	// Записываем вызов для анализа в тестах
	m.CallMethodCalls = append(m.CallMethodCalls, MockCallMethodCall{
		Method: method,
		Params: params,
		Time:   time.Now(),
	})

	if m.ShouldFail {
		return nil, fmt.Errorf("мок ошибка: %s", m.FailureMessage)
	}

	// Имитируем разные методы
	switch method {
	case "info":
		return &OneCResponse{
			Success: true,
			Data: map[string]interface{}{
				"database": credentials.Database,
				"version":  "8.3.15",
				"status":   "active",
			},
		}, nil

	case "counterparties":
		return m.handleGetCounterparties(params)

	case "counterparties/create":
		return m.handleCreateCounterparty(params)

	case "payment-registry/import":
		return m.handleExportPaymentRegistry(params)

	case "payments/get-by-external-id":
		return m.handleGetPaymentStatus(params)

	case "payments/update-status":
		return m.handleUpdatePaymentStatus(params)

	default:
		return &OneCResponse{
			Success: false,
			Error: struct {
				Code        string `json:"code"`
				Message     string `json:"message"`
				Description string `json:"description"`
			}{
				Code:        "METHOD_NOT_FOUND",
				Message:     "Метод не найден",
				Description: fmt.Sprintf("Метод %s не реализован в моке", method),
			},
		}, nil
	}
}

// handleGetCounterparties обрабатывает получение списка контрагентов
func (m *OneCClientMock) handleGetCounterparties(params map[string]interface{}) (*OneCResponse, error) {
	limit := 100
	offset := 0

	if l, ok := params["limit"].(int); ok {
		limit = l
	}
	if o, ok := params["offset"].(int); ok {
		offset = o
	}

	// Фильтруем активные контрагенты
	var activeCounterparties []OneCCounterparty
	for _, cp := range m.Counterparties {
		if cp.IsActive {
			activeCounterparties = append(activeCounterparties, cp)
		}
	}

	// Применяем пагинацию
	total := len(activeCounterparties)
	end := offset + limit
	if end > total {
		end = total
	}

	var result []interface{}
	if offset < total {
		for _, cp := range activeCounterparties[offset:end] {
			result = append(result, map[string]interface{}{
				"Ref_Key":     cp.ID,
				"Code":        cp.Code,
				"Description": cp.Description,
				"FullName":    cp.FullName,
				"INN":         cp.INN,
				"KPP":         cp.KPP,
				"OGRN":        cp.OGRN,
				"Phone":       cp.Phone,
				"Email":       cp.Email,
				"IsActive":    cp.IsActive,
			})
		}
	}

	return &OneCResponse{
		Success: true,
		Data:    result,
		Metadata: struct {
			Total  int `json:"total"`
			Offset int `json:"offset"`
			Limit  int `json:"limit"`
		}{
			Total:  total,
			Offset: offset,
			Limit:  limit,
		},
	}, nil
}

// handleCreateCounterparty обрабатывает создание контрагента
func (m *OneCClientMock) handleCreateCounterparty(params map[string]interface{}) (*OneCResponse, error) {
	// Создаем нового контрагента
	newID := fmt.Sprintf("counterparty-%d", len(m.Counterparties)+1)

	counterparty := OneCCounterparty{
		ID:       newID,
		IsActive: true,
	}

	if code, ok := params["Code"].(string); ok {
		counterparty.Code = code
	}
	if desc, ok := params["Description"].(string); ok {
		counterparty.Description = desc
	}
	if fullName, ok := params["FullName"].(string); ok {
		counterparty.FullName = fullName
	}
	if inn, ok := params["INN"].(string); ok {
		counterparty.INN = inn
	}
	if kpp, ok := params["KPP"].(string); ok {
		counterparty.KPP = kpp
	}
	if phone, ok := params["Phone"].(string); ok {
		counterparty.Phone = phone
	}
	if email, ok := params["Email"].(string); ok {
		counterparty.Email = email
	}

	// Добавляем в список
	m.Counterparties = append(m.Counterparties, counterparty)
	m.CreateCounterpartyCalls = append(m.CreateCounterpartyCalls, counterparty)

	return &OneCResponse{
		Success: true,
		Data: map[string]interface{}{
			"Ref_Key": newID,
		},
	}, nil
}

// handleExportPaymentRegistry обрабатывает экспорт реестра платежей
func (m *OneCClientMock) handleExportPaymentRegistry(params map[string]interface{}) (*OneCResponse, error) {
	registry := OneCPaymentRegistry{}

	if registryNumber, ok := params["RegistryNumber"].(string); ok {
		registry.RegistryNumber = registryNumber
	}
	if totalAmount, ok := params["TotalAmount"].(float64); ok {
		registry.TotalAmount = totalAmount
	}
	if paymentsCount, ok := params["PaymentsCount"].(int); ok {
		registry.PaymentsCount = paymentsCount
	}

	// Добавляем в список экспортированных реестров
	m.PaymentRegistries = append(m.PaymentRegistries, registry)
	m.ExportRegistryCalls = append(m.ExportRegistryCalls, registry)

	return &OneCResponse{
		Success: true,
		Data: map[string]interface{}{
			"registry_id": fmt.Sprintf("registry-%d", len(m.PaymentRegistries)),
			"status":      "imported",
		},
	}, nil
}

// handleGetPaymentStatus обрабатывает получение статуса платежа
func (m *OneCClientMock) handleGetPaymentStatus(params map[string]interface{}) (*OneCResponse, error) {
	externalID, ok := params["ExternalID"].(string)
	if !ok {
		return &OneCResponse{
			Success: false,
			Error: struct {
				Code        string `json:"code"`
				Message     string `json:"message"`
				Description string `json:"description"`
			}{
				Code:    "MISSING_EXTERNAL_ID",
				Message: "Не указан внешний ID",
			},
		}, nil
	}

	payment, exists := m.Payments[externalID]
	if !exists {
		return &OneCResponse{
			Success: false,
			Error: struct {
				Code        string `json:"code"`
				Message     string `json:"message"`
				Description string `json:"description"`
			}{
				Code:    "PAYMENT_NOT_FOUND",
				Message: "Платеж не найден",
			},
		}, nil
	}

	return &OneCResponse{
		Success: true,
		Data: map[string]interface{}{
			"Ref_Key":    payment.ID,
			"Number":     payment.Number,
			"Date":       payment.Date.Format("2006-01-02T15:04:05"),
			"Posted":     payment.Posted,
			"Amount":     payment.Amount,
			"Purpose":    payment.Purpose,
			"ExternalID": payment.ExternalID,
		},
	}, nil
}

// handleUpdatePaymentStatus обрабатывает обновление статуса платежа
func (m *OneCClientMock) handleUpdatePaymentStatus(params map[string]interface{}) (*OneCResponse, error) {
	paymentID, ok := params["PaymentID"].(string)
	if !ok {
		return &OneCResponse{
			Success: false,
			Error: struct {
				Code        string `json:"code"`
				Message     string `json:"message"`
				Description string `json:"description"`
			}{
				Code:    "MISSING_PAYMENT_ID",
				Message: "Не указан ID платежа",
			},
		}, nil
	}

	// Ищем платеж по ID
	for _, payment := range m.Payments {
		if payment.ID == paymentID {
			if status, ok := params["Status"].(string); ok {
				payment.Posted = (status == "posted")
			}
			break
		}
	}

	return &OneCResponse{
		Success: true,
		Data: map[string]interface{}{
			"updated": true,
		},
	}, nil
}

// GetCounterparties получает список контрагентов (мок)
func (m *OneCClientMock) GetCounterparties(ctx context.Context, credentials *OneCCredentials, limit int, offset int) ([]OneCCounterparty, int, error) {
	if m.ShouldFail {
		return nil, 0, fmt.Errorf("мок ошибка: %s", m.FailureMessage)
	}

	// Фильтруем активные контрагенты
	var activeCounterparties []OneCCounterparty
	for _, cp := range m.Counterparties {
		if cp.IsActive {
			activeCounterparties = append(activeCounterparties, cp)
		}
	}

	total := len(activeCounterparties)
	end := offset + limit
	if end > total {
		end = total
	}

	if offset >= total {
		return []OneCCounterparty{}, total, nil
	}

	return activeCounterparties[offset:end], total, nil
}

// CreateCounterparty создает контрагента (мок)
func (m *OneCClientMock) CreateCounterparty(ctx context.Context, credentials *OneCCredentials, counterparty *OneCCounterparty) (string, error) {
	if m.ShouldFail {
		return "", fmt.Errorf("мок ошибка: %s", m.FailureMessage)
	}

	newID := fmt.Sprintf("counterparty-%d", len(m.Counterparties)+1)
	newCounterparty := *counterparty
	newCounterparty.ID = newID
	newCounterparty.IsActive = true

	m.Counterparties = append(m.Counterparties, newCounterparty)
	m.CreateCounterpartyCalls = append(m.CreateCounterpartyCalls, newCounterparty)

	if m.Logger != nil {
		m.Logger.Printf("Мок: контрагент создан с ID %s", newID)
	}

	return newID, nil
}

// ExportPaymentRegistry экспортирует реестр платежей (мок)
func (m *OneCClientMock) ExportPaymentRegistry(ctx context.Context, credentials *OneCCredentials, registry *OneCPaymentRegistry) error {
	if m.ShouldFail {
		return fmt.Errorf("мок ошибка: %s", m.FailureMessage)
	}

	m.PaymentRegistries = append(m.PaymentRegistries, *registry)
	m.ExportRegistryCalls = append(m.ExportRegistryCalls, *registry)

	if m.Logger != nil {
		m.Logger.Printf("Мок: реестр платежей экспортирован: %s", registry.RegistryNumber)
	}

	return nil
}

// UpdatePaymentStatus обновляет статус платежа (мок)
func (m *OneCClientMock) UpdatePaymentStatus(ctx context.Context, credentials *OneCCredentials, paymentID string, status string) error {
	if m.ShouldFail {
		return fmt.Errorf("мок ошибка: %s", m.FailureMessage)
	}

	// Ищем платеж по ID
	for _, payment := range m.Payments {
		if payment.ID == paymentID {
			payment.Posted = (status == "posted")
			if m.Logger != nil {
				m.Logger.Printf("Мок: статус платежа %s обновлен на %s", paymentID, status)
			}
			return nil
		}
	}

	return fmt.Errorf("платеж с ID %s не найден", paymentID)
}

// GetPaymentStatus получает статус платежа (мок)
func (m *OneCClientMock) GetPaymentStatus(ctx context.Context, credentials *OneCCredentials, externalID string) (*OneCPayment, error) {
	if m.ShouldFail {
		return nil, fmt.Errorf("мок ошибка: %s", m.FailureMessage)
	}

	payment, exists := m.Payments[externalID]
	if !exists {
		return nil, fmt.Errorf("платеж с внешним ID %s не найден", externalID)
	}

	return payment, nil
}

// IsHealthy проверяет доступность API (мок)
func (m *OneCClientMock) IsHealthy(ctx context.Context, credentials *OneCCredentials) error {
	if m.ShouldFail {
		return fmt.Errorf("мок ошибка: %s", m.FailureMessage)
	}

	if !m.ConnectionHealthy {
		return fmt.Errorf("мок: подключение к 1С недоступно")
	}

	return nil
}

// CallWithRetry выполняет запрос с повторами (мок)
func (m *OneCClientMock) CallWithRetry(req interface{}, config RetryConfig) (interface{}, error) {
	if m.ShouldFail {
		return nil, fmt.Errorf("мок ошибка: %s", m.FailureMessage)
	}

	// В моке просто возвращаем успешный результат
	return map[string]interface{}{"success": true}, nil
}

// GetCallHistory возвращает историю вызовов для анализа в тестах
func (m *OneCClientMock) GetCallHistory() []MockCallMethodCall {
	return m.CallMethodCalls
}

// GetCreatedCounterparties возвращает список созданных контрагентов
func (m *OneCClientMock) GetCreatedCounterparties() []OneCCounterparty {
	return m.CreateCounterpartyCalls
}

// GetExportedRegistries возвращает список экспортированных реестров
func (m *OneCClientMock) GetExportedRegistries() []OneCPaymentRegistry {
	return m.ExportRegistryCalls
}

// Reset сбрасывает состояние мока
func (m *OneCClientMock) Reset() {
	m.ShouldFail = false
	m.FailureMessage = ""
	m.ConnectionHealthy = true
	m.CallMethodCalls = []MockCallMethodCall{}
	m.CreateCounterpartyCalls = []OneCCounterparty{}
	m.ExportRegistryCalls = []OneCPaymentRegistry{}
	m.Counterparties = []OneCCounterparty{}
	m.Payments = make(map[string]*OneCPayment)
	m.PaymentRegistries = []OneCPaymentRegistry{}
}

// AddPayment добавляет платеж в мок
func (m *OneCClientMock) AddPayment(externalID string, payment *OneCPayment) {
	m.Payments[externalID] = payment
}

// GetCallCount возвращает количество вызовов определенного метода
func (m *OneCClientMock) GetCallCount(method string) int {
	count := 0
	for _, call := range m.CallMethodCalls {
		if call.Method == method {
			count++
		}
	}
	return count
}

// GetLastCall возвращает последний вызов определенного метода
func (m *OneCClientMock) GetLastCall(method string) *MockCallMethodCall {
	for i := len(m.CallMethodCalls) - 1; i >= 0; i-- {
		if m.CallMethodCalls[i].Method == method {
			return &m.CallMethodCalls[i]
		}
	}
	return nil
}
