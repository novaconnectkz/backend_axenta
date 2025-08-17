package services

// Глобальная переменная для сервиса интеграций
var GlobalIntegrationService *IntegrationService

// GetIntegrationService возвращает глобальный сервис интеграций
func GetIntegrationService() *IntegrationService {
	return GlobalIntegrationService
}

// SetIntegrationService устанавливает глобальный сервис интеграций
func SetIntegrationService(service *IntegrationService) {
	GlobalIntegrationService = service
}
