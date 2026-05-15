// One-shot миграция: шифрует все plaintext-пароли в skif_connections.
// Идемпотентно — уже зашифрованные (с префиксом enc:v1:) пропускает.
//
// Запуск на проде:
//   go run ./cmd/encrypt_skif_passwords
package main

import (
	"log"

	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/utils"
)

func main() {
	if _, err := config.LoadConfig(); err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("db: %v", err)
	}

	var conns []models.SkifConnection
	if err := database.DB.Unscoped().Find(&conns).Error; err != nil {
		log.Fatalf("find: %v", err)
	}

	encrypted, skipped, errored := 0, 0, 0
	for _, c := range conns {
		if c.Password == "" {
			skipped++
			continue
		}
		if utils.IsEncrypted(c.Password) {
			skipped++
			continue
		}
		enc, err := utils.EncryptString(c.Password)
		if err != nil {
			log.Printf("❌ id=%d (%s): encrypt: %v", c.ID, c.Name, err)
			errored++
			continue
		}
		if err := database.DB.Unscoped().
			Model(&models.SkifConnection{}).
			Where("id = ?", c.ID).
			Update("password", enc).Error; err != nil {
			log.Printf("❌ id=%d (%s): update: %v", c.ID, c.Name, err)
			errored++
			continue
		}
		log.Printf("✅ id=%d (%s, login=%s): зашифрован", c.ID, c.Name, c.Login)
		encrypted++
	}

	log.Printf("📊 Итого: всего=%d, зашифровано=%d, пропущено=%d, ошибки=%d",
		len(conns), encrypted, skipped, errored)
}
