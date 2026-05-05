package api

import (
	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"
	"backend_axenta/services"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// getAccountIDFromToken получает account_id из токена через Axenta API
func getAccountIDFromToken(token string) uint {
	if token == "" {
		return 0
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("GET", "https://axenta.cloud/api/current_user/", nil)
	if err != nil {
		return 0
	}

	req.Header.Set("Authorization", "Token "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return 0
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0
	}

	// Парсим ответ для получения информации о пользователе
	var userData map[string]interface{}
	if err := json.Unmarshal(body, &userData); err != nil {
		return 0
	}

	// Ищем accountId в ответе
	if accountID, ok := userData["accountId"].(float64); ok {
		return uint(accountID)
	}

	if accountID, ok := userData["accountId"].(string); ok {
		if id, err := strconv.ParseUint(accountID, 10, 32); err == nil {
			return uint(id)
		}
	}

	return 0
}

// CmsUserCreateRequest представляет запрос на создание CMS пользователя
type CmsUserCreateRequest struct {
	Name             string                 `json:"name" binding:"required"`
	Username         string                 `json:"username" binding:"required"`
	Email            string                 `json:"email" binding:"required,email"`
	Password         string                 `json:"password" binding:"required,min=6"`
	HasAdminAccess   bool                   `json:"hasAdminAccess"`
	VisibleTabsNames []string               `json:"visibleTabsNames"`
	Accesses         map[string]interface{} `json:"accesses"`
}

// CmsUserResponse представляет ответ с данными CMS пользователя
type CmsUserResponse struct {
	ID                      uint                   `json:"id"`
	Email                   string                 `json:"email"`
	Name                    string                 `json:"name"`
	Username                string                 `json:"username"`
	CreatorName             string                 `json:"creatorName"`
	LastLogin               *string                `json:"lastLogin"`
	CreationDatetime        string                 `json:"creationDatetime"`
	AccountID               string                 `json:"accountId"`
	AccountName             string                 `json:"accountName"`
	AccountType             string                 `json:"accountType"`
	AccountIsActive         bool                   `json:"accountIsActive"`
	AccountBlockingDatetime *string                `json:"accountBlockingDatetime"`
	IsActive                bool                   `json:"isActive"`
	Language                string                 `json:"language"`
	Timezone                int                    `json:"timezone"`
	IsAdmin                 bool                   `json:"isAdmin"`
	HasAdminAccess          bool                   `json:"hasAdminAccess"`
	VisibleTabsNames        []string               `json:"visibleTabsNames"`
	CurrentUserAccess       []string               `json:"currentUserAccess"`
	AddressFormat           []string               `json:"addressFormat"`
	ObjectCardSettings      map[string]interface{} `json:"objectCardSettings"`
	MonitoringItemSetup     map[string]interface{} `json:"monitoringItemSetup"`
	VisibleObjectsIds       []int                  `json:"visibleObjectsIds"`
	VisibleObjectsCount     int                    `json:"visibleObjectsCount"`
	VisibleGeozoneIds       []int                  `json:"visibleGeozoneIds"`
	CommonAccesses          []string               `json:"commonAccesses"`
}

// CreateCmsUser создает нового CMS пользователя
func CreateCmsUser(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	var req CmsUserCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  translateValidationError(err),
		})
		return
	}

	// Валидация обязательных полей
	if req.Name == "" || req.Username == "" || req.Email == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Fields name, username, email, password are required",
		})
		return
	}

	// Проверка длины пароля
	if len(req.Password) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Password must be at least 6 characters long",
		})
		return
	}

	// Проверка уникальности email и username
	var existingUser models.User
	if err := db.Where("email = ? OR username = ?", req.Email, req.Username).First(&existingUser).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"status": "error",
			"error":  "Пользователь с таким email или именем пользователя уже существует",
		})
		return
	}

	// Хешируем пароль
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to hash password",
		})
		return
	}

	// Получаем accountId из переменной окружения
	accountID := os.Getenv("AXENTA_DEFAULT_ACCOUNT_ID")
	if accountID == "" {
		accountID = "1" // Значение по умолчанию
	}

	// Начинаем транзакцию
	tx := db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Создаем пользователя
	user := models.User{
		Username:   req.Username,
		Email:      req.Email,
		Password:   string(hashedPassword),
		Name:       req.Name,
		IsActive:   true,
		UserType:   "cms_user",
		LoginCount: 0,
		CompanyID:  1, // Временно используем ID 1
		// RoleID не устанавливаем - будет NULL
	}

	// Отладочная информация
	fmt.Printf("Creating user with RoleID: %d\n", user.RoleID)

	if err := tx.Create(&user).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to create user: " + err.Error(),
		})
		return
	}

	// Создаем user_tabs
	for _, tabName := range req.VisibleTabsNames {
		userTab := models.UserTab{
			UserID: user.ID,
			Name:   tabName,
		}
		if err := tx.Create(&userTab).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Failed to create user tab: " + err.Error(),
			})
			return
		}
	}

	// Создаем user_accesses
	for scope, perms := range req.Accesses {
		permsJSON, err := json.Marshal(perms)
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Failed to serialize permissions: " + err.Error(),
			})
			return
		}

		userAccess := models.UserAccess{
			UserID: user.ID,
			Scope:  scope,
			Perms:  string(permsJSON),
		}
		if err := tx.Create(&userAccess).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Failed to create user access: " + err.Error(),
			})
			return
		}
	}

	// Завершаем транзакцию
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to commit transaction: " + err.Error(),
		})
		return
	}

	// Создаем пользователя в Axenta Cloud в фоновом режиме
	go func() {
		axentaService := services.NewAxentaUserService(database.DB)

		// Получаем токен из заголовка
		authHeader := c.GetHeader("Authorization")
		var token string
		if strings.HasPrefix(authHeader, "Token ") {
			token = strings.TrimPrefix(authHeader, "Token ")
		} else {
			fmt.Printf("Failed to get token from Authorization header for user %s\n", user.Username)
			return
		}

		// Преобразуем accesses в правильный формат
		accesses := make(map[string][]string)
		for scope, perms := range req.Accesses {
			if permsSlice, ok := perms.([]interface{}); ok {
				accesses[scope] = make([]string, len(permsSlice))
				for i, perm := range permsSlice {
					if permStr, ok := perm.(string); ok {
						accesses[scope][i] = permStr
					}
				}
			}
		}

		// Создаем пользователя в Axenta Cloud
		axentaUser, err := axentaService.CreateUserInAxenta(token, &user, req.VisibleTabsNames, accesses)
		if err != nil {
			fmt.Printf("Failed to create user %s in Axenta Cloud: %v\n", user.Username, err)

			// Обновляем пользователя в локальной базе, отмечая что синхронизация не удалась
			user.IsAxentaUser = false
			user.AxentaUserType = "local"
			database.DB.Save(&user)
		} else {
			fmt.Printf("Successfully created user %s in Axenta Cloud with ID: %d\n", user.Username, axentaUser.ID)

			// Обновляем пользователя в локальной базе, отмечая что он синхронизирован с Axenta
			user.IsAxentaUser = true
			user.AxentaUserType = "partner" // По умолчанию создаем как партнера
			user.AxentaUserID = fmt.Sprintf("%d", axentaUser.ID)
			user.ExternalSource = "axenta"
			user.ExternalID = fmt.Sprintf("%d", axentaUser.ID)
			database.DB.Save(&user)
		}
	}()

	// Формируем ответ
	response := CmsUserResponse{
		ID:                      user.ID,
		Email:                   user.Email,
		Name:                    user.Name,
		Username:                user.Username,
		CreatorName:             user.Name, // Используем имя пользователя как создателя
		LastLogin:               nil,       // Новый пользователь, последнего входа нет
		CreationDatetime:        user.CreatedAt.Format("2006-01-02T15:04:05Z"),
		AccountID:               accountID,
		AccountName:             "Default Account", // Значение по умолчанию
		AccountType:             "partner",
		AccountIsActive:         true,
		AccountBlockingDatetime: nil,
		IsActive:                user.IsActive,
		Language:                "ru",
		Timezone:                3, // UTC+3
		IsAdmin:                 req.HasAdminAccess,
		HasAdminAccess:          req.HasAdminAccess,
		VisibleTabsNames:        req.VisibleTabsNames,
		CurrentUserAccess:       []string{"view", "edit"}, // Доступ по умолчанию
		AddressFormat:           []string{},
		ObjectCardSettings:      map[string]interface{}{},
		MonitoringItemSetup:     map[string]interface{}{},
		VisibleObjectsIds:       []int{},
		VisibleObjectsCount:     0,
		VisibleGeozoneIds:       []int{},
		CommonAccesses:          []string{"view", "edit"},
	}

	c.JSON(http.StatusCreated, response)
}

// GetCmsUser возвращает данные CMS пользователя
func GetCmsUser(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	userID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid user ID",
		})
		return
	}

	var user models.User
	if err := db.First(&user, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"status": "error",
				"error":  "User not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to fetch user: " + err.Error(),
		})
		return
	}

	// Получаем вкладки пользователя
	var userTabs []models.UserTab
	if err := db.Where("user_id = ?", user.ID).Find(&userTabs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to fetch user tabs: " + err.Error(),
		})
		return
	}

	// Получаем доступы пользователя
	var userAccesses []models.UserAccess
	if err := db.Where("user_id = ?", user.ID).Find(&userAccesses).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to fetch user accesses: " + err.Error(),
		})
		return
	}

	// Формируем списки
	visibleTabsNames := make([]string, len(userTabs))
	for i, tab := range userTabs {
		visibleTabsNames[i] = tab.Name
	}

	// Получаем accountId из переменной окружения
	accountID := os.Getenv("AXENTA_DEFAULT_ACCOUNT_ID")
	if accountID == "" {
		accountID = "1"
	}

	// Формируем дату последнего входа
	var lastLoginFormatted *string
	if user.LastLogin != nil {
		formatted := user.LastLogin.Format("2006-01-02T15:04:05Z")
		lastLoginFormatted = &formatted
	}

	response := CmsUserResponse{
		ID:                      user.ID,
		Email:                   user.Email,
		Name:                    user.Name,
		Username:                user.Username,
		CreatorName:             user.Name,
		LastLogin:               lastLoginFormatted,
		CreationDatetime:        user.CreatedAt.Format("2006-01-02T15:04:05Z"),
		AccountID:               accountID,
		AccountName:             "Default Account",
		AccountType:             "partner",
		AccountIsActive:         true,
		AccountBlockingDatetime: nil,
		IsActive:                user.IsActive,
		Language:                "ru",
		Timezone:                3,
		IsAdmin:                 false, // Определить по user_accesses
		HasAdminAccess:          false, // Определить по user_accesses
		VisibleTabsNames:        visibleTabsNames,
		CurrentUserAccess:       []string{"view", "edit"},
		AddressFormat:           []string{},
		ObjectCardSettings:      map[string]interface{}{},
		MonitoringItemSetup:     map[string]interface{}{},
		VisibleObjectsIds:       []int{},
		VisibleObjectsCount:     0,
		VisibleGeozoneIds:       []int{},
		CommonAccesses:          []string{"view", "edit"},
	}

	// Определяем админские права по доступам
	for _, access := range userAccesses {
		if access.Scope == "admin" {
			var perms []string
			if err := json.Unmarshal([]byte(access.Perms), &perms); err == nil {
				for _, perm := range perms {
					if perm == "full" || perm == "admin" {
						response.IsAdmin = true
						response.HasAdminAccess = true
						break
					}
				}
			}
		}
	}

	c.JSON(http.StatusOK, response)
}

// TestCreateUserInAxenta тестовый endpoint для создания пользователя в Axenta Cloud без проверки токена
func TestCreateUserInAxenta(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	var req CmsUserCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  translateValidationError(err),
		})
		return
	}

	// Валидация обязательных полей
	if req.Name == "" || req.Username == "" || req.Email == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Fields name, username, email, password are required",
		})
		return
	}

	// Проверка длины пароля
	if len(req.Password) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Password must be at least 6 characters long",
		})
		return
	}

	// Хешируем пароль
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to hash password",
		})
		return
	}

	// Создаем пользователя в локальной базе
	user := models.User{
		Username:   req.Username,
		Email:      req.Email,
		Password:   string(hashedPassword),
		Name:       req.Name,
		IsActive:   true,
		UserType:   "cms_user",
		LoginCount: 0,
		CompanyID:  1,
	}

	if err := db.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to create user: " + err.Error(),
		})
		return
	}

	// Пытаемся создать пользователя в Axenta Cloud
	axentaService := services.NewAxentaUserService(db)

	// Преобразуем accesses в правильный формат
	accesses := make(map[string][]string)
	for scope, perms := range req.Accesses {
		if permsSlice, ok := perms.([]interface{}); ok {
			accesses[scope] = make([]string, len(permsSlice))
			for i, perm := range permsSlice {
				if permStr, ok := perm.(string); ok {
					accesses[scope][i] = permStr
				}
			}
		}
	}

	// Попробуем создать пользователя в Axenta Cloud с тестовым токеном
	testToken := "test-token-123"
	axentaUser, err := axentaService.CreateUserInAxenta(testToken, &user, req.VisibleTabsNames, accesses)

	var response gin.H
	if err != nil {
		fmt.Printf("Failed to create user %s in Axenta Cloud: %v\n", user.Username, err)

		// Обновляем пользователя в локальной базе, отмечая что синхронизация не удалась
		user.IsAxentaUser = false
		user.AxentaUserType = "local"
		db.Save(&user)

		response = gin.H{
			"status":  "partial_success",
			"message": "User created locally but failed to sync with Axenta Cloud",
			"data": gin.H{
				"user_id":        user.ID,
				"username":       user.Username,
				"email":          user.Email,
				"is_axenta_user": user.IsAxentaUser,
				"axenta_error":   err.Error(),
			},
		}
	} else {
		fmt.Printf("Successfully created user %s in Axenta Cloud with ID: %d\n", user.Username, axentaUser.ID)

		// Обновляем пользователя в локальной базе, отмечая что он синхронизирован с Axenta
		user.IsAxentaUser = true
		user.AxentaUserType = "partner"
		user.AxentaUserID = fmt.Sprintf("%d", axentaUser.ID)
		user.ExternalSource = "axenta"
		user.ExternalID = fmt.Sprintf("%d", axentaUser.ID)
		db.Save(&user)

		response = gin.H{
			"status":  "success",
			"message": "User created successfully in both local database and Axenta Cloud",
			"data": gin.H{
				"user_id":         user.ID,
				"username":        user.Username,
				"email":           user.Email,
				"is_axenta_user":  user.IsAxentaUser,
				"axenta_user_id":  axentaUser.ID,
				"axenta_response": axentaUser,
			},
		}
	}

	c.JSON(http.StatusOK, response)
}

// CreateUserDirectly создает пользователя напрямую в локальной базе (без проверки токена)
func CreateUserDirectly(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	var req CmsUserCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  translateValidationError(err),
		})
		return
	}

	// Валидация обязательных полей
	if req.Name == "" || req.Username == "" || req.Email == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Fields name, username, email, password are required",
		})
		return
	}

	// Проверка длины пароля
	if len(req.Password) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Password must be at least 6 characters long",
		})
		return
	}

	// Проверка уникальности email и username
	var existingUser models.User
	if err := db.Where("email = ? OR username = ?", req.Email, req.Username).First(&existingUser).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"status": "error",
			"error":  "Пользователь с таким email или именем пользователя уже существует",
		})
		return
	}

	// Хешируем пароль
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to hash password",
		})
		return
	}

	// Начинаем транзакцию
	tx := db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Создаем пользователя
	user := models.User{
		Username:   req.Username,
		Email:      req.Email,
		Password:   string(hashedPassword),
		Name:       req.Name,
		IsActive:   true,
		UserType:   "cms_user",
		LoginCount: 0,
		CompanyID:  1,
	}

	if err := tx.Create(&user).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to create user: " + err.Error(),
		})
		return
	}

	// Создаем user_tabs
	for _, tabName := range req.VisibleTabsNames {
		userTab := models.UserTab{
			UserID: user.ID,
			Name:   tabName,
		}
		if err := tx.Create(&userTab).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Failed to create user tab: " + err.Error(),
			})
			return
		}
	}

	// Создаем user_accesses
	for scope, perms := range req.Accesses {
		permsJSON, err := json.Marshal(perms)
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Failed to marshal permissions: " + err.Error(),
			})
			return
		}

		userAccess := models.UserAccess{
			UserID: user.ID,
			Scope:  scope,
			Perms:  string(permsJSON),
		}
		if err := tx.Create(&userAccess).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Failed to create user access: " + err.Error(),
			})
			return
		}
	}

	// Завершаем транзакцию
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to commit transaction: " + err.Error(),
		})
		return
	}

	// Формируем ответ
	response := CmsUserResponse{
		ID:                      user.ID,
		Email:                   user.Email,
		Name:                    user.Name,
		Username:                user.Username,
		CreatorName:             user.Name,
		LastLogin:               nil,
		CreationDatetime:        user.CreatedAt.Format("2006-01-02T15:04:05Z"),
		AccountID:               "1",
		AccountName:             "Default Account",
		AccountType:             "partner",
		AccountIsActive:         true,
		AccountBlockingDatetime: nil,
		IsActive:                user.IsActive,
		Language:                "ru",
		Timezone:                3,
		IsAdmin:                 req.HasAdminAccess,
		HasAdminAccess:          req.HasAdminAccess,
		VisibleTabsNames:        req.VisibleTabsNames,
		CurrentUserAccess:       []string{"view", "edit"},
		AddressFormat:           []string{},
		ObjectCardSettings:      map[string]interface{}{},
		MonitoringItemSetup:     map[string]interface{}{},
		VisibleObjectsIds:       []int{},
		VisibleObjectsCount:     0,
		VisibleGeozoneIds:       []int{},
		CommonAccesses:          []string{"view", "edit"},
	}

	c.JSON(http.StatusCreated, response)
}

// CreateCmsUserWithCurrentToken создает пользователя CMS используя токен текущего авторизованного пользователя
func CreateCmsUserWithCurrentToken(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	var req CmsUserCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  translateValidationError(err),
		})
		return
	}

	// Валидация обязательных полей
	if req.Name == "" || req.Username == "" || req.Email == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Fields name, username, email, password are required",
		})
		return
	}

	// Проверка длины пароля
	if len(req.Password) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Password must be at least 6 characters long",
		})
		return
	}

	// Проверка уникальности email и username
	var existingUser models.User
	if err := db.Where("email = ? OR username = ?", req.Email, req.Username).First(&existingUser).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"status": "error",
			"error":  "Пользователь с таким email или именем пользователя уже существует",
		})
		return
	}

	// Хешируем пароль
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to hash password",
		})
		return
	}

	// Получаем токен из заголовка Authorization для идентификации пользователя
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Authorization header is required",
		})
		return
	}

	// Извлекаем токен из заголовка "Token <token>"
	var requestToken string
	if strings.HasPrefix(authHeader, "Token ") {
		requestToken = strings.TrimPrefix(authHeader, "Token ")
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Invalid authorization header format. Expected 'Token <token>'",
		})
		return
	}

	// Получаем account_id из токена для фильтрации
	accountID := getAccountIDFromToken(requestToken)

	// Создаем сервис для работы с токенами пользователей
	userTokenService := services.NewUserTokenService(db)

	// Находим пользователя по токену из заголовка с фильтрацией по account_id
	var currentUser models.User
	if accountID > 0 {
		if err := db.Where("id IN (SELECT user_id FROM user_tokens WHERE token = ? AND is_active = ? AND account_id = ?)", requestToken, true, accountID).First(&currentUser).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status": "error",
				"error":  "User not found or token invalid",
			})
			return
		}
	} else {
		if err := db.Where("id IN (SELECT user_id FROM user_tokens WHERE token = ? AND is_active = ?)", requestToken, true).First(&currentUser).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status": "error",
				"error":  "User not found or token invalid",
			})
			return
		}
	}

	// Получаем сохраненный токен пользователя для Axenta Cloud
	userToken, err := userTokenService.GetUserToken(currentUser.ID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "No valid Axenta token found for user",
		})
		return
	}

	// Используем сохраненный токен для создания пользователя в Axenta Cloud
	currentToken := userToken.Token

	// Получаем accountId из переменной окружения (используем для создания пользователя в Axenta)
	defaultAccountID := os.Getenv("AXENTA_DEFAULT_ACCOUNT_ID")
	if defaultAccountID == "" {
		defaultAccountID = "1" // Значение по умолчанию
	}

	// Начинаем транзакцию
	tx := db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Создаем пользователя
	user := models.User{
		Username:   req.Username,
		Email:      req.Email,
		Password:   string(hashedPassword),
		Name:       req.Name,
		IsActive:   true,
		UserType:   "cms_user",
		LoginCount: 0,
		CompanyID:  1,
	}

	if err := tx.Create(&user).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to create user: " + err.Error(),
		})
		return
	}

	// Создаем user_tabs
	for _, tabName := range req.VisibleTabsNames {
		userTab := models.UserTab{
			UserID: user.ID,
			Name:   tabName,
		}
		if err := tx.Create(&userTab).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Failed to create user tab: " + err.Error(),
			})
			return
		}
	}

	// Создаем user_accesses
	for scope, perms := range req.Accesses {
		permsJSON, err := json.Marshal(perms)
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Failed to marshal permissions: " + err.Error(),
			})
			return
		}

		userAccess := models.UserAccess{
			UserID: user.ID,
			Scope:  scope,
			Perms:  string(permsJSON),
		}
		if err := tx.Create(&userAccess).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Failed to create user access: " + err.Error(),
			})
			return
		}
	}

	// Завершаем транзакцию
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to commit transaction: " + err.Error(),
		})
		return
	}

	// Создаем пользователя в Axenta Cloud в фоновом режиме
	go func() {
		axentaService := services.NewAxentaUserService(database.DB)

		// Преобразуем accesses в правильный формат
		accesses := make(map[string][]string)
		for scope, perms := range req.Accesses {
			if permsSlice, ok := perms.([]interface{}); ok {
				accesses[scope] = make([]string, len(permsSlice))
				for i, perm := range permsSlice {
					if permStr, ok := perm.(string); ok {
						accesses[scope][i] = permStr
					}
				}
			}
		}

		// Создаем пользователя в Axenta Cloud
		axentaUser, err := axentaService.CreateUserInAxenta(currentToken, &user, req.VisibleTabsNames, accesses)
		if err != nil {
			fmt.Printf("Failed to create user %s in Axenta Cloud: %v\n", user.Username, err)

			// Обновляем пользователя в локальной базе, отмечая что синхронизация не удалась
			user.IsAxentaUser = false
			user.AxentaUserType = "local"
			database.DB.Save(&user)
		} else {
			fmt.Printf("Successfully created user %s in Axenta Cloud with ID: %d\n", user.Username, axentaUser.ID)

			// Обновляем пользователя в локальной базе, отмечая что он синхронизирован с Axenta
			user.IsAxentaUser = true
			user.AxentaUserType = "partner"
			user.AxentaUserID = fmt.Sprintf("%d", axentaUser.ID)
			user.ExternalSource = "axenta"
			user.ExternalID = fmt.Sprintf("%d", axentaUser.ID)
			database.DB.Save(&user)
		}
	}()

	// Формируем ответ
	response := CmsUserResponse{
		ID:                      user.ID,
		Email:                   user.Email,
		Name:                    user.Name,
		Username:                user.Username,
		CreatorName:             user.Name,
		LastLogin:               nil,
		CreationDatetime:        user.CreatedAt.Format("2006-01-02T15:04:05Z"),
		AccountID:               defaultAccountID,
		AccountName:             "Default Account",
		AccountType:             "partner",
		AccountIsActive:         true,
		AccountBlockingDatetime: nil,
		IsActive:                user.IsActive,
		Language:                "ru",
		Timezone:                3,
		IsAdmin:                 req.HasAdminAccess,
		HasAdminAccess:          req.HasAdminAccess,
		VisibleTabsNames:        req.VisibleTabsNames,
		CurrentUserAccess:       []string{"view", "edit"},
		AddressFormat:           []string{},
		ObjectCardSettings:      map[string]interface{}{},
		MonitoringItemSetup:     map[string]interface{}{},
		VisibleObjectsIds:       []int{},
		VisibleObjectsCount:     0,
		VisibleGeozoneIds:       []int{},
		CommonAccesses:          []string{"view", "edit"},
	}

	c.JSON(http.StatusCreated, response)
}

// TestEndpoint тестовый endpoint
func TestEndpoint(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Test endpoint is working",
	})
}

// CreateUserInAxentaCloud создает пользователя в Axenta Cloud используя токен администратора
func CreateUserInAxentaCloud(c *gin.Context) {
	var req CmsUserCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  translateValidationError(err),
		})
		return
	}

	// Валидация обязательных полей
	if req.Name == "" || req.Username == "" || req.Email == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Fields name, username, email, password are required",
		})
		return
	}

	// Проверка длины пароля
	if len(req.Password) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Password must be at least 6 characters long",
		})
		return
	}

	// Получаем токен из заголовка Authorization
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Authorization header is required",
		})
		return
	}

	// Извлекаем токен из заголовка "Token <token>"
	var adminToken string
	if strings.HasPrefix(authHeader, "Token ") {
		adminToken = strings.TrimPrefix(authHeader, "Token ")
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Invalid authorization header format. Expected 'Token <token>'",
		})
		return
	}

	// Создаем пользователя в Axenta Cloud напрямую
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Подготавливаем данные для Axenta Cloud
	userData := map[string]interface{}{
		"name":             req.Name,
		"username":         req.Username,
		"email":            req.Email,
		"password":         req.Password,
		"confirmPassword":  req.Password,
		"language":         "ru",
		"timezone":         3,
		"visibleTabsNames": req.VisibleTabsNames,
		"accesses": map[string]interface{}{
			"common": map[string]interface{}{},
		},
	}

	jsonData, err := json.Marshal(userData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to marshal request data",
		})
		return
	}

	axentaURL := "https://axenta.cloud/api/users/"
	httpReq, err := http.NewRequest("POST", axentaURL, bytes.NewBuffer(jsonData))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to create request to Axenta Cloud",
		})
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Token "+adminToken)

	resp, err := client.Do(httpReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to connect to Axenta Cloud",
		})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to read response from Axenta Cloud",
		})
		return
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		c.JSON(resp.StatusCode, gin.H{
			"status":  "error",
			"error":   "Axenta Cloud request failed",
			"details": string(body),
		})
		return
	}

	var axentaResponse map[string]interface{}
	if err := json.Unmarshal(body, &axentaResponse); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to parse Axenta Cloud response",
		})
		return
	}

	// Триггерим резинк snapshot'ов чтобы новый юзер появился в /unified/users
	// и в creator-полях /unified/objects без ожидания scheduled cron.
	if adminID, err := middleware.GetAdminAccountID(c); err == nil {
		services.GetSnapshotInvalidator().Invalidate(adminID, "user.create")
	}

	// Возвращаем успешный ответ
	c.JSON(http.StatusCreated, gin.H{
		"status":  "success",
		"message": "User created successfully in Axenta Cloud",
		"data":    axentaResponse,
	})
}

// CreateCmsUserWithAdminToken создает пользователя CMS используя токен администратора
func CreateCmsUserWithAdminToken(c *gin.Context) {
	var req CmsUserCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  translateValidationError(err),
		})
		return
	}

	// Валидация обязательных полей
	if req.Name == "" || req.Username == "" || req.Email == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Fields name, username, email, password are required",
		})
		return
	}

	// Проверка длины пароля
	if len(req.Password) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Password must be at least 6 characters long",
		})
		return
	}

	// Получаем токен из заголовка Authorization
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Authorization header is required",
		})
		return
	}

	// Извлекаем токен из заголовка "Token <token>"
	var adminToken string
	if strings.HasPrefix(authHeader, "Token ") {
		adminToken = strings.TrimPrefix(authHeader, "Token ")
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Invalid authorization header format. Expected 'Token <token>'",
		})
		return
	}

	// Преобразуем accesses в правильный формат
	accesses := make(map[string][]string)
	for scope, perms := range req.Accesses {
		if permsSlice, ok := perms.([]interface{}); ok {
			accesses[scope] = make([]string, len(permsSlice))
			for i, perm := range permsSlice {
				if permStr, ok := perm.(string); ok {
					accesses[scope][i] = permStr
				}
			}
		}
	}

	// Создаем пользователя в Axenta Cloud напрямую
	axentaService := services.NewAxentaUserService(database.DB)

	// Создаем временный объект пользователя для передачи в сервис
	userData := &models.User{
		Username: req.Username,
		Email:    req.Email,
		Name:     req.Name,
		Password: req.Password, // Пароль будет захеширован в сервисе
	}

	// Создаем пользователя в Axenta Cloud
	axentaUser, err := axentaService.CreateUserInAxenta(adminToken, userData, req.VisibleTabsNames, accesses)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Failed to create user in Axenta Cloud: %v", err),
		})
		return
	}

	// Возвращаем успешный ответ
	c.JSON(http.StatusCreated, gin.H{
		"status":  "success",
		"message": "User created successfully in Axenta Cloud",
		"data": gin.H{
			"id":          axentaUser.ID,
			"username":    axentaUser.Username,
			"email":       axentaUser.Email,
			"name":        axentaUser.Name,
			"accountName": axentaUser.AccountName,
			"accountType": axentaUser.AccountType,
			"creatorName": axentaUser.CreatorName,
			"lastLogin":   axentaUser.LastLogin,
			"accountId":   axentaUser.AccountID,
		},
	})
}

// CmsLoginAsRequest структура запроса для входа как другой пользователь
type CmsLoginAsRequest struct {
	UserID int    `json:"userId" binding:"required"`
	Type   string `json:"type" binding:"required,oneof=monitoring cms"`
}

// CmsLoginAsResponse структура ответа для входа как другой пользователь
type CmsLoginAsResponse struct {
	RedirectURL string `json:"redirectUrl"`
}

// LoginAs позволяет войти как другой пользователь CMS
func LoginAs(c *gin.Context) {
	var req CmsLoginAsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid request format",
		})
		return
	}

	// Получаем токен из заголовка Authorization
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Authorization header is required",
		})
		return
	}

	// Извлекаем токен из заголовка "Token <token>"
	var requestToken string
	if strings.HasPrefix(authHeader, "Token ") {
		requestToken = strings.TrimPrefix(authHeader, "Token ")
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Invalid authorization header format. Expected 'Token <token>'",
		})
		return
	}

	// Получаем базу данных
	db := database.DB

	// Получаем account_id из токена для фильтрации
	accountID := getAccountIDFromToken(requestToken)

	// Создаем сервис для работы с токенами пользователей
	userTokenService := services.NewUserTokenService(db)

	// Находим пользователя по токену из заголовка с фильтрацией по account_id
	var currentUser models.User
	var err error
	if accountID > 0 {
		err = db.Where("id IN (SELECT user_id FROM user_tokens WHERE token = ? AND is_active = ? AND account_id = ?)", requestToken, true, accountID).First(&currentUser).Error
	} else {
		err = db.Where("id IN (SELECT user_id FROM user_tokens WHERE token = ? AND is_active = ?)", requestToken, true).First(&currentUser).Error
	}
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "User not found or token invalid",
		})
		return
	}

	// Получаем сохраненный токен пользователя для Axenta Cloud
	userToken, err := userTokenService.GetUserToken(currentUser.ID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "No valid Axenta token found for user",
		})
		return
	}

	// Используем сохраненный токен для запроса к Axenta Cloud
	axentaToken := userToken.Token

	// Отправляем запрос к Axenta Cloud для получения токена другого пользователя
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	loginAsData := map[string]interface{}{
		"userId": req.UserID,
		"type":   req.Type,
	}

	jsonData, err := json.Marshal(loginAsData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to marshal request data",
		})
		return
	}

	axentaURL := "https://axenta.cloud/api/cms/users/login_as/"
	httpReq, err := http.NewRequest("POST", axentaURL, bytes.NewBuffer(jsonData))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to create request to Axenta Cloud",
		})
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Token "+axentaToken)

	resp, err := client.Do(httpReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to connect to Axenta Cloud",
		})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to read response from Axenta Cloud",
		})
		return
	}

	if resp.StatusCode != http.StatusOK {
		c.JSON(resp.StatusCode, gin.H{
			"status":  "error",
			"error":   "Axenta Cloud request failed",
			"details": string(body),
		})
		return
	}

	var axentaResponse map[string]interface{}
	if err := json.Unmarshal(body, &axentaResponse); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to parse Axenta Cloud response",
		})
		return
	}

	// Извлекаем redirectUrl из ответа
	redirectURL, ok := axentaResponse["redirectUrl"].(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "No redirectUrl in Axenta Cloud response",
		})
		return
	}

	// Возвращаем ответ
	response := CmsLoginAsResponse{
		RedirectURL: redirectURL,
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   response,
	})
}
