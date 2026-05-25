// One-off: наполняет wialon_account_statuses (partner billing Ф2) на проде.
//
// Делегирует в WialonStatsService.CollectAccountsForConnectionFull: логин +
// units_count (units.billing_id→object_stats.resource_id→user_id) + dealer_rights +
// parentAccountId (account/get_account_data) + is_direct_dealer (parent == bact
// токен-овнера). units/object_stats должны быть уже наполнены scheduler'ом.
// Пишет ТОЛЬКО wialon_account_statuses — работающий сервис её ещё не читает.
//
// Запуск на проде:
//
//	PATH=/usr/local/go/bin:$PATH go run ./cmd/populate_wialon_accounts
package main

import (
	"log"

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
	if err := database.DB.AutoMigrate(&models.WialonAccountStatus{}); err != nil {
		log.Fatalf("automigrate: %v", err)
	}

	var conns []models.WialonConnection
	if err := database.DB.Find(&conns).Error; err != nil { // AfterFind расшифрует token
		log.Fatalf("find connections: %v", err)
	}
	log.Printf("🔌 wialon_connections: %d", len(conns))

	stats := services.NewWialonStatsService()
	total := 0
	for _, conn := range conns {
		if conn.Token == "" || conn.Host == "" {
			log.Printf("⏭️  conn=%d (%s): нет host/token", conn.ID, conn.Name)
			continue
		}
		n, err := stats.CollectAccountsForConnectionFull(conn)
		if err != nil {
			log.Printf("❌ conn=%d (%s): %v", conn.ID, conn.Name, err)
			continue
		}
		log.Printf("✅ conn=%d (%s): accounts=%d", conn.ID, conn.Name, n)
		total += n
	}
	log.Printf("📊 ИТОГО accounts=%d", total)

	// Sanity: прямые дилеры по коннектам.
	type row struct {
		ConnectionID uint
		Direct       int
		AllDealers   int
	}
	var rows []row
	database.DB.Raw(`SELECT connection_id,
		COUNT(*) FILTER (WHERE is_direct_dealer) AS direct,
		COUNT(*) FILTER (WHERE dealer_rights) AS all_dealers
		FROM wialon_account_statuses GROUP BY connection_id ORDER BY connection_id`).Scan(&rows)
	for _, r := range rows {
		log.Printf("🔎 conn=%d: прямых дилеров=%d из всех дилеров=%d", r.ConnectionID, r.Direct, r.AllDealers)
	}
}
