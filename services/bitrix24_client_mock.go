package services

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// MockBitrix24Client мок клиент для тестирования интеграции с Битрикс24
type MockBitrix24Client struct {
	contacts     map[string]*Bitrix24Contact
	deals        map[string]*Bitrix24Deal
	companies    map[string]*Bitrix24Company
	nextID       int
	shouldFail   bool
	failMessage  string
	healthStatus bool
}

// NewMockBitrix24Client создает новый мок клиент
func NewMockBitrix24Client() *MockBitrix24Client {
	return &MockBitrix24Client{
		contacts:     make(map[string]*Bitrix24Contact),
		deals:        make(map[string]*Bitrix24Deal),
		companies:    make(map[string]*Bitrix24Company),
		nextID:       1,
		healthStatus: true,
	}
}

// SetShouldFail устанавливает режим ошибок для тестирования
func (m *MockBitrix24Client) SetShouldFail(shouldFail bool, message string) {
	m.shouldFail = shouldFail
	m.failMessage = message
}

// SetHealthStatus устанавливает статус здоровья для тестирования
func (m *MockBitrix24Client) SetHealthStatus(healthy bool) {
	m.healthStatus = healthy
}

// CallMethod выполняет вызов метода Битрикс24 API (мок)
func (m *MockBitrix24Client) CallMethod(ctx context.Context, credentials *Bitrix24Credentials, method string, params map[string]interface{}) (*Bitrix24Response, error) {
	if m.shouldFail {
		return nil, fmt.Errorf("мок ошибка: %s", m.failMessage)
	}

	// Симулируем различные методы API
	switch method {
	case "profile":
		return &Bitrix24Response{
			Result: map[string]interface{}{
				"ID":   "1",
				"NAME": "Test User",
			},
		}, nil
	case "crm.contact.add":
		return &Bitrix24Response{
			Result: float64(m.nextID),
		}, nil
	case "crm.deal.add":
		return &Bitrix24Response{
			Result: float64(m.nextID),
		}, nil
	default:
		return &Bitrix24Response{
			Result: map[string]interface{}{
				"success": true,
			},
		}, nil
	}
}

// CreateContact создает контакт в Битрикс24 (мок)
func (m *MockBitrix24Client) CreateContact(ctx context.Context, credentials *Bitrix24Credentials, contact *Bitrix24Contact) (string, error) {
	if m.shouldFail {
		return "", fmt.Errorf("мок ошибка создания контакта: %s", m.failMessage)
	}

	contactID := strconv.Itoa(m.nextID)
	m.nextID++

	// Копируем контакт и сохраняем
	newContact := *contact
	newContact.ID = contactID
	m.contacts[contactID] = &newContact

	return contactID, nil
}

// UpdateContact обновляет контакт в Битрикс24 (мок)
func (m *MockBitrix24Client) UpdateContact(ctx context.Context, credentials *Bitrix24Credentials, contactID string, contact *Bitrix24Contact) error {
	if m.shouldFail {
		return fmt.Errorf("мок ошибка обновления контакта: %s", m.failMessage)
	}

	if _, exists := m.contacts[contactID]; !exists {
		return fmt.Errorf("контакт %s не найден", contactID)
	}

	// Обновляем контакт
	updatedContact := *contact
	updatedContact.ID = contactID
	m.contacts[contactID] = &updatedContact

	return nil
}

// GetContact получает контакт из Битрикс24 (мок)
func (m *MockBitrix24Client) GetContact(ctx context.Context, credentials *Bitrix24Credentials, contactID string) (*Bitrix24Contact, error) {
	if m.shouldFail {
		return nil, fmt.Errorf("мок ошибка получения контакта: %s", m.failMessage)
	}

	contact, exists := m.contacts[contactID]
	if !exists {
		return nil, fmt.Errorf("контакт %s не найден", contactID)
	}

	return contact, nil
}

// GetContacts получает список контактов из Битрикс24 (мок)
func (m *MockBitrix24Client) GetContacts(ctx context.Context, credentials *Bitrix24Credentials, limit int, start int) ([]Bitrix24Contact, int, error) {
	if m.shouldFail {
		return nil, 0, fmt.Errorf("мок ошибка получения списка контактов: %s", m.failMessage)
	}

	var contacts []Bitrix24Contact
	for _, contact := range m.contacts {
		contacts = append(contacts, *contact)
	}

	total := len(contacts)

	// Применяем пагинацию
	if start >= total {
		return []Bitrix24Contact{}, total, nil
	}

	end := start + limit
	if end > total {
		end = total
	}

	return contacts[start:end], total, nil
}

// CreateDeal создает сделку в Битрикс24 (мок)
func (m *MockBitrix24Client) CreateDeal(ctx context.Context, credentials *Bitrix24Credentials, deal *Bitrix24Deal) (string, error) {
	if m.shouldFail {
		return "", fmt.Errorf("мок ошибка создания сделки: %s", m.failMessage)
	}

	dealID := strconv.Itoa(m.nextID)
	m.nextID++

	// Копируем сделку и сохраняем
	newDeal := *deal
	newDeal.ID = dealID
	newDeal.DateCreate = time.Now()
	newDeal.DateModify = time.Now()
	m.deals[dealID] = &newDeal

	return dealID, nil
}

// UpdateDeal обновляет сделку в Битрикс24 (мок)
func (m *MockBitrix24Client) UpdateDeal(ctx context.Context, credentials *Bitrix24Credentials, dealID string, deal *Bitrix24Deal) error {
	if m.shouldFail {
		return fmt.Errorf("мок ошибка обновления сделки: %s", m.failMessage)
	}

	if _, exists := m.deals[dealID]; !exists {
		return fmt.Errorf("сделка %s не найдена", dealID)
	}

	// Обновляем сделку
	updatedDeal := *deal
	updatedDeal.ID = dealID
	updatedDeal.DateModify = time.Now()
	m.deals[dealID] = &updatedDeal

	return nil
}

// GetDeal получает сделку из Битрикс24 (мок)
func (m *MockBitrix24Client) GetDeal(ctx context.Context, credentials *Bitrix24Credentials, dealID string) (*Bitrix24Deal, error) {
	if m.shouldFail {
		return nil, fmt.Errorf("мок ошибка получения сделки: %s", m.failMessage)
	}

	deal, exists := m.deals[dealID]
	if !exists {
		return nil, fmt.Errorf("сделка %s не найдена", dealID)
	}

	return deal, nil
}

// GetDeals получает список сделок из Битрикс24 (мок)
func (m *MockBitrix24Client) GetDeals(ctx context.Context, credentials *Bitrix24Credentials, limit int, start int) ([]Bitrix24Deal, int, error) {
	if m.shouldFail {
		return nil, 0, fmt.Errorf("мок ошибка получения списка сделок: %s", m.failMessage)
	}

	var deals []Bitrix24Deal
	for _, deal := range m.deals {
		deals = append(deals, *deal)
	}

	total := len(deals)

	// Применяем пагинацию
	if start >= total {
		return []Bitrix24Deal{}, total, nil
	}

	end := start + limit
	if end > total {
		end = total
	}

	return deals[start:end], total, nil
}

// IsHealthy проверяет доступность Битрикс24 API (мок)
func (m *MockBitrix24Client) IsHealthy(ctx context.Context, credentials *Bitrix24Credentials) error {
	if !m.healthStatus {
		return fmt.Errorf("мок API недоступно")
	}

	if m.shouldFail {
		return fmt.Errorf("мок ошибка проверки здоровья: %s", m.failMessage)
	}

	return nil
}

// GetContactsCount возвращает количество контактов в моке
func (m *MockBitrix24Client) GetContactsCount() int {
	return len(m.contacts)
}

// GetDealsCount возвращает количество сделок в моке
func (m *MockBitrix24Client) GetDealsCount() int {
	return len(m.deals)
}

// ClearData очищает все данные в моке
func (m *MockBitrix24Client) ClearData() {
	m.contacts = make(map[string]*Bitrix24Contact)
	m.deals = make(map[string]*Bitrix24Deal)
	m.companies = make(map[string]*Bitrix24Company)
	m.nextID = 1
}

// AddTestContact добавляет тестовый контакт в мок
func (m *MockBitrix24Client) AddTestContact(contact *Bitrix24Contact) string {
	contactID := strconv.Itoa(m.nextID)
	m.nextID++

	testContact := *contact
	testContact.ID = contactID
	m.contacts[contactID] = &testContact

	return contactID
}

// AddTestDeal добавляет тестовую сделку в мок
func (m *MockBitrix24Client) AddTestDeal(deal *Bitrix24Deal) string {
	dealID := strconv.Itoa(m.nextID)
	m.nextID++

	testDeal := *deal
	testDeal.ID = dealID
	testDeal.DateCreate = time.Now()
	testDeal.DateModify = time.Now()
	m.deals[dealID] = &testDeal

	return dealID
}
