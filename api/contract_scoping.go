package api

import (
	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ============================================================================
// Scoping менеджера: роль `manager` видит/правит ТОЛЬКО свои договоры
// (Contract.manager_id = свой user_id). admin/superadmin — всё.
//
// ВАЖНО (Codex review): scoping обязан быть СКВОЗНЫМ — не только /contracts, но и
// все смежные финансовые endpoint'ы (ledger, invoices, subscriptions, breakdown,
// dashboard). Для таблиц, привязанных к договору по contract_id, используем
// managerScopedContractIDs(): отдаёт множество ID договоров менеджера.
// ============================================================================

// isManagerScoped — true, если текущий пользователь должен быть ограничен своими
// договорами: роль `manager` и НЕ superadmin. admin/superadmin → false (видят всё).
func isManagerScoped(c *gin.Context) bool {
	if v, _ := c.Get("is_superadmin"); v == true {
		return false
	}
	role, _ := c.Get("role")
	r, _ := role.(string)
	return r == models.RoleManager
}

// currentUserID — id текущего пользователя из контекста (обёртка над middleware).
func currentUserID(c *gin.Context) (uint, bool) {
	return middleware.GetCurrentUserID(c)
}

// managerScopedContractIDs возвращает (ids, applies, err):
//   - applies=false → ограничение не нужно (admin/superadmin), ids игнорировать;
//   - applies=true  → список ID договоров менеджера в текущей тенант-схеме
//     (для фильтра `contract_id IN ids` на ledger/invoices/subscriptions и т.п.).
//
// Если у менеджера нет user_id или нет договоров — возвращает (nil или []{0}, true, nil),
// чтобы фильтр гарантированно НЕ раскрыл чужое (пустой/невозможный IN).
func managerScopedContractIDs(c *gin.Context) ([]uint, bool, error) {
	if !isManagerScoped(c) {
		return nil, false, nil
	}
	uid, ok := currentUserID(c)
	if !ok {
		return []uint{0}, true, nil // нет user_id → ничего не показываем
	}
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		tenantDB = database.DB
	}
	adminAccountID, _ := middleware.GetAdminAccountID(c)

	var ids []uint
	q := tenantDB.Model(&models.Contract{}).Where("manager_id = ?", uid)
	if adminAccountID != 0 {
		q = q.Where("admin_account_id = ?", adminAccountID)
	}
	if err := q.Pluck("id", &ids).Error; err != nil {
		return []uint{0}, true, err
	}
	if len(ids) == 0 {
		return []uint{0}, true, nil // нет своих договоров → пустой результат
	}
	return ids, true, nil
}

// managerCanAccessContract — может ли текущий пользователь видеть конкретный договор.
// admin/superadmin → всегда true. manager → только если он назначен на этот договор.
func managerCanAccessContract(c *gin.Context, contractID uint) bool {
	if !isManagerScoped(c) {
		return true
	}
	uid, ok := currentUserID(c)
	if !ok {
		return false
	}
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		tenantDB = database.DB
	}
	var count int64
	if err := tenantDB.Model(&models.Contract{}).
		Where("id = ? AND manager_id = ?", contractID, uid).Count(&count).Error; err != nil {
		return false // fail-closed
	}
	return count > 0
}

// requireContractAssignAccess — назначать/менять менеджера договора может только
// admin/superadmin (менеджер не может переназначить договор с/на себя).
func requireContractAssignAccess(c *gin.Context) bool {
	if v, _ := c.Get("is_superadmin"); v == true {
		return true
	}
	role, _ := c.Get("role")
	if r, ok := role.(string); ok && r == models.RoleAdmin {
		return true
	}
	return false
}

// applyManagerScope добавляет к gorm-запросу по таблице с колонкой contract_id
// фильтр по договорам менеджера (если scope применяется). Без эффекта для admin.
func applyManagerScope(c *gin.Context, q *gorm.DB) *gorm.DB {
	ids, applies, err := managerScopedContractIDs(c)
	if !applies || err != nil {
		if err != nil {
			return q.Where("1 = 0") // fail-closed при ошибке выборки договоров менеджера
		}
		return q
	}
	return q.Where("contract_id IN ?", ids)
}
