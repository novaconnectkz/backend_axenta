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
	apiToken      = "5e515a8f2874fc78f31c74af45260333f2c84c35"
)

type AxentaObject struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	// Другие поля не нужны для проверки дубликатов
}

type AxentaResponse struct {
	Count   int            `json:"count"`
	Next    *string        `json:"next"`
	Results []AxentaObject `json:"results"`
}

func main() {
	log.Println("🔍 Проверка дубликатов в ответе Axenta API...")

	client := &http.Client{
		Timeout: 180 * time.Second,
	}

	allObjects := make([]AxentaObject, 0)
	objectIDMap := make(map[int]int) // external_object_id -> количество раз

	page := 1
	perPage := 100

	log.Printf("📥 Загружаем объекты из API (размер страницы: %d)...\n", perPage)

	for {
		url := fmt.Sprintf("%s/api/cms/objects/?page=%d&per_page=%d", axentaAPIBase, page, perPage)
		log.Printf("   📄 Страница %d: запрашиваем объекты...", page)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			log.Fatalf("❌ Ошибка создания запроса: %v", err)
		}

		req.Header.Set("Authorization", "Token "+apiToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			log.Fatalf("❌ Ошибка запроса: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			log.Fatalf("❌ API вернул статус %d: %s", resp.StatusCode, string(body))
		}

		var apiResponse AxentaResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
			resp.Body.Close()
			log.Fatalf("❌ Ошибка парсинга JSON: %v", err)
		}
		resp.Body.Close()

		if page == 1 {
			log.Printf("   ✅ Первая страница получена!")
			log.Printf("   📊 Всего объектов в API: %d\n", apiResponse.Count)
		}

		allObjects = append(allObjects, apiResponse.Results...)

		// Подсчитываем дубликаты
		for _, obj := range apiResponse.Results {
			objectIDMap[obj.ID]++
		}

		log.Printf("   ✅ Страница %d: получено %d объектов (всего загружено: %d)",
			page, len(apiResponse.Results), len(allObjects))

		if apiResponse.Next == nil || *apiResponse.Next == "" {
			log.Printf("   ✅ Достигнут конец списка")
			break
		}

		page++
		time.Sleep(200 * time.Millisecond)
	}

	log.Printf("\n✅ Загрузка завершена: всего объектов в ответе API: %d\n", len(allObjects))

	// Анализируем дубликаты
	fmt.Println("\n🔍 Анализ дубликатов:")
	duplicates := make(map[int]int)
	for objectID, count := range objectIDMap {
		if count > 1 {
			duplicates[objectID] = count
		}
	}

	fmt.Printf("   - Уникальных external_object_id: %d\n", len(objectIDMap))
	fmt.Printf("   - Всего объектов в ответе: %d\n", len(allObjects))
	fmt.Printf("   - Дубликатов по external_object_id: %d\n", len(duplicates))

	if len(duplicates) > 0 {
		totalDuplicates := 0
		for _, count := range duplicates {
			totalDuplicates += count - 1 // -1 потому что один объект нормальный
		}
		fmt.Printf("   - Всего дублирующихся записей: %d\n", totalDuplicates)

		fmt.Printf("\n   📋 Примеры дубликатов (первые 10):\n")
		i := 0
		for objectID, count := range duplicates {
			if i >= 10 {
				break
			}
			fmt.Printf("      %d. Object ID %d встречается %d раз\n", i+1, objectID, count)
			i++
		}

		// Проверяем, объясняет ли это разницу
		expectedUnique := len(allObjects) - totalDuplicates
		fmt.Printf("\n   💡 Ожидаемое количество уникальных объектов: %d\n", expectedUnique)
		fmt.Printf("   💡 Фактическое количество уникальных: %d\n", len(objectIDMap))

		if expectedUnique == len(objectIDMap) {
			fmt.Printf("   ✅ Разница объясняется дубликатами в API!\n")
			fmt.Printf("   📊 Итого:\n")
			fmt.Printf("      - Объектов в ответе API: %d\n", len(allObjects))
			fmt.Printf("      - Уникальных объектов: %d\n", len(objectIDMap))
			fmt.Printf("      - Дубликатов: %d\n", totalDuplicates)
		}
	} else {
		fmt.Printf("   ✅ Дубликатов в API нет!\n")
		fmt.Printf("   ❌ Проблема в другом месте - все %d объектов должны были сохраниться\n", len(allObjects))
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("✅ Проверка завершена")
	fmt.Println(strings.Repeat("=", 60))
}
