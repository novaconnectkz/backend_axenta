package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
)

// Догрузка Counterparty к договорам БЕЗ Preload.
//
// counterparties живёт ТОЛЬКО в public-схеме, а tenantDB подключён с
// search_path=tenant_<id> (public НЕ включён, см. database.ConnectToTenant +
// search_path_guard_test). Поэтому `tenantDB.Preload("Counterparty")` падает
// `relation "counterparties" does not exist` (42P01) — в отличие от TariffPlan,
// чья таблица billing_plans реплицирована в tenant-схемы.
//
// database.DB — main-пул в public (без search_path в DSN) → counterparties
// видна. Грузим одним запросом по уникальным id (без N+1).

// attachCounterparties догружает Counterparty к срезу договоров (для списков).
func attachCounterparties(contracts []models.Contract) {
	ids := make([]uint, 0, len(contracts))
	seen := make(map[uint]bool)
	for i := range contracts {
		id := contracts[i].CounterpartyID
		if id != 0 && !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	if len(ids) == 0 {
		return
	}
	var cps []models.Counterparty
	if err := database.DB.Table("public.counterparties").Where("id IN ?", ids).Find(&cps).Error; err != nil {
		return // best-effort: пустой Counterparty → Display-методы упадут на client_* fallback
	}
	byID := make(map[uint]*models.Counterparty, len(cps))
	for i := range cps {
		byID[cps[i].ID] = &cps[i]
	}
	for i := range contracts {
		if cp, ok := byID[contracts[i].CounterpartyID]; ok {
			contracts[i].Counterparty = cp
		}
	}
}

// attachCounterpartiesPtr — как attachCounterparties, но для среза указателей
// (договоры, привязанные к другим сущностям, напр. invoice.Contract). Один
// запрос по уникальным id (без N+1 в циклах).
func attachCounterpartiesPtr(contracts []*models.Contract) {
	ids := make([]uint, 0, len(contracts))
	seen := make(map[uint]bool)
	for _, ct := range contracts {
		if ct == nil || ct.CounterpartyID == 0 || seen[ct.CounterpartyID] {
			continue
		}
		ids = append(ids, ct.CounterpartyID)
		seen[ct.CounterpartyID] = true
	}
	if len(ids) == 0 {
		return
	}
	var cps []models.Counterparty
	if err := database.DB.Table("public.counterparties").Where("id IN ?", ids).Find(&cps).Error; err != nil {
		return
	}
	byID := make(map[uint]*models.Counterparty, len(cps))
	for i := range cps {
		byID[cps[i].ID] = &cps[i]
	}
	for _, ct := range contracts {
		if ct == nil {
			continue
		}
		if cp, ok := byID[ct.CounterpartyID]; ok {
			ct.Counterparty = cp
		}
	}
}

// attachCounterpartyToContract догружает Counterparty к одному договору.
func attachCounterpartyToContract(contract *models.Contract) {
	if contract == nil || contract.CounterpartyID == 0 {
		return
	}
	var cp models.Counterparty
	if err := database.DB.Table("public.counterparties").Where("id = ?", contract.CounterpartyID).First(&cp).Error; err == nil {
		contract.Counterparty = &cp
	}
}
