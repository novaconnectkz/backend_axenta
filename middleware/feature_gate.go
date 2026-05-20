package middleware

import (
	"backend_axenta/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Гейт фич монетизации (Фаза 2, S4).
//
// ВАЖНО: ставить ТОЛЬКО на ЛОКАЛЬНЫЕ ACRM-роуты. НЕ навешивать на
// Axenta-passthrough (objects/accounts/... проксируют в axenta.cloud —
// их отвязка это Фаза 3, ортогонально монетизации).
//
// company_id берётся из контекста, который проставляет TenantMiddleware
// (uint, из доверенного JWT-claim, риск B5). Значит RequireFeature
// должен стоять В ЦЕПОЧКЕ ПОСЛЕ RequireAuth+SetTenant.
//
// Применение к реальным локальным роутам — продуктовое решение (какая
// фича какой эндпоинт закрывает); делается точечно по мере введения
// платных фич, НЕ массово (иначе регрессия). Пример:
//
//	apiGroup.GET("/reports/advanced",
//	    middleware.RequireFeature(entSvc, "advanced_reports"),
//	    handler)

// RequireFeature — 403 feature_not_enabled, если фича недоступна
// компании. EntitlementService сам fail-closed при ошибке БД.
func RequireFeature(ent *services.EntitlementService, featureCode string) gin.HandlerFunc {
	return func(c *gin.Context) {
		companyID := GetCompanyID(c) // uint из контекста (после SetTenant)
		if companyID == 0 || !ent.IsEnabled(companyID, featureCode) {
			c.JSON(http.StatusForbidden, gin.H{
				"status":  "error",
				"error":   "feature_not_enabled",
				"feature": featureCode,
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// FeatureEnabled — проверка из тела хендлера (когда гейт не на весь
// роут, а на ветку логики).
func FeatureEnabled(c *gin.Context, ent *services.EntitlementService, featureCode string) bool {
	companyID := GetCompanyID(c)
	return companyID != 0 && ent.IsEnabled(companyID, featureCode)
}
