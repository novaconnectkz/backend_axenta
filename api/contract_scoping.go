package api

import (
	"net/http"
	"strings"

	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// GetBillingManagers — GET /api/auth/billing/managers
// Список локальных пользователей (роли manager/admin) для селектора «Менеджер» на договоре.
func GetBillingManagers(c *gin.Context) {
	// Список менеджеров для назначения — только admin/superadmin (manager не назначает).
	if !requireContractAssignAccess(c) {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"status": "error", "error": "доступно только администратору"})
		return
	}
	var users []models.LocalUser
	database.DB.Table("public.local_users").
		Where("role IN ?", []string{models.RoleManager, models.RoleAdmin}).
		Order("name").Find(&users)
	out := make([]gin.H, 0, len(users))
	for _, u := range users {
		name := strings.TrimSpace(u.Name)
		if name == "" {
			name = u.Username
		}
		out = append(out, gin.H{"id": u.ID, "name": name, "role": u.Role})
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": out})
}

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
		return []uint{0}, true, nil // нет tenant-БД у manager → ничего (fail-closed)
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

// managerScopedCounterparties — Ф4: контрагенты, доступные менеджеру = контрагенты его
// договоров (cp<>0). Возвращает (cpIDs, applies, err): applies=false → admin/superadmin (все).
// applies=true с []{0} → у менеджера нет своих контрагентов (пустой/невозможный IN).
func managerScopedCounterparties(c *gin.Context) ([]uint, bool, error) {
	contractIDs, applies, err := managerScopedContractIDs(c)
	if !applies || err != nil {
		return nil, applies, err
	}
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		return []uint{0}, true, nil // fail-closed
	}
	var cpIDs []uint
	if e := tenantDB.Model(&models.Contract{}).
		Where("id IN ? AND counterparty_id <> 0", contractIDs).
		Distinct().Pluck("counterparty_id", &cpIDs).Error; e != nil {
		return []uint{0}, true, e
	}
	if len(cpIDs) == 0 {
		return []uint{0}, true, nil
	}
	return cpIDs, true, nil
}

// applyCounterpartyManagerScope — добавляет к запросу по public.counterparties фильтр
// `id IN <контрагенты менеджера>` (для роли manager). admin/superadmin — без изменений.
func applyCounterpartyManagerScope(c *gin.Context, q *gorm.DB) *gorm.DB {
	cpIDs, applies, err := managerScopedCounterparties(c)
	if !applies {
		return q
	}
	if err != nil {
		return q.Where("1 = 0") // fail-closed
	}
	return q.Where("id IN ?", cpIDs)
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
		return false // для manager отсутствие tenant-БД = deny (fail-closed)
	}
	var count int64
	if err := tenantDB.Model(&models.Contract{}).
		Where("id = ? AND manager_id = ?", contractID, uid).Count(&count).Error; err != nil {
		return false // fail-closed
	}
	return count > 0
}

// requireBillingOperation — разрешена ли операция (перевод/отсрочка/обещание) по
// политике OperationRoleThreshold компании. admin/superadmin — всегда; manager —
// только если threshold='manager'. Иначе (нет политики / прочие роли) — deny.
func requireBillingOperation(c *gin.Context, adminAccountID, companyID uint) bool {
	if requireContractAssignAccess(c) {
		return true // admin/superadmin
	}
	if !isManagerScoped(c) {
		return false // не manager и не admin → deny
	}
	var bs models.BillingSettings
	if err := database.DB.Table("public.billing_settings").
		Where("company_id = ? AND admin_account_id = ?", companyID, adminAccountID).
		First(&bs).Error; err != nil {
		return false // нет политики → deny
	}
	return bs.OperationRoleThreshold == models.RoleManager
}

// validateBillingModePolicy проверяет, что выбранный режим/лимит допустимы политикой
// компании: postpaid разрешён только при AllowPostpaid, credit_limit ≤ MaxCreditLimit.
// Возвращает (ok, errMsg). Пустой/prepaid режим всегда ок.
func validateBillingModePolicy(adminAccountID, companyID uint, mode string, limit decimal.Decimal) (bool, string) {
	if limit.IsNegative() {
		return false, "кредит-лимит не может быть отрицательным"
	}
	if mode == "" || mode == "prepaid" {
		if limit.GreaterThan(decimal.Zero) {
			return false, "кредит-лимит допустим только в режиме постоплаты"
		}
		return true, ""
	}
	if mode != "postpaid" {
		return false, "неизвестный режим биллинга"
	}
	var bs models.BillingSettings
	if err := database.DB.Table("public.billing_settings").
		Where("company_id = ? AND admin_account_id = ?", companyID, adminAccountID).First(&bs).Error; err != nil {
		return false, "политика биллинга не настроена — постоплата недоступна"
	}
	if !bs.AllowPostpaid {
		return false, "постоплата не разрешена политикой компании"
	}
	if bs.MaxCreditLimit.GreaterThan(decimal.Zero) && limit.GreaterThan(bs.MaxCreditLimit) {
		return false, "кредит-лимит превышает максимум политики (" + bs.MaxCreditLimit.StringFixed(2) + ")"
	}
	return true, ""
}

// resolveManagerName проверяет, что назначаемый пользователь существует и имеет
// допустимую роль (manager/admin), и возвращает денорм-имя. ok=false → назначение
// невалидно (нельзя привязать произвольного local_user как менеджера).
func resolveManagerName(id uint) (string, bool) {
	var lu models.LocalUser
	if err := database.DB.Table("public.local_users").
		Where("id = ? AND role IN ?", id, []string{models.RoleManager, models.RoleAdmin}).
		First(&lu).Error; err != nil {
		return "", false
	}
	name := strings.TrimSpace(lu.Name)
	if name == "" {
		name = lu.Username
	}
	return name, true
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

// applyContractTableScope — для запросов ПО таблице contracts (колонка manager_id).
// manager → только свои (manager_id=self); admin/superadmin — без изменений.
func applyContractTableScope(c *gin.Context, q *gorm.DB) *gorm.DB {
	if !isManagerScoped(c) {
		return q
	}
	if uid, ok := currentUserID(c); ok {
		return q.Where("manager_id = ?", uid)
	}
	return q.Where("1 = 0")
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
