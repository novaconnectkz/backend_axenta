// One-off: наполняет wialon_account_statuses (partner billing Ф2) на проде.
//
// Читает Wialon API (SearchUsersWithHost) для каждого wialon_connection →
// dealer_rights/parent_id/is_active, units_count считает DB-джойном по уже
// наполненным scheduler'ом wialon_units+wialon_object_stats. Пишет ТОЛЬКО
// таблицу wialon_account_statuses — работающий сервис её ещё не читает (до W3),
// поэтому безопасно гонять на проде без деплоя.
//
// Запуск на проде:
//   PATH=/usr/local/go/bin:$PATH go run ./cmd/populate_wialon_accounts
package main

import (
	"log"
	"time"

	"gorm.io/gorm/clause"

	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
)

func main() {
	if _, err := config.LoadConfig(); err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("db: %v", err)
	}

	// Создаём таблицу если её ещё нет (глобальная, public).
	if err := database.DB.AutoMigrate(&models.WialonAccountStatus{}); err != nil {
		log.Fatalf("automigrate: %v", err)
	}

	var conns []models.WialonConnection
	if err := database.DB.Find(&conns).Error; err != nil { // AfterFind расшифрует token
		log.Fatalf("find connections: %v", err)
	}
	log.Printf("🔌 wialon_connections: %d", len(conns))

	ws := services.NewWialonService()
	totalAccounts, totalDealers := 0, 0

	for _, conn := range conns {
		if conn.Token == "" || conn.Host == "" {
			log.Printf("⏭️  conn=%d (%s): нет host/token", conn.ID, conn.Name)
			continue
		}
		loginResp, err := ws.LoginWithHost(conn.Host, conn.Token)
		if err != nil {
			log.Printf("❌ conn=%d (%s): login: %v", conn.ID, conn.Name, err)
			continue
		}

		accounts, err := ws.SearchUsersWithHost(conn.Host, loginResp.Eid)
		if err != nil {
			log.Printf("❌ conn=%d (%s): search users: %v", conn.ID, conn.Name, err)
			_ = ws.LogoutWithHost(conn.Host, loginResp.Eid)
			continue
		}

		// units_count per user_id: unit → billing-resource → resource creator (user).
		type userCount struct {
			UserID int64
			Cnt    int
		}
		var counts []userCount
		if err := database.DB.Raw(`
			SELECT s.user_id AS user_id, COUNT(DISTINCT u.unit_id) AS cnt
			FROM wialon_units u
			JOIN wialon_object_stats s
			  ON s.resource_id = u.billing_id AND s.connection_id = u.connection_id
			WHERE u.connection_id = ?
			GROUP BY s.user_id`, conn.ID).Scan(&counts).Error; err != nil {
			log.Printf("⚠️ conn=%d: units_count join: %v", conn.ID, err)
		}
		countByUser := make(map[int64]int, len(counts))
		for _, c := range counts {
			countByUser[c.UserID] = c.Cnt
		}

		cycleStart := time.Now()
		batch := make([]models.WialonAccountStatus, 0, len(accounts))
		dealers := 0
		for _, a := range accounts {
			if a.DealerRights {
				dealers++
			}
			batch = append(batch, models.WialonAccountStatus{
				ConnectionID:    conn.ID,
				WialonUserID:    a.ID,
				Name:            a.Name,
				ParentUserID:    a.ParentId,
				DealerRights:    a.DealerRights,
				IsActive:        a.IsActive,
				UnitsCount:      countByUser[a.ID],
				LastCollectedAt: cycleStart,
			})
		}

		const chunkSize = 500
		for i := 0; i < len(batch); i += chunkSize {
			end := i + chunkSize
			if end > len(batch) {
				end = len(batch)
			}
			if err := database.DB.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "connection_id"}, {Name: "wialon_user_id"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"name", "parent_user_id", "dealer_rights", "is_active",
					"units_count", "last_collected_at", "updated_at",
				}),
			}).Create(batch[i:end]).Error; err != nil {
				log.Printf("❌ conn=%d: upsert chunk [%d:%d]: %v", conn.ID, i, end, err)
			}
		}
		if err := database.DB.Where("connection_id = ? AND last_collected_at < ?", conn.ID, cycleStart).
			Delete(&models.WialonAccountStatus{}).Error; err != nil {
			log.Printf("⚠️ conn=%d: cleanup stale: %v", conn.ID, err)
		}

		_ = ws.LogoutWithHost(conn.Host, loginResp.Eid)
		log.Printf("✅ conn=%d (%s): accounts=%d dealers=%d", conn.ID, conn.Name, len(batch), dealers)
		totalAccounts += len(batch)
		totalDealers += dealers
	}

	log.Printf("📊 ИТОГО: accounts=%d dealers=%d", totalAccounts, totalDealers)

	// Sanity: топ-дилеры по прямому units_count + сверка с recursive snapshot.
	type row struct {
		ConnectionID uint
		WialonUserID int64
		Name         string
		UnitsCount   int
		DealerRights bool
	}
	var top []row
	database.DB.Raw(`SELECT connection_id, wialon_user_id, name, units_count, dealer_rights
		FROM wialon_account_statuses ORDER BY units_count DESC LIMIT 10`).Scan(&top)
	log.Printf("🔎 Топ-10 аккаунтов по прямому units_count:")
	for _, r := range top {
		log.Printf("   conn=%d user=%d dealer=%v units=%d %q", r.ConnectionID, r.WialonUserID, r.DealerRights, r.UnitsCount, r.Name)
	}
}
