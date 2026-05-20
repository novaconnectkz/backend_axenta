package middleware

import (
	"backend_axenta/services"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// OperatorAuthMiddleware защищает control-plane (/api/control/*).
//
// Изоляция от tenant-контура двойная:
//  1. криптографическая — operator-токен подписан OPERATOR_JWT_SECRET,
//     tenant-токен другим ключом → ValidateAccessToken (operator)
//     отвергает tenant-токен, и наоборот tenant LocalAuthMiddleware
//     отвергает operator-токен;
//  2. claim Audience=control-plane (defense-in-depth).
//
// Тут НЕТ tenant-middleware: оператор не привязан к company/схеме.
type OperatorAuthMiddleware struct {
	svc *services.OperatorJWTService
}

func NewOperatorAuthMiddleware(svc *services.OperatorJWTService) *OperatorAuthMiddleware {
	return &OperatorAuthMiddleware{svc: svc}
}

// RequireOperator — Bearer operator access-JWT обязателен.
func (m *OperatorAuthMiddleware) RequireOperator() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status": "error", "error": "Authorization header is required",
			})
			c.Abort()
			return
		}
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status": "error", "error": "Invalid authorization format. Use: Bearer <token>",
			})
			c.Abort()
			return
		}

		claims, err := m.svc.ValidateAccessToken(parts[1])
		if err != nil {
			// Сюда же попадает попытка зайти tenant-токеном
			// (другой секрет/нет aud) — отвергаем.
			c.JSON(http.StatusUnauthorized, gin.H{
				"status": "error", "error": "Invalid or expired operator token",
			})
			c.Abort()
			return
		}

		c.Set("operator_id", claims.OperatorID)
		c.Set("operator_username", claims.Username)
		c.Set("operator_claims", claims)
		c.Next()
	}
}

// GetOperatorID — ID оператора из контекста.
func GetOperatorID(c *gin.Context) (uint, bool) {
	v, ok := c.Get("operator_id")
	if !ok {
		return 0, false
	}
	id, ok := v.(uint)
	return id, ok
}
