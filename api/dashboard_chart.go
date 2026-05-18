package api

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"backend_axenta/middleware"
)

// CurrencyRevenue — выручка в одной валюте за месяц.
type CurrencyRevenue struct {
	Currency string  `json:"currency"` // "RUB", "KZT", ...
	Raw      float64 `json:"raw"`      // Числовое значение
	Text     string  `json:"text"`     // Форматированная строка с валютным символом
}

// ChartPoint — одна точка timeseries на месяц.
type ChartPoint struct {
	Month      string            `json:"month"`       // "2026-01"
	MonthLabel string            `json:"month_label"` // "Янв"
	Objects    int64             `json:"objects"`     // Сумма по всем источникам
	Axenta     int64             `json:"axenta"`      // Axenta — точная история
	WH         int64             `json:"wh"`          // Wialon Hosting — proxy через wialon_units.created_at
	WL         int64             `json:"wl"`          // Wialon Local — proxy
	Skif       int64             `json:"skif"`        // SKIF.PRO — proxy через skif_units.created_at
	Gelios     int64             `json:"gelios"`      // GELIOS — proxy через gelios_created_at
	Revenues   []CurrencyRevenue `json:"revenues"`    // Выручки по валютам

	// Per-bucket created/deleted (sum events за окно бакета).
	// Источники: wialon_daily_snapshots / skif_daily_snapshots / axenta_object_snapshots.
	// GELIOS — proxy из gelios_created_at/gelios_deleted_at (нет operation-log API,
	// deleted = дата когда scheduler подтвердил исчезновение, не реального удаления).
	AxentaCreated int64 `json:"axenta_created"`
	AxentaDeleted int64 `json:"axenta_deleted"`
	WHCreated     int64 `json:"wh_created"`
	WHDeleted     int64 `json:"wh_deleted"`
	WLCreated     int64 `json:"wl_created"`
	WLDeleted     int64 `json:"wl_deleted"`
	SkifCreated   int64 `json:"skif_created"`
	SkifDeleted   int64 `json:"skif_deleted"`
	GeliosCreated int64 `json:"gelios_created"`
	GeliosDeleted int64 `json:"gelios_deleted"`
}

// ChartResponse — payload ответа /api/auth/dashboard/chart.
type ChartResponse struct {
	Points      []ChartPoint `json:"points"`
	Currencies  []string     `json:"currencies"` // Уникальные валюты, встретившиеся в окне
	GeneratedAt time.Time    `json:"generated_at"`
}

// bucket — диапазон [start, end) с label для оси.
type chartBucket struct {
	start time.Time
	end   time.Time
	label string
	key   string
}

// buildBuckets создаёт список бакетов под выбранный period.
//
// period:
//   - 7d  : 7 дневных бакетов (последние 7 дней включая сегодня)
//   - 1m  : ~30 дневных бакетов (с 1-го числа предыдущего месяца до сегодня)
//   - 3m  : ~13 недельных бакетов (последние 13 недель)
//   - 6m  : 6 месячных бакетов
//   - 1y  : 12 месячных бакетов
func buildBuckets(now time.Time, period string) []chartBucket {
	loc := now.Location()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	switch period {
	case "7d":
		out := make([]chartBucket, 7)
		for i := 0; i < 7; i++ {
			start := today.AddDate(0, 0, -(6 - i))
			end := start.AddDate(0, 0, 1)
			out[i] = chartBucket{start, end, dayShort(start), start.Format("2006-01-02")}
		}
		return out
	case "1m":
		// 30 дней
		out := make([]chartBucket, 30)
		for i := 0; i < 30; i++ {
			start := today.AddDate(0, 0, -(29 - i))
			end := start.AddDate(0, 0, 1)
			out[i] = chartBucket{start, end, dayShort(start), start.Format("2006-01-02")}
		}
		return out
	case "3m":
		// 13 недельных бакетов (понедельник как старт)
		weekday := int(today.Weekday())
		if weekday == 0 {
			weekday = 7 // воскресенье → 7 чтобы понедельник = 1
		}
		monday := today.AddDate(0, 0, -(weekday - 1))
		out := make([]chartBucket, 13)
		for i := 0; i < 13; i++ {
			start := monday.AddDate(0, 0, -7*(12-i))
			end := start.AddDate(0, 0, 7)
			out[i] = chartBucket{start, end, weekShort(start), start.Format("2006-01-02")}
		}
		return out
	case "1y":
		// 12 месячных бакетов
		months := 12
		currentMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
		out := make([]chartBucket, months)
		for i := 0; i < months; i++ {
			start := currentMonthStart.AddDate(0, -(months-1-i), 0)
			end := start.AddDate(0, 1, 0)
			out[i] = chartBucket{start, end, monthShortRu(start.Month()), start.Format("2006-01")}
		}
		return out
	default: // "6m" + fallback
		months := 6
		currentMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
		out := make([]chartBucket, months)
		for i := 0; i < months; i++ {
			start := currentMonthStart.AddDate(0, -(months-1-i), 0)
			end := start.AddDate(0, 1, 0)
			out[i] = chartBucket{start, end, monthShortRu(start.Month()), start.Format("2006-01")}
		}
		return out
	}
}

// GetDashboardChart возвращает timeseries за выбранный период.
//
// GET /api/auth/dashboard/chart?period=7d|1m|3m|6m|1y
//
// По умолчанию period=7d.
//
// Метрики на каждом бакете:
//   - axenta: count axenta_object_snapshots на end_of_bucket
//     (created_at <= end AND deleted_at IS NULL AND
//     (axenta_deleted_at IS NULL OR axenta_deleted_at > end))
//   - wh / wl: count wialon_units JOIN wialon_connections GROUP BY connection_type
//   - revenues: SUM(invoices.paid_amount) GROUP BY currency в окне [start, end)
//
// Если в счетах несколько валют — frontend рисует основную и показывает разбивку
// в tooltip; конвертацию не делаем (нет курса).
func GetDashboardChart(c *gin.Context) {
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подключения к базе данных компании",
		})
		return
	}

	// Backwards-compat: если есть `months=N` — мапим на 6m/1y
	period := c.Query("period")
	if period == "" {
		if m := c.Query("months"); m != "" {
			if v, err := strconv.Atoi(m); err == nil {
				switch {
				case v <= 1:
					period = "1m"
				case v <= 3:
					period = "3m"
				case v <= 6:
					period = "6m"
				default:
					period = "1y"
				}
			}
		}
	}
	switch period {
	case "7d", "1m", "3m", "6m", "1y":
	default:
		period = "7d"
	}

	companyID := middleware.GetCompanyID(c)
	publicDB := publicDBOrTenant(tenantDB)

	// Проверяем включена ли история Wialon для company — определяет источник данных WH/WL
	historyEnabled := false
	if companyID > 0 {
		var settings struct {
			Enabled bool
		}
		publicDB.Table("wialon_history_settings").
			Select("enabled").
			Where("company_id = ?", companyID).
			Scan(&settings)
		historyEnabled = settings.Enabled
	}

	// Текущий count wialon_units per type (для fallback когда нет истории)
	type currentRow struct {
		ConnType string
		Cnt      int64
	}
	currentByType := map[string]int64{}
	{
		var rows []currentRow
		baseQ := `
			SELECT wc.connection_type AS conn_type, COUNT(*) AS cnt
			FROM ` + publicTable(publicDB, "wialon_units") + ` wu
			JOIN ` + publicTable(publicDB, "wialon_connections") + ` wc ON wc.id = wu.connection_id
		`
		if companyID > 0 {
			publicDB.Raw(baseQ+" WHERE wc.company_id = ? GROUP BY wc.connection_type", companyID).Scan(&rows)
		} else {
			publicDB.Raw(baseQ + " GROUP BY wc.connection_type").Scan(&rows)
		}
		for _, r := range rows {
			currentByType[r.ConnType] = r.Cnt
		}
	}

	now := time.Now()
	buckets := buildBuckets(now, period)

	currencySet := make(map[string]struct{})
	points := make([]ChartPoint, 0, len(buckets))

	for _, b := range buckets {
		eom := b.end.Add(-time.Nanosecond)

		// Axenta: используем axenta_created_at (реальная дата создания в Axenta Cloud)
		// с COALESCE на наш created_at для записей где axenta_created_at NULL.
		// Это даёт точную историю объектов, а не "когда scheduler впервые увидел".
		// Источник истины — axenta_deleted_at (реальное состояние в Axenta Cloud).
		// gorm soft-delete (deleted_at) — внутренний cleanup, в chart не учитываем:
		// возвращённые после soft-delete объекты иначе теряются.
		var axentaCount int64
		tenantDB.Unscoped().Table("axenta_object_snapshots").
			Where("COALESCE(axenta_created_at, created_at) <= ?", eom).
			Where("(axenta_deleted_at IS NULL OR axenta_deleted_at > ?)", eom).
			Count(&axentaCount)

		type whwlRow struct {
			ConnType string
			Cnt      int64
		}
		var whwlRows []whwlRow

		// Fallback proxy: count wialon_units по нашему created_at — всегда считаем,
		// используется per-type если snapshot отсутствует.
		var fallbackRows []whwlRow
		fallbackQuery := `
			SELECT wc.connection_type AS conn_type, COUNT(*) AS cnt
			FROM ` + publicTable(publicDB, "wialon_units") + ` wu
			JOIN ` + publicTable(publicDB, "wialon_connections") + ` wc ON wc.id = wu.connection_id
			WHERE wu.created_at <= ?
			GROUP BY wc.connection_type
		`
		if companyID > 0 {
			fallbackQuery = `
				SELECT wc.connection_type AS conn_type, COUNT(*) AS cnt
				FROM ` + publicTable(publicDB, "wialon_units") + ` wu
				JOIN ` + publicTable(publicDB, "wialon_connections") + ` wc ON wc.id = wu.connection_id
				WHERE wc.company_id = ? AND wu.created_at <= ?
				GROUP BY wc.connection_type
			`
			publicDB.Raw(fallbackQuery, companyID, eom).Scan(&fallbackRows)
		} else {
			publicDB.Raw(fallbackQuery, eom).Scan(&fallbackRows)
		}

		fallbackByType := map[string]int64{}
		for _, r := range fallbackRows {
			fallbackByType[r.ConnType] = r.Cnt
		}

		// История из snapshots — per-type.
		// Для каждой пары (connection_id, snapshot_date) берём только ОДНУ row (latest по
		// updated_at) — иначе SUM суммирует разные account_wialon_id per resource и даёт
		// двукратный/тройной счёт когда scheduler писал snapshot для нескольких ресурсов.
		// См. инцидент 2026-05-09 — bad row 33810 на glomoskz из-за разных account_wialon_id.
		snapshotByType := map[string]int64{}
		if historyEnabled && companyID > 0 {
			snapDate := time.Date(eom.Year(), eom.Month(), eom.Day(), 0, 0, 0, 0, time.UTC)
			publicDB.Raw(`
				WITH latest AS (
					SELECT DISTINCT ON (connection_id, snapshot_date)
					       connection_id, snapshot_date, total_units
					FROM `+publicTable(publicDB, "wialon_daily_snapshots")+`
					WHERE snapshot_date = ?
					ORDER BY connection_id, snapshot_date, updated_at DESC, id DESC
				)
				SELECT wc.connection_type AS conn_type, COALESCE(SUM(latest.total_units), 0) AS cnt
				FROM latest
				JOIN `+publicTable(publicDB, "wialon_connections")+` wc ON wc.id = latest.connection_id
				WHERE wc.company_id = ?
				GROUP BY wc.connection_type
			`, snapDate, companyID).Scan(&whwlRows)
			for _, r := range whwlRows {
				snapshotByType[r.ConnType] = r.Cnt
			}
		}

		// Per-type: priority snapshot > fallback (history) > current count (плоско когда нет истории)
		pickType := func(t string) int64 {
			if v, ok := snapshotByType[t]; ok && v > 0 {
				return v
			}
			if v, ok := fallbackByType[t]; ok && v > 0 {
				return v
			}
			// Нет ни snapshot, ни wialon_units на дату — берём current (плоская history)
			return currentByType[t]
		}
		whCount := pickType("hosting")
		wlCount := pickType("local")

		// SKIF: COUNT по реальной дате создания в SKIF (skif_created_at).
		// Fallback на наш created_at для записей где skif_created_at NULL.
		// Также игнорируем юниты помеченные deleted после eom (они существовали тогда).
		var skifCount int64
		if companyID > 0 {
			publicDB.Raw(`
				SELECT COUNT(*) FROM `+publicTable(publicDB, "skif_units")+`
				WHERE company_id = ?
				  AND COALESCE(skif_created_at, created_at) <= ?
				  AND (skif_deleted_at IS NULL OR skif_deleted_at > ?)
			`, companyID, eom, eom).Scan(&skifCount)
		}

		// GELIOS: COUNT по реальной дате создания в GELIOS (gelios_created_at),
		// fallback на наш created_at где gelios_created_at NULL. Юниты помеченные
		// gelios_deleted_at после eom существовали тогда (как SKIF).
		// gelios_deleted_at = момент подтверждённого исчезновения из выгрузки
		// (не реального удаления в GELIOS — API operation-log нет).
		var geliosCount int64
		if companyID > 0 {
			publicDB.Raw(`
				SELECT COUNT(*) FROM `+publicTable(publicDB, "gelios_units")+`
				WHERE company_id = ?
				  AND COALESCE(gelios_created_at, created_at) <= ?
				  AND (gelios_deleted_at IS NULL OR gelios_deleted_at > ?)
			`, companyID, eom, eom).Scan(&geliosCount)
		}

		objectsCount := axentaCount + whCount + wlCount + skifCount + geliosCount

		// Per-bucket created/deleted events (окно [b.start, b.end))
		startStr := b.start.Format("2006-01-02")
		endStr := b.end.Format("2006-01-02")

		// Axenta: count events по реальной дате создания / удаления
		var axentaCreated, axentaDeleted int64
		tenantDB.Unscoped().Table("axenta_object_snapshots").
			Where("COALESCE(axenta_created_at, created_at) >= ? AND COALESCE(axenta_created_at, created_at) < ?", b.start, b.end).
			Count(&axentaCreated)
		tenantDB.Unscoped().Table("axenta_object_snapshots").
			Where("axenta_deleted_at >= ? AND axenta_deleted_at < ?", b.start, b.end).
			Count(&axentaDeleted)

		// Wialon: SUM(units_created/deleted) GROUP BY connection_type
		type whwlEvents struct {
			ConnType string
			Created  int64
			Deleted  int64
		}
		var whwlEvRows []whwlEvents
		var whCreated, whDeleted, wlCreated, wlDeleted int64
		if companyID > 0 {
			// Events SUM: тоже берём только одну row per (connection, day) — latest, чтобы
			// не складывать created/deleted разных account_wialon_id одного connection.
			publicDB.Raw(`
				WITH latest AS (
					SELECT DISTINCT ON (connection_id, snapshot_date)
					       connection_id, snapshot_date, units_created, units_deleted
					FROM `+publicTable(publicDB, "wialon_daily_snapshots")+`
					WHERE snapshot_date >= ? AND snapshot_date < ?
					ORDER BY connection_id, snapshot_date, updated_at DESC, id DESC
				)
				SELECT wc.connection_type AS conn_type,
				       COALESCE(SUM(latest.units_created), 0) AS created,
				       COALESCE(SUM(latest.units_deleted), 0) AS deleted
				FROM latest
				JOIN `+publicTable(publicDB, "wialon_connections")+` wc ON wc.id = latest.connection_id
				WHERE wc.company_id = ?
				GROUP BY wc.connection_type
			`, startStr, endStr, companyID).Scan(&whwlEvRows)
			for _, r := range whwlEvRows {
				switch r.ConnType {
				case "hosting":
					whCreated, whDeleted = r.Created, r.Deleted
				case "local":
					wlCreated, wlDeleted = r.Created, r.Deleted
				}
			}
		}

		// SKIF: SUM(units_created/deleted) из skif_daily_snapshots
		var skifCreated, skifDeleted int64
		if companyID > 0 {
			var skifEv struct {
				Created int64
				Deleted int64
			}
			publicDB.Raw(`
				SELECT COALESCE(SUM(sds.units_created), 0) AS created,
				       COALESCE(SUM(sds.units_deleted), 0) AS deleted
				FROM `+publicTable(publicDB, "skif_daily_snapshots")+` sds
				JOIN `+publicTable(publicDB, "skif_connections")+` sc ON sc.id = sds.connection_id
				WHERE sc.company_id = ? AND sds.snapshot_date >= ? AND sds.snapshot_date < ?
			`, companyID, startStr, endStr).Scan(&skifEv)
			skifCreated, skifDeleted = skifEv.Created, skifEv.Deleted
		}

		// GELIOS: proxy created/deleted (нет skif_daily_snapshots-аналога).
		// created = gelios_created_at в окне, deleted = gelios_deleted_at в окне.
		var geliosCreated, geliosDeleted int64
		if companyID > 0 {
			publicDB.Raw(`
				SELECT COUNT(*) FROM `+publicTable(publicDB, "gelios_units")+`
				WHERE company_id = ?
				  AND COALESCE(gelios_created_at, created_at) >= ?
				  AND COALESCE(gelios_created_at, created_at) < ?
			`, companyID, b.start, b.end).Scan(&geliosCreated)
			publicDB.Raw(`
				SELECT COUNT(*) FROM `+publicTable(publicDB, "gelios_units")+`
				WHERE company_id = ?
				  AND gelios_deleted_at IS NOT NULL
				  AND gelios_deleted_at >= ?
				  AND gelios_deleted_at < ?
			`, companyID, b.start, b.end).Scan(&geliosDeleted)
		}

		revenues := sumPaidByCurrency(publicDB, companyID, b.start, b.end)
		for _, r := range revenues {
			currencySet[r.Currency] = struct{}{}
		}

		points = append(points, ChartPoint{
			Month:         b.key,
			MonthLabel:    b.label,
			Objects:       objectsCount,
			Axenta:        axentaCount,
			WH:            whCount,
			WL:            wlCount,
			Skif:          skifCount,
			Gelios:        geliosCount,
			Revenues:      revenues,
			AxentaCreated: axentaCreated,
			AxentaDeleted: axentaDeleted,
			WHCreated:     whCreated,
			WHDeleted:     whDeleted,
			WLCreated:     wlCreated,
			WLDeleted:     wlDeleted,
			SkifCreated:   skifCreated,
			SkifDeleted:   skifDeleted,
			GeliosCreated: geliosCreated,
			GeliosDeleted: geliosDeleted,
		})
	}

	currencies := make([]string, 0, len(currencySet))
	for cur := range currencySet {
		currencies = append(currencies, cur)
	}
	sort.Strings(currencies)
	// RUB всегда первая, если есть
	for i, cur := range currencies {
		if cur == "RUB" {
			currencies = append([]string{"RUB"}, append(currencies[:i], currencies[i+1:]...)...)
			break
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": ChartResponse{
			Points:      points,
			Currencies:  currencies,
			GeneratedAt: now,
		},
	})
}

// sumPaidByCurrency агрегирует invoices.paid_amount GROUP BY currency
// для company_id и окна [from, to).
func sumPaidByCurrency(db *gorm.DB, companyID uint, from, to time.Time) []CurrencyRevenue {
	type row struct {
		Currency string
		Sum      decimal.Decimal
	}
	var rows []row
	db.Table(publicTable(db, "invoices")).
		Select("COALESCE(currency, 'RUB') AS currency, COALESCE(SUM(paid_amount), 0) AS sum").
		Where("company_id = ? AND paid_at >= ? AND paid_at < ?", companyID, from, to).
		Group("currency").
		Scan(&rows)

	out := make([]CurrencyRevenue, 0, len(rows))
	for _, r := range rows {
		amount, _ := r.Sum.Float64()
		out = append(out, CurrencyRevenue{
			Currency: r.Currency,
			Raw:      amount,
			Text:     formatCurrency(r.Sum, r.Currency),
		})
	}
	// Stable sort: RUB first, остальные по алфавиту
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Currency == "RUB" && out[j].Currency != "RUB" {
			return true
		}
		if out[j].Currency == "RUB" && out[i].Currency != "RUB" {
			return false
		}
		return out[i].Currency < out[j].Currency
	})
	return out
}

// formatCurrency форматирует сумму с символом валюты (₽, ₸, $, €).
func formatCurrency(v decimal.Decimal, currency string) string {
	rounded := v.Round(0).IntPart()
	formatted := formatThousands(rounded)
	switch currency {
	case "RUB":
		return formatted + " ₽"
	case "KZT":
		return formatted + " ₸"
	case "USD":
		return "$" + formatted
	case "EUR":
		return "€" + formatted
	default:
		return formatted + " " + currency
	}
}

// formatThousands добавляет неразрывные пробелы как разделители тысяч.
func formatThousands(n int64) string {
	negative := n < 0
	if negative {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		if negative {
			return "-" + s
		}
		return s
	}
	var out []byte
	for i, ch := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, 0xC2, 0xA0) // U+00A0 NBSP
		}
		out = append(out, byte(ch))
	}
	if negative {
		return "-" + string(out)
	}
	return string(out)
}

// dayShort — короткая подпись дня "06.05".
func dayShort(t time.Time) string {
	return fmt.Sprintf("%02d.%02d", t.Day(), int(t.Month()))
}

// weekShort — короткая подпись недели по дате понедельника "06.05".
func weekShort(t time.Time) string {
	return dayShort(t)
}

// monthShortRu — краткое русское название месяца.
func monthShortRu(m time.Month) string {
	switch m {
	case time.January:
		return "Янв"
	case time.February:
		return "Фев"
	case time.March:
		return "Мар"
	case time.April:
		return "Апр"
	case time.May:
		return "Май"
	case time.June:
		return "Июн"
	case time.July:
		return "Июл"
	case time.August:
		return "Авг"
	case time.September:
		return "Сен"
	case time.October:
		return "Окт"
	case time.November:
		return "Ноя"
	case time.December:
		return "Дек"
	}
	return fmt.Sprintf("%d", m)
}
