package services

import (
	"backend_axenta/models"

	"gorm.io/gorm"
)

// attachCounterpartiesViaDB догружает Counterparty к договорам через явную
// квалификацию public.counterparties.
//
// counterparties живёт ТОЛЬКО в public-схеме. Если db — tenant-conn
// (search_path=tenant_<id> без public), `Preload("Counterparty")` падает
// `relation "counterparties" does not exist` (42P01). Явный
// Table("public.counterparties") резолвится при любом search_path. Один
// запрос по уникальным id (без N+1).
func attachCounterpartiesViaDB(db *gorm.DB, contracts []models.Contract) {
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
	if err := db.Table("public.counterparties").Where("id IN ?", ids).Find(&cps).Error; err != nil {
		return // best-effort: Display-методы упадут на client_* fallback
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
