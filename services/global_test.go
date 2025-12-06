package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGetIntegrationService тестирует GetIntegrationService
func TestGetIntegrationService(t *testing.T) {
	service := GetIntegrationService()
	// Временно отключено, должен вернуть nil
	assert.Nil(t, service)
}

// TestSetIntegrationService тестирует SetIntegrationService
func TestSetIntegrationService(t *testing.T) {
	// Временно отключено, не должно паниковать
	SetIntegrationService(nil)
	// Не должно паниковать
}
