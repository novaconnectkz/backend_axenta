package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
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

	// Успешное получение данных пользователя
	userIDStr := fmt.Sprintf("%d", axentaUser.ID)
	logAuthOperation("login_success_full", req.Username, userIDStr, "", map[string]interface{}{
		"status":       "success",
		"account_type": axentaUser.AccountType,
		"account_name": axentaUser.AccountName,
		"account_id":   axentaUser.AccountID,
		"is_admin":     axentaUser.IsAdmin,
		"email":        axentaUser.Email,
		"full_data":    true,
	})

	// Формируем ответ с правильными типами данных
	userResponse := gin.H{
		"id":                      userIDStr,
		"username":                axentaUser.Username,
		"name":                    axentaUser.Name,
		"email":                   axentaUser.Email,
		"accountName":             axentaUser.AccountName,
		"accountType":             axentaUser.AccountType,
		"creatorName":             axentaUser.CreatorName,
		"lastLogin":               axentaUser.LastLogin,
		"accountBlockingDatetime": axentaUser.AccountBlockingDatetime,
		"accountId":               axentaUser.AccountID,
		"isAdmin":                 axentaUser.IsAdmin,
		"isActive":                axentaUser.IsActive,
		"language":                axentaUser.Language,
		"timezone":                axentaUser.Timezone,
	}

	c.JSON(200, gin.H{
		"status": "success",
		"data": gin.H{
			"token": axentaLogin.Token,
			"user":  userResponse,
		},
	})
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
