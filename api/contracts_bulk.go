package api

import (
	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// validContractStatuses — допустимые статусы договора (см. models.Contract.Status).
var validContractStatuses = map[string]bool{
	"draft": true, "active": true, "expired": true, "cancelled": true, "suspended": true,
}

// maxBulkContracts — потолок на размер пакета (защита от случайного «выделить всё» на
// многотысячных списках, бьющего одной транзакцией-циклом).
const maxBulkContracts = 500

// bulkContractResult — единый ответ массовых операций: что применилось, что нет.
type bulkContractResult struct {
	Updated int      `json:"updated"`
	Failed  []uint   `json:"failed"`
	Errors  []string `json:"errors,omitempty"`
}

// bulkPreflight — общие проверки массовых операций: доступ (только admin/superadmin),
// adminAccountID, tenantDB, разбор contract_ids с лимитом. Возвращает false если уже
// ответил ошибкой.
func bulkPreflight(c *gin.Context, ids []uint) (uint, *gorm.DB, bool) {
	if !requireContractAssignAccess(c) {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "массовые действия доступны только администратору"})
		return 0, nil, false
	}
	if len(ids) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "не выбрано ни одного договора"})
		return 0, nil, false
	}
	if len(ids) > maxBulkContracts {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": fmt.Sprintf("за раз не более %d договоров", maxBulkContracts)})
		return 0, nil, false
	}
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": err.Error()})
		return 0, nil, false
	}
	// Fail-closed: для массовых мутаций НЕ падаем на global database.DB при пустом
	// tenant-контексте (Codex HIGH: иначе bulk бьёт по public/чужой схеме при ID-коллизии).
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		log.Printf("❌ Bulk: tenant DB из контекста пуст — массовая операция отклонена")
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "не определён tenant-контекст"})
		return 0, nil, false
	}
	return adminAccountID, tenantDB, true
}

// loadOwnedContract — грузит договор со скоупом admin_account + проверкой доступа
// менеджера. Возвращает (nil, false) если не найден / вне доступа (пропускаем в bulk).
func loadOwnedContract(c *gin.Context, tenantDB *gorm.DB, adminAccountID, id uint) (*models.Contract, bool) {
	var ct models.Contract
	if err := tenantDB.Where("id = ? AND admin_account_id = ?", id, adminAccountID).First(&ct).Error; err != nil {
		return nil, false
	}
	if !managerCanAccessContract(c, ct.ID) {
		return nil, false
	}
	return &ct, true
}

// BulkAssignManager — POST /api/auth/contracts/bulk/manager
// Body: {"contract_ids":[..], "manager_id": <uint|null>}. manager_id null/0 — снять менеджера.
func BulkAssignManager(c *gin.Context) {
	var req struct {
		ContractIDs []uint `json:"contract_ids"`
		ManagerID   *uint  `json:"manager_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "некорректное тело запроса"})
		return
	}
	adminAccountID, tenantDB, ok := bulkPreflight(c, req.ContractIDs)
	if !ok {
		return
	}

	// Менеджера резолвим один раз (валидация роли + имя). Снятие — manager_id пуст.
	var managerID *uint
	var managerName string
	if req.ManagerID != nil && *req.ManagerID > 0 {
		name, valid := resolveManagerName(*req.ManagerID)
		if !valid {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "недопустимый менеджер для назначения"})
			return
		}
		managerID = req.ManagerID
		managerName = name
	}

	res := bulkContractResult{}
	for _, id := range req.ContractIDs {
		ct, owned := loadOwnedContract(c, tenantDB, adminAccountID, id)
		if !owned {
			res.Failed = append(res.Failed, id)
			continue
		}
		// Явный map-update: manager_id может стать NULL (снятие) — Updates(struct) пропустил бы nil.
		if err := tenantDB.Model(&models.Contract{}).Where("id = ?", ct.ID).
			Updates(map[string]interface{}{"manager_id": managerID, "manager_name": managerName}).Error; err != nil {
			res.Failed = append(res.Failed, id)
			res.Errors = append(res.Errors, fmt.Sprintf("договор %d: %v", id, err))
			continue
		}
		res.Updated++
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": res})
}

// BulkSetStatus — POST /api/auth/contracts/bulk/status
// Body: {"contract_ids":[..], "status":"active|draft|suspended|expired|cancelled"}.
func BulkSetStatus(c *gin.Context) {
	var req struct {
		ContractIDs []uint `json:"contract_ids"`
		Status      string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "некорректное тело запроса"})
		return
	}
	if !validContractStatuses[req.Status] {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "недопустимый статус договора"})
		return
	}
	adminAccountID, tenantDB, ok := bulkPreflight(c, req.ContractIDs)
	if !ok {
		return
	}

	res := bulkContractResult{}
	for _, id := range req.ContractIDs {
		ct, owned := loadOwnedContract(c, tenantDB, adminAccountID, id)
		if !owned {
			res.Failed = append(res.Failed, id)
			continue
		}
		if err := tenantDB.Model(&models.Contract{}).Where("id = ?", ct.ID).
			Update("status", req.Status).Error; err != nil {
			res.Failed = append(res.Failed, id)
			res.Errors = append(res.Errors, fmt.Sprintf("договор %d: %v", id, err))
			continue
		}
		res.Updated++
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": res})
}

// contractHasActiveObjects — есть ли у договора привязанные активные объекты (junction).
// Ошибку Count возвращаем наружу — вызывающий обязан трактовать её как «не знаем» и
// блокировать удаление (Codex MEDIUM: молчаливый fail-open удалил бы активный договор).
func contractHasActiveObjects(tenantDB *gorm.DB, contractID uint) (bool, error) {
	var cnt int64
	if err := tenantDB.Model(&models.ContractObject{}).
		Where("contract_id = ? AND status = ?", contractID, "active").Count(&cnt).Error; err != nil {
		return false, err
	}
	return cnt > 0, nil
}

// contractDeletable — можно ли удалять договор. Бизнес-правило: активный договор с
// привязанными объектами удалять нельзя (сначала отвязать объекты или сменить статус).
// При ошибке проверки — fail-closed (не удаляем).
func contractDeletable(tenantDB *gorm.DB, ct *models.Contract) (bool, string) {
	if ct.Status == "active" {
		has, err := contractHasActiveObjects(tenantDB, ct.ID)
		if err != nil {
			return false, "не удалось проверить привязанные объекты — удаление отклонено"
		}
		if has {
			return false, "активный договор с привязанными объектами нельзя удалить — отвяжите объекты или смените статус"
		}
	}
	return true, ""
}

// BulkDeleteContracts — POST /api/auth/contracts/bulk-delete
// Body: {"contract_ids":[..]}. Мягкое удаление + запись в корзину (как DeleteContract).
// Активные договоры с привязанными объектами пропускаются (→ failed).
func BulkDeleteContracts(c *gin.Context) {
	var req struct {
		ContractIDs []uint `json:"contract_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "некорректное тело запроса"})
		return
	}
	adminAccountID, tenantDB, ok := bulkPreflight(c, req.ContractIDs)
	if !ok {
		return
	}

	res := bulkContractResult{}
	for _, id := range req.ContractIDs {
		ct, owned := loadOwnedContract(c, tenantDB, adminAccountID, id)
		if !owned {
			res.Failed = append(res.Failed, id)
			continue
		}
		if deletable, reason := contractDeletable(tenantDB, ct); !deletable {
			res.Failed = append(res.Failed, id)
			res.Errors = append(res.Errors, fmt.Sprintf("договор %d: %s", id, reason))
			continue
		}
		if err := purgeContractCleanup(c, tenantDB, ct); err != nil {
			res.Failed = append(res.Failed, id)
			res.Errors = append(res.Errors, fmt.Sprintf("договор %d: %v", id, err))
			continue
		}
		res.Updated++
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": res})
}

// purgeContractCleanup — общий путь удаления договора (для DeleteContract и bulk):
// отвязка объектов, чистка junction/приложений, запись в корзину, мягкое удаление.
// Возвращает error только если упало само мягкое удаление (остальное best-effort с логом).
func purgeContractCleanup(c *gin.Context, tenantDB *gorm.DB, contract *models.Contract) error {
	// 1. Отвязываем объекты (contract_id → NULL).
	if err := tenantDB.Model(&models.Object{}).Where("contract_id = ?", contract.ID).
		UpdateColumn("contract_id", gorm.Expr("NULL")).Error; err != nil {
		log.Printf("⚠️ Не удалось отвязать объекты от договора %d: %v", contract.ID, err)
	}
	// 2. Junction-связи.
	if err := tenantDB.Where("contract_id = ?", contract.ID).Delete(&models.ContractObject{}).Error; err != nil {
		log.Printf("⚠️ Не удалось удалить junction договора %d: %v", contract.ID, err)
	}
	// NB: подписки (public.subscriptions) тут НЕ трогаем — это soft-delete (договор в корзине,
	// восстановим через restoreContract); подписки нужны для restore, а charge-движок soft-deleted
	// договор и так пропускает (First не находит). Чистка подписок — только при ПЕРМАНЕНТНОМ
	// удалении (permanentlyDeleteContract, api/trash.go), где договор уже невосстановим.
	// 3. Приложения (tenant → fallback public).
	if err := tenantDB.Where("contract_id = ?", contract.ID).Delete(&models.ContractAppendix{}).Error; err != nil {
		publicDB := database.DB.Session(&gorm.Session{})
		if e := publicDB.Exec("SET search_path TO public").Error; e == nil {
			if e := publicDB.Where("contract_id = ?", contract.ID).Delete(&models.ContractAppendix{}).Error; e != nil {
				log.Printf("⚠️ Приложения договора %d: tenant+public не удалены: %v", contract.ID, e)
			}
		}
	}
	// 4. Корзина (best-effort).
	var deletedBy uint
	var deletedByName string
	var companyIDUint uint
	if userID, exists := c.Get("user_id"); exists {
		deletedBy, _ = userID.(uint)
		var user models.User
		if err := tenantDB.First(&user, deletedBy).Error; err == nil {
			if user.FirstName != "" || user.LastName != "" {
				deletedByName = fmt.Sprintf("%s %s", user.FirstName, user.LastName)
			} else {
				deletedByName = user.Username
			}
		}
	}
	if companyID, exists := c.Get("company_id"); exists {
		companyIDUint, _ = companyID.(uint)
	}
	entityName := fmt.Sprintf("Договор %s", contract.Number)
	entityDescription := fmt.Sprintf("Клиент: %s", contract.DisplayName())
	if contract.Title != "" {
		entityDescription += fmt.Sprintf(", %s", contract.Title)
	}
	contractSimplified := map[string]interface{}{
		"id": contract.ID, "number": contract.Number, "title": contract.Title,
		"description": contract.Description, "client_type": contract.ClientType,
		"client_name": contract.ClientName, "client_short_name": contract.ClientShortName,
		"client_inn": contract.ClientINN, "client_kpp": contract.ClientKPP,
		"client_email": contract.ClientEmail, "client_phone": contract.ClientPhone,
		"status": contract.Status, "start_date": contract.StartDate, "end_date": contract.EndDate,
		"total_amount": contract.TotalAmount, "created_at": contract.CreatedAt, "updated_at": contract.UpdatedAt,
	}
	if err := RecordDeletion(tenantDB, "contract", contract.ID, contractSimplified,
		deletedBy, deletedByName, companyIDUint, entityName, entityDescription); err != nil {
		log.Printf("⚠️ Корзина: договор %d не записан: %v", contract.ID, err)
	}
	// 5. Мягкое удаление — единственный шаг, чья ошибка фатальна.
	return tenantDB.Delete(contract).Error
}
