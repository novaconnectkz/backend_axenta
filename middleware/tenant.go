package middleware

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TenantMiddleware управляет мультитенантностью
type TenantMiddleware struct {
	DB *gorm.DB
}

// NewTenantMiddleware создает новый экземпляр TenantMiddleware
func NewTenantMiddleware(db *gorm.DB) *TenantMiddleware {
	return &TenantMiddleware{DB: db}
}

// SetTenant определяет текущую компанию и переключает схему БД
func (tm *TenantMiddleware) SetTenant() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Пропускаем публичные маршруты
		if isPublicRoute(c.Request.URL.Path) {
			c.Next()
			return
		}

		// Получаем компанию из различных источников
		company, err := tm.extractCompany(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status": "error",
				"error":  "Не удалось определить компанию: " + err.Error(),
			})
			c.Abort()
			return
		}

		if company == nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status": "error",
				"error":  "Компания не найдена",
			})
			c.Abort()
			return
		}

		// Проверяем активность компании
		if !company.IsActive {
			c.JSON(http.StatusForbidden, gin.H{
				"status": "error",
				"error":  "Компания деактивирована",
			})
			c.Abort()
			return
		}

		// Переключаем схему БД
		tenantDB := tm.switchToTenantSchema(company.GetSchemaName())
		if tenantDB == nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Ошибка подключения к схеме компании",
			})
			c.Abort()
			return
		}

		// Сохраняем информацию о текущей компании и БД в контексте
		c.Set("company", company)
		c.Set("tenant_db", tenantDB)
		c.Set("company_id", company.ID.String())
		c.Set("schema_name", company.GetSchemaName())

		c.Next()
	}
}

// extractCompany извлекает информацию о компании из запроса
func (tm *TenantMiddleware) extractCompany(c *gin.Context) (*models.Company, error) {
	// Временно: всегда используем компанию по умолчанию для упрощения отладки
	// TODO: восстановить полную логику после исправления типов данных
	return tm.getDefaultCompany()

	/* Отключено до исправления типов данных:
	// 1. Пробуем получить из заголовка X-Tenant-ID
	if tenantID := c.GetHeader("X-Tenant-ID"); tenantID != "" {
		return tm.getCompanyByID(tenantID)
	}

	// 2. Пробуем получить из поддомена
	if host := c.GetHeader("Host"); host != "" {
		if company := tm.getCompanyByDomain(host); company != nil {
			return company, nil
		}
	}

	// 3. Пробуем получить из JWT токена (если есть информация о компании)
	if companyID := tm.extractCompanyFromToken(c); companyID != "" {
		return tm.getCompanyByID(companyID)
	}

	// 4. Временно: для разработки используем компанию по умолчанию
	// Если не удалось найти компанию другими способами, используем первую активную
	return tm.getDefaultCompany()
	*/
}

// getCompanyByID получает компанию по ID с кэшированием
func (tm *TenantMiddleware) getCompanyByID(tenantID string) (*models.Company, error) {
	// Пробуем получить из кэша
	cacheKey := fmt.Sprintf("company:id:%s", tenantID)
	var company models.Company

	if err := database.CacheGetJSON(cacheKey, &company); err == nil {
		return &company, nil
	}

	// Если нет в кэше, получаем из БД (используем основную схему)
	// Создаем новое подключение к БД с основной схемой
	mainDB := tm.DB.Session(&gorm.Session{})
	if err := mainDB.Exec("SET search_path TO public").Error; err != nil {
		return nil, fmt.Errorf("ошибка переключения на основную схему: %v", err)
	}
	// Парсим UUID из строки
	companyUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("некорректный формат ID компании: %v", err)
	}

	if err := mainDB.Where("id = ? AND is_active = ?", companyUUID, true).First(&company).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("компания с ID %s не найдена", tenantID)
		}
		return nil, fmt.Errorf("ошибка поиска компании: %v", err)
	}

	// Кэшируем на 15 минут
	database.CacheSetJSON(cacheKey, &company, 15*time.Minute)

	return &company, nil
}

// getCompanyByDomain получает компанию по домену
func (tm *TenantMiddleware) getCompanyByDomain(host string) *models.Company {
	// Убираем порт из host если есть
	if strings.Contains(host, ":") {
		host = strings.Split(host, ":")[0]
	}

	var company models.Company
	if err := tm.DB.Where("domain = ? AND is_active = ?", host, true).First(&company).Error; err != nil {
		// Если точного совпадения нет, пробуем найти по поддомену
		if strings.Contains(host, ".") {
			subdomain := strings.Split(host, ".")[0]
			if err := tm.DB.Where("database_schema = ? AND is_active = ?", "tenant_"+subdomain, true).First(&company).Error; err == nil {
				return &company
			}
		}
		return nil
	}
	return &company
}

// extractCompanyFromToken извлекает ID компании из JWT токена
func (tm *TenantMiddleware) extractCompanyFromToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return ""
	}

	// Извлекаем токен из заголовка
	var tokenString string
	if strings.HasPrefix(authHeader, "Bearer ") {
		tokenString = strings.TrimPrefix(authHeader, "Bearer ")
	} else if strings.HasPrefix(authHeader, "Token ") {
		tokenString = strings.TrimPrefix(authHeader, "Token ")
	} else {
		tokenString = authHeader
	}

	if tokenString == "" {
		return ""
	}

	// Метод 1: Пробуем получить company_id через API current_user
	if companyID := tm.getCompanyIDFromAxentaAPI(tokenString); companyID != "" {
		return companyID
	}

	// Метод 2: Если это JWT токен, пробуем парсить его (без проверки подписи)
	if companyID := tm.parseJWTForCompanyID(tokenString); companyID != "" {
		return companyID
	}

	return ""
}

// getCompanyIDFromAxentaAPI получает company_id через Axenta API
func (tm *TenantMiddleware) getCompanyIDFromAxentaAPI(token string) string {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("GET", "https://axenta.cloud/api/current_user/", nil)
	if err != nil {
		return ""
	}

	// Используем тот же формат заголовка, что и в auth.go
	req.Header.Set("Authorization", "Token "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	// Парсим ответ для получения информации о пользователе
	var userData map[string]interface{}
	if err := json.Unmarshal(body, &userData); err != nil {
		return ""
	}

	// Ищем company_id или accountId в ответе
	if companyID, ok := userData["accountId"].(float64); ok {
		return fmt.Sprintf("%.0f", companyID)
	}

	if companyID, ok := userData["company_id"].(float64); ok {
		return fmt.Sprintf("%.0f", companyID)
	}

	if companyID, ok := userData["accountId"].(string); ok {
		return companyID
	}

	if companyID, ok := userData["company_id"].(string); ok {
		return companyID
	}

	return ""
}

// parseJWTForCompanyID пытается извлечь company_id из JWT токена (без проверки подписи)
func (tm *TenantMiddleware) parseJWTForCompanyID(tokenString string) string {
	// Парсим JWT токен без проверки подписи
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())

	token, _, err := parser.ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return ""
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		// Ищем различные поля, которые могут содержать company_id
		fields := []string{"company_id", "tenant_id", "account_id", "accountId", "companyId"}

		for _, field := range fields {
			if value, exists := claims[field]; exists {
				switch v := value.(type) {
				case string:
					return v
				case float64:
					return fmt.Sprintf("%.0f", v)
				case int:
					return fmt.Sprintf("%d", v)
				}
			}
		}
	}

	return ""
}

// getDefaultCompany возвращает компанию по умолчанию (для разработки)
func (tm *TenantMiddleware) getDefaultCompany() (*models.Company, error) {
	var company models.Company
	if err := tm.DB.Where("is_active = ?", true).First(&company).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// Создаем компанию по умолчанию если её нет
			return tm.createDefaultCompany()
		}
		return nil, fmt.Errorf("ошибка поиска компании по умолчанию: %v", err)
	}
	return &company, nil
}

// createDefaultCompany создает компанию по умолчанию
func (tm *TenantMiddleware) createDefaultCompany() (*models.Company, error) {
	company := &models.Company{
		Name:           "Компания по умолчанию",
		DatabaseSchema: "tenant_default",
		AxetnaLogin:    "default",
		AxetnaPassword: "encrypted_password", // В реальности должен быть зашифрован
		ContactEmail:   "admin@example.com",
		IsActive:       true,
	}

	if err := tm.DB.Create(company).Error; err != nil {
		return nil, fmt.Errorf("ошибка создания компании по умолчанию: %v", err)
	}

	// Создаем схему БД для новой компании
	if err := tm.createTenantSchema(company.GetSchemaName()); err != nil {
		return nil, fmt.Errorf("ошибка создания схемы БД: %v", err)
	}

	return company, nil
}

// SwitchToTenantSchema переключается на схему БД конкретной компании (публичный метод для тестов)
func (tm *TenantMiddleware) SwitchToTenantSchema(schemaName string) *gorm.DB {
	return tm.switchToTenantSchema(schemaName)
}

// switchToTenantSchema переключается на схему БД конкретной компании
func (tm *TenantMiddleware) switchToTenantSchema(schemaName string) *gorm.DB {
	// Клонируем подключение к БД
	tenantDB := database.DB.Session(&gorm.Session{})

	// Переключаемся на схему компании
	if err := tenantDB.Exec(fmt.Sprintf("SET search_path TO %s", schemaName)).Error; err != nil {
		fmt.Printf("Ошибка переключения на схему %s: %v\n", schemaName, err)
		return nil
	}

	return tenantDB
}

// CreateTenantSchema создает новую схему БД для компании (публичный метод для тестов)
func (tm *TenantMiddleware) CreateTenantSchema(schemaName string) error {
	return tm.createTenantSchema(schemaName)
}

// createTenantSchema создает новую схему БД для компании
func (tm *TenantMiddleware) createTenantSchema(schemaName string) error {
	// Создаем схему
	if err := tm.DB.Exec(fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", schemaName)).Error; err != nil {
		return fmt.Errorf("ошибка создания схемы %s: %v", schemaName, err)
	}

	// Переключаемся на новую схему
	tenantDB := tm.switchToTenantSchema(schemaName)
	if tenantDB == nil {
		return fmt.Errorf("ошибка переключения на схему %s", schemaName)
	}

	// Выполняем миграции для новой схемы
	if err := tm.runTenantMigrations(tenantDB); err != nil {
		return fmt.Errorf("ошибка миграций для схемы %s: %v", schemaName, err)
	}

	return nil
}

// runTenantMigrations выполняет миграции для схемы компании
func (tm *TenantMiddleware) runTenantMigrations(tenantDB *gorm.DB) error {
	// Список моделей для миграции в схеме компании
	models := []interface{}{
		// Пользователи и роли
		&models.Permission{},
		&models.Role{},
		&models.User{},
		&models.UserTemplate{},

		// Объекты и шаблоны
		&models.ObjectTemplate{},
		&models.MonitoringTemplate{},
		&models.NotificationTemplate{},
		&models.Object{},

		// Локации и монтажники
		&models.Location{},
		&models.Installer{},
		&models.Installation{},

		// Оборудование
		&models.Equipment{},

		// Договоры и тарифы
		&models.BillingPlan{},
		&models.TariffPlan{},
		&models.Contract{},
		&models.ContractAppendix{},
		&models.Subscription{},
	}

	for _, model := range models {
		if err := tenantDB.AutoMigrate(model); err != nil {
			return fmt.Errorf("ошибка миграции модели %T: %v", model, err)
		}
	}

	return nil
}

// isPublicRoute проверяет, является ли маршрут публичным
func isPublicRoute(path string) bool {
	publicRoutes := []string{
		"/ping",
		"/api/auth/login",
		"/health",
		"/metrics",
	}

	for _, route := range publicRoutes {
		if strings.HasPrefix(path, route) {
			return true
		}
	}
	return false
}

// GetTenantDB возвращает подключение к БД текущей компании из контекста
func GetTenantDB(c *gin.Context) *gorm.DB {
	if db, exists := c.Get("tenant_db"); exists {
		if tenantDB, ok := db.(*gorm.DB); ok {
			return tenantDB
		}
	}
	return nil
}

// GetCurrentCompany возвращает текущую компанию из контекста
func GetCurrentCompany(c *gin.Context) *models.Company {
	if company, exists := c.Get("company"); exists {
		if comp, ok := company.(*models.Company); ok {
			return comp
		}
	}
	return nil
}

// GetCompanyID возвращает ID текущей компании из контекста
func GetCompanyID(c *gin.Context) string {
	if companyID, exists := c.Get("company_id"); exists {
		if id, ok := companyID.(string); ok {
			return id
		}
	}
	return ""
}
