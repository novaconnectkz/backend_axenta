package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type UserToken struct {
	ID          uint
	Token       string
	AccountID   uint
	IsActive    bool
	ExpiresAt   time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func main() {
	// Подключение к БД
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "host=localhost user=postgres password=postgres dbname=axenta_db port=5432 sslmode=disable"
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("Не удалось подключиться к БД:", err)
	}

	fmt.Println("🔑 ПОИСК ДЕЙСТВУЮЩИХ ТОКЕНОВ")
	fmt.Println("==========================================")
	fmt.Println()

	// Получаем компании
	var companies []struct {
		ID             uint
		DatabaseSchema string
	}
	
	db.Exec("SET search_path TO public")
	db.Table("companies").Select("id, database_schema").Find(&companies)

	for _, company := range companies {
		tenantSchema := company.DatabaseSchema
		db.Exec("SET search_path TO " + tenantSchema)

		fmt.Printf("🏢 Компания: %s (ID=%d)\n", tenantSchema, company.ID)

		var tokens []UserToken
		err := db.Table("user_tokens").
			Where("is_active = ? AND expires_at > ?", true, time.Now()).
			Order("expires_at DESC").
			Find(&tokens).Error

		if err != nil {
			fmt.Printf("   ⚠️ Ошибка: %v\n\n", err)
			continue
		}

		if len(tokens) == 0 {
			fmt.Printf("   ℹ️ Нет активных токенов\n\n")
			continue
		}

		fmt.Printf("   📋 Найдено активных токенов: %d\n", len(tokens))
		for i, token := range tokens {
			expiresIn := time.Until(token.ExpiresAt)
			fmt.Printf("\n   %d. AccountID=%d\n", i+1, token.AccountID)
			tokenPreview := token.Token
			if len(tokenPreview) > 40 {
				tokenPreview = tokenPreview[:40] + "..."
			}
			fmt.Printf("      Токен: %s\n", tokenPreview)
			fmt.Printf("      Истекает через: %v\n", expiresIn.Round(time.Hour))
			fmt.Printf("      Создан: %s\n", token.CreatedAt.Format("2006-01-02 15:04:05"))
			
			if i == 0 && expiresIn > 24*time.Hour {
				fmt.Printf("\n   💡 Рекомендуемый токен для AXENTA_ADMIN_TOKEN:\n")
				fmt.Printf("      export AXENTA_ADMIN_TOKEN='%s'\n", token.Token)
			}
		}
		fmt.Println()
	}

	fmt.Println("==========================================")
	fmt.Println("\n📝 Для использования токена запустите:")
	fmt.Println("   export AXENTA_ADMIN_TOKEN='<ваш_токен>'")
	fmt.Println("   ./backend_axenta")
}

