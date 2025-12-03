package middleware

import (
	"backend_axenta/audit"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// getMapKeys возвращает список ключей из map (вспомогательная функция)
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// AuthMiddleware проверяет аутентификацию пользователя
type AuthMiddleware struct{}

// NewAuthMiddleware создает новый экземпляр AuthMiddleware
func NewAuthMiddleware() *AuthMiddleware {
	return &AuthMiddleware{}
}

// RequireAuth middleware для проверки аутентификации
func (am *AuthMiddleware) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Получаем токен из заголовка
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			authHeader = c.GetHeader("authorization")
		}

		if authHeader == "" {
			// Логируем неудачную попытку авторизации
			audit.LogError(c, "auth.failed", fmt.Errorf("missing authorization header"), gin.H{
				"reason": "authorization_header_missing",
			})
			c.JSON(http.StatusUnauthorized, gin.H{
				"status": "error",
				"error":  "Authorization header is required",
			})
			c.Abort()
			return
		}

		// Извлекаем токен из заголовка
		var token string
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		} else if strings.HasPrefix(authHeader, "Token ") {
			token = strings.TrimPrefix(authHeader, "Token ")
		} else {
			token = authHeader
		}

		if token == "" {
			// Логируем неудачную попытку авторизации
			audit.LogError(c, "auth.failed", fmt.Errorf("invalid authorization format"), gin.H{
				"reason": "invalid_token_format",
			})
			c.JSON(http.StatusUnauthorized, gin.H{
				"status": "error",
				"error":  "Invalid authorization format",
			})
			c.Abort()
			return
		}

		// Проверяем токен через Axenta API
		user, err := am.validateToken(token)
		if err != nil {
			// Логируем неудачную попытку авторизации
			audit.LogError(c, "auth.failed", err, gin.H{
				"reason": "token_validation_failed",
			})
			c.JSON(http.StatusUnauthorized, gin.H{
				"status": "error",
				"error":  "Invalid or expired token: " + err.Error(),
			})
			c.Abort()
			return
		}

		// Проверяем тип аккаунта - только партнеры могут использовать API
		if accountType, ok := user["accountType"].(string); ok {
			if accountType != "partner" {
				// Логируем попытку доступа неавторизованного типа аккаунта
				audit.LogError(c, "auth.forbidden", fmt.Errorf("invalid account type"), gin.H{
					"reason":        "account_type_not_partner",
					"account_type":  accountType,
					"required_type": "partner",
				})
				c.JSON(http.StatusForbidden, gin.H{
					"status": "error",
					"error":  "Доступ к API разрешен только партнерам Axenta",
					"details": gin.H{
						"account_type":  accountType,
						"required_type": "partner",
					},
				})
				c.Abort()
				return
			}
		}

		// Сохраняем информацию о пользователе в контексте
		c.Set("user", user)
		c.Set("token", token)

		// Извлекаем и устанавливаем user_id в контекст для удобства
		var userID uint
		if id, ok := user["id"]; ok {
			fmt.Printf("🔍 AuthMiddleware: найден id в user, тип: %T, значение: %v\n", id, id)
			switch v := id.(type) {
			case float64:
				userID = uint(v)
			case int:
				userID = uint(v)
			case int64:
				userID = uint(v)
			case string:
				if parsed, err := strconv.ParseUint(v, 10, 32); err == nil {
					userID = uint(parsed)
				} else {
					fmt.Printf("⚠️ AuthMiddleware: не удалось распарсить id из строки: %s, ошибка: %v\n", v, err)
				}
			default:
				fmt.Printf("⚠️ AuthMiddleware: неизвестный тип id: %T, значение: %v\n", v, v)
			}
		} else {
			fmt.Printf("⚠️ AuthMiddleware: поле 'id' не найдено в объекте user. Доступные ключи: %v\n", getMapKeys(user))
		}
		
		if userID > 0 {
			c.Set("user_id", userID)
			fmt.Printf("✅ AuthMiddleware: user_id установлен в контекст: %d\n", userID)
		} else {
			fmt.Printf("❌ AuthMiddleware: не удалось установить user_id (userID = 0)\n")
		}

		// Логируем успешную авторизацию
		userIDStr := ""
		if id, ok := user["id"]; ok {
			userIDStr = fmt.Sprintf("%v", id)
		}
		username := ""
		if name, ok := user["username"].(string); ok {
			username = name
		}
		audit.LogSuccess(c, "auth.success", gin.H{
			"user_id":  userIDStr,
			"username": username,
		})

		c.Next()
	}
}

// validateToken проверяет токен через Axenta API
func (am *AuthMiddleware) validateToken(token string) (map[string]interface{}, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("GET", "https://axenta.cloud/api/current_user/", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Authorization", "Token "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to validate token: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token validation failed with status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	var user map[string]interface{}
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	return user, nil
}

// OptionalAuth middleware для опциональной аутентификации
func (am *AuthMiddleware) OptionalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Получаем токен из заголовка
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			authHeader = c.GetHeader("authorization")
		}

		if authHeader != "" {
			// Извлекаем токен из заголовка
			var token string
			if strings.HasPrefix(authHeader, "Bearer ") {
				token = strings.TrimPrefix(authHeader, "Bearer ")
			} else if strings.HasPrefix(authHeader, "Token ") {
				token = strings.TrimPrefix(authHeader, "Token ")
			} else {
				token = authHeader
			}

			if token != "" {
				// Пробуем проверить токен
				user, err := am.validateToken(token)
				if err == nil {
					// Сохраняем информацию о пользователе в контексте
					c.Set("user", user)
					c.Set("token", token)
				}
			}
		}

		c.Next()
	}
}

// GetCurrentUser возвращает текущего пользователя из контекста
func GetCurrentUser(c *gin.Context) map[string]interface{} {
	if user, exists := c.Get("user"); exists {
		if userMap, ok := user.(map[string]interface{}); ok {
			return userMap
		}
	}
	return nil
}

// GetCurrentToken возвращает текущий токен из контекста
func GetCurrentToken(c *gin.Context) string {
	if token, exists := c.Get("token"); exists {
		if tokenStr, ok := token.(string); ok {
			return tokenStr
		}
	}
	return ""
}

// GetAdminAccountID извлекает accountId партнера (администратора) из контекста запроса
func GetAdminAccountID(c *gin.Context) (uint, error) {
	if cachedID, exists := c.Get("admin_account_id"); exists {
		if id, ok := cachedID.(uint); ok && id > 0 {
			return id, nil
		}
	}

	var adminID uint

	// 1. Пробуем получить из информации о текущем пользователе
	if user := GetCurrentUser(c); user != nil {
		switch value := user["accountId"].(type) {
		case float64:
			if value > 0 {
				adminID = uint(value)
			}
		case int:
			if value > 0 {
				adminID = uint(value)
			}
		case string:
			if parsed, err := strconv.ParseUint(value, 10, 64); err == nil && parsed > 0 {
				adminID = uint(parsed)
			}
		}

		if adminID == 0 {
			// Некоторые ответы могут содержать account_id
			if value, ok := user["account_id"]; ok {
				switch v := value.(type) {
				case float64:
					if v > 0 {
						adminID = uint(v)
					}
				case int:
					if v > 0 {
						adminID = uint(v)
					}
				case string:
					if parsed, err := strconv.ParseUint(v, 10, 64); err == nil && parsed > 0 {
						adminID = uint(parsed)
					}
				}
			}
		}
	}

	// 2. Пробуем получить из заголовков
	if adminID == 0 {
		headerCandidates := []string{
			"X-Admin-ID",
			"X-Account-ID",
			"X-Axenta-Account",
		}

		for _, header := range headerCandidates {
			if value := c.GetHeader(header); value != "" {
				if parsed, err := strconv.ParseUint(value, 10, 64); err == nil && parsed > 0 {
					adminID = uint(parsed)
					break
				}
			}
		}
	}

	// 3. Пробуем получить из query параметров
	if adminID == 0 {
		queryCandidates := []string{
			"admin_id",
			"account_id",
			"axenta_account_id",
		}

		for _, param := range queryCandidates {
			if value := c.Query(param); value != "" {
				if parsed, err := strconv.ParseUint(value, 10, 64); err == nil && parsed > 0 {
					adminID = uint(parsed)
					break
				}
			}
		}
	}

	if adminID == 0 {
		return 0, fmt.Errorf("не удалось определить accountId администратора Axenta")
	}

	c.Set("admin_account_id", adminID)
	return adminID, nil
}
