package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"golang.org/x/text/encoding/charmap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CurrencyRateService — загрузка и выдача курсов валют (П5 мультивалюта).
// Источник cbr_rf: ЦБ РФ XML_daily (https://www.cbr.ru/scripts/XML_daily.asp).
// nbk_kz — задел на будущее (нацбанк РК), пока не реализован.
type CurrencyRateService struct {
	db     *gorm.DB
	client *http.Client
}

func NewCurrencyRateService() *CurrencyRateService {
	return &CurrencyRateService{
		db:     database.DB,
		client: &http.Client{Timeout: 20 * time.Second},
	}
}

// quoteCcyForSource — валюта котировки (за что считаем) по источнику.
// CBR котирует всё в рублях → quote=RUB. НБ РК котировал бы в тенге.
func quoteCcyForSource(source string) string {
	switch source {
	case "nbk_kz":
		return "KZT"
	default:
		return "RUB"
	}
}

// --- CBR XML структуры ---

type cbrValCurs struct {
	XMLName xml.Name     `xml:"ValCurs"`
	Date    string       `xml:"Date,attr"` // dd.mm.yyyy
	Valutes []cbrValute  `xml:"Valute"`
}

type cbrValute struct {
	CharCode string `xml:"CharCode"` // EUR, USD, KZT
	Nominal  string `xml:"Nominal"`  // за сколько единиц указан Value
	Value    string `xml:"Value"`    // курс за Nominal единиц, RU-decimal (запятая)
}

// FetchCBRForDate загружает курсы ЦБ РФ на указанную дату и upsert'ит в currency_rates.
// Идемпотентно: ON CONFLICT (rate_date, base, quote, source) DO UPDATE rate
// (курс на дату может уточняться ЦБ задним числом — берём последнее значение).
// ВАЖНО: пишем под ФАКТИЧЕСКОЙ датой курса из <ValCurs Date>, а не под запрошенной —
// CBR в выходные/праздники отдаёт ближайший рабочий день, и хранить его под
// запрошенной датой ломало бы fallback/staleness (Codex #2).
func (s *CurrencyRateService) FetchCBRForDate(date time.Time) (int, error) {
	day := dayFloor(date.UTC())
	url := fmt.Sprintf("https://www.cbr.ru/scripts/XML_daily.asp?date_req=%s", day.Format("02/01/2006"))
	effDate, rates, err := s.fetchCBR(url)
	if err != nil {
		return 0, err
	}
	if effDate.IsZero() {
		effDate = day // дата в ответе не распарсилась — fallback на запрошенную
	}
	return s.upsertRates(effDate, "cbr_rf", "RUB", rates)
}

// fetchCBR парсит CBR XML, возвращает (effective_date, map[CharCode]rate-за-1-единицу).
func (s *CurrencyRateService) fetchCBR(url string) (time.Time, map[string]decimal.Decimal, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return time.Time{}, nil, err
	}
	// CBR отдаёт 403 на дефолтный Go-http-client UA — ставим браузерный (Codex-staging fix).
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; ACRM-billing/1.0)")
	req.Header.Set("Accept", "application/xml, text/xml, */*")
	resp, err := s.client.Do(req)
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("CBR fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return time.Time{}, nil, fmt.Errorf("CBR fetch: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20)) // 2MB cap
	if err != nil {
		return time.Time{}, nil, err
	}
	return parseCBRXML(body)
}

// parseCBRXML — pure-парсер CBR XML (тестируется без сети). Возвращает фактическую
// дату курса (из атрибута ValCurs Date, dd.mm.yyyy) и курсы. CBR XML в windows-1251 —
// декодируем через charmap (Codex #1: <Name> на кириллице иначе ломает xml-декодер).
// Value: запятая→точка; нормализуем на Nominal к за-1-единицу.
func parseCBRXML(body []byte) (time.Time, map[string]decimal.Decimal, error) {
	var vc cbrValCurs
	dec := xml.NewDecoder(strings.NewReader(string(body)))
	dec.CharsetReader = func(label string, input io.Reader) (io.Reader, error) {
		// CBR всегда windows-1251; для любых cp1251/windows-1251 лейблов — декодер.
		l := strings.ToLower(strings.TrimSpace(label))
		if strings.Contains(l, "1251") || l == "" {
			return charmap.Windows1251.NewDecoder().Reader(input), nil
		}
		return input, nil
	}
	if err := dec.Decode(&vc); err != nil {
		return time.Time{}, nil, fmt.Errorf("CBR parse: %w", err)
	}

	// Фактическая дата курса из ответа.
	var effDate time.Time
	if d := strings.TrimSpace(vc.Date); d != "" {
		if t, e := time.Parse("02.01.2006", d); e == nil {
			effDate = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		}
	}

	out := make(map[string]decimal.Decimal, len(vc.Valutes))
	for _, v := range vc.Valutes {
		code := strings.ToUpper(strings.TrimSpace(v.CharCode))
		if code == "" {
			continue
		}
		valStr := strings.ReplaceAll(strings.TrimSpace(v.Value), ",", ".")
		valStr = strings.ReplaceAll(valStr, " ", "")
		val, err := decimal.NewFromString(valStr)
		if err != nil || val.LessThanOrEqual(decimal.Zero) {
			continue // битый/непол. Value — пропускаем (Codex #5: не только zero, но и ≤0)
		}
		// Nominal обязателен для нормализации — битый Nominal НЕ глушим в 1
		// (для KZT/AMD/JPY Nominal=100 → завышенный курс). Пропускаем валюту (Codex #5).
		nominal, err := decimal.NewFromString(strings.TrimSpace(v.Nominal))
		if err != nil || nominal.LessThanOrEqual(decimal.Zero) {
			continue
		}
		out[code] = val.Div(nominal).Round(8) // rate за 1 единицу базовой валюты
	}
	if len(out) == 0 {
		return time.Time{}, nil, fmt.Errorf("CBR parse: пустой набор курсов")
	}
	return effDate, out, nil
}

// upsertRates пишет курсы за день идемпотентно (ON CONFLICT по unique-индексу).
func (s *CurrencyRateService) upsertRates(day time.Time, source, quote string, rates map[string]decimal.Decimal) (int, error) {
	if len(rates) == 0 {
		return 0, nil
	}
	rows := make([]models.CurrencyRate, 0, len(rates))
	for base, rate := range rates {
		if base == quote {
			continue // RUB→RUB не храним
		}
		rows = append(rows, models.CurrencyRate{
			RateDate: day, BaseCcy: base, QuoteCcy: quote, Source: source, Rate: rate,
		})
	}
	if len(rows) == 0 {
		return 0, nil
	}
	err := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "rate_date"}, {Name: "base_ccy"}, {Name: "quote_ccy"}, {Name: "source"}},
		DoUpdates: clause.AssignmentColumns([]string{"rate", "updated_at"}),
	}).Create(&rows).Error
	if err != nil {
		return 0, err
	}
	return len(rows), nil
}

// maxRateStalenessDays — макс. возраст fallback-курса (Codex [M] max staleness).
// Дальше — курс возвращается, но с флагом stale=true (вызывающий алертит/решает).
const maxRateStalenessDays = 3

// GetRate возвращает курс base→quote на дату date из источника source.
// base==quote → rate=1. Если на дату курса нет — fallback на последний доступный
// ≤ date (выходные/праздники ЦБ не публикует). stale=true если fallback старше
// maxRateStalenessDays. err — если курса нет вовсе.
func (s *CurrencyRateService) GetRate(date time.Time, base, quote, source string) (rate decimal.Decimal, rateDate time.Time, stale bool, err error) {
	base = strings.ToUpper(base)
	quote = strings.ToUpper(quote)
	if base == quote {
		return decimal.NewFromInt(1), dayFloor(date.UTC()), false, nil
	}
	day := dayFloor(date.UTC())
	var cr models.CurrencyRate
	e := s.db.Where("base_ccy = ? AND quote_ccy = ? AND source = ? AND rate_date <= ?",
		base, quote, source, day).
		Order("rate_date DESC").First(&cr).Error
	if e != nil {
		return decimal.Zero, time.Time{}, false, fmt.Errorf("нет курса %s→%s (%s) на %s или ранее", base, quote, source, day.Format("2006-01-02"))
	}
	staleness := int(day.Sub(cr.RateDate).Hours() / 24)
	return cr.Rate, cr.RateDate, staleness > maxRateStalenessDays, nil
}

// GetConversionRate — курс конверсии from→to через валюту-пивот источника (П5 фаза 3,
// для кросс-валютного перевода между ЛС). Источник котирует всё в pivot (cbr_rf → RUB):
//   - from==to → 1;
//   - to==pivot   → прямой курс from→pivot (X→RUB);
//   - from==pivot → inverse 1/(to→pivot) (RUB→X);
//   - оба ≠ pivot → cross (from→pivot)/(to→pivot) (X→RUB→Y).
// stale=true если ЛЮБОЙ задействованный курс устарел. err — если любого курса нет.
// Возвращает rate (>0) с округлением до 8 знаков.
func (s *CurrencyRateService) GetConversionRate(date time.Time, from, to, source string) (rate decimal.Decimal, stale bool, err error) {
	from = strings.ToUpper(from)
	to = strings.ToUpper(to)
	if from == to {
		return decimal.NewFromInt(1), false, nil
	}
	pivot := quoteCcyForSource(source)

	// Контракт: возвращаем rate > 0. Каждый задействованный курс проверяем на >0
	// внутри сервиса, не полагаясь на вызывающего (Codex #4).
	switch {
	case to == pivot:
		r, _, st, e := s.GetRate(date, from, pivot, source)
		if e != nil {
			return decimal.Zero, false, e
		}
		if r.LessThanOrEqual(decimal.Zero) {
			return decimal.Zero, false, fmt.Errorf("некорректный курс %s→%s = %s", from, pivot, r)
		}
		return r, st, nil
	case from == pivot:
		r, _, st, e := s.GetRate(date, to, pivot, source)
		if e != nil {
			return decimal.Zero, false, e
		}
		if r.LessThanOrEqual(decimal.Zero) {
			return decimal.Zero, false, fmt.Errorf("некорректный курс %s→%s = %s", to, pivot, r)
		}
		return decimal.NewFromInt(1).Div(r).Round(8), st, nil
	default:
		rFrom, _, st1, e1 := s.GetRate(date, from, pivot, source)
		if e1 != nil {
			return decimal.Zero, false, e1
		}
		rTo, _, st2, e2 := s.GetRate(date, to, pivot, source)
		if e2 != nil {
			return decimal.Zero, false, e2
		}
		if rFrom.LessThanOrEqual(decimal.Zero) {
			return decimal.Zero, false, fmt.Errorf("некорректный курс %s→%s = %s", from, pivot, rFrom)
		}
		if rTo.LessThanOrEqual(decimal.Zero) {
			return decimal.Zero, false, fmt.Errorf("некорректный курс %s→%s = %s", to, pivot, rTo)
		}
		return rFrom.Div(rTo).Round(8), st1 || st2, nil
	}
}
