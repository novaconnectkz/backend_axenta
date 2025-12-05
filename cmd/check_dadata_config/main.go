package main

import (
	"fmt"
	"log"
	"os"

	"backend_axenta/config"

	"github.com/joho/godotenv"
)

func main() {
	// Загружаем .env файл если он существует
	if err := godotenv.Load(); err != nil {
		log.Printf("⚠️  Предупреждение: .env файл не найден или не может быть загружен: %v", err)
	}

	fmt.Println("🔍 Проверка конфигурации Dadata API...")
	fmt.Println("=" + string(make([]byte, 50)) + "=")

	// Проверяем переменную окружения напрямую
	envKey := os.Getenv("DADATA_API_KEY")
	if envKey == "" {
		fmt.Println("❌ DADATA_API_KEY не найден в переменных окружения")
	} else {
		fmt.Printf("✅ DADATA_API_KEY найден в переменных окружения\n")
		fmt.Printf("   Длина токена: %d символов\n", len(envKey))
		fmt.Printf("   Первые 10 символов: %s...\n", envKey[:min(10, len(envKey))])
	}

	fmt.Println()

	// Загружаем конфигурацию через config пакет
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("❌ Ошибка при загрузке конфигурации: %v", err)
	}

	fmt.Println("📋 Конфигурация через config.LoadConfig():")
	if cfg.External.DaDataAPIKey == "" {
		fmt.Println("❌ DaDataAPIKey пустой в конфигурации!")
		fmt.Println("💡 Возможные причины:")
		fmt.Println("   1. Переменная DADATA_API_KEY не установлена в .env файле")
		fmt.Println("   2. .env файл не загружается правильно")
		fmt.Println("   3. Сервер не был перезапущен после изменения .env")
	} else {
		fmt.Printf("✅ DaDataAPIKey найден в конфигурации\n")
		fmt.Printf("   Длина токена: %d символов\n", len(cfg.External.DaDataAPIKey))
		fmt.Printf("   Первые 10 символов: %s...\n", cfg.External.DaDataAPIKey[:min(10, len(cfg.External.DaDataAPIKey))])

		// Сравниваем с переменной окружения
		if envKey != "" && envKey != cfg.External.DaDataAPIKey {
			fmt.Println("⚠️  ВНИМАНИЕ: Токен в переменных окружения отличается от токена в конфигурации!")
		} else if envKey == "" && cfg.External.DaDataAPIKey != "" {
			fmt.Println("ℹ️  Токен загружен из .env файла через godotenv")
		}
	}

	fmt.Println()
	fmt.Println("=" + string(make([]byte, 50)) + "=")
	fmt.Println("💡 Рекомендации:")
	fmt.Println("   1. Убедитесь, что DADATA_API_KEY установлен в .env файле")
	fmt.Println("   2. Перезапустите сервер после изменения .env файла")
	fmt.Println("   3. Проверьте, что токен не истек на сайте dadata.ru")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
