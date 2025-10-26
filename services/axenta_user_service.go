package services

import (
	"backend_axenta/models"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"gorm.io/gorm"
)

// AxentaUserService управляет пользователями из Axenta API
type AxentaUserService struct {
	db         *gorm.DB
	httpClient *http.Client
	baseURL    string
}

// AxentaUserResponse представляет ответ от Axenta API с информацией о пользователе
type AxentaUserResponse struct {
	ID                      int     `json:"id"`
	Username                string  `json:"username"`
	Name                    string  `json:"name"`
	Email                   string  `json:"email"`
	AccountName             string  `json:"accountName"`
	AccountType             string  `json:"accountType"` // partner, client
	CreatorName             string  `json:"creatorName"`
	LastLogin               string  `json:"lastLogin"`
	AccountBlockingDatetime *string `json:"accountBlockingDatetime"`
	AccountID               int     `json:"accountId"`
	IsAdmin                 bool    `json:"isAdmin"`
	IsActive                bool    `json:"isActive"`
	Language                string  `json:"language"`
	Timezone                int     `json:"timezone"`
}

// NewAxentaUserService создает новый экземпляр сервиса
func NewAxentaUserService(db *gorm.DB) *AxentaUserService {
	return &AxentaUserService{
		db: db,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: "https://axenta.cloud/api",
	}
}

// GetDB возвращает экземпляр базы данных (для использования в других пакетах)
func (s *AxentaUserService) GetDB() *gorm.DB {
	return s.db
}

// GetUserFromAxenta получает информацию о пользователе из Axenta API
func (s *AxentaUserService) GetUserFromAxenta(token string) (*AxentaUserResponse, error) {
	url := fmt.Sprintf("%s/current_user/", s.baseURL)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Token "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request to Axenta API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Axenta API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var axentaUser AxentaUserResponse
	if err := json.Unmarshal(body, &axentaUser); err != nil {
		return nil, fmt.Errorf("failed to parse Axenta user response: %w", err)
	}

	return &axentaUser, nil
}

// SyncUserWithAxenta синхронизирует пользователя с данными из Axenta
func (s *AxentaUserService) SyncUserWithAxenta(token string, username string) (*models.User, error) {
	// Получаем данные пользователя из Axenta
	axentaUser, err := s.GetUserFromAxenta(token)
	if err != nil {
		log.Printf("Failed to get user from Axenta: %v", err)
		return nil, err
	}

	// Ищем существующего пользователя в базе данных
	var user models.User
	err = s.db.Where("username = ? OR axenta_user_id = ?", username, fmt.Sprintf("%d", axentaUser.ID)).First(&user).Error

	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	// Если пользователь не найден, создаем нового
	if err == gorm.ErrRecordNotFound {
		user = models.User{
			Username:  axentaUser.Username,
			Email:     axentaUser.Email,
			FirstName: axentaUser.Name,
			Name:      axentaUser.Name,
			IsActive:  axentaUser.IsActive,
			Password:  "axenta_user", // Пароль не используется для Axenta пользователей
		}
	}

	// Обновляем данные пользователя из Axenta
	user.SetAxentaRole(s.mapAccountTypeToUserType(axentaUser.AccountType), fmt.Sprintf("%d", axentaUser.ID))
	user.Email = axentaUser.Email
	user.Name = axentaUser.Name
	user.FirstName = axentaUser.Name
	user.IsActive = axentaUser.IsActive

	// Устанавливаем роль в системе на основе типа аккаунта Axenta
	roleID, err := s.getRoleIDForAxentaUserType(axentaUser.AccountType)
	if err != nil {
		log.Printf("Failed to get role for Axenta user type %s: %v", axentaUser.AccountType, err)
		// Используем роль по умолчанию
		roleID = s.getDefaultRoleID()
	}
	user.RoleID = &roleID

	// Сохраняем пользователя
	if user.ID == 0 {
		// Создаем нового пользователя
		if err := s.db.Create(&user).Error; err != nil {
			return nil, fmt.Errorf("failed to create user: %w", err)
		}
		log.Printf("Created new Axenta user: %s (type: %s)", user.Username, user.AxentaUserType)
	} else {
		// Обновляем существующего пользователя
		if err := s.db.Save(&user).Error; err != nil {
			return nil, fmt.Errorf("failed to update user: %w", err)
		}
		log.Printf("Updated Axenta user: %s (type: %s)", user.Username, user.AxentaUserType)
	}

	// Загружаем роль для возврата
	if err := s.db.Preload("Role").First(&user, user.ID).Error; err != nil {
		log.Printf("Failed to load user role: %v", err)
	}

	return &user, nil
}

// MapAccountTypeToUserType преобразует тип аккаунта Axenta в тип пользователя системы (публичный метод)
func (s *AxentaUserService) MapAccountTypeToUserType(accountType string) string {
	return s.mapAccountTypeToUserType(accountType)
}

// GetRoleIDForAxentaUserType возвращает ID роли для типа пользователя Axenta (публичный метод)
func (s *AxentaUserService) GetRoleIDForAxentaUserType(accountType string) (uint, error) {
	return s.getRoleIDForAxentaUserType(accountType)
}

// mapAccountTypeToUserType преобразует тип аккаунта Axenta в тип пользователя системы
func (s *AxentaUserService) mapAccountTypeToUserType(accountType string) string {
	switch accountType {
	case "partner":
		return "partner"
	case "client":
		return "client"
	default:
		log.Printf("Unknown Axenta account type: %s, defaulting to client", accountType)
		return "client"
	}
}

// getRoleIDForAxentaUserType возвращает ID роли для типа пользователя Axenta
func (s *AxentaUserService) getRoleIDForAxentaUserType(accountType string) (uint, error) {
	var roleName string

	switch accountType {
	case "partner":
		roleName = "partner" // Роль партнера
	case "client":
		roleName = "client" // Роль клиента
	default:
		roleName = "user" // Роль пользователя по умолчанию
	}

	var role models.Role
	err := s.db.Where("name = ?", roleName).First(&role).Error
	if err != nil {
		return 0, fmt.Errorf("role %s not found: %w", roleName, err)
	}

	return role.ID, nil
}

// getDefaultRoleID возвращает ID роли по умолчанию
func (s *AxentaUserService) getDefaultRoleID() uint {
	var role models.Role
	err := s.db.Where("name = ?", "user").First(&role).Error
	if err != nil {
		log.Printf("Default role 'user' not found: %v", err)
		// Возвращаем первую доступную роль
		err = s.db.First(&role).Error
		if err != nil {
			log.Printf("No roles found in database: %v", err)
			return 1 // Fallback ID
		}
	}
	return role.ID
}

// CreateLocalUser создает локального пользователя (не из Axenta)
func (s *AxentaUserService) CreateLocalUser(username, email, password string, roleID uint) (*models.User, error) {
	user := models.User{
		Username:       username,
		Email:          email,
		Password:       password, // Должен быть захеширован вызывающим кодом
		RoleID:         &roleID,
		IsActive:       true,
		AxentaUserType: "local",
		IsAxentaUser:   false,
	}

	if err := s.db.Create(&user).Error; err != nil {
		return nil, fmt.Errorf("failed to create local user: %w", err)
	}

	log.Printf("Created new local user: %s", user.Username)
	return &user, nil
}

// CreateUserInAxenta создает пользователя в Axenta Cloud
func (s *AxentaUserService) CreateUserInAxenta(token string, userData *models.User, visibleTabs []string, accesses map[string][]string) (*AxentaUserResponse, error) {
	// Подготавливаем данные для отправки в Axenta
	axentaUserData := map[string]interface{}{
		"username":           userData.Username,
		"email":              userData.Email,
		"name":               userData.Name,
		"password":           userData.Password, // Пароль уже захеширован
		"hasAdminAccess":     false,             // По умолчанию false
		"visibleTabsNames":   visibleTabs,
		"accesses":           accesses,
	}

	jsonData, err := json.Marshal(axentaUserData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal user data: %w", err)
	}

	// Временное логирование для отладки
	log.Printf("🔍 DEBUG: Отправляем данные в Axenta Cloud: %s", string(jsonData))
	log.Printf("🔍 DEBUG: URL: %s", s.baseURL+"/cms/users/")
	log.Printf("🔍 DEBUG: Token: %s", token[:20]+"...")

	// Создаем HTTP запрос
	req, err := http.NewRequest("POST", s.baseURL+"/cms/users/", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Token "+token)

	// Выполняем запрос
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Читаем ответ
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to create user in Axenta, status: %d, body: %s", resp.StatusCode, string(body))
	}

	// Парсим ответ
	var axentaUser AxentaUserResponse
	if err := json.Unmarshal(body, &axentaUser); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	log.Printf("Successfully created user %s in Axenta Cloud with ID: %d", userData.Username, axentaUser.ID)
	return &axentaUser, nil
}

// GetUsersByType возвращает пользователей по типу (partner, client, local)
func (s *AxentaUserService) GetUsersByType(userType string) ([]models.User, error) {
	var users []models.User
	query := s.db.Preload("Role")

	switch userType {
	case "partner", "client":
		query = query.Where("is_axenta_user = ? AND axenta_user_type = ?", true, userType)
	case "local":
		query = query.Where("is_axenta_user = ? OR axenta_user_type = ?", false, "local")
	case "all":
		// Возвращаем всех пользователей
	default:
		return nil, fmt.Errorf("invalid user type: %s", userType)
	}

	if err := query.Find(&users).Error; err != nil {
		return nil, fmt.Errorf("failed to get users by type: %w", err)
	}

	return users, nil
}

// EnsureDefaultRoles создает роли по умолчанию, если они не существуют
func (s *AxentaUserService) EnsureDefaultRoles() error {
	defaultRoles := []models.Role{
		{
			Name:        "partner",
			DisplayName: "Партнер",
			Description: "Роль партнера из Axenta",
			Color:       "#2196F3",
			Priority:    100,
			IsActive:    true,
			IsSystem:    true,
		},
		{
			Name:        "client",
			DisplayName: "Клиент",
			Description: "Роль клиента из Axenta",
			Color:       "#4CAF50",
			Priority:    50,
			IsActive:    true,
			IsSystem:    true,
		},
		{
			Name:        "user",
			DisplayName: "Пользователь",
			Description: "Локальный пользователь системы",
			Color:       "#FF9800",
			Priority:    25,
			IsActive:    true,
			IsSystem:    true,
		},
	}

	for _, role := range defaultRoles {
		var existingRole models.Role
		err := s.db.Where("name = ?", role.Name).First(&existingRole).Error

		if err == gorm.ErrRecordNotFound {
			if err := s.db.Create(&role).Error; err != nil {
				return fmt.Errorf("failed to create role %s: %w", role.Name, err)
			}
			log.Printf("Created default role: %s", role.Name)
		} else if err != nil {
			return fmt.Errorf("failed to check role %s: %w", role.Name, err)
		}
	}

	return nil
}

// SyncUserFromAxentaData синхронизирует пользователя из данных Axenta API
func (s *AxentaUserService) SyncUserFromAxentaData(axentaUser *AxentaUserResponse) (*models.User, error) {
	// Ищем существующего пользователя в базе данных
	var user models.User
	err := s.db.Where("username = ? OR axenta_user_id = ?", axentaUser.Username, fmt.Sprintf("%d", axentaUser.ID)).First(&user).Error

	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	// Если пользователь не найден, создаем нового
	if err == gorm.ErrRecordNotFound {
		user = models.User{
			Username:  axentaUser.Username,
			Email:     axentaUser.Email,
			FirstName: axentaUser.Name,
			Name:      axentaUser.Name,
			IsActive:  axentaUser.IsActive,
			Password:  "axenta_user", // Пароль не используется для Axenta пользователей
		}
	}

	// Обновляем данные пользователя из Axenta
	user.SetAxentaRole(s.mapAccountTypeToUserType(axentaUser.AccountType), fmt.Sprintf("%d", axentaUser.ID))
	user.Email = axentaUser.Email
	user.Name = axentaUser.Name
	user.FirstName = axentaUser.Name
	user.IsActive = axentaUser.IsActive

	// Устанавливаем роль в системе на основе типа аккаунта Axenta
	roleID, err := s.getRoleIDForAxentaUserType(axentaUser.AccountType)
	if err != nil {
		log.Printf("Failed to get role for Axenta user type %s: %v", axentaUser.AccountType, err)
		// Используем роль по умолчанию
		roleID = s.getDefaultRoleID()
	}
	user.RoleID = &roleID

	// Сохраняем пользователя
	if user.ID == 0 {
		// Создаем нового пользователя
		if err := s.db.Create(&user).Error; err != nil {
			return nil, fmt.Errorf("failed to create user: %w", err)
		}
		log.Printf("Created new Axenta user: %s (type: %s)", user.Username, user.AxentaUserType)
	} else {
		// Обновляем существующего пользователя
		if err := s.db.Save(&user).Error; err != nil {
			return nil, fmt.Errorf("failed to update user: %w", err)
		}
		log.Printf("Updated Axenta user: %s (type: %s)", user.Username, user.AxentaUserType)
	}

	// Загружаем роль для возврата
	if err := s.db.Preload("Role").First(&user, user.ID).Error; err != nil {
		log.Printf("Failed to load user role: %v", err)
	}

	return &user, nil
}
