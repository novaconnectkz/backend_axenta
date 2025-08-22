package middleware

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

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
			c.JSON(http.StatusUnauthorized, gin.H{
				"status": "error",
				"error":  "Invalid or expired token: " + err.Error(),
			})
			c.Abort()
			return
		}

		// Сохраняем информацию о пользователе в контексте
		c.Set("user", user)
		c.Set("token", token)

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
