package services

import (
	"log"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNewBitrix24Client тестирует создание нового Bitrix24Client
func TestNewBitrix24Client(t *testing.T) {
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	client := NewBitrix24Client(logger)

	assert.NotNil(t, client)
	assert.NotNil(t, client.HTTPClient)
	assert.NotNil(t, client.Logger)
}

// TestBitrix24Credentials_Validation тестирует валидацию Bitrix24Credentials
func TestBitrix24Credentials_Validation(t *testing.T) {
	creds := Bitrix24Credentials{
		WebhookURL:  "https://example.bitrix24.ru/rest/1/xxx/",
		ClientID:    "client-id",
		AccessToken: "access-token",
	}

	assert.NotEmpty(t, creds.WebhookURL)
	assert.NotEmpty(t, creds.ClientID)
}

// TestBitrix24Contact_Structure тестирует структуру Bitrix24Contact
func TestBitrix24Contact_Structure(t *testing.T) {
	contact := Bitrix24Contact{
		ID:       "1",
		Name:     "John",
		LastName: "Doe",
		Email:    "john@example.com",
		Phone:    "+1234567890",
	}

	assert.Equal(t, "1", contact.ID)
	assert.Equal(t, "John", contact.Name)
	assert.Equal(t, "john@example.com", contact.Email)
}

// TestBitrix24Deal_Structure тестирует структуру Bitrix24Deal
func TestBitrix24Deal_Structure(t *testing.T) {
	deal := Bitrix24Deal{
		ID:          "1",
		Title:       "Test Deal",
		StageID:     "NEW",
		Opportunity: 10000.0,
		CurrencyID:  "RUB",
	}

	assert.Equal(t, "1", deal.ID)
	assert.Equal(t, "Test Deal", deal.Title)
	assert.Equal(t, 10000.0, deal.Opportunity)
}

// TestBitrix24Deal_GetCustomField тестирует GetCustomField
func TestBitrix24Deal_GetCustomField(t *testing.T) {
	deal := Bitrix24Deal{
		CustomFields: map[string]interface{}{
			"UF_CRM_TEST": "test-value",
		},
	}

	value := deal.GetCustomField("UF_CRM_TEST")
	assert.Equal(t, "test-value", value)
}

// TestBitrix24Deal_GetCustomField_NotFound тестирует GetCustomField когда поле не найдено
func TestBitrix24Deal_GetCustomField_NotFound(t *testing.T) {
	deal := Bitrix24Deal{
		CustomFields: map[string]interface{}{},
	}

	value := deal.GetCustomField("UF_CRM_MISSING")
	assert.Nil(t, value)
}

// TestBitrix24Deal_GetCustomFieldString тестирует GetCustomFieldString
func TestBitrix24Deal_GetCustomFieldString(t *testing.T) {
	deal := Bitrix24Deal{
		CustomFields: map[string]interface{}{
			"UF_CRM_TEST": "test-value",
		},
	}

	value := deal.GetCustomFieldString("UF_CRM_TEST")
	assert.Equal(t, "test-value", value)
}

// TestBitrix24Company_Structure тестирует структуру Bitrix24Company
func TestBitrix24Company_Structure(t *testing.T) {
	company := Bitrix24Company{
		ID:      "1",
		Title:   "Test Company",
		Email:   "company@example.com",
		Phone:   "+1234567890",
		Address: "123 Main St",
	}

	assert.Equal(t, "1", company.ID)
	assert.Equal(t, "Test Company", company.Title)
	assert.Equal(t, "company@example.com", company.Email)
}
