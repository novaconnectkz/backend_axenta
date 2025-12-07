package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	axentaAPIBase = "https://axenta.cloud"
	apiToken      = "5e515a8f2874fc78f31c74af45260333f2c84c35" // Токен из .cursorrules
)

func main() {
	log.Println("🔍 Проверка подключения к Axenta API...")
	log.Printf("📍 Базовый URL: %s\n", axentaAPIBase)
	log.Printf("🔑 Токен: %s...%s (длина: %d)\n\n", apiToken[:10], apiToken[len(apiToken)-10:], len(apiToken))

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Тест 1: Проверка подключения к API (получение аккаунтов)
	fmt.Println("📡 Тест 1: Получение списка аккаунтов...")
	accountsURL := fmt.Sprintf("%s/api/cms/accounts/?page=1&per_page=5", axentaAPIBase)
	if err := testAPIRequest(client, accountsURL, apiToken, "аккаунты"); err != nil {
		log.Printf("❌ Ошибка: %v\n", err)
	} else {
		fmt.Println("✅ Успешно получены аккаунты\n")
	}

	// Тест 2: Проверка получения объектов
	fmt.Println("📡 Тест 2: Получение списка объектов...")
	objectsURL := fmt.Sprintf("%s/api/cms/objects/?page=1&per_page=5", axentaAPIBase)
	if err := testAPIRequest(client, objectsURL, apiToken, "объекты"); err != nil {
		log.Printf("❌ Ошибка: %v\n", err)
	} else {
		fmt.Println("✅ Успешно получены объекты\n")
	}

	// Тест 3: Получение статистики (общее количество)
	fmt.Println("📡 Тест 3: Получение статистики объектов...")
	statsURL := fmt.Sprintf("%s/api/cms/objects/?page=1&per_page=1", axentaAPIBase)
	if err := testAPIRequestWithStats(client, statsURL, apiToken); err != nil {
		log.Printf("❌ Ошибка: %v\n", err)
	} else {
		fmt.Println("✅ Статистика получена\n")
	}

	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("✅ Проверка подключения завершена")
	fmt.Println(strings.Repeat("=", 60))
}

func testAPIRequest(client *http.Client, url, token, resourceType string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("Authorization", "Token "+token)
	req.Header.Set("Content-Type", "application/json")

	fmt.Printf("   URL: %s\n", url)
	fmt.Printf("   Метод: GET\n")
	fmt.Printf("   Заголовок Authorization: Token %s...%s\n", token[:10], token[len(token)-10:])

	startTime := time.Now()
	resp, err := client.Do(req)
	duration := time.Since(startTime)

	if err != nil {
		return fmt.Errorf("ошибка выполнения запроса: %w", err)
	}
	defer resp.Body.Close()

	fmt.Printf("   Статус ответа: %d %s\n", resp.StatusCode, resp.Status)
	fmt.Printf("   Время ответа: %v\n", duration)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API вернул ошибку %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("ошибка парсинга JSON: %w", err)
	}

	if count, ok := result["count"].(float64); ok {
		fmt.Printf("   Всего %s: %.0f\n", resourceType, count)
	}

	if results, ok := result["results"].([]interface{}); ok {
		fmt.Printf("   Получено в ответе: %d\n", len(results))
	}

	return nil
}

func testAPIRequestWithStats(client *http.Client, url, token string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("Authorization", "Token "+token)
	req.Header.Set("Content-Type", "application/json")

	fmt.Printf("   URL: %s\n", url)

	startTime := time.Now()
	resp, err := client.Do(req)
	duration := time.Since(startTime)

	if err != nil {
		return fmt.Errorf("ошибка выполнения запроса: %w", err)
	}
	defer resp.Body.Close()

	fmt.Printf("   Статус ответа: %d %s\n", resp.StatusCode, resp.Status)
	fmt.Printf("   Время ответа: %v\n", duration)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API вернул ошибку %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("ошибка парсинга JSON: %w", err)
	}

	if count, ok := result["count"].(float64); ok {
		fmt.Printf("   📊 Всего объектов в системе: %.0f\n", count)
	}

	if next, ok := result["next"].(string); ok && next != "" {
		fmt.Printf("   ✅ Есть следующая страница\n")
	} else {
		fmt.Printf("   ℹ️ Нет следующей страницы\n")
	}

	return nil
}
