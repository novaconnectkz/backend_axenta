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

	partnerCompanyID := uint(17153)
	contractID := uint(42) // Договор Т-051225/104

	// Дочерние аккаунты из Excel
	childAccountIDs := []int64{17153, 22911, 24188, 23660, 23662, 23752, 25514, 25492, 25671, 25672}

	fmt.Printf("🔧 Исправление снимков для Partner Company ID: %d\n", partnerCompanyID)
	fmt.Printf("📋 Дочерние аккаунты: %v\n", childAccountIDs)
	fmt.Printf(strings.Repeat("=", 100) + "\n\n")

	// Получаем все компании
	var allCompanies []models.Company
	if err := database.DB.Find(&allCompanies).Error; err != nil {
		fmt.Fprintf(os.Stderr, "❌ Ошибка получения списка компаний: %v\n", err)
		os.Exit(1)
	}

	// Находим компанию GLOMOS (ID=186)
	var glomosCompany *models.Company
	for i := range allCompanies {
		if allCompanies[i].ID == 186 {
			glomosCompany = &allCompanies[i]
			break
		}
	}

	if glomosCompany == nil {
		fmt.Fprintf(os.Stderr, "❌ Компания GLOMOS (ID=186) не найдена\n")
		os.Exit(1)
	}

	tenantDB := database.GetTenantDBByID(glomosCompany.ID)
	if tenantDB == nil {
		fmt.Fprintf(os.Stderr, "❌ Не удалось получить tenant DB для компании %s\n", glomosCompany.Name)
		os.Exit(1)
	}

	// Получаем договор
	var contract models.Contract
	if err := tenantDB.Where("id = ?", contractID).First(&contract).Error; err != nil {
		fmt.Fprintf(os.Stderr, "❌ Договор ID=%d не найден: %v\n", contractID, err)
		os.Exit(1)
	}

	// Получаем тарифный план (из public схемы)
	var tariffPlan models.BillingPlan
	if contract.TariffPlanID != nil && *contract.TariffPlanID > 0 {
		// Тарифные планы хранятся в public схеме
		publicDB := database.DB.Session(&gorm.Session{})
		if err := publicDB.Exec("SET search_path TO public").Error; err == nil {
			if err := publicDB.Where("id = ? AND admin_account_id = ?", *contract.TariffPlanID, contract.AdminAccountID).First(&tariffPlan).Error; err != nil {
				fmt.Fprintf(os.Stderr, "❌ Тарифный план ID=%d не найден: %v\n", *contract.TariffPlanID, err)
				os.Exit(1)
			}
		} else {
			fmt.Fprintf(os.Stderr, "❌ Ошибка установки search_path: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Fprintf(os.Stderr, "❌ У договора нет тарифного плана\n")
		os.Exit(1)
	}

	// Даты для исправления: 01.12.2025 - 05.12.2025
	startDate := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2025, 12, 5, 0, 0, 0, 0, time.UTC)

	currentDate := startDate
	fixedCount := 0

	for !currentDate.After(endDate) {
		dateKey := currentDate.Format("2006-01-02")
		snapshotEndOfDay := time.Date(currentDate.Year(), currentDate.Month(), currentDate.Day(), 23, 59, 59, 999999999, time.UTC)

		fmt.Printf("📅 Обработка даты: %s\n", dateKey)

		// Подсчитываем объекты из БД во всех схемах
		allObjectIDs := make(map[int64]bool)
		activeObjectIDs := make(map[int64]bool)

		for _, company := range allCompanies {
			tenantDBForSearch := database.GetTenantDBByID(company.ID)
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

		fmt.Printf("   Найдено объектов в БД: %d (активных: %d)\n", totalObjects, activeObjects)

		// Находим существующий снимок (включая мягко удаленные)
		// Ищем по partner_company_id и дате, так как снимки могут быть с contract_id=0
		var snapshot models.PartnerDailySnapshot
		err := tenantDB.Unscoped().
			Where("partner_company_id = ? AND DATE(snapshot_date AT TIME ZONE 'UTC') = ? AND (contract_id = ? OR contract_id = 0)",
				partnerCompanyID, dateKey, contractID).
			First(&snapshot).Error

		if err == nil {
			// Снимок существует, обновляем
			oldTotal := snapshot.TotalObjectsCount
			oldActive := snapshot.ActiveObjectsCount

			if oldTotal != totalObjects || oldActive != activeObjects {
				fmt.Printf("   ⚠️ Обновление снимка: было %d/%d, будет %d/%d\n", oldTotal, oldActive, totalObjects, activeObjects)

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
					fmt.Printf("   ❌ Ошибка обновления снимка: %v\n", err)
				} else {
					fmt.Printf("   ✅ Снимок обновлен: стоимость %.2f₽\n", dailyCost.InexactFloat64())
					fixedCount++
				}
			} else {
				fmt.Printf("   ✅ Снимок уже правильный (%d/%d объектов)\n", totalObjects, activeObjects)
			}
		} else if err == gorm.ErrRecordNotFound {
			// Снимок не существует, создаем новый
			fmt.Printf("   ➕ Создание нового снимка\n")

			dailyPrice := tariffPlan.Price.Div(decimal.NewFromInt(30))
			costBeforeDiscount := dailyPrice.Mul(decimal.NewFromInt(int64(activeObjects)))

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

			newSnapshot := models.PartnerDailySnapshot{
				AdminAccountID:     contract.AdminAccountID,
				CompanyID:          glomosCompany.ID,
				ContractID:         contract.ID,
				SnapshotDate:       currentDate,
				PartnerCompanyID:   partnerCompanyID,
				TariffPlanID:       tariffPlan.ID,
				MonthlyPrice:       tariffPlan.Price,
				DailyPrice:         dailyPrice.Round(6),
				TotalObjectsCount:  totalObjects,
				ActiveObjectsCount: activeObjects,
				DiscountType:       discountType,
				DiscountPercent:    discountPercent,
				DiscountFixed:      discountFixed,
				CostBeforeDiscount: costBeforeDiscount.Round(4),
				DiscountAmount:     discountAmount.Round(4),
				DailyCost:          dailyCost.Round(4),
				Status:             "completed",
				Notes:              "Создан из данных БД с учетом всех дочерних аккаунтов",
			}

			if err := tenantDB.Create(&newSnapshot).Error; err != nil {
				fmt.Printf("   ❌ Ошибка создания снимка: %v\n", err)
			} else {
				fmt.Printf("   ✅ Снимок создан: стоимость %.2f₽\n", dailyCost.InexactFloat64())
				fixedCount++
			}
		} else {
			fmt.Printf("   ❌ Ошибка поиска снимка: %v\n", err)
		}

		fmt.Println()
		currentDate = currentDate.AddDate(0, 0, 1)
	}

	fmt.Printf(strings.Repeat("=", 100) + "\n")
	fmt.Printf("✅ Исправлено снимков: %d\n", fixedCount)
	fmt.Printf(strings.Repeat("=", 100) + "\n")
}
