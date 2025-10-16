package api

import (
	"backend_axenta/middleware"
	"backend_axenta/models"
	"backend_axenta/services"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// LocalAuthAPI API для локальной авторизации
type LocalAuthAPI struct {
	db         *gorm.DB
	jwtService *services.JWTService
}

// getPublicDB возвращает подключение к схеме public для глобальных таблиц
func (api *LocalAuthAPI) getPublicDB() *gorm.DB {
	// Клонируем подключение и устанавливаем схему public
	publicDB := api.db.Session(&gorm.Session{})
	if err := publicDB.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️ Не удалось переключиться на схему public: %v", err)
	}
	return publicDB
}

// NewLocalAuthAPI создает новый API для локальной авторизации
func NewLocalAuthAPI(db *gorm.DB, jwtService *services.JWTService) *LocalAuthAPI {
	return &LocalAuthAPI{
		db:         db,
		jwtService: jwtService,
	}
}

// LocalLoginRequest структура запроса для локального входа
type LocalLoginRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required,min=3,max=128"`
}

// LocalLoginResponse структура ответа при входе
type LocalLoginResponse struct {
	Status string `json:"status"`
	Data   struct {
		AccessToken  string                 `json:"access_token"`
		RefreshToken string                 `json:"refresh_token"`
		TokenType    string                 `json:"token_type"`
		ExpiresIn    int                    `json:"expires_in"`
		User         map[string]interface{} `json:"user"`
	} `json:"data"`
}

// RefreshTokenRequest структура запроса для обновления токена
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// RegisterRequest структура запроса для регистрации
type RegisterRequest struct {
	Username  string `json:"username" binding:"required,min=3,max=64"`
	Password  string `json:"password" binding:"required,min=6,max=128"`
	Email     string `json:"email" binding:"required,email"`
	Name      string `json:"name" binding:"required,min=1,max=255"`
	CompanyID string `json:"company_id" binding:"required,uuid"`
	Role      string `json:"role" binding:"required"`
}

// Структурированное логирование для локальной авторизации
func logLocalAuthOperation(operation, username, userID, companyID string, details map[string]interface{}) {
	logData := map[string]interface{}{
		"timestamp":  time.Now().Format(time.RFC3339),
		"operation":  operation,
		"username":   username,
		"user_id":    userID,
		"company_id": companyID,
		"auth_type":  "local",
	}

	for key, value := range details {
		logData[key] = value
	}

	logJSON, _ := json.Marshal(logData)
	log.Printf("LOCAL_AUTH_LOG: %s", string(logJSON))
}

// LocalLogin обрабатывает локальный вход
func (api *LocalAuthAPI) LocalLogin(c *gin.Context) {
	var req LocalLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logLocalAuthOperation("login_validation_error", req.Username, "", "", map[string]interface{}{
			"error":      err.Error(),
			"status":     "failed",
			"ip_address": c.ClientIP(),
		})
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат запроса",
		})
		return
	}

	logLocalAuthOperation("login_attempt", req.Username, "", "", map[string]interface{}{
		"ip_address": c.ClientIP(),
		"user_agent": c.GetHeader("User-Agent"),
	})

	// Находим пользователя в схеме public
	var user models.LocalUser
	publicDB := api.getPublicDB()
	if err := publicDB.Where("username = ? AND is_active = true", req.Username).First(&user).Error; err != nil {
		// Пользователь не найден в локальной базе - пробуем Axenta Cloud
		logLocalAuthOperation("login_user_not_found_trying_axenta", req.Username, "", "", map[string]interface{}{
			"status": "attempting_axenta_auth",
		})

		// Пробуем авторизацию через Axenta Cloud
		axentaUser, _, axentaErr := api.tryAxentaAuth(req.Username, req.Password)
		if axentaErr != nil {
			logLocalAuthOperation("login_axenta_failed", req.Username, "", "", map[string]interface{}{
				"error":  axentaErr.Error(),
				"status": "failed",
			})
			c.JSON(http.StatusUnauthorized, gin.H{
				"status": "error",
				"error":  "Неверные учетные данные",
			})
			return
		}

		// Проверяем тип аккаунта - доступ только для партнеров
		if axentaUser.AccountType != "partner" {
			logLocalAuthOperation("login_access_denied", req.Username, "", "", map[string]interface{}{
				"status":       "access_denied",
				"account_type": axentaUser.AccountType,
				"reason":       "only_partners_allowed",
			})
			c.JSON(http.StatusForbidden, gin.H{
				"status": "error",
				"error":  "Доступ к CRM разрешен только партнерам Axenta",
				"details": gin.H{
					"account_type":  axentaUser.AccountType,
					"required_type": "partner",
				},
			})
			return
		}

		// Axenta авторизация успешна - создаем локального пользователя
		role := "user" // По умолчанию обычный пользователь
		if axentaUser.IsAdmin {
			role = "admin" // Если в Axenta админ, то и у нас админ
		}

		newUser := models.LocalUser{
			Username:  req.Username,
			Email:     axentaUser.Email,
			Name:      axentaUser.Name,
			CompanyID: fmt.Sprintf("axenta-%s-%d", axentaUser.AccountType, axentaUser.AccountID),
			Role:      role,
			IsActive:  axentaUser.IsActive,
		}

		// Устанавливаем пароль
		if err := newUser.SetPassword(req.Password); err != nil {
			logLocalAuthOperation("login_password_hash_error", req.Username, "", "", map[string]interface{}{
				"error":  err.Error(),
				"status": "failed",
			})
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Ошибка создания пользователя",
			})
			return
		}

		// Сохраняем в БД
		log.Printf("🔍 DB_DEBUG: Attempting to create user: %+v", newUser)
		if err := publicDB.Create(&newUser).Error; err != nil {
			log.Printf("🔍 DB_ERROR: Ошибка создания пользователя: %v", err)
			logLocalAuthOperation("login_user_creation_error", req.Username, "", "", map[string]interface{}{
				"error":  err.Error(),
				"status": "failed",
			})
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Ошибка создания пользователя",
			})
			return
		}
		log.Printf("🔍 DB_SUCCESS: User created with ID: %d", newUser.ID)

		user = newUser // Используем созданного пользователя

		logLocalAuthOperation("login_user_auto_created", req.Username, fmt.Sprintf("%d", user.ID), user.CompanyID, map[string]interface{}{
			"status":      "success",
			"axenta_auth": true,
		})
	}

	// Для существующих пользователей тоже проверяем тип аккаунта в Axenta
	if user.ID != 0 { // Пользователь уже существует
		// Проверяем актуальный статус в Axenta Cloud
		axentaUser, _, axentaErr := api.tryAxentaAuth(req.Username, req.Password)
		if axentaErr != nil {
			logLocalAuthOperation("login_axenta_recheck_failed", req.Username, fmt.Sprintf("%d", user.ID), user.CompanyID, map[string]interface{}{
				"error":  axentaErr.Error(),
				"status": "failed",
			})
			c.JSON(http.StatusUnauthorized, gin.H{
				"status": "error",
				"error":  "Неверные учетные данные",
			})
			return
		}

		// Проверяем тип аккаунта для существующего пользователя
		if axentaUser.AccountType != "partner" {
			logLocalAuthOperation("login_access_denied_existing", req.Username, fmt.Sprintf("%d", user.ID), user.CompanyID, map[string]interface{}{
				"status":       "access_denied",
				"account_type": axentaUser.AccountType,
				"reason":       "only_partners_allowed",
				"user_exists":  true,
			})
			c.JSON(http.StatusForbidden, gin.H{
				"status": "error",
				"error":  "Доступ к CRM разрешен только партнерам Axenta",
				"details": gin.H{
					"account_type":  axentaUser.AccountType,
					"required_type": "partner",
				},
			})
			return
		}

		// Обновляем данные пользователя из Axenta (на случай изменений)
		user.Email = axentaUser.Email
		user.Name = axentaUser.Name
		if axentaUser.IsAdmin {
			user.Role = "admin"
		} else {
			user.Role = "user"
		}
		publicDB.Save(&user)
	} else {
		// Для новых пользователей проверяем пароль локально
		if !user.CheckPassword(req.Password) {
			logLocalAuthOperation("login_invalid_password", req.Username, fmt.Sprintf("%d", user.ID), user.CompanyID, map[string]interface{}{
				"status": "failed",
			})
			c.JSON(http.StatusUnauthorized, gin.H{
				"status": "error",
				"error":  "Неверные учетные данные",
			})
			return
		}
	}

	// Генерируем токены
	accessToken, refreshToken, err := api.jwtService.GenerateTokenPair(&user)
	if err != nil {
		logLocalAuthOperation("login_token_generation_error", req.Username, fmt.Sprintf("%d", user.ID), user.CompanyID, map[string]interface{}{
			"error":  err.Error(),
			"status": "failed",
		})
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка генерации токенов",
		})
		return
	}

	// Обновляем время последнего входа
	if err := user.UpdateLastLogin(api.db); err != nil {
		log.Printf("Failed to update last login for user %d: %v", user.ID, err)
	}

	logLocalAuthOperation("login_success", req.Username, fmt.Sprintf("%d", user.ID), user.CompanyID, map[string]interface{}{
		"status": "success",
		"role":   user.Role,
	})

	// Формируем ответ
	response := LocalLoginResponse{
		Status: "success",
	}
	response.Data.AccessToken = accessToken
	response.Data.RefreshToken = refreshToken
	response.Data.TokenType = "Bearer"
	response.Data.ExpiresIn = 3600 // 1 час в секундах
	response.Data.User = user.ToPublicUser()

	c.JSON(http.StatusOK, response)
}

// RefreshToken обновляет access токен
func (api *LocalAuthAPI) RefreshToken(c *gin.Context) {
	var req RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат запроса",
		})
		return
	}

	// Обновляем токен
	newAccessToken, err := api.jwtService.RefreshAccessToken(req.RefreshToken)
	if err != nil {
		logLocalAuthOperation("refresh_token_error", "", "", "", map[string]interface{}{
			"error":  err.Error(),
			"status": "failed",
		})
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Неверный или истекший refresh токен",
		})
		return
	}

	logLocalAuthOperation("refresh_token_success", "", "", "", map[string]interface{}{
		"status": "success",
	})

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"access_token": newAccessToken,
			"token_type":   "Bearer",
			"expires_in":   3600,
		},
	})
}

// LocalCurrentUser возвращает данные текущего пользователя
func (api *LocalAuthAPI) LocalCurrentUser(c *gin.Context) {
	userID, exists := middleware.GetCurrentUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Пользователь не найден в контексте",
		})
		return
	}

	// Получаем пользователя из БД (схема public)
	var user models.LocalUser
	publicDB := api.getPublicDB()
	if err := publicDB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Пользователь не найден",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   user.ToPublicUser(),
	})
}

// LocalLogout выход из системы (отзыв refresh токена)
func (api *LocalAuthAPI) LocalLogout(c *gin.Context) {
	var req RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Если нет refresh токена в запросе, просто возвращаем успех
		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"message": "Logged out successfully",
		})
		return
	}

	// Отзываем refresh токен
	if err := api.jwtService.RevokeRefreshToken(req.RefreshToken); err != nil {
		log.Printf("Failed to revoke refresh token: %v", err)
	}

	if userID, exists := middleware.GetCurrentUserID(c); exists {
		logLocalAuthOperation("logout", "", fmt.Sprintf("%d", userID), "", map[string]interface{}{
			"status": "success",
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Logged out successfully",
	})
}

// RegisterLocalUser регистрирует нового локального пользователя (только для админов)
func (api *LocalAuthAPI) RegisterLocalUser(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат запроса",
		})
		return
	}

	// Проверяем валидность роли
	if !models.IsValidRole(req.Role) {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid role",
		})
		return
	}

	// Проверяем, что пользователь с таким username не существует (схема public)
	var existingUser models.LocalUser
	publicDB := api.getPublicDB()
	if err := publicDB.Where("username = ?", req.Username).First(&existingUser).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"status": "error",
			"error":  "Имя пользователя уже существует",
		})
		return
	}

	// Создаем нового пользователя
	user := models.LocalUser{
		Username:  req.Username,
		Email:     req.Email,
		Name:      req.Name,
		CompanyID: req.CompanyID,
		Role:      req.Role,
		IsActive:  true,
	}

	// Хешируем пароль
	if err := user.SetPassword(req.Password); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to hash password",
		})
		return
	}

	// Сохраняем в БД (схема public)
	if err := publicDB.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка создания пользователя",
		})
		return
	}

	logLocalAuthOperation("user_registered", req.Username, fmt.Sprintf("%d", user.ID), req.CompanyID, map[string]interface{}{
		"status": "success",
		"role":   req.Role,
	})

	c.JSON(http.StatusCreated, gin.H{
		"status": "success",
		"data":   user.ToPublicUser(),
	})
}

// RegisterRoutes регистрирует маршруты для локальной авторизации
func (api *LocalAuthAPI) RegisterRoutes(router *gin.RouterGroup) {
	// Публичные маршруты
	router.POST("/local/login", api.LocalLogin)
	router.POST("/local/refresh", api.RefreshToken)

	// Защищенные маршруты
	authMiddleware := middleware.NewLocalAuthMiddleware(api.jwtService)
	protected := router.Group("")
	protected.Use(authMiddleware.RequireAuth())
	{
		protected.GET("/local/current_user", api.LocalCurrentUser)
		protected.POST("/local/logout", api.LocalLogout)

		// Только для админов
		adminOnly := protected.Group("")
		adminOnly.Use(authMiddleware.RequireRole(models.RoleAdmin))
		{
			adminOnly.POST("/local/register", api.RegisterLocalUser)
		}
	}
}

// AxentaUserData структура для данных пользователя от Axenta
type AxentaUserData struct {
	Username    string `json:"username"`
	Email       string `json:"email"`
	Name        string `json:"name"`
	ID          int    `json:"id"`
	AccountID   int    `json:"accountId"`
	AccountName string `json:"accountName"`
	AccountType string `json:"accountType"`
	IsAdmin     bool   `json:"isAdmin"`
	IsActive    bool   `json:"isActive"`
	Language    string `json:"language"`
	Timezone    int    `json:"timezone"`
}

// tryAxentaAuth пытается авторизоваться через Axenta Cloud
func (api *LocalAuthAPI) tryAxentaAuth(username, password string) (*AxentaUserData, string, error) {
	// Создаем HTTP клиент
	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	// Подготавливаем данные для запроса
	loginData := map[string]string{
		"username": username,
		"password": password,
	}

	jsonData, err := json.Marshal(loginData)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal login data: %v", err)
	}

	// Делаем запрос к Axenta Cloud
	resp, err := client.Post("https://axenta.cloud/api/auth/login/", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, "", fmt.Errorf("failed to connect to Axenta Cloud: %v", err)
	}
	defer resp.Body.Close()

	// Читаем ответ
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response: %v", err)
	}

	// Логируем ответ от Axenta для диагностики
	log.Printf("🔍 AXENTA_DEBUG: Status=%d, Body=%s", resp.StatusCode, string(body))

	// Проверяем статус ответа
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("Axenta auth failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Парсим ответ
	var axentaResponse map[string]interface{}
	if err := json.Unmarshal(body, &axentaResponse); err != nil {
		return nil, "", fmt.Errorf("failed to parse Axenta response: %v", err)
	}

	log.Printf("🔍 AXENTA_PARSED: %+v", axentaResponse)

	// Извлекаем токен
	var token string
	if accessToken, ok := axentaResponse["access"].(string); ok {
		token = accessToken
	} else if tokenField, ok := axentaResponse["token"].(string); ok {
		token = tokenField
	} else {
		return nil, "", fmt.Errorf("no token found in Axenta response")
	}

	// Получаем информацию о пользователе от Axenta Cloud
	userData, err := api.getAxentaUserInfo(token)
	if err != nil {
		// Если не удалось получить данные пользователя, используем базовые
		log.Printf("⚠️ Failed to get user info from Axenta, using basic data: %v", err)
		userData = &AxentaUserData{
			Username: username,
			Email:    username + "@axenta.cloud",
			Name:     username,
			ID:       0,
			IsAdmin:  false,
			IsActive: true,
			Language: "ru",
			Timezone: 3,
		}
	}

	log.Printf("🔍 AXENTA_USER_DATA: %+v", userData)

	return userData, token, nil
}

// getAxentaUserInfo получает информацию о пользователе от Axenta Cloud
func (api *LocalAuthAPI) getAxentaUserInfo(token string) (*AxentaUserData, error) {
	// Создаем HTTP клиент
	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	// Создаем запрос к Axenta для получения информации о пользователе
	req, err := http.NewRequest("GET", "https://axenta.cloud/api/current_user/", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Добавляем токен авторизации
	req.Header.Set("Authorization", "Token "+token)
	req.Header.Set("Content-Type", "application/json")

	// Выполняем запрос
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info from Axenta: %v", err)
	}
	defer resp.Body.Close()

	// Читаем ответ
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read user info response: %v", err)
	}

	log.Printf("🔍 AXENTA_USER_INFO: Status=%d, Body=%s", resp.StatusCode, string(body))

	// Проверяем статус ответа
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get user info, status %d: %s", resp.StatusCode, string(body))
	}

	// Парсим ответ
	var userData AxentaUserData
	if err := json.Unmarshal(body, &userData); err != nil {
		return nil, fmt.Errorf("failed to parse user info: %v", err)
	}

	return &userData, nil
}
