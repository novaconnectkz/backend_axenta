package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type LoginRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required,min=3,max=64"`
}

type AxentaLoginResponse struct {
	Token   string `json:"token"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

type AxentaUserResponse struct {
	AccountBlockingDatetime *string `json:"accountBlockingDatetime"`
	AccountName             string  `json:"accountName"`
	AccountType             string  `json:"accountType"`
	CreatorName             string  `json:"creatorName"`
	ID                      int     `json:"id"`
	LastLogin               string  `json:"lastLogin"`
	Name                    string  `json:"name"`
	Username                string  `json:"username"`
	Email                   string  `json:"email,omitempty"`
	AccountID               int     `json:"accountId,omitempty"`
	IsAdmin                 bool    `json:"isAdmin,omitempty"`
	IsActive                bool    `json:"isActive,omitempty"`
	Language                string  `json:"language,omitempty"`
	Timezone                int     `json:"timezone,omitempty"`
}

// Структурированное логирование для авторизации
func logAuthOperation(operation, username, userID, companyID string, details map[string]interface{}) {
	logData := map[string]interface{}{
		"timestamp":  time.Now().Format(time.RFC3339),
		"operation":  operation,
		"username":   username,
		"user_id":    userID,
		"company_id": companyID,
	}

	for key, value := range details {
		logData[key] = value
	}

	logJSON, _ := json.Marshal(logData)
	log.Printf("AUTH_LOG: %s", string(logJSON))
}

func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logAuthOperation("login_validation_error", req.Username, "", "", map[string]interface{}{
			"error":      err.Error(),
			"status":     "failed",
			"ip_address": c.ClientIP(),
		})
		c.JSON(400, gin.H{"status": "error", "error": "Invalid username or password"})
		return
	}

	logAuthOperation("login_attempt", req.Username, "", "", map[string]interface{}{
		"ip_address": c.ClientIP(),
		"user_agent": c.GetHeader("User-Agent"),
	})

	// Запрос к Axenta Cloud для получения токена
	axentaReq, _ := json.Marshal(req)
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	axentaLoginURL := "https://axenta.cloud/api/auth/login/"
	resp, err := client.Post(axentaLoginURL, "application/json", bytes.NewBuffer(axentaReq))
	if err != nil {
		logAuthOperation("axenta_login_connection_error", req.Username, "", "", map[string]interface{}{
			"error":      err.Error(),
			"status":     "failed",
			"axenta_url": axentaLoginURL,
		})
		c.JSON(500, gin.H{"status": "error", "error": "Failed to connect to Axenta Cloud"})
		return
	}
	defer resp.Body.Close()

	// Читаем ответ для логирования
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logAuthOperation("axenta_login_read_error", req.Username, "", "", map[string]interface{}{
			"error":         err.Error(),
			"status":        "failed",
			"response_code": resp.StatusCode,
		})
		c.JSON(500, gin.H{"status": "error", "error": "Failed to read login response"})
		return
	}

	logAuthOperation("axenta_login_response", req.Username, "", "", map[string]interface{}{
		"response_code": resp.StatusCode,
		"response_size": len(body),
		"content_type":  resp.Header.Get("Content-Type"),
	})

	if resp.StatusCode != http.StatusOK {
		logAuthOperation("axenta_login_failed", req.Username, "", "", map[string]interface{}{
			"response_code": resp.StatusCode,
			"response_body": string(body),
			"status":        "failed",
		})
		c.JSON(401, gin.H{"status": "error", "error": "Invalid credentials"})
		return
	}

	var axentaLogin AxentaLoginResponse
	if err := json.Unmarshal(body, &axentaLogin); err != nil {
		logAuthOperation("axenta_login_parse_error", req.Username, "", "", map[string]interface{}{
			"error":         err.Error(),
			"response_body": string(body),
			"status":        "failed",
		})
		c.JSON(500, gin.H{"status": "error", "error": "Failed to parse login response"})
		return
	}

	// Валидация токена
	if axentaLogin.Token == "" {
		logAuthOperation("axenta_login_empty_token", req.Username, "", "", map[string]interface{}{
			"response_body": string(body),
			"status":        "failed",
		})
		c.JSON(500, gin.H{"status": "error", "error": "Empty token received"})
		return
	}

	logAuthOperation("axenta_login_success", req.Username, "", "", map[string]interface{}{
		"token_length": len(axentaLogin.Token),
		"status":       "success",
	})

	// Запрос к Axenta Cloud для получения данных пользователя
	userClient := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil // Следуем редиректам
		},
	}

	axentaUserURL := "https://axenta.cloud/api/current_user/"
	userReq, err := http.NewRequest("GET", axentaUserURL, nil)
	if err != nil {
		logAuthOperation("axenta_user_request_error", req.Username, "", "", map[string]interface{}{
			"error":  err.Error(),
			"status": "failed",
			"url":    axentaUserURL,
		})
		c.JSON(500, gin.H{"status": "error", "error": "Failed to create user request"})
		return
	}

	authHeader := "Token " + axentaLogin.Token
	userReq.Header.Set("Authorization", authHeader)
	userReq.Header.Set("Content-Type", "application/json")

	// ОТЛАДКА: логируем заголовок авторизации
	logAuthOperation("axenta_user_request_headers", req.Username, "", "", map[string]interface{}{
		"auth_header": authHeader[:min(20, len(authHeader))] + "...",
		"url":         axentaUserURL,
	})

	userResp, err := userClient.Do(userReq)
	if err != nil {
		logAuthOperation("axenta_user_connection_error", req.Username, "", "", map[string]interface{}{
			"error":      err.Error(),
			"status":     "fallback",
			"token_used": "Token " + axentaLogin.Token[:min(10, len(axentaLogin.Token))] + "...",
		})

		// Временно возвращаем успешный ответ с минимальными данными
		fallbackUser := createFallbackUser(req.Username)
		logAuthOperation("login_success_fallback", req.Username, "temp_id", "", map[string]interface{}{
			"status":        "success",
			"fallback_mode": true,
			"reason":        "user_api_connection_failed",
		})

		c.JSON(200, gin.H{
			"status": "success",
			"data": gin.H{
				"token": axentaLogin.Token,
				"user":  fallbackUser,
			},
		})
		return
	}
	defer userResp.Body.Close()

	// Читаем ответ для логирования
	userBody, err := io.ReadAll(userResp.Body)
	if err != nil {
		logAuthOperation("axenta_user_read_error", req.Username, "", "", map[string]interface{}{
			"error":         err.Error(),
			"status":        "fallback",
			"response_code": userResp.StatusCode,
		})

		fallbackUser := createFallbackUser(req.Username)
		c.JSON(200, gin.H{
			"status": "success",
			"data": gin.H{
				"token": axentaLogin.Token,
				"user":  fallbackUser,
			},
		})
		return
	}

	logAuthOperation("axenta_user_response", req.Username, "", "", map[string]interface{}{
		"response_code": userResp.StatusCode,
		"response_size": len(userBody),
		"content_type":  userResp.Header.Get("Content-Type"),
	})

	if userResp.StatusCode != http.StatusOK {
		logAuthOperation("axenta_user_failed", req.Username, "", "", map[string]interface{}{
			"response_code": userResp.StatusCode,
			"response_body": string(userBody),
			"status":        "fallback",
		})

		fallbackUser := createFallbackUser(req.Username)
		logAuthOperation("login_success_fallback", req.Username, "temp_id", "", map[string]interface{}{
			"status":        "success",
			"fallback_mode": true,
			"reason":        "user_api_failed",
		})

		c.JSON(200, gin.H{
			"status": "success",
			"data": gin.H{
				"token": axentaLogin.Token,
				"user":  fallbackUser,
			},
		})
		return
	}

	var axentaUser AxentaUserResponse
	if err := json.Unmarshal(userBody, &axentaUser); err != nil {
		logAuthOperation("axenta_user_parse_error", req.Username, "", "", map[string]interface{}{
			"error":         err.Error(),
			"response_body": string(userBody),
			"status":        "fallback",
		})

		fallbackUser := createFallbackUser(req.Username)
		logAuthOperation("login_success_fallback", req.Username, "temp_id", "", map[string]interface{}{
			"status":        "success",
			"fallback_mode": true,
			"reason":        "user_parse_failed",
		})

		c.JSON(200, gin.H{
			"status": "success",
			"data": gin.H{
				"token": axentaLogin.Token,
				"user":  fallbackUser,
			},
		})
		return
	}

	// Получаем базу данных для работы с пользователями
	db := database.GetDB()
	if db == nil {
		logAuthOperation("database_connection_error", req.Username, "", "", map[string]interface{}{
			"status": "error",
			"error":  "database_not_available",
		})
		c.JSON(500, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	if axentaUser.AccountID > 0 {
		company, created, ensureErr := ensureCompanyExists(db, axentaUser, req.Username)
		if ensureErr != nil {
			logAuthOperation("company_sync_error", req.Username, "", "", map[string]interface{}{
				"status":       "warning",
				"error":        ensureErr.Error(),
				"account_id":   axentaUser.AccountID,
				"account_name": axentaUser.AccountName,
			})
		} else if company != nil {
			event := "company_checked"
			if created {
				event = "company_created"
			}
			logAuthOperation(event, req.Username, "", fmt.Sprintf("%d", company.ID), map[string]interface{}{
				"status":       "success",
				"account_id":   axentaUser.AccountID,
				"account_name": axentaUser.AccountName,
				"schema":       company.DatabaseSchema,
			})
		}
	}

	// Создаем сервис для работы с пользователями Axenta
	axentaUserService := services.NewAxentaUserService(db)

	// Создаем сервис для работы с токенами пользователей
	userTokenService := services.NewUserTokenService(db)

	// Убеждаемся, что роли по умолчанию существуют
	if err := axentaUserService.EnsureDefaultRoles(); err != nil {
		log.Printf("Failed to ensure default roles: %v", err)
	}

	// Синхронизируем пользователя с Axenta (поддерживаем как партнеров, так и клиентов)
	user, err := axentaUserService.SyncUserWithAxenta(axentaLogin.Token, req.Username)
	if err != nil {
		userIDStr := fmt.Sprintf("%d", axentaUser.ID)
		logAuthOperation("user_sync_error", req.Username, userIDStr, "", map[string]interface{}{
			"status":       "error",
			"account_type": axentaUser.AccountType,
			"error":        err.Error(),
		})

		// Вместо возврата ошибки, продолжаем с данными из Axenta (fallback режим)
		log.Printf("⚠️ User sync failed, continuing with Axenta data only: %v", err)

		// Создаем fallback пользователя на основе данных из Axenta
		user = &models.User{
			ID:             uint(axentaUser.ID), // Используем ID из Axenta как временный
			Username:       axentaUser.Username,
			Email:          axentaUser.Email,
			FirstName:      axentaUser.Name,
			Name:           axentaUser.Name,
			IsActive:       axentaUser.IsActive,
			AxentaUserType: axentaUserService.MapAccountTypeToUserType(axentaUser.AccountType),
			AxentaUserID:   fmt.Sprintf("%d", axentaUser.ID),
			IsAxentaUser:   true,
			ExternalSource: "axenta",
			ExternalID:     fmt.Sprintf("%d", axentaUser.ID),
		}

		// Пытаемся назначить роль, если возможно
		if roleID, roleErr := axentaUserService.GetRoleIDForAxentaUserType(axentaUser.AccountType); roleErr == nil {
			user.RoleID = &roleID
			// Пытаемся загрузить роль для отображения
			var role models.Role
			if db.First(&role, roleID).Error == nil {
				user.Role = &role
			}
		}

		logAuthOperation("login_success_fallback_sync", req.Username, userIDStr, "", map[string]interface{}{
			"status":           "success",
			"fallback_mode":    true,
			"reason":           "user_sync_failed",
			"account_type":     axentaUser.AccountType,
			"axenta_user_type": user.AxentaUserType,
		})
	}

	// Сохраняем токен пользователя
	accountIDUint := uint(0)
	if axentaUser.AccountID > 0 {
		accountIDUint = uint(axentaUser.AccountID)
	}

	if err := userTokenService.SaveUserToken(
		user.ID,
		req.Username,
		accountIDUint,
		axentaLogin.Token,
		c.GetHeader("User-Agent"),
		c.ClientIP(),
	); err != nil {
		log.Printf("⚠️ Failed to save user token: %v", err)
		// Не прерываем процесс авторизации из-за ошибки сохранения токена
	}

	// Успешное получение данных пользователя
	userIDStr := fmt.Sprintf("%d", user.ID)
	logAuthOperation("login_success_full", req.Username, userIDStr, "", map[string]interface{}{
		"status":           "success",
		"account_type":     axentaUser.AccountType,
		"account_name":     axentaUser.AccountName,
		"account_id":       axentaUser.AccountID,
		"is_admin":         axentaUser.IsAdmin,
		"email":            axentaUser.Email,
		"full_data":        true,
		"axenta_user_type": user.AxentaUserType,
		"is_axenta_user":   user.IsAxentaUser,
		"role_id":          user.RoleID,
	})

	// Формируем ответ с правильными типами данных, включая роль из базы данных
	userResponse := gin.H{
		"id":                      userIDStr,
		"username":                user.Username,
		"name":                    user.Name,
		"email":                   user.Email,
		"accountName":             axentaUser.AccountName,
		"accountType":             axentaUser.AccountType,
		"creatorName":             axentaUser.CreatorName,
		"lastLogin":               axentaUser.LastLogin,
		"accountBlockingDatetime": axentaUser.AccountBlockingDatetime,
		"accountId":               axentaUser.AccountID,
		"isAdmin":                 axentaUser.IsAdmin,
		"isActive":                user.IsActive,
		"language":                axentaUser.Language,
		"timezone":                axentaUser.Timezone,
		// Добавляем информацию о роли из нашей системы
		"axentaUserType": user.AxentaUserType,
		"isAxentaUser":   user.IsAxentaUser,
		"roleId":         user.RoleID,
		"role":           user.Role,
	}

	c.JSON(200, gin.H{
		"status": "success",
		"data": gin.H{
			"token": axentaLogin.Token,
			"user":  userResponse,
		},
	})
}

func ensureCompanyExists(db *gorm.DB, axentaUser AxentaUserResponse, username string) (*models.Company, bool, error) {
	if db == nil {
		return nil, false, fmt.Errorf("database connection is nil")
	}
	if axentaUser.AccountID <= 0 {
		return nil, false, fmt.Errorf("account id is not provided")
	}

	schemaName := fmt.Sprintf("tenant_%d", axentaUser.AccountID)
	companyName := axentaUser.AccountName
	if companyName == "" {
		companyName = fmt.Sprintf("Компания %d", axentaUser.AccountID)
	}

	var company models.Company
	err := db.Where("id = ?", axentaUser.AccountID).First(&company).Error
	created := false
	needSchemaMigration := false

	if errors.Is(err, gorm.ErrRecordNotFound) {
		company = models.Company{
			ID:             uint(axentaUser.AccountID),
			Name:           companyName,
			DatabaseSchema: schemaName,
			AxetnaLogin:    username,
			AxetnaPassword: "",
			ContactEmail:   axentaUser.Email,
			IsActive:       true,
		}

		if err := db.Create(&company).Error; err != nil {
			return nil, false, err
		}
		created = true
		needSchemaMigration = true
	} else if err != nil {
		return nil, false, err
	} else {
		updates := make(map[string]interface{})

		if company.Name != companyName && companyName != "" {
			updates["name"] = companyName
		}

		if company.DatabaseSchema == "" {
			updates["database_schema"] = schemaName
			company.DatabaseSchema = schemaName
			needSchemaMigration = true
		} else if company.DatabaseSchema != schemaName {
			updates["database_schema"] = schemaName
			company.DatabaseSchema = schemaName
			needSchemaMigration = true
		}

		if company.AxetnaLogin == "" && username != "" {
			updates["axetna_login"] = username
		}

		if company.AxetnaPassword == "" {
			updates["axetna_password"] = ""
		}

		if company.ContactEmail == "" && axentaUser.Email != "" {
			updates["contact_email"] = axentaUser.Email
		}

		if len(updates) > 0 {
			if err := db.Model(&company).Updates(updates).Error; err != nil {
				return nil, false, err
			}
		}
	}

	if company.DatabaseSchema == "" {
		company.DatabaseSchema = schemaName
		if err := db.Model(&company).Update("database_schema", schemaName).Error; err == nil {
			needSchemaMigration = true
		}
	}

	if needSchemaMigration {
		if err := database.CreateTenantSchema(company.ID, company.DatabaseSchema); err != nil {
			return &company, created, err
		}
	}

	return &company, created, nil
}

// Создание fallback данных пользователя
func createFallbackUser(username string) gin.H {
	return gin.H{
		"username":                username,
		"name":                    username,
		"id":                      fmt.Sprintf("temp_%s_%d", username, time.Now().Unix()),
		"accountName":             "Unknown",
		"accountType":             "user",
		"creatorName":             "Unknown",
		"lastLogin":               time.Now().Format(time.RFC3339),
		"accountBlockingDatetime": "",
		"company_id":              "",
	}
}

// Вспомогательная функция min для Go < 1.21
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
