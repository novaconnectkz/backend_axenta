package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// WialonStatsService собирает статистику объектов из Wialon connections и upsert-ит в БД.
// Используется фоновым WialonStatsScheduler (раз в N минут) и опционально через ручной endpoint.
//
// Зачем нужно: live-запрос /api/wialon/connections/:id/objects-stats для WH с 3412 ресурсов
// занимает 6.5 минут (account/get_account_data по 100 ресурсов × 34 батча × ~10s каждый).
// Это неприемлемо для UI — пользователь видит спиннер 6.5 минут. Сервис собирает данные
// в фоне в БД, endpoint отдаёт мгновенно из public.wialon_object_stats.
type WialonStatsService struct {
	db            *gorm.DB
	wialonService *WialonService
}

func NewWialonStatsService() *WialonStatsService {
	return &WialonStatsService{
		db:            database.DB,
		wialonService: NewWialonService(),
	}
}

// CollectAll обходит все активные wialon_connections и обновляет stats для каждого.
// Возвращает статистику запуска: сколько подключений обработано, сколько ресурсов сохранено.
func (s *WialonStatsService) CollectAll() (CollectStatsResult, error) {
	t0 := time.Now()
	result := CollectStatsResult{StartedAt: t0}

	var conns []models.WialonConnection
	if err := s.db.Where("is_active = ?", true).Find(&conns).Error; err != nil {
		return result, fmt.Errorf("ошибка получения wialon_connections: %w", err)
	}

	for _, conn := range conns {
		stats, err := s.collectForConnection(conn)
		if err != nil {
			log.Printf("⚠️ WialonStats: ошибка для connection=%d (%s): %v", conn.ID, conn.Name, err)
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("conn=%d: %v", conn.ID, err))
			continue
		}
		result.ProcessedConnections++
		result.UpsertedResources += stats
	}

	result.Duration = time.Since(t0)
	log.Printf("✅ WialonStats: обработано %d/%d connections, %d ресурсов записано, за %s",
		result.ProcessedConnections, len(conns), result.UpsertedResources, result.Duration)
	return result, nil
}

// CollectForConnectionID собирает stats для одного подключения (ручной trigger через API).
func (s *WialonStatsService) CollectForConnectionID(connectionID uint) (int, error) {
	var conn models.WialonConnection
	if err := s.db.Where("id = ? AND is_active = ?", connectionID, true).First(&conn).Error; err != nil {
		return 0, fmt.Errorf("connection %d не найден или неактивен: %w", connectionID, err)
	}
	return s.collectForConnection(conn)
}

// collectForConnection — реальная работа: login → GetUnitsCountWithHierarchy → upsert в БД.
func (s *WialonStatsService) collectForConnection(conn models.WialonConnection) (int, error) {
	t0 := time.Now()
	loginResp, err := s.wialonService.LoginWithHost(conn.Host, conn.Token)
	if err != nil {
		return 0, fmt.Errorf("login: %w", err)
	}
	defer func() { _ = s.wialonService.LogoutWithHost(conn.Host, loginResp.Eid) }()

	// Получаем resourceID -> stats. countPerAccount возвращает ключи как resourceID, так и creatorID
	// (см. GetUnitsCountWithHierarchy). Для БД сохраняем по resourceID, а user_id = creatorID.
	countPerAccount, _, err := s.wialonService.GetUnitsCountWithHierarchy(conn.Host, loginResp.Eid)
	if err != nil {
		return 0, fmt.Errorf("get_account_data: %w", err)
	}

	// Параллельно нам нужен маппинг resourceID -> creatorID (user). GetUnitsCountWithHierarchy
	// сохраняет stats и для resourceID, и для creatorID — нам нужно отделить. Сделаем повторный
	// поиск ресурсов чтобы получить чёткий маппинг (быстрый — 1 search_items).
	resourceToCreator, err := s.fetchResourceCreatorMap(conn.Host, loginResp.Eid)
	if err != nil {
		log.Printf("⚠️ WialonStats: не удалось получить resourceToCreator для conn=%d: %v (продолжаем без user_id)", conn.ID, err)
		resourceToCreator = map[int64]int64{}
	}

	now := time.Now()
	upserts := make([]models.WialonObjectStat, 0, len(resourceToCreator))
	for resourceID, creatorID := range resourceToCreator {
		stats, ok := countPerAccount[resourceID]
		if !ok {
			// Ресурс есть, но usage нет (нет объектов / не биллинговый) — пишем нули, чтобы был свежий timestamp
			stats = UnitsStats{}
		}
		upserts = append(upserts, models.WialonObjectStat{
			ConnectionID:       conn.ID,
			ResourceID:         resourceID,
			UserID:             creatorID,
			ObjectsTotal:       stats.Total,
			ObjectsActive:      stats.Active,
			ObjectsDeactivated: stats.Deactivated,
			LastCollectedAt:    now,
		})
	}

	if len(upserts) == 0 {
		return 0, nil
	}

	// Batch upsert. ON CONFLICT (connection_id, resource_id) DO UPDATE.
	// Чанками по 500 чтобы не упереться в лимит параметров.
	const chunkSize = 500
	for i := 0; i < len(upserts); i += chunkSize {
		end := i + chunkSize
		if end > len(upserts) {
			end = len(upserts)
		}
		if err := s.db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "connection_id"}, {Name: "resource_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"user_id", "objects_total", "objects_active", "objects_deactivated",
				"last_collected_at", "updated_at",
			}),
		}).Create(upserts[i:end]).Error; err != nil {
			return 0, fmt.Errorf("upsert chunk [%d:%d]: %w", i, end, err)
		}
	}

	log.Printf("✅ WialonStats: connection=%d (%s), upserted=%d ресурсов за %s",
		conn.ID, conn.Name, len(upserts), time.Since(t0))
	return len(upserts), nil
}

// fetchResourceCreatorMap делает один search_items на avl_resource и возвращает resourceID -> creatorID.
// Дёшево — ~500ms для WH с 3412 ресурсами.
func (s *WialonStatsService) fetchResourceCreatorMap(host string, eid string) (map[int64]int64, error) {
	calls := []map[string]interface{}{
		{
			"svc": "core/search_items",
			"params": map[string]interface{}{
				"spec": map[string]interface{}{
					"itemsType":     "avl_resource",
					"propName":      "sys_name",
					"propValueMask": "*",
					"sortType":      "sys_name",
				},
				"force": 1,
				"flags": 5,
				"from":  0,
				"to":    0,
			},
		},
	}

	results, err := s.wialonService.callBatch(host, eid, calls)
	if err != nil {
		return nil, err
	}

	var resp WialonSearchResponse
	if err := json.Unmarshal(results[0], &resp); err != nil {
		return nil, fmt.Errorf("парсинг search_items: %w", err)
	}

	out := make(map[int64]int64, len(resp.Items))
	for _, item := range resp.Items {
		var resourceID, creatorID int64
		if id, ok := item["id"].(float64); ok {
			resourceID = int64(id)
		}
		if crt, ok := item["crt"].(float64); ok {
			creatorID = int64(crt)
		}
		if resourceID > 0 {
			out[resourceID] = creatorID
		}
	}
	return out, nil
}

// GetStatsForConnection читает кэш из БД по connection_id. user_id ключ совпадает с frontend.
func (s *WialonStatsService) GetStatsForConnection(connectionID uint) ([]models.WialonObjectStat, error) {
	var stats []models.WialonObjectStat
	if err := s.db.Where("connection_id = ?", connectionID).Find(&stats).Error; err != nil {
		return nil, err
	}
	return stats, nil
}

// CollectStatsResult — результат запуска CollectAll, для логов и API статуса
type CollectStatsResult struct {
	StartedAt            time.Time     `json:"started_at"`
	Duration             time.Duration `json:"duration"`
	ProcessedConnections int           `json:"processed_connections"`
	UpsertedResources    int           `json:"upserted_resources"`
	Failed               int           `json:"failed"`
	Errors               []string      `json:"errors,omitempty"`
}
