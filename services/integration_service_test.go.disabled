package services

import (
	"backend_axenta/models"
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewIntegrationService(t *testing.T) {
	t.Run("Успешное создание сервиса", func(t *testing.T) {
		service, err := NewIntegrationService("https://api.example.com", nil)
		require.NoError(t, err)
		assert.NotNil(t, service)
		assert.NotNil(t, service.AxetnaClient)
		assert.NotNil(t, service.credentialsCache)
		assert.NotNil(t, service.encryptionKey)
		assert.Equal(t, 32, len(service.encryptionKey))
	})

	t.Run("Создание сервиса с кастомным логгером", func(t *testing.T) {
		logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
		service, err := NewIntegrationService("https://api.example.com", logger)
		require.NoError(t, err)
		assert.Equal(t, logger, service.logger)
	})

	t.Run("Ошибка при неправильном ключе шифрования", func(t *testing.T) {
		// Устанавливаем неправильный ключ
		os.Setenv("ENCRYPTION_KEY", "short")
		defer os.Unsetenv("ENCRYPTION_KEY")

		service, err := NewIntegrationService("https://api.example.com", nil)
		assert.Error(t, err)
		assert.Nil(t, service)
		assert.Contains(t, err.Error(), "ключ шифрования должен быть длиной 32 символа")
	})
}

func TestIntegrationService_EncryptDecryptPassword(t *testing.T) {
	service, err := NewIntegrationService("https://api.example.com", nil)
	require.NoError(t, err)

	testPasswords := []string{
		"simple_password",
		"сложный_пароль_123!@#",
		"",
		"very_long_password_with_special_characters_!@#$%^&*()_+-=[]{}|;':\",./<>?",
	}

	for _, password := range testPasswords {
		t.Run("Пароль: "+password, func(t *testing.T) {
			// Шифруем
			encrypted, err := service.EncryptPassword(password)
			require.NoError(t, err)
			assert.NotEmpty(t, encrypted)
			assert.NotEqual(t, password, encrypted)

			// Расшифровываем
			decrypted, err := service.decryptPassword(encrypted)
			require.NoError(t, err)
			assert.Equal(t, password, decrypted)
		})
	}
}

func TestIntegrationService_SyncObjectCreate(t *testing.T) {
	// Создаем мок сервис
	service, mockClient := createMockIntegrationService(t)

	ctx := context.Background()
	tenantID := uint(1)

	object := &models.Object{
		ID:           123,
		Name:         "Test Object",
		Type:         "gps_tracker",
		Description:  "Test Description",
		IMEI:         "123456789012345",
		PhoneNumber:  "+1234567890",
		SerialNumber: "SN123456",
	}

	t.Run("Успешная синхронизация создания", func(t *testing.T) {
		mockClient.Reset()

		// Мокаем успешные вызовы
		mockClient.ShouldFailAuth = false
		mockClient.ShouldFailCreate = false

		err := service.SyncObjectCreate(ctx, tenantID, object)
		assert.NoError(t, err)

		// Проверяем, что методы были вызваны
		assert.Equal(t, 1, mockClient.AuthCallCount)
		assert.Equal(t, 1, mockClient.CreateCallCount)

		// Проверяем параметры вызова
		lastCreateCall := mockClient.GetLastCreateCall()
		require.NotNil(t, lastCreateCall)
		assert.Equal(t, object.Name, lastCreateCall.Object.Name)
		assert.Equal(t, object.Type, lastCreateCall.Object.Type)

		// Проверяем, что ExternalID был установлен
		assert.Equal(t, mockClient.CreateResponse.ID, object.ExternalID)
	})

	t.Run("Ошибка авторизации", func(t *testing.T) {
		mockClient.Reset()
		mockClient.ShouldFailAuth = true

		err := service.SyncObjectCreate(ctx, tenantID, object)
		assert.Error(t, err)

		integrationErr, ok := err.(*IntegrationError)
		require.True(t, ok)
		assert.Equal(t, tenantID, integrationErr.TenantID)
		assert.Equal(t, "create", integrationErr.Operation)
		assert.Equal(t, object.ID, integrationErr.ObjectID)
		assert.True(t, integrationErr.Retryable)
	})

	t.Run("Ошибка создания объекта", func(t *testing.T) {
		mockClient.Reset()
		mockClient.ShouldFailAuth = false
		mockClient.ShouldFailCreate = true

		err := service.SyncObjectCreate(ctx, tenantID, object)
		assert.Error(t, err)

		integrationErr, ok := err.(*IntegrationError)
		require.True(t, ok)
		assert.Equal(t, "create", integrationErr.Operation)
		assert.True(t, integrationErr.Retryable)
	})
}

func TestIntegrationService_SyncObjectUpdate(t *testing.T) {
	service, mockClient := createMockIntegrationService(t)

	ctx := context.Background()
	tenantID := uint(1)

	object := &models.Object{
		ID:         123,
		Name:       "Updated Object",
		Type:       "gps_tracker",
		ExternalID: "external_123",
	}

	t.Run("Успешная синхронизация обновления", func(t *testing.T) {
		mockClient.Reset()

		err := service.SyncObjectUpdate(ctx, tenantID, object)
		assert.NoError(t, err)

		assert.Equal(t, 1, mockClient.AuthCallCount)
		assert.Equal(t, 1, mockClient.UpdateCallCount)

		lastUpdateCall := mockClient.GetLastUpdateCall()
		require.NotNil(t, lastUpdateCall)
		assert.Equal(t, object.Name, lastUpdateCall.Object.Name)
	})

	t.Run("Ошибка отсутствия ExternalID", func(t *testing.T) {
		mockClient.Reset()

		objectWithoutExternalID := &models.Object{
			ID:   123,
			Name: "Object without external ID",
		}

		err := service.SyncObjectUpdate(ctx, tenantID, objectWithoutExternalID)
		assert.Error(t, err)

		integrationErr, ok := err.(*IntegrationError)
		require.True(t, ok)
		assert.Equal(t, "update", integrationErr.Operation)
		assert.False(t, integrationErr.Retryable) // Не retryable, так как нет ExternalID
	})
}

func TestIntegrationService_SyncObjectDelete(t *testing.T) {
	service, mockClient := createMockIntegrationService(t)

	ctx := context.Background()
	tenantID := uint(1)

	object := &models.Object{
		ID:         123,
		Name:       "Object to delete",
		ExternalID: "external_123",
	}

	t.Run("Успешная синхронизация удаления", func(t *testing.T) {
		mockClient.Reset()

		err := service.SyncObjectDelete(ctx, tenantID, object)
		assert.NoError(t, err)

		assert.Equal(t, 1, mockClient.AuthCallCount)
		assert.Equal(t, 1, mockClient.DeleteCallCount)

		lastDeleteCall := mockClient.GetLastDeleteCall()
		require.NotNil(t, lastDeleteCall)
		assert.Equal(t, object.ExternalID, lastDeleteCall.ExternalID)
	})

	t.Run("Удаление объекта без ExternalID (не ошибка)", func(t *testing.T) {
		mockClient.Reset()

		objectWithoutExternalID := &models.Object{
			ID:   123,
			Name: "Object without external ID",
		}

		err := service.SyncObjectDelete(ctx, tenantID, objectWithoutExternalID)
		assert.NoError(t, err) // Это не должно быть ошибкой

		// Проверяем, что никаких вызовов не было
		assert.Equal(t, 0, mockClient.AuthCallCount)
		assert.Equal(t, 0, mockClient.DeleteCallCount)
	})
}

func TestIntegrationService_CheckHealth(t *testing.T) {
	service, mockClient := createMockIntegrationService(t)

	ctx := context.Background()

	t.Run("Успешная проверка здоровья", func(t *testing.T) {
		mockClient.Reset()
		mockClient.ShouldFailHealth = false

		results := service.CheckHealth(ctx)

		require.Contains(t, results, "axetna_cloud")
		assert.NoError(t, results["axetna_cloud"])
		assert.Equal(t, 1, mockClient.HealthCallCount)
	})

	t.Run("Ошибка проверки здоровья", func(t *testing.T) {
		mockClient.Reset()
		mockClient.ShouldFailHealth = true

		results := service.CheckHealth(ctx)

		require.Contains(t, results, "axetna_cloud")
		assert.Error(t, results["axetna_cloud"])
	})
}

func TestIntegrationService_CredentialsCache(t *testing.T) {
	service, _ := createMockIntegrationService(t)

	t.Run("Очистка кэша для конкретной компании", func(t *testing.T) {
		// Добавляем данные в кэш
		service.credentialsCache[1] = &TenantCredentials{Login: "test1"}
		service.credentialsCache[2] = &TenantCredentials{Login: "test2"}

		assert.Equal(t, 2, service.GetCachedCredentialsCount())

		// Очищаем кэш для компании 1
		service.ClearCredentialsCache(1)

		assert.Equal(t, 1, service.GetCachedCredentialsCount())
		assert.Nil(t, service.credentialsCache[1])
		assert.NotNil(t, service.credentialsCache[2])
	})

	t.Run("Полная очистка кэша", func(t *testing.T) {
		// Добавляем данные в кэш
		service.credentialsCache[1] = &TenantCredentials{Login: "test1"}
		service.credentialsCache[2] = &TenantCredentials{Login: "test2"}

		assert.Equal(t, 2, service.GetCachedCredentialsCount())

		// Очищаем весь кэш
		service.ClearCredentialsCache(0)

		assert.Equal(t, 0, service.GetCachedCredentialsCount())
	})
}

func TestIntegrationService_AsyncOperations(t *testing.T) {
	service, mockClient := createMockIntegrationService(t)

	tenantID := uint(1)
	object := &models.Object{
		ID:   123,
		Name: "Async Test Object",
		Type: "gps_tracker",
	}

	t.Run("Асинхронное создание объекта", func(t *testing.T) {
		mockClient.Reset()

		// Запускаем асинхронную операцию
		service.SyncObjectAsync(tenantID, "create", object)

		// Ждем немного, чтобы горутина выполнилась
		time.Sleep(100 * time.Millisecond)

		// Проверяем, что операция была выполнена
		assert.Equal(t, 1, mockClient.AuthCallCount)
		assert.Equal(t, 1, mockClient.CreateCallCount)
	})

	t.Run("Асинхронная операция с неизвестным типом", func(t *testing.T) {
		mockClient.Reset()

		// Запускаем с неизвестным типом операции
		service.SyncObjectAsync(tenantID, "unknown_operation", object)

		// Ждем немного
		time.Sleep(100 * time.Millisecond)

		// Проверяем, что никаких вызовов не было
		assert.Equal(t, 0, mockClient.AuthCallCount)
		assert.Equal(t, 0, mockClient.CreateCallCount)
		assert.Equal(t, 0, mockClient.UpdateCallCount)
		assert.Equal(t, 0, mockClient.DeleteCallCount)
	})
}

// Вспомогательная функция для создания мок сервиса
func createMockIntegrationService(t *testing.T) (*IntegrationService, *MockAxetnaClient) {
	// Устанавливаем ключ шифрования для теста
	os.Setenv("ENCRYPTION_KEY", "test-encryption-key-32-chars!!")
	defer os.Unsetenv("ENCRYPTION_KEY")

	service, err := NewIntegrationService("https://api.example.com", nil)
	require.NoError(t, err)

	// Заменяем реальный клиент на мок
	mockClient := NewMockAxetnaClient()
	service.AxetnaClient = mockClient

	return service, mockClient
}

// Бенчмарки для проверки производительности
func BenchmarkIntegrationService_EncryptPassword(b *testing.B) {
	os.Setenv("ENCRYPTION_KEY", "test-encryption-key-32-chars!!")
	defer os.Unsetenv("ENCRYPTION_KEY")

	service, err := NewIntegrationService("https://api.example.com", nil)
	require.NoError(b, err)

	password := "test_password_123"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := service.EncryptPassword(password)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkIntegrationService_DecryptPassword(b *testing.B) {
	os.Setenv("ENCRYPTION_KEY", "test-encryption-key-32-chars!!")
	defer os.Unsetenv("ENCRYPTION_KEY")

	service, err := NewIntegrationService("https://api.example.com", nil)
	require.NoError(b, err)

	password := "test_password_123"
	encrypted, err := service.EncryptPassword(password)
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := service.decryptPassword(encrypted)
		if err != nil {
			b.Fatal(err)
		}
	}
}
