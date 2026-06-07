// cmd/wialon_gap_probe — одноразовый READ-ONLY замер gap для M1 (Wialon per-period billing).
//
// Цель: до постройки M2 эмпирически проверить на conn 7 четыре вещи:
//  1. ACRM-семантика active выводима ли из total-истории: Active(now) ?= Total - Deactivated (account_data).
//  2. Якорь: get_statistics avl_unit_total(сегодня) ?= account_data Total(сейчас).
//  3. recursive=1 (поддерево дилера) vs recursive=0 — над-счёт при суб-дилерах.
//  4. Если есть forward partner_daily_snapshots — реконструкция active vs записанное.
//
// Ничего не пишет в БД и в Wialon (только login/logout + search/get_statistics/get_account_data).
//
// Запуск: ./wialon_gap_probe [-conn 7] [-days 14]
package main

import (
	"flag"
	"fmt"
	"log"
	"sort"
	"time"

	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
)

type dealerRow struct {
	WialonUserID    int64
	Name            string
	ParentAccountID int64
	ObjectsTotal    int
	ObjectsActive   int
	IsDirectDealer  bool
	DealerRights    bool
	LastCollectedAt time.Time
}

func main() {
	connID := flag.Int("conn", 7, "wialon connection id")
	days := flag.Int("days", 14, "глубина истории в днях")
	limitDealers := flag.Int("limit", 8, "макс. дилеров для прогона (rate-limit)")
	noHist := flag.Bool("nohist", false, "только account_data NOW, без get_statistics (для крупных коннектов)")
	flag.Parse()

	if _, err := config.LoadConfig(); err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := database.ConnectDatabaseWithoutMigrations(); err != nil {
		log.Fatalf("db: %v", err)
	}

	var conn models.WialonConnection
	if err := database.DB.First(&conn, *connID).Error; err != nil {
		log.Fatalf("conn %d не найден: %v", *connID, err)
	}
	fmt.Printf("=== conn %d: %s (host=%s, type=%s, company=%d) ===\n",
		conn.ID, conn.Name, conn.Host, conn.ConnectionType, conn.CompanyID)

	ws := services.NewWialonService()
	ss := services.NewWialonStatsService()
	hs := services.NewWialonHistoryService(database.DB, ws, ss)

	login, err := ws.LoginWithHost(conn.Host, conn.Token)
	if err != nil {
		log.Fatalf("login: %v", err)
	}
	defer func() { _ = ws.LogoutWithHost(conn.Host, login.Eid) }()
	if login.User == nil || login.User.ID == 0 {
		log.Fatalf("login без user.id")
	}
	apiURL := conn.Host + "/wialon/ajax.html"
	fmt.Printf("login ok: user=%d (%s) eid=%s\n\n", login.User.ID, login.User.Name, login.Eid[:8]+"...")

	// NOW-срез по всем ресурсам коннекта: resID -> {Total, Active, Deactivated}
	nowStats, totalUnits, err := ws.GetUnitsCountWithHierarchy(conn.Host, login.Eid)
	if err != nil {
		log.Fatalf("GetUnitsCountWithHierarchy: %v", err)
	}
	fmt.Printf("account_data NOW: %d ресурсов, суммарно units=%d\n\n", len(nowStats), totalUnits)

	// Дилеры из БД (наполнено collectAccountsForConnection).
	var dealers []dealerRow
	database.DB.Raw(`
		SELECT wialon_user_id, name, parent_account_id, objects_total, objects_active,
		       is_direct_dealer, dealer_rights, last_collected_at
		FROM public.wialon_account_statuses
		WHERE connection_id = ?
		ORDER BY is_direct_dealer DESC, objects_total DESC`, conn.ID).Scan(&dealers)

	directCount := 0
	for _, d := range dealers {
		if d.IsDirectDealer {
			directCount++
		}
	}
	fmt.Printf("wialon_account_statuses: всего %d строк, прямых дилеров %d\n", len(dealers), directCount)

	// Берём прямых дилеров; если их нет — топ по objects_total (чтобы прогон не был пустым).
	subjects := make([]dealerRow, 0, *limitDealers)
	for _, d := range dealers {
		if d.IsDirectDealer {
			subjects = append(subjects, d)
		}
	}
	if len(subjects) == 0 {
		fmt.Println("⚠️ прямых дилеров нет — беру топ-аккаунты как субъекты")
		for i, d := range dealers {
			if i >= *limitDealers {
				break
			}
			subjects = append(subjects, d)
		}
	}
	if len(subjects) > *limitDealers {
		subjects = subjects[:*limitDealers]
	}
	fmt.Printf("прогон по %d субъектам\n", len(subjects))

	to := time.Now().UTC()
	from := to.AddDate(0, 0, -*days)

	type gapStat struct {
		mismatchActive int // Active != Total-Deactivated
		mismatchAnchor int // get_stat today != account_data Total
		overcount      int // r1 today > Total (над-счёт)
		undercount     int // r1 today < Total
		fwdCompared      int
		fwdMatched       int
		seasonalSubjects int
	}
	var gs gapStat

	for i, d := range subjects {
		fmt.Printf("\n──────────────────────────────────────────────\n")
		fmt.Printf("[%d/%d] dealer user=%d \"%s\" direct=%v dealerRights=%v\n",
			i+1, len(subjects), d.WialonUserID, d.Name, d.IsDirectDealer, d.DealerRights)
		fmt.Printf("  DB: objects_total=%d objects_active=%d (collected %s назад)\n",
			d.ObjectsTotal, d.ObjectsActive, time.Since(d.LastCollectedAt).Round(time.Minute))

		// bact-ресурс дилера для get_statistics / account_data lookup.
		bactRes, err := ss.ResolveBactForUser(conn.Host, login.Eid, d.WialonUserID)
		if err != nil || bactRes == 0 {
			fmt.Printf("  ⚠️ ResolveBactForUser: %v (bact=%d) — пропуск\n", err, bactRes)
			continue
		}
		now, ok := nowStats[bactRes]
		if !ok {
			fmt.Printf("  ⚠️ bactRes=%d нет в account_data NOW — пропуск\n", bactRes)
			continue
		}

		// (1) ACRM-семантика: Active ?= Total - Deactivated
		derived := now.Total - now.Deactivated
		flag1 := "OK"
		if derived != now.Active {
			flag1 = "‼️ РАСХОЖДЕНИЕ"
			gs.mismatchActive++
		}
		fmt.Printf("  NOW account_data: Total(avl_unit)=%d  Active(activated)=%d  Deactivated(seasonal)=%d\n",
			now.Total, now.Active, now.Deactivated)
		fmt.Printf("  (1) Active ?= Total-Deactivated: %d ?= %d  -> %s\n", now.Active, derived, flag1)
		if now.Deactivated > 0 {
			gs.seasonalSubjects++
		}

		if *noHist {
			time.Sleep(150 * time.Millisecond)
			continue
		}

		// История avl_unit_total per-date, recursive=0 и recursive=1.
		r0, err0 := hs.GetStatistics(apiURL, login.Eid, bactRes, from, to)
		r1, err1 := hs.GetStatisticsRecursive(apiURL, login.Eid, bactRes, from, to)
		if err0 != nil || err1 != nil {
			fmt.Printf("  ⚠️ get_statistics: r0=%v r1=%v\n", err0, err1)
			continue
		}
		r0m := byDate(r0)
		r1m := byDate(r1)

		todayKey := to.Format("2006-01-02")
		r0today := lastKnown(r0m, todayKey)
		r1today := lastKnown(r1m, todayKey)

		// (2) Якорь: get_statistics(today) vs account_data Total now.
		flag2 := "OK"
		if r0today != now.Total {
			flag2 = "‼️"
			gs.mismatchAnchor++
		}
		fmt.Printf("  (2) Якорь r0_today=%d  account_data Total=%d  -> %s\n", r0today, now.Total, flag2)

		// (3) recursive=1 поддерево vs Total now (над/недо-счёт).
		flag3 := "OK"
		switch {
		case r1today > now.Total:
			flag3 = fmt.Sprintf("над-счёт +%d (суб-дилеры?)", r1today-now.Total)
			gs.overcount++
		case r1today < now.Total:
			flag3 = fmt.Sprintf("недо-счёт %d", r1today-now.Total)
			gs.undercount++
		}
		fmt.Printf("  (3) recursive: r0_today=%d r1_today=%d  Total=%d  -> %s\n", r0today, r1today, now.Total, flag3)

		// Печать ряда total per-date (r1 — поддерево), active = total - deactivated(frozen).
		fmt.Printf("  история (recursive=1 total / active=total-%d):\n", now.Deactivated)
		keys := dateKeys(from, to)
		var prev int
		for _, k := range keys {
			tot := r1m[k]
			if tot == 0 { // пробел — несём предыдущий
				tot = prev
			}
			prev = tot
			act := tot - now.Deactivated
			if act < 0 {
				act = 0
			}
			cr, dl := 0, 0
			if v, ok := r1m["c:"+k]; ok {
				cr = v
			}
			if v, ok := r1m["d:"+k]; ok {
				dl = v
			}
			fmt.Printf("    %s  total=%-5d active=%-5d (+%d/-%d)\n", k, tot, act, cr, dl)
		}

		// (4) forward partner_daily_snapshots в tenant-схеме компании коннекта.
		gs.fwdCompared, gs.fwdMatched = compareForward(conn.CompanyID, conn.ID, d.WialonUserID, r1m, now.Deactivated, keys, gs.fwdCompared, gs.fwdMatched)

		time.Sleep(200 * time.Millisecond) // rate-limit
	}

	fmt.Printf("\n══════════════════ ИТОГ ══════════════════\n")
	fmt.Printf("субъектов прогнано: %d\n", len(subjects))
	fmt.Printf("(1) Active != Total-Deactivated: %d  <- если >0, ACRM-active НЕ выводится из total-истории\n", gs.mismatchActive)
	fmt.Printf("    субъектов с Deactivated(seasonal)>0: %d  <- сколько реально упражнили gate(1)\n", gs.seasonalSubjects)
	fmt.Printf("(2) якорь get_stat != account_data: %d\n", gs.mismatchAnchor)
	fmt.Printf("(3) recursive=1 над-счёт: %d, недо-счёт: %d\n", gs.overcount, gs.undercount)
	fmt.Printf("(4) forward сверено дней: %d, совпало: %d\n", gs.fwdCompared, gs.fwdMatched)
}

func byDate(stats []services.WialonDailyStat) map[string]int {
	m := make(map[string]int, len(stats)*3)
	for _, s := range stats {
		k := time.Unix(s.Timestamp, 0).UTC().Format("2006-01-02")
		m[k] = s.UnitTotal
		m["c:"+k] = s.UnitCreated
		m["d:"+k] = s.UnitDeleted
	}
	return m
}

func dateKeys(from, to time.Time) []string {
	var ks []string
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		ks = append(ks, d.Format("2006-01-02"))
	}
	return ks
}

// lastKnown — total на dateKey, либо ближайший предыдущий known (>0).
func lastKnown(m map[string]int, dateKey string) int {
	if v, ok := m[dateKey]; ok && v > 0 {
		return v
	}
	// собрать known-даты, взять максимально близкую <= dateKey
	var dates []string
	for k := range m {
		if len(k) == 10 && m[k] > 0 {
			dates = append(dates, k)
		}
	}
	sort.Strings(dates)
	best := 0
	for _, k := range dates {
		if k <= dateKey {
			best = m[k]
		}
	}
	return best
}

func compareForward(companyID, connID uint, userID int64, r1m map[string]int, deactivated int, keys []string, cmp, match int) (int, int) {
	schema := fmt.Sprintf("tenant_%d", companyID)
	type fwd struct {
		SnapshotDate       time.Time
		ActiveObjectsCount int
	}
	var rows []fwd
	q := fmt.Sprintf(`SELECT snapshot_date, active_objects_count FROM %s.partner_daily_snapshots
		WHERE partner_source='wialon' AND connection_id=? AND partner_external_id=?
		ORDER BY snapshot_date`, schema)
	if err := database.DB.Raw(q, connID, fmt.Sprintf("%d", userID)).Scan(&rows).Error; err != nil {
		return cmp, match // схемы/таблицы может не быть — тихо пропускаем
	}
	if len(rows) == 0 {
		return cmp, match
	}
	fmt.Printf("  (4) forward snapshots найдено %d — сверка reconstruction vs записанное:\n", len(rows))
	var prev int
	totByKey := map[string]int{}
	for _, k := range keys {
		t := r1m[k]
		if t == 0 {
			t = prev
		}
		prev = t
		totByKey[k] = t
	}
	for _, r := range rows {
		k := r.SnapshotDate.UTC().Format("2006-01-02")
		recon := totByKey[k] - deactivated
		if recon < 0 {
			recon = 0
		}
		if _, ok := totByKey[k]; !ok {
			continue
		}
		cmp++
		mark := "OK"
		if recon != r.ActiveObjectsCount {
			mark = fmt.Sprintf("Δ=%d", recon-r.ActiveObjectsCount)
		} else {
			match++
		}
		fmt.Printf("    %s  forward=%d  recon=%d  %s\n", k, r.ActiveObjectsCount, recon, mark)
	}
	return cmp, match
}
