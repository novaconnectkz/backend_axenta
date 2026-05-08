package main

import (
	"flag"
	"log"

	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
)

// Ad-hoc CLI: ручной триггер SkifService.SyncUsers для проверки этапа 2.
// Запуск: go run ./cmd/skif-sync-users -conn=1
func main() {
	connID := flag.Uint("conn", 1, "skif_connections.id")
	flag.Parse()

	if _, err := config.LoadConfig(); err != nil {
		log.Fatalf("LoadConfig: %v", err)
	}
	if err := database.ConnectDatabaseWithoutMigrations(); err != nil {
		log.Fatalf("ConnectDatabase: %v", err)
	}

	var conn models.SkifConnection
	if err := database.DB.First(&conn, *connID).Error; err != nil {
		log.Fatalf("load conn %d: %v", *connID, err)
	}

	log.Printf("🔄 SkifService.SyncUsers: conn=%d (%s)", conn.ID, conn.Name)
	svc := services.NewSkifService(database.DB)
	count, err := svc.SyncUsers(&conn)
	if err != nil {
		log.Fatalf("SyncUsers: %v", err)
	}
	log.Printf("✅ Готово: %d users", count)
}
