package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"backend_axenta/database"
	"backend_axenta/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// WialonHistoryService — on-demand backfill точной истории Wialon-юнитов.
//
// Алгоритм (per connection):
//  1. Login → eid
//  2. core/search_items itemsType=avl_resource → список resource_id
//  3. Для каждого resource:
//     - core/get_statistics(resourceId, fromTime, toTime, type=items, recursive=0)
//       → daily {avl_unit_total, avl_unit_created, avl_unit_deleted}
//     - текущий usage (avl_unit.usage) уже есть в wialon_object_stats — берём оттуда
//     - обратный walk: usage(N) = usage(N+1) - created(N+1) + deleted(N+1)
//  4. UPSERT в wialon_daily_snapshots
//
// Прогресс публикуется в Redis (key: wialon_history_progress:<company_id>) с TTL 1ч.
type WialonHistoryService struct {
	db            *gorm.DB
	wialonService *WialonService
	mu            sync.Mutex
	running       map[uint]bool // companyID → backfill в процессе
}

// NewWialonHistoryService создаёт сервис.
func NewWialonHistoryService(db *gorm.DB, ws *WialonService) *WialonHistoryService {
	return &WialonHistoryService{
		db:            db,
		wialonService: ws,
		running:       make(map[uint]bool),
	}
}

// BackfillProgress — состояние backfill для polling из UI.
type BackfillProgress struct {
	Running              bool       `json:"running"`
	CompanyID            uint       `json:"company_id"`
	TotalConnections     int        `json:"total_connections"`
	ProcessedConnections int        `json:"processed_connections"`
	TotalResources       int        `json:"total_resources"`
	ProcessedResources   int        `json:"processed_resources"`
	RowsUpserted         int        `json:"rows_upserted"`
	From                 time.Time  `json:"from"`
	To                   time.Time  `json:"to"`
	StartedAt            time.Time  `json:"started_at"`
	FinishedAt           *time.Time `json:"finished_at,omitempty"`
	Error                string     `json:"error,omitempty"`
}

// progressKey — Redis key для прогресса.
func progressKey(companyID uint) string {
	return fmt.Sprintf("wialon_history_progress:%d", companyID)
}

// publishProgress сохраняет прогресс в Redis с TTL 1ч.
func publishProgress(p BackfillProgress) {
	if database.RedisClient == nil {
		return
	}
	data, _ := json.Marshal(p)
	database.RedisClient.Set(context.Background(), progressKey(p.CompanyID), data, time.Hour)
}

// GetProgress читает прогресс из Redis.
func (s *WialonHistoryService) GetProgress(companyID uint) *BackfillProgress {
	if database.RedisClient == nil {
		return nil
	}
	val, err := database.RedisClient.Get(context.Background(), progressKey(companyID)).Bytes()
	if err != nil {
		return nil
	}
	var p BackfillProgress
	if err := json.Unmarshal(val, &p); err != nil {
		return nil
	}
	return &p
}

// IsRunning проверяет запущен ли backfill для company.
func (s *WialonHistoryService) IsRunning(companyID uint) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running[companyID]
}

// StartBackfill запускает асинхронный backfill. Возвращает ошибку если уже идёт.
func (s *WialonHistoryService) StartBackfill(companyID uint, from, to time.Time) error {
	s.mu.Lock()
	if s.running[companyID] {
		s.mu.Unlock()
		return fmt.Errorf("backfill уже запущен для компании %d", companyID)
	}
	s.running[companyID] = true
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			s.running[companyID] = false
			s.mu.Unlock()
		}()
		s.runBackfill(companyID, from, to)
	}()

	return nil
}

// runBackfill — синхронная реализация backfill (вызывается из goroutine).
func (s *WialonHistoryService) runBackfill(companyID uint, from, to time.Time) {
	progress := BackfillProgress{
		Running:   true,
		CompanyID: companyID,
		From:      from,
		To:        to,
		StartedAt: time.Now(),
	}
	publishProgress(progress)

	defer func() {
		now := time.Now()
		progress.Running = false
		progress.FinishedAt = &now
		publishProgress(progress)
	}()

	// 1. Получаем все wialon_connections компании
	var connections []models.WialonConnection
	if err := s.db.Where("company_id = ? AND is_active = true AND deleted_at IS NULL", companyID).Find(&connections).Error; err != nil {
		progress.Error = fmt.Sprintf("get connections: %v", err)
		publishProgress(progress)
		return
	}
	progress.TotalConnections = len(connections)
	publishProgress(progress)

	if len(connections) == 0 {
		log.Printf("WialonHistory: нет wialon_connections для company=%d", companyID)
		return
	}

	totalRows := 0
	for connIdx, conn := range connections {
		log.Printf("WialonHistory: connection %d/%d: %s (%s)",
			connIdx+1, len(connections), conn.Name, conn.Host)

		rows, err := s.backfillConnection(conn, from, to, &progress)
		if err != nil {
			log.Printf("WialonHistory: ошибка для conn=%d: %v", conn.ID, err)
			progress.Error = fmt.Sprintf("conn %d: %v", conn.ID, err)
			publishProgress(progress)
			continue
		}
		totalRows += rows
		progress.ProcessedConnections = connIdx + 1
		progress.RowsUpserted = totalRows
		publishProgress(progress)
	}

	// Сохраняем результат в WialonHistorySettings
	now := time.Now()
	s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "company_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"last_backfill_from", "last_backfill_to", "last_backfill_at", "last_backfill_rows", "updated_at"}),
	}).Create(&models.WialonHistorySettings{
		CompanyID:        companyID,
		Enabled:          true, // backfill подразумевает enabled
		LastBackfillFrom: &from,
		LastBackfillTo:   &to,
		LastBackfillAt:   &now,
		LastBackfillRows: totalRows,
	})

	log.Printf("WialonHistory: backfill завершён для company=%d, %d строк", companyID, totalRows)
}

// backfillConnection — backfill за период для одного connection.
//
// Берём ТОЛЬКО top-level resource (макс objects_total в wialon_object_stats) +
// recursive=1 в get_statistics → получаем агрегированную историю всего connection
// без double-counting иерархии.
//
// Skip для shared-connections (несколько dealer'ов на одном Wialon-подключении):
// если connection_summary.objects_total меньше суммы distinct user_id из stats,
// API recursive=1 от top-resource вернёт всю иерархию (включая других dealer'ов),
// а не только нашего. В этом случае backfill даст несоответствие с summary.
// Признак: ConnectionType='local' с маленьким usage (типичная ситуация для WL).
// Тогда история берётся из fallback через wialon_units.created_at в chart endpoint.
func (s *WialonHistoryService) backfillConnection(conn models.WialonConnection, from, to time.Time, progress *BackfillProgress) (int, error) {
	// Skip 'local' connections с маленьким usage — обычно WL c shared-Wialon-сервером.
	// Точная история через get_statistics recursive=1 даст несоответствие с summary
	// (API возвращает count по всей иерархии, не только нашего dealer'а).
	var summary struct{ ObjectsTotal int64 }
	s.db.Table("wialon_connection_summary").
		Select("objects_total").
		Where("connection_id = ?", conn.ID).
		Scan(&summary)
	if conn.ConnectionType == "local" && summary.ObjectsTotal < 1000 {
		log.Printf("WialonHistory: skip conn=%d (%s, type=local, usage=%d) — shared-server, fallback на wialon_units history",
			conn.ID, conn.Name, summary.ObjectsTotal)
		return 0, nil
	}

	loginResp, err := s.wialonService.LoginWithHost(conn.Host, conn.Token)
	if err != nil {
		return 0, fmt.Errorf("login: %w", err)
	}
	defer func() { _ = s.wialonService.LogoutWithHost(conn.Host, loginResp.Eid) }()

	// conn.Host уже содержит схему (например "https://hst-api.regwialon.com")
	apiURL := conn.Host + "/wialon/ajax.html"

	// 1. Top-level avl_resource.id (биллинг-resource с max usage в иерархии).
	//    Wialon core/get_statistics с recursive=1 + этим resource'ом возвращает
	//    avl_unit_total per day по всей иерархии — точная история без reverse walk.
	type topRow struct {
		ResourceID   int64
		ObjectsTotal int64
	}
	var top topRow
	s.db.Table("wialon_object_stats").
		Select("resource_id, objects_total").
		Where("connection_id = ?", conn.ID).
		Order("objects_total DESC").
		Limit(1).
		Scan(&top)

	if top.ResourceID == 0 {
		return 0, fmt.Errorf("нет resource_id в wialon_object_stats для conn=%d", conn.ID)
	}

	resourceIDs := []int64{top.ResourceID}
	progress.TotalResources += len(resourceIDs)
	publishProgress(*progress)

	// 2. Для каждого resource — get_statistics → прямой avl_unit_total per day
	totalRows := 0
	for _, resID := range resourceIDs {
		stats, err := s.GetStatisticsRecursive(apiURL, loginResp.Eid, resID, from, to)
		if err != nil {
			log.Printf("WialonHistory: get_statistics для resource=%d: %v (skip)", resID, err)
			progress.ProcessedResources++
			publishProgress(*progress)
			continue
		}

		// Wialon API возвращает avl_unit_total напрямую для каждого дня → не нужен reverse walk.
		// Берём прямые значения. created/deleted тоже из API (если есть).
		dates := []time.Time{}
		for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
			dates = append(dates, time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC))
		}

		totalByDate := make(map[string]int, len(dates))
		createdByDate := make(map[string]int, len(dates))
		deletedByDate := make(map[string]int, len(dates))
		for _, ds := range stats {
			t := time.Unix(ds.Timestamp, 0).UTC()
			key := t.Format("2006-01-02")
			totalByDate[key] = ds.UnitTotal
			createdByDate[key] = ds.UnitCreated
			deletedByDate[key] = ds.UnitDeleted
		}

		// Заполняем пробелы: если день не вернулся — копируем предыдущий known total
		var lastTotal int
		for _, d := range dates {
			key := d.Format("2006-01-02")
			if v, ok := totalByDate[key]; ok && v > 0 {
				lastTotal = v
			} else {
				totalByDate[key] = lastTotal
			}
		}

		// 4. UPSERT в wialon_daily_snapshots
		snapshots := make([]models.WialonDailySnapshot, 0, len(dates))
		for _, d := range dates {
			key := d.Format("2006-01-02")
			snapshots = append(snapshots, models.WialonDailySnapshot{
				ConnectionID:    conn.ID,
				AccountWialonID: resID,
				SnapshotDate:    d,
				TotalUnits:      totalByDate[key],
				UnitsCreated:    createdByDate[key],
				UnitsDeleted:    deletedByDate[key],
			})
		}

		if len(snapshots) > 0 {
			err := s.db.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "connection_id"}, {Name: "account_wialon_id"}, {Name: "snapshot_date"}},
				DoUpdates: clause.AssignmentColumns([]string{"total_units", "units_created", "units_deleted", "units_deactivated", "updated_at"}),
			}).CreateInBatches(snapshots, 500).Error
			if err != nil {
				log.Printf("WialonHistory: upsert для resource=%d: %v", resID, err)
			} else {
				totalRows += len(snapshots)
			}
		}

		progress.ProcessedResources++
		progress.RowsUpserted += len(snapshots)
		publishProgress(*progress)

		// Rate-limit: pause между resource'ами чтобы не упереться в 500 req/min
		time.Sleep(150 * time.Millisecond)
	}

	return totalRows, nil
}

// WialonDailyStat — одна точка из core/get_statistics.
type WialonDailyStat struct {
	Timestamp   int64
	UnitTotal   int
	UnitCreated int
	UnitDeleted int
}

// GetStatistics — Wialon API core/get_statistics (recursive=0, только этот аккаунт).
func (s *WialonHistoryService) GetStatistics(apiURL, sid string, resourceID int64, from, to time.Time) ([]WialonDailyStat, error) {
	return s.callGetStatistics(apiURL, sid, resourceID, from, to, 0)
}

// GetStatisticsRecursive — recursive=1, агрегирует по всей иерархии.
func (s *WialonHistoryService) GetStatisticsRecursive(apiURL, sid string, resourceID int64, from, to time.Time) ([]WialonDailyStat, error) {
	return s.callGetStatistics(apiURL, sid, resourceID, from, to, 1)
}

func (s *WialonHistoryService) callGetStatistics(apiURL, sid string, resourceID int64, from, to time.Time, recursive int) ([]WialonDailyStat, error) {
	params := map[string]interface{}{
		"resourceId": resourceID,
		"timeFrom":   from.Unix(),
		"timeTo":     to.Add(24 * time.Hour).Unix(),
		"type":       "items",
		"recursive":  recursive,
	}
	paramsJSON, _ := json.Marshal(params)

	form := url.Values{}
	form.Set("svc", "core/get_statistics")
	form.Set("sid", sid)
	form.Set("params", string(paramsJSON))

	resp, err := http.PostForm(apiURL, form)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Проверка на ошибку Wialon API
	var errCheck struct {
		Error *int `json:"error"`
	}
	if json.Unmarshal(body, &errCheck) == nil && errCheck.Error != nil {
		return nil, fmt.Errorf("wialon error code %d", *errCheck.Error)
	}

	// Ответ: { "<timestamp>": { "<resourceId>": { "avl_unit_total": N, "avl_unit_created": M, ... } }, "users": {...} }
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	dateMap := make(map[string]*WialonDailyStat)

	for tsStr, data := range raw {
		if tsStr == "users" {
			continue
		}
		ts, err := strconv.ParseInt(tsStr, 10, 64)
		if err != nil {
			continue
		}
		var resourceData map[string]map[string]int
		if err := json.Unmarshal(data, &resourceData); err != nil {
			continue
		}
		t := time.Unix(ts, 0).UTC()
		key := t.Format("2006-01-02")
		stat, exists := dateMap[key]
		if !exists {
			stat = &WialonDailyStat{Timestamp: ts}
			dateMap[key] = stat
		}
		for _, fields := range resourceData {
			stat.UnitTotal += fields["avl_unit_total"]
			stat.UnitCreated += fields["avl_unit_created"]
			stat.UnitDeleted += fields["avl_unit_deleted"]
		}
	}

	out := make([]WialonDailyStat, 0, len(dateMap))
	for _, st := range dateMap {
		out = append(out, *st)
	}
	return out, nil
}

// searchAllResourceIDs — список avl_resource в подключении.
func (s *WialonHistoryService) searchAllResourceIDs(apiURL, sid string) ([]int64, error) {
	form := url.Values{}
	form.Set("svc", "core/search_items")
	form.Set("sid", sid)
	form.Set("params", `{"spec":{"itemsType":"avl_resource","propName":"sys_name","propValueMask":"*","sortType":"sys_name"},"force":1,"flags":1,"from":0,"to":0}`)

	resp, err := http.PostForm(apiURL, form)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
		Error *int `json:"error"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	if raw.Error != nil {
		return nil, fmt.Errorf("wialon error %d", *raw.Error)
	}

	ids := make([]int64, 0, len(raw.Items))
	for _, it := range raw.Items {
		ids = append(ids, it.ID)
	}
	return ids, nil
}

// GetSettings возвращает настройки истории для company (создаёт пустые если нет).
func (s *WialonHistoryService) GetSettings(companyID uint) (*models.WialonHistorySettings, error) {
	var settings models.WialonHistorySettings
	err := s.db.Where("company_id = ?", companyID).First(&settings).Error
	if err == gorm.ErrRecordNotFound {
		settings = models.WialonHistorySettings{CompanyID: companyID, Enabled: false}
		return &settings, nil
	}
	if err != nil {
		return nil, err
	}
	return &settings, nil
}

// SetEnabled включает/выключает использование истории для company.
func (s *WialonHistoryService) SetEnabled(companyID uint, enabled bool) error {
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "company_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"enabled", "updated_at"}),
	}).Create(&models.WialonHistorySettings{
		CompanyID: companyID,
		Enabled:   enabled,
	}).Error
}

// DeleteSnapshots — стереть исторические данные за период.
func (s *WialonHistoryService) DeleteSnapshots(companyID uint, from, to time.Time) (int64, error) {
	// Берём connection_ids компании
	var connIDs []uint
	s.db.Model(&models.WialonConnection{}).
		Where("company_id = ?", companyID).
		Pluck("id", &connIDs)
	if len(connIDs) == 0 {
		return 0, nil
	}
	res := s.db.Where("connection_id IN ? AND snapshot_date >= ? AND snapshot_date <= ?", connIDs, from, to).
		Delete(&models.WialonDailySnapshot{})
	return res.RowsAffected, res.Error
}
