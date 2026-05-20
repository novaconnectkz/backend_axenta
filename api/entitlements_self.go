package api

import (
	"backend_axenta/middleware"
	"backend_axenta/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

// SelfEntitlementsAPI — tenant-приложение узнаёт СВОИ эффективные
// фичи (Фаза 2, S4). Под tenant-auth (apiGroup): company_id из
// доверенного JWT-claim/SetTenant. Фронт по этому ответу скрывает/
// показывает платные фичи; backend всё равно гейтит RequireFeature
// (UI-проверка не заменяет серверную).
type SelfEntitlementsAPI struct {
	ent *services.EntitlementService
}

func NewSelfEntitlementsAPI(ent *services.EntitlementService) *SelfEntitlementsAPI {
	return &SelfEntitlementsAPI{ent: ent}
}

// MyEntitlements — GET /api/auth/my-entitlements. Возвращает карту
// feature_code → {enabled, limits, source} и плоский enabled-список.
func (a *SelfEntitlementsAPI) MyEntitlements(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "company не определена"})
		return
	}
	eff := a.ent.Effective(companyID)
	enabled := make([]string, 0, len(eff))
	for code, e := range eff {
		if e.Enabled {
			enabled = append(enabled, code)
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"company_id":   companyID,
			"features":     eff,
			"enabled_list": enabled,
		},
	})
}
