package services

import (
	"backend_axenta/models"
	"context"
	"fmt"
	"net/http"
	"time"
)

// MockAxetnaClient мок клиент для тестирования
// Реализует интерфейс AxetnaClientInterface
type MockAxetnaClient struct {
	// Настройки мока
	ShouldFailAuth   bool
	ShouldFailCreate bool
	ShouldFailUpdate bool
	ShouldFailDelete bool
	ShouldFailHealth bool
	AuthDelay        time.Duration
	CreateDelay      time.Duration
	UpdateDelay      time.Duration
	DeleteDelay      time.Duration

	// Данные для возврата
	AuthResponse   *TenantCredentials
	CreateResponse *AxetnaObjectResponse
	UpdateResponse *AxetnaObjectResponse

	// Счетчики вызовов
	AuthCallCount   int
	CreateCallCount int
	UpdateCallCount int
	DeleteCallCount int
	HealthCallCount int

	// Логи вызовов
	AuthCalls   []AuthCall
	CreateCalls []CreateCall
	UpdateCalls []UpdateCall
	DeleteCalls []DeleteCall
}

// Структуры для логирования вызовов
type AuthCall struct {
	Login    string
	Password string
	Time     time.Time
}

type CreateCall struct {
	Object *models.Object
	Time   time.Time
}

type UpdateCall struct {
	Object *models.Object
	Time   time.Time
}

type DeleteCall struct {
	ExternalID string
	Time       time.Time
}

// NewMockAxetnaClient создает новый мок клиент
func NewMockAxetnaClient() *MockAxetnaClient {
	return &MockAxetnaClient{
		AuthResponse: &TenantCredentials{
			Login:     "test_login",
			Password:  "test_password",
			Token:     "mock_token_123",
			ExpiresAt: time.Now().Add(1 * time.Hour),
		},
		CreateResponse: &AxetnaObjectResponse{
			ID:        "mock_external_id_123",
			Name:      "Test Object",
			Type:      "gps_tracker",
			Status:    "active",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		UpdateResponse: &AxetnaObjectResponse{
			ID:        "mock_external_id_123",
			Name:      "Updated Test Object",
			Type:      "gps_tracker",
			Status:    "active",
			CreatedAt: time.Now().Add(-1 * time.Hour),
			UpdatedAt: time.Now(),
		},
	}
}

// Authenticate мок авторизации
func (m *MockAxetnaClient) Authenticate(ctx context.Context, login, password string) (*TenantCredentials, error) {
	m.AuthCallCount++
	m.AuthCalls = append(m.AuthCalls, AuthCall{
		Login:    login,
		Password: password,
		Time:     time.Now(),
	})

	if m.AuthDelay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(m.AuthDelay):
		}
	}

	if m.ShouldFailAuth {
		return nil, fmt.Errorf("мок ошибка авторизации")
	}

	// Возвращаем копию ответа с переданными данными
	response := *m.AuthResponse
	response.Login = login
	response.Password = password

	return &response, nil
}

// CreateObject мок создания объекта
func (m *MockAxetnaClient) CreateObject(ctx context.Context, credentials *TenantCredentials, object *models.Object) (*AxetnaObjectResponse, error) {
	m.CreateCallCount++
	m.CreateCalls = append(m.CreateCalls, CreateCall{
		Object: object,
		Time:   time.Now(),
	})

	if m.CreateDelay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(m.CreateDelay):
		}
	}

	if m.ShouldFailCreate {
		return nil, fmt.Errorf("мок ошибка создания объекта")
	}

	// Возвращаем копию ответа с данными объекта
	response := *m.CreateResponse
	response.Name = object.Name
	response.Type = object.Type

	return &response, nil
}

// UpdateObject мок обновления объекта
func (m *MockAxetnaClient) UpdateObject(ctx context.Context, credentials *TenantCredentials, object *models.Object) (*AxetnaObjectResponse, error) {
	m.UpdateCallCount++
	m.UpdateCalls = append(m.UpdateCalls, UpdateCall{
		Object: object,
		Time:   time.Now(),
	})

	if m.UpdateDelay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(m.UpdateDelay):
		}
	}

	if m.ShouldFailUpdate {
		return nil, fmt.Errorf("мок ошибка обновления объекта")
	}

	// Возвращаем копию ответа с данными объекта
	response := *m.UpdateResponse
	response.ID = object.ExternalID
	response.Name = object.Name
	response.Type = object.Type

	return &response, nil
}

// DeleteObject мок удаления объекта
func (m *MockAxetnaClient) DeleteObject(ctx context.Context, credentials *TenantCredentials, externalID string) error {
	m.DeleteCallCount++
	m.DeleteCalls = append(m.DeleteCalls, DeleteCall{
		ExternalID: externalID,
		Time:       time.Now(),
	})

	if m.DeleteDelay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(m.DeleteDelay):
		}
	}

	if m.ShouldFailDelete {
		return fmt.Errorf("мок ошибка удаления объекта")
	}

	return nil
}

// CallWithRetry мок retry механизма
func (m *MockAxetnaClient) CallWithRetry(req *http.Request, config RetryConfig) (*http.Response, error) {
	// В моке не реализуем полную логику HTTP запросов
	return nil, fmt.Errorf("CallWithRetry не поддерживается в моке")
}

// IsHealthy мок проверки здоровья
func (m *MockAxetnaClient) IsHealthy(ctx context.Context) error {
	m.HealthCallCount++

	if m.ShouldFailHealth {
		return fmt.Errorf("мок ошибка проверки здоровья API")
	}

	return nil
}

// Reset сбрасывает состояние мока
func (m *MockAxetnaClient) Reset() {
	m.ShouldFailAuth = false
	m.ShouldFailCreate = false
	m.ShouldFailUpdate = false
	m.ShouldFailDelete = false
	m.ShouldFailHealth = false

	m.AuthDelay = 0
	m.CreateDelay = 0
	m.UpdateDelay = 0
	m.DeleteDelay = 0

	m.AuthCallCount = 0
	m.CreateCallCount = 0
	m.UpdateCallCount = 0
	m.DeleteCallCount = 0
	m.HealthCallCount = 0

	m.AuthCalls = nil
	m.CreateCalls = nil
	m.UpdateCalls = nil
	m.DeleteCalls = nil
}

// GetLastAuthCall возвращает последний вызов авторизации
func (m *MockAxetnaClient) GetLastAuthCall() *AuthCall {
	if len(m.AuthCalls) == 0 {
		return nil
	}
	return &m.AuthCalls[len(m.AuthCalls)-1]
}

// GetLastCreateCall возвращает последний вызов создания объекта
func (m *MockAxetnaClient) GetLastCreateCall() *CreateCall {
	if len(m.CreateCalls) == 0 {
		return nil
	}
	return &m.CreateCalls[len(m.CreateCalls)-1]
}

// GetLastUpdateCall возвращает последний вызов обновления объекта
func (m *MockAxetnaClient) GetLastUpdateCall() *UpdateCall {
	if len(m.UpdateCalls) == 0 {
		return nil
	}
	return &m.UpdateCalls[len(m.UpdateCalls)-1]
}

// GetLastDeleteCall возвращает последний вызов удаления объекта
func (m *MockAxetnaClient) GetLastDeleteCall() *DeleteCall {
	if len(m.DeleteCalls) == 0 {
		return nil
	}
	return &m.DeleteCalls[len(m.DeleteCalls)-1]
}

// SetAuthResponse устанавливает ответ для авторизации
func (m *MockAxetnaClient) SetAuthResponse(response *TenantCredentials) {
	m.AuthResponse = response
}

// SetCreateResponse устанавливает ответ для создания объекта
func (m *MockAxetnaClient) SetCreateResponse(response *AxetnaObjectResponse) {
	m.CreateResponse = response
}

// SetUpdateResponse устанавливает ответ для обновления объекта
func (m *MockAxetnaClient) SetUpdateResponse(response *AxetnaObjectResponse) {
	m.UpdateResponse = response
}
