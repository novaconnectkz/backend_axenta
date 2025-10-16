package middleware

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// AxentaAPITokensMiddleware middleware для проверки токенов из AXENTA_API_TOKENS
type AxentaAPITokensMiddleware struct{}

// NewAxentaAPITokensMiddleware создает новый middleware для проверки API токенов
func NewAxentaAPITokensMiddleware() *AxentaAPITokensMiddleware {
	return &AxentaAPITokensMiddleware{}
}

// RequireValidToken middleware для проверки валидного токена из AXENTA_API_TOKENS
func (m *AxentaAPITokensMiddleware) RequireValidToken() gin.HandlerFunc {
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

		// Проверяем токен в списке валидных токенов
		if !m.isValidToken(token) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status": "error",
				"error":  "Invalid token",
			})
			c.Abort()
			return
		}

		// Сохраняем токен в контексте
		c.Set("api_token", token)

		c.Next()
	}
}

// isValidToken проверяет, является ли токен валидным
func (m *AxentaAPITokensMiddleware) isValidToken(token string) bool {
	// Получаем список валидных токенов из переменной окружения
	validTokensStr := os.Getenv("AXENTA_API_TOKENS")
	if validTokensStr == "" {
		return false
	}

	// Разбиваем строку по запятым
	validTokens := strings.Split(validTokensStr, ",")

	// Отладочная информация
	fmt.Printf("DEBUG: Checking token: '%s'\n", token)
	fmt.Printf("DEBUG: Valid tokens: %v\n", validTokens)

	// Проверяем каждый токен (убираем пробелы)
	for _, validToken := range validTokens {
		trimmedToken := strings.TrimSpace(validToken)
		fmt.Printf("DEBUG: Comparing '%s' with '%s'\n", token, trimmedToken)
		if trimmedToken == token {
			fmt.Printf("DEBUG: Token match found!\n")
			return true
		}
	}

	fmt.Printf("DEBUG: No token match found\n")
	return false
}

// GetCurrentAPIToken возвращает текущий API токен из контекста
func GetCurrentAPIToken(c *gin.Context) string {
	if token, exists := c.Get("api_token"); exists {
		if tokenStr, ok := token.(string); ok {
			return tokenStr
		}
	}
	return ""
}
