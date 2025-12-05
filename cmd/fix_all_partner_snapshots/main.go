package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// NullWriter для подавления логов
type NullWriter struct{}

func (w *NullWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func main() {
	// Отключаем лишние логи для чистого вывода
	log.SetOutput(&NullWriter{})

	// Загружаем конфигурацию
	config.LoadConfig()

	// Подключаемся к базе данных
	if err := database.ConnectDatabase(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Не удалось подключиться к базе данных: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		sqlDB, _ := database.DB.DB()
		sqlDB.Close()
	}()

	fmt.Printf("🔧 Исправление снимков для всех партнерских договоров\n")
	fmt.Printf(strings.Repeat("=", 100) + "\n\n")

	// Получаем все компании
	var allCompanies []models.Company
	if err := database.DB.Find(&allCompanies).Error; err != nil {
		fmt.Fprintf(os.Stderr, "❌ Ошибка получения списка компаний: %v\n", err)
		os.Exit(1)
	}

	totalFixed := 0
	totalChecked := 0

	// Обрабатываем каждую компанию
	for _, company := range allCompanies {
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			continue
		}

		// Получаем все партнерские договоры с partner_company_id
		var contracts []models.Contract
		if err := tenantDB.
			Where("contract_type = ? AND partner_company_id IS NOT NULL AND tariff_plan_id IS NOT NULL", "partner").
			Find(&contracts).Error; err != nil {
			continue
		}

		if len(contracts) == 0 {
			continue
		}

		fmt.Printf("🏢 Компания: %s (ID=%d, схема: %s)\n", company.Name, company.ID, company.DatabaseSchema)
		fmt.Printf("   Найдено партнерских договоров: %d\n\n", len(contracts))

		// Обрабатываем каждый договор
		for _, contract := range contracts {
			if contract.PartnerCompanyID == nil {
				continue
			}

			partnerCompanyID := *contract.PartnerCompanyID
			totalChecked++

			fmt.Printf("   📄 Договор ID=%d, номер=%s, Partner Company ID=%d\n", contract.ID, contract.Number, partnerCompanyID)

			// Получаем тарифный план (из public схемы)
			var tariffPlan models.BillingPlan
			if contract.TariffPlanID != nil && *contract.TariffPlanID > 0 {
				publicDB := database.DB.Session(&gorm.Session{})
				if err := publicDB.Exec("SET search_path TO public").Error; err == nil {
					if err := publicDB.Where("id = ? AND admin_account_id = ?", *contract.TariffPlanID, contract.AdminAccountID).First(&tariffPlan).Error; err != nil {
						fmt.Printf("      ⚠️ Тарифный план ID=%d не найден, пропускаем\n\n", *contract.TariffPlanID)
						continue
					}
				} else {
					fmt.Printf("      ⚠️ Ошибка установки search_path, пропускаем\n\n")
					continue
				}
			} else {
				fmt.Printf("      ⚠️ У договора нет тарифного плана, пропускаем\n\n")
				continue
			}

			// Находим все дочерние аккаунты партнера во всех схемах
			childAccountIDs := []int64{int64(partnerCompanyID)} // Добавляем сам партнер
			partnerIDStr := fmt.Sprintf("%d", partnerCompanyID)
			seenAccountIDs := make(map[int64]bool)
			seenAccountIDs[int64(partnerCompanyID)] = true

			for _, comp := range allCompanies {
				tenantDBForSearch := database.GetTenantDBByID(comp.ID)
				if tenantDBForSearch == nil {
					continue
				}

				var childAccounts []models.AxentaAccountSnapshot
				if err := tenantDBForSearch.Model(&models.AxentaAccountSnapshot{}).
					Where("admin_account_id = ?", contract.AdminAccountID).
					Where("(hierarchy LIKE ? OR hierarchy LIKE ? OR hierarchy LIKE ? OR external_account_id = ?)",
						fmt.Sprintf("%%/%%%s/%%", partnerIDStr),
						fmt.Sprintf("%%/%%%s%%", partnerIDStr),
						fmt.Sprintf("%%/%s/%%", partnerIDStr),
						int64(partnerCompanyID)).
					Find(&childAccounts).Error; err == nil {
					for _, acc := range childAccounts {
						if !seenAccountIDs[acc.ExternalAccountID] {
							childAccountIDs = append(childAccountIDs, acc.ExternalAccountID)
							seenAccountIDs[acc.ExternalAccountID] = true
						}
					}
				}
			}

			fmt.Printf("      Найдено дочерних аккаунтов: %d\n", len(childAccountIDs)-1)

			// Получаем все существующие снимки для этого договора
			var existingSnapshots []models.PartnerDailySnapshot
			tenantDB.Unscoped().
				Where("contract_id = ? OR (partner_company_id = ? AND contract_id = 0)", contract.ID, partnerCompanyID).
				Order("snapshot_date DESC").
				Find(&existingSnapshots)

			if len(existingSnapshots) == 0 {
				fmt.Printf("      ℹ️ Снимков не найдено\n\n")
				continue
			}

			fmt.Printf("      Найдено снимков: %d\n", len(existingSnapshots))

			// Обрабатываем каждый снимок
			fixedForContract := 0
			for _, snapshot := range existingSnapshots {
				dateKey := snapshot.SnapshotDate.Format("2006-01-02")
				snapshotEndOfDay := time.Date(
					snapshot.SnapshotDate.Year(),
					snapshot.SnapshotDate.Month(),
					snapshot.SnapshotDate.Day(),
					23, 59, 59, 999999999, time.UTC)

				// Подсчитываем объекты из БД во всех схемах
				allObjectIDs := make(map[int64]bool)
				activeObjectIDs := make(map[int64]bool)

				for _, comp := range allCompanies {
					tenantDBForSearch := database.GetTenantDBByID(comp.ID)
					if tenantDBForSearch == nil {
						continue
					}

					var objects []struct {
						ExternalObjectID int64 `gorm:"column:external_object_id"`
						IsActive         bool  `gorm:"column:is_active"`
					}

					err := tenantDBForSearch.Raw(`
						SELECT DISTINCT aos1.external_object_id, aos1.is_active
						FROM axenta_object_snapshots aos1
						WHERE aos1.account_external_id IN ?
							AND (aos1.axenta_created_at IS NULL OR aos1.axenta_created_at <= ?)
							AND (aos1.axenta_deleted_at IS NULL OR aos1.axenta_deleted_at > ?)
							AND aos1.last_synced_at = (
								SELECT MAX(aos2.last_synced_at)
								FROM axenta_object_snapshots aos2
								WHERE aos2.external_object_id = aos1.external_object_id
							)
					`, childAccountIDs, snapshotEndOfDay, snapshotEndOfDay).
						Scan(&objects).Error

					if err == nil {
						for _, obj := range objects {
							allObjectIDs[obj.ExternalObjectID] = true
							if obj.IsActive {
								activeObjectIDs[obj.ExternalObjectID] = true
							}
						}
					}
				}

				totalObjects := len(allObjectIDs)
				activeObjects := len(activeObjectIDs)

				// Проверяем, нужно ли обновлять
				if snapshot.TotalObjectsCount == totalObjects && snapshot.ActiveObjectsCount == activeObjects {
					continue // Уже правильный
				}

				// Обновляем снимок
				oldTotal := snapshot.TotalObjectsCount
				oldActive := snapshot.ActiveObjectsCount

				// Пересчитываем стоимость
				dailyPrice := tariffPlan.Price.Div(decimal.NewFromInt(30))
				costBeforeDiscount := dailyPrice.Mul(decimal.NewFromInt(int64(activeObjects)))

				// Скидки
				discountPercent := contract.GetDiscountPercent(activeObjects)
				discountFixed := contract.GetDiscountFixed()
				var discountAmount decimal.Decimal
				discountType := "none"

				if discountFixed.GreaterThan(decimal.Zero) {
					discountType = "manual"
					effectiveMonthlyPrice := tariffPlan.Price.Sub(discountFixed)
					if effectiveMonthlyPrice.IsNegative() {
						effectiveMonthlyPrice = decimal.Zero
					}
					effectiveDailyPrice := effectiveMonthlyPrice.Div(decimal.NewFromInt(30))
					dailyPrice = effectiveDailyPrice
					discountAmount = discountFixed.Div(decimal.NewFromInt(30)).Mul(decimal.NewFromInt(int64(activeObjects)))
				} else if discountPercent.GreaterThan(decimal.Zero) {
					discountType = "auto"
					discountMultiplier := discountPercent.Div(decimal.NewFromInt(100))
					discountAmount = costBeforeDiscount.Mul(discountMultiplier)
				}

				dailyCost := costBeforeDiscount.Sub(discountAmount)

				// Обновляем снимок
				snapshot.TotalObjectsCount = totalObjects
				snapshot.ActiveObjectsCount = activeObjects
				snapshot.DailyPrice = dailyPrice.Round(6)
				snapshot.CostBeforeDiscount = costBeforeDiscount.Round(4)
				snapshot.DiscountAmount = discountAmount.Round(4)
				snapshot.DailyCost = dailyCost.Round(4)
				snapshot.DiscountType = discountType
				snapshot.DiscountPercent = discountPercent
				snapshot.DiscountFixed = discountFixed
				snapshot.Notes = fmt.Sprintf("Исправлено: было %d/%d объектов, стало %d/%d", oldTotal, oldActive, totalObjects, activeObjects)

				if err := tenantDB.Save(&snapshot).Error; err != nil {
					fmt.Printf("         ❌ Ошибка обновления снимка %s: %v\n", dateKey, err)
				} else {
					fmt.Printf("         ✅ %s: %d/%d → %d/%d объектов (%.2f₽)\n",
						dateKey, oldTotal, oldActive, totalObjects, activeObjects, dailyCost.InexactFloat64())
					fixedForContract++
					totalFixed++
				}
			}

			if fixedForContract > 0 {
				fmt.Printf("      ✅ Исправлено снимков: %d\n\n", fixedForContract)
			} else {
				fmt.Printf("      ℹ️ Все снимки уже правильные\n\n")
			}
		}
	}

	fmt.Printf(strings.Repeat("=", 100) + "\n")
	fmt.Printf("📊 ИТОГОВАЯ СТАТИСТИКА:\n")
	fmt.Printf("   Проверено договоров: %d\n", totalChecked)
	fmt.Printf("   Исправлено снимков: %d\n", totalFixed)
	fmt.Printf(strings.Repeat("=", 100) + "\n")
}
