package middleware

import (
	"backend_axenta/services"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// LocalAuthMiddleware middleware для локальной JWT авторизации
type LocalAuthMiddleware struct {
	jwtService *services.JWTService
}

// NewLocalAuthMiddleware создает новый middleware для локальной авторизации
func NewLocalAuthMiddleware(jwtService *services.JWTService) *LocalAuthMiddleware {
	return &LocalAuthMiddleware{
		jwtService: jwtService,
	}
}

// RequireAuth middleware для проверки JWT токена
func (m *LocalAuthMiddleware) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Получаем токен из заголовка
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status": "error",
				"error":  "Authorization header is required",
			})
			c.Abort()
			return
		}

		// Проверяем формат Bearer токена
		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status": "error",
				"error":  "Invalid authorization format. Use: Bearer <token>",
			})
			c.Abort()
			return
		}

		token := tokenParts[1]

		// Валидируем токен
		claims, err := m.jwtService.ValidateAccessToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status": "error",
				"error":  "Invalid or expired token",
			})
			c.Abort()
			return
		}

		// Добавляем данные пользователя в контекст
		c.Set("user_id", claims.UserID)
		c.Set("company_id", claims.CompanyID)
		c.Set("role", claims.Role)
		c.Set("username", claims.Username)
		c.Set("is_superadmin", claims.IsSuperadmin)
		c.Set("jwt_claims", claims)

		// Compat-shim для legacy-хендлеров, которые ждут c.Get("user")
		// как map[string]interface{} (email.go, и т.п.). LocalAuthMW
		// ставит user_id/role/etc. отдельными ключами — этот map даёт
		// прежний shape без переписывания хендлеров.
		c.Set("user", map[string]interface{}{
			"id":            claims.UserID,
			"username":      claims.Username,
			"email":         "", // в JWT не передаётся; legacy email.go использует только id
			"company_id":    claims.CompanyID,
			"role":          claims.Role,
			"is_superadmin": claims.IsSuperadmin,
		})

		// auth_company_id — ДОВЕРЕННЫЙ tenant из подписанного JWT.
		// TenantMiddleware обязан брать схему отсюда, НЕ из клиентского
		// X-Tenant-ID (риск B5: horizontal privilege escalation между
		// схемами при подмене заголовка).
		c.Set("auth_company_id", claims.CompanyID)

		c.Next()
	}
}

// OptionalAuth middleware для опциональной авторизации
func (m *LocalAuthMiddleware) OptionalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Next()
			return
		}

		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
			c.Next()
			return
		}

		token := tokenParts[1]
		claims, err := m.jwtService.ValidateAccessToken(token)
		if err != nil {
			c.Next()
			return
		}

		// Добавляем данные пользователя в контекст
		c.Set("user_id", claims.UserID)
		c.Set("company_id", claims.CompanyID)
		c.Set("role", claims.Role)
		c.Set("username", claims.Username)
		c.Set("jwt_claims", claims)

		c.Next()
	}
}

// RequireRole middleware для проверки роли пользователя
func (m *LocalAuthMiddleware) RequireRole(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status": "error",
				"error":  "User role not found in context",
			})
			c.Abort()
			return
		}

		userRole, ok := role.(string)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "Invalid role format",
			})
			c.Abort()
			return
		}

		// Проверяем роль
		for _, allowedRole := range allowedRoles {
			if userRole == allowedRole {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusForbidden, gin.H{
			"status": "error",
			"error":  "Insufficient permissions",
		})
		c.Abort()
	}
}

// RequireCompany middleware для проверки принадлежности к компании
func (m *LocalAuthMiddleware) RequireCompany(companyID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userCompanyID, exists := c.Get("company_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status": "error",
				"error":  "Company ID not found in context",
			})
			c.Abort()
			return
		}

		if userCompanyID != companyID {
			c.JSON(http.StatusForbidden, gin.H{
				"status": "error",
				"error":  "Access denied for this company",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// GetCurrentUserID возвращает ID текущего пользователя из контекста
func GetCurrentUserID(c *gin.Context) (uint, bool) {
	userID, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}

	id, ok := userID.(uint)
	return id, ok
}

// GetCurrentCompanyID возвращает ID компании текущего пользователя из контекста
func GetCurrentCompanyID(c *gin.Context) (string, bool) {
	companyID, exists := c.Get("company_id")
	if !exists {
		return "", false
	}

	id, ok := companyID.(string)
	return id, ok
}

// GetCurrentUserRole возвращает роль текущего пользователя из контекста
func GetCurrentUserRole(c *gin.Context) (string, bool) {
	role, exists := c.Get("role")
	if !exists {
		return "", false
	}

	userRole, ok := role.(string)
	return userRole, ok
}

// GetCurrentUsername возвращает имя текущего пользователя из контекста
func GetCurrentUsername(c *gin.Context) (string, bool) {
	username, exists := c.Get("username")
	if !exists {
		return "", false
	}

	name, ok := username.(string)
	return name, ok
}

// GetJWTClaims возвращает JWT claims из контекста
func GetJWTClaims(c *gin.Context) (*services.JWTClaims, bool) {
	claims, exists := c.Get("jwt_claims")
	if !exists {
		return nil, false
	}

	jwtClaims, ok := claims.(*services.JWTClaims)
	return jwtClaims, ok
}
