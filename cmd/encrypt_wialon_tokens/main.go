// One-shot миграция: шифрует все plaintext-токены в wialon_connections.
// Идемпотентно — уже зашифрованные (с префиксом enc:v1:) пропускает.
//
// SkipHooks обязательно: иначе AfterFind расшифрует, BeforeUpdate зашифрует
// заново с новым nonce — бесполезная запись и риск рассинхрона.
//
// Запуск на проде:
//   PATH=/usr/local/go/bin:$PATH go run ./cmd/encrypt_wialon_tokens
package main

import (
	"log"

	"gorm.io/gorm"

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

	raw := database.DB.Session(&gorm.Session{SkipHooks: true})

	var conns []models.WialonConnection
	if err := raw.Unscoped().Find(&conns).Error; err != nil {
		log.Fatalf("find: %v", err)
	}

	encrypted, skipped, errored := 0, 0, 0
	for _, c := range conns {
		if c.Token == "" {
			skipped++
			continue
		}
		if utils.IsEncrypted(c.Token) {
			skipped++
			continue
		}
		enc, err := utils.EncryptString(c.Token)
		if err != nil {
			log.Printf("❌ id=%d (%s): encrypt: %v", c.ID, c.Name, err)
			errored++
			continue
		}
		if err := raw.Unscoped().
			Model(&models.WialonConnection{}).
			Where("id = ?", c.ID).
			Update("token", enc).Error; err != nil {
			log.Printf("❌ id=%d (%s): update: %v", c.ID, c.Name, err)
			errored++
			continue
		}
		log.Printf("✅ id=%d (%s, type=%s): зашифрован", c.ID, c.Name, c.ConnectionType)
		encrypted++
	}

	log.Printf("📊 Итого: всего=%d, зашифровано=%d, пропущено=%d, ошибки=%d",
		len(conns), encrypted, skipped, errored)
}
