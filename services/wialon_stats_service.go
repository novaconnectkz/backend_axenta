package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
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

	// Дополнительно — собираем DISTINCT-реестр юнитов (для dashboard COUNT DISTINCT).
	// Не критично если упадёт — wialon_object_stats остаётся источником для /accounts.
	if unitsCount, uerr := s.collectUnitsForConnection(conn, loginResp.Eid); uerr != nil {
		log.Printf("⚠️ WialonStats units: connection=%d не удалось собрать: %v", conn.ID, uerr)
	} else {
		log.Printf("📦 WialonStats units: connection=%d, distinct units=%d", conn.ID, unitsCount)
	}

	// Реестр avl_user'ов с prp.label (short_name) — для подмены creator/account
	// в /unified/objects реальными именами 1:1 с CMS Manager.
	if usersCount, uerr := s.collectUsersForConnection(conn, loginResp.Eid); uerr != nil {
		log.Printf("⚠️ WialonStats users: connection=%d не удалось собрать: %v", conn.ID, uerr)
	} else {
		log.Printf("👤 WialonStats users: connection=%d, users=%d", conn.ID, usersCount)
	}

	// Точные active/inactive counts от top-level resource (user.bact).
	if loginResp.User != nil && loginResp.User.ID > 0 {
		if serr := s.collectSummaryForConnection(conn, loginResp.Eid, loginResp.User.ID); serr != nil {
			log.Printf("⚠️ WialonStats summary: connection=%d не удалось собрать: %v", conn.ID, serr)
		}
	}

	// Реестр Wialon-аккаунтов (parent_id + dealer_rights + units_count + is_direct_dealer)
	// для partner billing Ф2. Идёт после units/object_stats — units_count считается
	// DB-джойном по уже свежим данным. ownerUserID нужен для is_direct_dealer.
	ownerUserID := int64(0)
	if loginResp.User != nil {
		ownerUserID = loginResp.User.ID
	}
	if accCount, aerr := s.collectAccountsForConnection(conn, loginResp.Eid, ownerUserID); aerr != nil {
		log.Printf("⚠️ WialonStats accounts: connection=%d не удалось собрать: %v", conn.ID, aerr)
	} else {
		log.Printf("🏢 WialonStats accounts: connection=%d, accounts=%d", conn.ID, accCount)
	}

	return len(upserts), nil
}

// collectAccountsForConnection — снимок Wialon-аккаунтов в wialon_account_statuses
// для partner billing Ф2 (см. models.WialonAccountStatus).
//
// Источник: SearchUsersWithHost (avl_user) — даёт IsActive (fl&0x1),
// DealerRights (fl&0x10), ParentId (crt — иерархия через создателя).
// units_count — прямой счёт юнитов аккаунта DB-джойном: wialon_units.billing_id
// → wialon_object_stats.resource_id → user_id (per connection). НЕ recursive.
//
// После цикла удаляет строки не обновлённые в этом проходе (исчезнувшие аккаунты).
func (s *WialonStatsService) collectAccountsForConnection(conn models.WialonConnection, eid string, ownerUserID int64) (int, error) {
	cycleStart := time.Now()

	accounts, err := s.wialonService.SearchUsersWithHost(conn.Host, eid)
	if err != nil {
		return 0, fmt.Errorf("search users: %w", err)
	}
	if len(accounts) == 0 {
		return 0, nil
	}

	// Аккаунт интеграции (токен-овнер) = его bact (resource). Прямой дилер =
	// тот, чей parentAccountId == ownerBact. ownerBact=0 (сбой resolve) → is_direct
	// неопределим → НЕ перезаписываем прежнее значение (см. known/unknown ниже).
	ownerBact := int64(0)
	if ownerUserID > 0 {
		if b, berr := s.ResolveBactForUser(conn.Host, eid, ownerUserID); berr != nil {
			log.Printf("⚠️ WialonStats accounts: ResolveBactForUser(owner=%d) conn=%d: %v", ownerUserID, conn.ID, berr)
		} else {
			ownerBact = b
		}
	}

	// parentAccountId по дилерам (account/get_account_data, батчами).
	parentByUser := s.fetchDealerParentAccounts(conn.Host, eid, accounts)

	// units_count per user_id: unit → billing-resource → resource creator (user).
	type userCount struct {
		UserID int64
		Cnt    int
	}
	var counts []userCount
	// Явные public.*: collectForConnection вызывается с public search_path, но
	// квалифицируем для устойчивости. ОШИБКУ возвращаем (не продолжаем с нулями):
	// иначе upsert перезапишет units_count=0 всем → недобиллинг партнёров (Codex C1).
	if err := s.db.Raw(`
		SELECT s.user_id AS user_id, COUNT(DISTINCT u.unit_id) AS cnt
		FROM public.wialon_units u
		JOIN public.wialon_object_stats s
		  ON s.resource_id = u.billing_id AND s.connection_id = u.connection_id
		WHERE u.connection_id = ?
		GROUP BY s.user_id`, conn.ID).Scan(&counts).Error; err != nil {
		return 0, fmt.Errorf("units_count join conn=%d: %w", conn.ID, err)
	}
	countByUser := make(map[int64]int, len(counts))
	for _, c := range counts {
		countByUser[c.UserID] = c.Cnt
	}

	// Делим на «known»/«unknown» по определимости is_direct_dealer.
	// Определимо когда: НЕ дилер (is_direct всегда false) ИЛИ
	// (ownerBact известен И parentAccountId дилера разрезолвлен).
	// Неопределимые НЕ перезаписывают parent_account_id/is_direct_dealer
	// (preserve prior) — иначе transient-сбой ResolveBactForUser (ownerBact=0,
	// Codex B2) или item-error get_account_data (Codex SF1) молча обнулит
	// дропдаун партнёров на следующем синке.
	known := make([]models.WialonAccountStatus, 0, len(accounts))
	unknown := make([]models.WialonAccountStatus, 0)
	for _, a := range accounts {
		parentAcc, resolved := parentByUser[a.ID]
		rec := models.WialonAccountStatus{
			ConnectionID:    conn.ID,
			WialonUserID:    a.ID,
			Name:            a.Name,
			ParentUserID:    a.ParentId,
			DealerRights:    a.DealerRights,
			ParentAccountID: parentAcc,
			IsDirectDealer:  a.DealerRights && ownerBact != 0 && resolved && parentAcc == ownerBact,
			IsActive:        a.IsActive,
			UnitsCount:      countByUser[a.ID],
			LastCollectedAt: cycleStart,
		}
		directKnown := !a.DealerRights || (ownerBact != 0 && resolved)
		if directKnown {
			known = append(known, rec)
		} else {
			unknown = append(unknown, rec)
		}
	}

	if err := s.upsertAccountBatch(known, true); err != nil {
		return 0, err
	}
	if err := s.upsertAccountBatch(unknown, false); err != nil {
		return 0, err
	}

	// Чистим исчезнувшие аккаунты (не обновлены в этом проходе).
	if err := s.db.Where("connection_id = ? AND last_collected_at < ?", conn.ID, cycleStart).
		Delete(&models.WialonAccountStatus{}).Error; err != nil {
		log.Printf("⚠️ WialonStats accounts: cleanup stale conn=%d: %v", conn.ID, err)
	}

	return len(known) + len(unknown), nil
}

// upsertAccountBatch — upsert чанками по wialon_account_statuses. withDirect=false
// исключает parent_account_id/is_direct_dealer из DoUpdates → существующие строки
// сохраняют прежнее значение (не обнуляем при неопределимом is_direct_dealer).
func (s *WialonStatsService) upsertAccountBatch(batch []models.WialonAccountStatus, withDirect bool) error {
	if len(batch) == 0 {
		return nil
	}
	cols := []string{"name", "parent_user_id", "dealer_rights", "is_active", "units_count", "last_collected_at", "updated_at"}
	if withDirect {
		cols = append(cols, "parent_account_id", "is_direct_dealer")
	}
	const chunkSize = 500
	for i := 0; i < len(batch); i += chunkSize {
		end := i + chunkSize
		if end > len(batch) {
			end = len(batch)
		}
		if err := s.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "connection_id"}, {Name: "wialon_user_id"}},
			DoUpdates: clause.AssignmentColumns(cols),
		}).Create(batch[i:end]).Error; err != nil {
			return fmt.Errorf("upsert accounts chunk [%d:%d]: %w", i, end, err)
		}
	}
	return nil
}

// CollectAccountsForConnectionFull — публичная обёртка для one-off populate (cmd):
// логин + collectAccountsForConnection (units_count + dealer_rights + parentAccountId
// + is_direct_dealer). units/object_stats должны быть уже наполнены (scheduler).
func (s *WialonStatsService) CollectAccountsForConnectionFull(conn models.WialonConnection) (int, error) {
	login, err := s.wialonService.LoginWithHost(conn.Host, conn.Token)
	if err != nil {
		return 0, fmt.Errorf("login: %w", err)
	}
	defer func() { _ = s.wialonService.LogoutWithHost(conn.Host, login.Eid) }()
	ownerID := int64(0)
	if login.User != nil {
		ownerID = login.User.ID
	}
	return s.collectAccountsForConnection(conn, login.Eid, ownerID)
}

// fetchDealerParentAccounts — parentAccountId по дилерам через
// account/get_account_data (type:1), батчами по 100. Возвращает
// map[wialon_user_id]parentAccountId. Только дилеры (dealer_rights) — у клиентов
// родитель для дропдауна не нужен. Ошибки батча логируются и пропускаются
// (parentAccountId=0 → is_direct_dealer=false, дилер просто не попадёт в дропдаун).
func (s *WialonStatsService) fetchDealerParentAccounts(host, eid string, accounts []WialonAccount) map[int64]int64 {
	result := make(map[int64]int64)

	type dealerRef struct{ userID, bact int64 }
	dealers := make([]dealerRef, 0)
	for _, a := range accounts {
		if a.DealerRights && a.BillingAccountID > 0 {
			dealers = append(dealers, dealerRef{a.ID, a.BillingAccountID})
		}
	}
	if len(dealers) == 0 {
		return result
	}

	// callBatch шлёт весь payload в URL query → большой батч даёт 414 (<html>).
	// 25 get_account_data ≈ 5KB URL, безопасно.
	const batchSize = 25
	for i := 0; i < len(dealers); i += batchSize {
		end := i + batchSize
		if end > len(dealers) {
			end = len(dealers)
		}
		chunk := dealers[i:end]
		calls := make([]map[string]interface{}, 0, len(chunk))
		for _, d := range chunk {
			calls = append(calls, map[string]interface{}{
				"svc":    "account/get_account_data",
				"params": map[string]interface{}{"itemId": d.bact, "type": 1},
			})
		}
		results, err := s.wialonService.callBatch(host, eid, calls)
		if err != nil {
			log.Printf("⚠️ WialonStats accounts: get_account_data batch [%d:%d]: %v", i, end, err)
			continue
		}
		for j, raw := range results {
			if j >= len(chunk) {
				break
			}
			var data struct {
				ParentAccountID int64 `json:"parentAccountId"`
				Error           int   `json:"error"`
			}
			if err := json.Unmarshal(raw, &data); err != nil {
				log.Printf("⚠️ WialonStats accounts: get_account_data unmarshal user=%d: %v", chunk[j].userID, err)
				continue
			}
			// Wialon при ошибке item'а отдаёт {"error":N} — НЕ трактуем как parentAccountId=0,
			// иначе дилер тихо станет не-прямым. Пропускаем → останется неразрезолвленным.
			if data.Error != 0 {
				log.Printf("⚠️ WialonStats accounts: get_account_data error=%d user=%d", data.Error, chunk[j].userID)
				continue
			}
			// Присутствие в map = разрезолвлен (даже parentAccountId=0 — валидный «нет родителя»).
			result[chunk[j].userID] = data.ParentAccountID
		}
	}
	return result
}

// collectSummaryForConnection — точные счётчики объектов из главного resource юзера (user.bact).
// Один search_item для resolve bact + один account/get_account_data → upsert в wialon_connection_summary.
//
// Wialon-семантика:
//   - avl_unit.usage         = total объектов в ресурсе
//   - activated_units.usage  = активные (active)
//   - seasonal_units.usage   = сезонные/отключённые (inactive)
//
// Это те же числа что показывает Wialon CMS Manager на корневом аккаунте.
func (s *WialonStatsService) collectSummaryForConnection(conn models.WialonConnection, eid string, ownUserID int64) error {
	// 1. Resolve user.bact (главный resource)
	resourceID, err := s.ResolveBactForUser(conn.Host, eid, ownUserID)
	if err != nil {
		return fmt.Errorf("resolve bact for user=%d: %w", ownUserID, err)
	}
	if resourceID == 0 {
		return fmt.Errorf("user %d has no bact", ownUserID)
	}

	// 2. account/get_account_data
	stat, err := s.fetchSingleResourceStat(conn.Host, eid, resourceID)
	if err != nil {
		return fmt.Errorf("get_account_data resource=%d: %w", resourceID, err)
	}
	if stat == nil {
		return fmt.Errorf("get_account_data resource=%d: nil", resourceID)
	}

	// 3. Upsert
	summary := models.WialonConnectionSummary{
		ConnectionID:    conn.ID,
		ObjectsTotal:    stat.ObjectsTotal,
		ObjectsActive:   stat.ObjectsActive,
		ObjectsInactive: stat.ObjectsDeactivated, // seasonal_units → inactive
		LastCollectedAt: time.Now(),
	}
	if err := s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "connection_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"objects_total", "objects_active", "objects_inactive",
			"last_collected_at", "updated_at",
		}),
	}).Create(&summary).Error; err != nil {
		return fmt.Errorf("upsert summary: %w", err)
	}

	log.Printf("📊 WialonStats summary: connection=%d (bact=%d) → total=%d active=%d inactive=%d",
		conn.ID, resourceID, summary.ObjectsTotal, summary.ObjectsActive, summary.ObjectsInactive)
	return nil
}

// collectUnitsForConnection — paginated search_items avl_unit → upsert в wialon_units.
// UNIQUE(connection_id, unit_id) гарантирует дедупликацию: если у нескольких билинг-ресурсов
// один токен видит одни и те же объекты, в таблице будет одна строка.
//
// После полного цикла удаляем строки которые не были обновлены (last_collected_at < cycleStart) —
// это чистит юниты которые исчезли из Wialon (удалены/перенесены в другой аккаунт).
func (s *WialonStatsService) collectUnitsForConnection(conn models.WialonConnection, eid string) (int, error) {
	cycleStart := time.Now()

	// Wialon возвращает максимум ~10к items за один search_items вызов.
	// Пагинируем через `from`/`to` пока не получим меньше pageSize.
	const pageSize = 10000
	apiURL := conn.Host + "/wialon/ajax.html"

	// flags: 1 (base id+name) | 4 (billing: crt+bact) — минимально нужное.
	const flags = 0x00000001 | 0x00000004

	type unitItem struct {
		ID   int64   `json:"id"`
		Nm   string  `json:"nm"`
		Crt  float64 `json:"crt"`  // creator user.id
		Bact float64 `json:"bact"` // billing-account = avl_resource.id
	}

	totalSeen := 0
	from := 0
	for page := 0; page < 100; page++ { // защита от бесконечного цикла
		to := from + pageSize - 1

		searchParams := map[string]interface{}{
			"spec": map[string]interface{}{
				"itemsType":     "avl_unit",
				"propName":      "sys_name",
				"propValueMask": "*",
				"sortType":      "sys_name",
			},
			"force": 1,
			"flags": flags,
			"from":  from,
			"to":    to,
		}
		paramsJSON, _ := json.Marshal(searchParams)

		params := url.Values{}
		params.Set("svc", "core/search_items")
		params.Set("sid", eid)
		params.Set("params", string(paramsJSON))

		resp, err := s.wialonService.httpClient.Post(apiURL+"?"+params.Encode(), "application/x-www-form-urlencoded", nil)
		if err != nil {
			return totalSeen, fmt.Errorf("search_items units page=%d: %w", page, err)
		}
		body, rerr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if rerr != nil {
			return totalSeen, fmt.Errorf("read body page=%d: %w", page, rerr)
		}

		var sr struct {
			TotalItemsCount int        `json:"totalItemsCount"`
			Items           []unitItem `json:"items"`
		}
		if err := json.Unmarshal(body, &sr); err != nil {
			return totalSeen, fmt.Errorf("parse page=%d: %w", page, err)
		}

		if len(sr.Items) == 0 {
			break
		}

		// Upsert батчем по 1000
		batch := make([]models.WialonUnit, 0, len(sr.Items))
		for _, it := range sr.Items {
			if it.ID == 0 {
				continue
			}
			batch = append(batch, models.WialonUnit{
				ConnectionID:    conn.ID,
				UnitID:          it.ID,
				Name:            it.Nm,
				IsActive:        true, // активацию per-unit не получаем — оставим true
				CreatorID:       int64(it.Crt),
				BillingID:       int64(it.Bact),
				LastCollectedAt: cycleStart,
			})
		}

		const chunk = 1000
		for i := 0; i < len(batch); i += chunk {
			end := i + chunk
			if end > len(batch) {
				end = len(batch)
			}
			if err := s.db.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "connection_id"}, {Name: "unit_id"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"name", "creator_id", "billing_id", "last_collected_at", "updated_at",
				}),
			}).Create(batch[i:end]).Error; err != nil {
				return totalSeen, fmt.Errorf("upsert units chunk: %w", err)
			}
		}

		totalSeen += len(sr.Items)

		// Если вернулось меньше pageSize — последняя страница
		if len(sr.Items) < pageSize {
			break
		}
		from += pageSize
	}

	// Удаляем юниты которые исчезли из Wialon (не обновлены в текущем цикле)
	if err := s.db.Where("connection_id = ? AND last_collected_at < ?", conn.ID, cycleStart).
		Delete(&models.WialonUnit{}).Error; err != nil {
		log.Printf("⚠️ WialonStats units cleanup conn=%d: %v", conn.ID, err)
	}

	return totalSeen, nil
}

// collectUsersForConnection — paginated search_items avl_user → upsert в wialon_users.
// flags=0x00000002 (custom prop) даёт prp.label/prp.short_name которое CMS показывает в
// колонке "Создатель" (короткое имя вместо полного name). Используется в /unified/objects.
func (s *WialonStatsService) collectUsersForConnection(conn models.WialonConnection, eid string) (int, error) {
	cycleStart := time.Now()
	const pageSize = 10000
	apiURL := conn.Host + "/wialon/ajax.html"

	// 1 (base id+nm) + 2 (custom prop = prp) — нужно prp.label/short_name
	const flags = 0x00000001 | 0x00000002

	type userItem struct {
		ID  int64                  `json:"id"`
		Nm  string                 `json:"nm"`
		Prp map[string]interface{} `json:"prp"`
	}

	totalSeen := 0
	from := 0
	for page := 0; page < 100; page++ {
		to := from + pageSize - 1
		searchParams := map[string]interface{}{
			"spec": map[string]interface{}{
				"itemsType":     "avl_user",
				"propName":      "sys_name",
				"propValueMask": "*",
				"sortType":      "sys_name",
			},
			"force": 1,
			"flags": flags,
			"from":  from,
			"to":    to,
		}
		paramsJSON, _ := json.Marshal(searchParams)

		params := url.Values{}
		params.Set("svc", "core/search_items")
		params.Set("sid", eid)
		params.Set("params", string(paramsJSON))

		resp, err := s.wialonService.httpClient.Post(apiURL+"?"+params.Encode(), "application/x-www-form-urlencoded", nil)
		if err != nil {
			return totalSeen, fmt.Errorf("search_items users page=%d: %w", page, err)
		}
		body, rerr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if rerr != nil {
			return totalSeen, fmt.Errorf("read body page=%d: %w", page, rerr)
		}

		var sr struct {
			TotalItemsCount int        `json:"totalItemsCount"`
			Items           []userItem `json:"items"`
		}
		if err := json.Unmarshal(body, &sr); err != nil {
			return totalSeen, fmt.Errorf("parse page=%d: %w", page, err)
		}
		if len(sr.Items) == 0 {
			break
		}

		batch := make([]models.WialonUser, 0, len(sr.Items))
		for _, it := range sr.Items {
			if it.ID == 0 {
				continue
			}
			short := ""
			if it.Prp != nil {
				// CMS показывает либо prp.label, либо prp.short_name (зависит от installation)
				if v, ok := it.Prp["label"].(string); ok && v != "" {
					short = v
				} else if v, ok := it.Prp["short_name"].(string); ok && v != "" {
					short = v
				}
			}
			batch = append(batch, models.WialonUser{
				ConnectionID:    conn.ID,
				UserID:          it.ID,
				Name:            it.Nm,
				ShortName:       short,
				IsActive:        true,
				LastCollectedAt: cycleStart,
			})
		}

		const chunk = 1000
		for i := 0; i < len(batch); i += chunk {
			end := i + chunk
			if end > len(batch) {
				end = len(batch)
			}
			if err := s.db.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "connection_id"}, {Name: "user_id"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"name", "short_name", "last_collected_at", "updated_at",
				}),
			}).Create(batch[i:end]).Error; err != nil {
				return totalSeen, fmt.Errorf("upsert users chunk: %w", err)
			}
		}

		totalSeen += len(sr.Items)
		if len(sr.Items) < pageSize {
			break
		}
		from += pageSize
	}

	// cleanup устаревших
	if err := s.db.Where("connection_id = ? AND last_collected_at < ?", conn.ID, cycleStart).
		Delete(&models.WialonUser{}).Error; err != nil {
		log.Printf("⚠️ WialonStats users cleanup conn=%d: %v", conn.ID, err)
	}

	return totalSeen, nil
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

// RefreshSingleAccount обновляет stats одной учётки точечно: live-запрос к Wialon на 1 ресурс
// (account/get_account_data) → upsert в БД. ETA <1 сек. Используется при ручных действиях
// пользователя (после toggle/edit), чтобы показать свежие числа без запуска полного scheduler-сбора.
//
// userID — это user.id в Wialon, по которому фронт идентифицирует аккаунт. Внутри определяем
// resourceID из таблицы wialon_object_stats (там user_id есть от предыдущего полного сбора).
func (s *WialonStatsService) RefreshSingleAccount(connectionID uint, userID int64) (*models.WialonObjectStat, error) {
	t0 := time.Now()

	var conn models.WialonConnection
	if err := s.db.Where("id = ? AND is_active = ?", connectionID, true).First(&conn).Error; err != nil {
		return nil, fmt.Errorf("connection %d не найден или неактивен: %w", connectionID, err)
	}

	loginResp, err := s.wialonService.LoginWithHost(conn.Host, conn.Token)
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	defer func() { _ = s.wialonService.LogoutWithHost(conn.Host, loginResp.Eid) }()

	// Wialon связь user → собственный resource хранится в user.bact (billing_account_id) = resource.id.
	// Резолвим bact через search_items на конкретного user.
	resourceID, err := s.ResolveBactForUser(conn.Host, loginResp.Eid, userID)
	if err != nil {
		log.Printf("⚠️ RefreshSingleAccount: ResolveBactForUser(user=%d) не вышел: %v, фолбэк к кандидатам", userID, err)
	}

	candidates := []int64{}
	if resourceID > 0 {
		candidates = append(candidates, resourceID)
	}
	// Фолбэк: пробуем существующий маппинг из БД, потом стандартные id+1 / id
	var existing models.WialonObjectStat
	if err := s.db.Where("connection_id = ? AND user_id = ?", connectionID, userID).First(&existing).Error; err == nil && existing.ResourceID > 0 {
		candidates = append(candidates, existing.ResourceID)
	}
	candidates = append(candidates, userID+1, userID)

	// Один account/get_account_data на 1 ресурс — самый быстрый путь
	for _, resourceID := range candidates {
		stat, err := s.fetchSingleResourceStat(conn.Host, loginResp.Eid, resourceID)
		if err != nil {
			log.Printf("⚠️ RefreshSingleAccount: resource=%d не вышло: %v, пробуем следующий кандидат", resourceID, err)
			continue
		}
		if stat == nil {
			continue
		}
		stat.ConnectionID = connectionID
		stat.UserID = userID
		stat.ResourceID = resourceID
		stat.LastCollectedAt = time.Now()

		if err := s.db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "connection_id"}, {Name: "resource_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"user_id", "objects_total", "objects_active", "objects_deactivated",
				"last_collected_at", "updated_at",
			}),
		}).Create(stat).Error; err != nil {
			return nil, fmt.Errorf("upsert: %w", err)
		}

		log.Printf("⚡ RefreshSingleAccount: conn=%d user=%d resource=%d → total=%d active=%d за %s",
			connectionID, userID, resourceID, stat.ObjectsTotal, stat.ObjectsActive, time.Since(t0))
		return stat, nil
	}

	return nil, fmt.Errorf("не удалось найти ресурс для user_id=%d (пробовали %v)", userID, candidates)
}

// ResolveBactForUser — найти resourceID (bact) который "принадлежит" пользователю.
// Wialon: user.bact указывает на user-собственный billing-resource (avl_resource).
// flags=5 даёт id+nm+bact для user.
func (s *WialonStatsService) ResolveBactForUser(host, eid string, userID int64) (int64, error) {
	calls := []map[string]interface{}{
		{
			"svc": "core/search_item",
			"params": map[string]interface{}{
				"id":    userID,
				"flags": 5,
			},
		},
	}
	results, err := s.wialonService.callBatch(host, eid, calls)
	if err != nil {
		return 0, err
	}
	var resp struct {
		Item map[string]interface{} `json:"item"`
	}
	if err := json.Unmarshal(results[0], &resp); err != nil {
		return 0, fmt.Errorf("парсинг search_item: %w", err)
	}
	if resp.Item == nil {
		return 0, fmt.Errorf("user %d не найден", userID)
	}
	if bact, ok := resp.Item["bact"].(float64); ok && bact > 0 {
		return int64(bact), nil
	}
	return 0, fmt.Errorf("у user %d нет bact", userID)
}

// fetchSingleResourceStat — один запрос account/get_account_data на ресурс. Парсинг как
// в GetUnitsCountWithHierarchy, но для одной записи.
func (s *WialonStatsService) fetchSingleResourceStat(host, eid string, resourceID int64) (*models.WialonObjectStat, error) {
	calls := []map[string]interface{}{
		{
			"svc": "account/get_account_data",
			"params": map[string]interface{}{
				"itemId": resourceID,
				"type":   2,
			},
		},
	}
	results, err := s.wialonService.callBatch(host, eid, calls)
	if err != nil {
		return nil, err
	}

	var accountData map[string]interface{}
	if err := json.Unmarshal(results[0], &accountData); err != nil {
		return nil, fmt.Errorf("парсинг account_data: %w", err)
	}

	// Wialon при ошибке возвращает {"error": N, "reason": "..."}. Если поле error != 0 — нет такого ресурса.
	if errCode, ok := accountData["error"].(float64); ok && errCode != 0 {
		return nil, fmt.Errorf("wialon error %d", int(errCode))
	}

	stat := &models.WialonObjectStat{}
	if settings, ok := accountData["settings"].(map[string]interface{}); ok {
		if combined, ok := settings["combined"].(map[string]interface{}); ok {
			if svcs, ok := combined["services"].(map[string]interface{}); ok {
				if avlUnit, ok := svcs["avl_unit"].(map[string]interface{}); ok {
					if usage, ok := avlUnit["usage"].(float64); ok {
						stat.ObjectsTotal = int(usage)
					}
				}
				if act, ok := svcs["activated_units"].(map[string]interface{}); ok {
					if usage, ok := act["usage"].(float64); ok {
						stat.ObjectsActive = int(usage)
					}
				}
				if season, ok := svcs["seasonal_units"].(map[string]interface{}); ok {
					if usage, ok := season["usage"].(float64); ok {
						stat.ObjectsDeactivated = int(usage)
					}
				}
				if stat.ObjectsTotal == 0 && (stat.ObjectsActive > 0 || stat.ObjectsDeactivated > 0) {
					stat.ObjectsTotal = stat.ObjectsActive + stat.ObjectsDeactivated
				}
				return stat, nil
			}
		}
	}
	return stat, nil // ресурс существует, но без объектов
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
