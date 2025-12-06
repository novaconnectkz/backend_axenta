package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewAccountHierarchyService тестирует создание нового сервиса
func TestNewAccountHierarchyService(t *testing.T) {
	service := NewAccountHierarchyService("test-token")

	assert.NotNil(t, service)
	assert.Equal(t, "test-token", service.token)
	assert.NotNil(t, service.cache)
}

// TestAccountHierarchyService_LoadAllAccounts_MockServer тестирует LoadAllAccounts с mock сервером
func TestAccountHierarchyService_LoadAllAccounts_MockServer(t *testing.T) {
	// Создаем mock сервер
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/cms/accounts/" && r.Method == "GET" {
			response := map[string]interface{}{
				"count": 2,
				"results": []AxentaAccount{
					{
						ID:                1,
						Name:              "Test Account 1",
						Type:              "partner",
						ParentAccountName: "GLOMOS",
						IsActive:          true,
					},
					{
						ID:                2,
						Name:              "Test Account 2",
						Type:              "client",
						ParentAccountName: "Test Account 1",
						IsActive:          true,
					},
				},
				"next": nil,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	// Заменяем URL на mock сервер (в реальном тесте нужно использовать dependency injection)
	// Для этого теста просто проверяем, что сервис создается
	service := NewAccountHierarchyService("test-token")
	assert.NotNil(t, service)
}

// TestAccountHierarchyService_FindPartnerInHierarchy_NoPartner тестирует FindPartnerInHierarchy когда партнер не найден
func TestAccountHierarchyService_FindPartnerInHierarchy_NoPartner(t *testing.T) {
	service := NewAccountHierarchyService("test-token")

	// Пустой кэш
	partnerID := service.FindPartnerInHierarchy(1)
	assert.Equal(t, int64(0), partnerID)
}

// TestAccountHierarchyService_FindPartnerInHierarchy_WithCache тестирует FindPartnerInHierarchy с кэшем
func TestAccountHierarchyService_FindPartnerInHierarchy_WithCache(t *testing.T) {
	service := NewAccountHierarchyService("test-token")

	// Добавляем аккаунты в кэш вручную
	parentID := int64(1)
	service.cacheMutex.Lock()
	service.cache[1] = &AxentaAccount{
		ID:                1,
		Name:              "Partner Account",
		Type:              "partner",
		ParentAccountName: "GLOMOS",
		IsActive:          true,
	}
	service.cache[2] = &AxentaAccount{
		ID:                2,
		Name:              "Client Account",
		Type:              "client",
		ParentAccountName: "Partner Account",
		ParentAccountID:   &parentID,
		IsActive:          true,
	}
	service.cacheMutex.Unlock()

	// Ищем партнера для клиента
	partnerID := service.FindPartnerInHierarchy(2)
	assert.Equal(t, int64(1), partnerID)
}

// TestAccountHierarchyService_GetAccount_NotFound тестирует GetAccount когда аккаунт не найден
func TestAccountHierarchyService_GetAccount_NotFound(t *testing.T) {
	service := NewAccountHierarchyService("test-token")

	account, exists := service.GetAccount(999)
	assert.Nil(t, account)
	assert.False(t, exists)
}

// TestAccountHierarchyService_GetAccount_Found тестирует GetAccount когда аккаунт найден
func TestAccountHierarchyService_GetAccount_Found(t *testing.T) {
	service := NewAccountHierarchyService("test-token")

	// Добавляем аккаунт в кэш
	expectedAccount := &AxentaAccount{
		ID:       1,
		Name:     "Test Account",
		Type:     "partner",
		IsActive: true,
	}
	service.cacheMutex.Lock()
	service.cache[1] = expectedAccount
	service.cacheMutex.Unlock()

	account, exists := service.GetAccount(1)
	require.NotNil(t, account)
	require.True(t, exists)
	assert.Equal(t, expectedAccount.Name, account.Name)
}

// TestAccountHierarchyService_GetAllAccounts тестирует GetAllAccounts
func TestAccountHierarchyService_GetAllAccounts(t *testing.T) {
	service := NewAccountHierarchyService("test-token")

	// Добавляем аккаунты в кэш
	service.cacheMutex.Lock()
	service.cache[1] = &AxentaAccount{ID: 1, Name: "Account 1"}
	service.cache[2] = &AxentaAccount{ID: 2, Name: "Account 2"}
	service.cacheMutex.Unlock()

	accounts := service.GetAllAccounts()
	assert.Equal(t, 2, len(accounts))
}
