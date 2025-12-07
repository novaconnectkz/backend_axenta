package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
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
	ID int `json:"id"`
}

type AxentaResponse struct {
	Count   int            `json:"count"`
	Next    *string        `json:"next"`
	Results []AxentaObject `json:"results"`
}

func main() {
	log.Println("🔍 Проверка: почему 1770 объектов не сохраняются в БД...")

	// Загружаем конфигурацию
	_, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("❌ Не удалось загрузить конфигурацию: %v", err)
	}

	// Подключаемся к базе данных
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("❌ Не удалось подключиться к базе данных: %v", err)
	}
	defer func() {
		sqlDB, _ := database.DB.DB()
		sqlDB.Close()
	}()

	// Получаем первую активную компанию
	var firstCompany models.Company
	if err := database.DB.Table("public.companies").Where("is_active = ?", true).Order("id ASC").First(&firstCompany).Error; err != nil {
		log.Fatalf("❌ Не найдено активных компаний: %v", err)
	}

	fmt.Printf("🏢 Компания: %s (ID=%d, схема: %s)\n\n", firstCompany.Name, firstCompany.ID, firstCompany.DatabaseSchema)

	// Получаем tenant DB
	tenantDB := database.GetTenantDBByID(firstCompany.ID)
	if tenantDB == nil {
		log.Fatalf("❌ Не удалось получить tenant DB для компании %d", firstCompany.ID)
	}

	// Загружаем объекты из API
	fmt.Println("📥 Загружаем объекты из API...")
	client := &http.Client{Timeout: 180 * time.Second}
	allObjectIDs := make([]int, 0)
	objectIDSet := make(map[int]bool)

	page := 1
	perPage := 100
	nextURL := fmt.Sprintf("%s/api/cms/objects/?page=%d&per_page=%d", axentaAPIBase, page, perPage)

	for nextURL != "" {
		req, err := http.NewRequest("GET", nextURL, nil)
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
			if resp.StatusCode == 500 && page > 30 {
				fmt.Printf("   ⚠️ API вернул 500 на странице %d, прекращаем загрузку\n", page)
				break
			}
			log.Fatalf("❌ API вернул статус %d: %s", resp.StatusCode, string(body))
		}

		var apiResponse AxentaResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
			resp.Body.Close()
			log.Fatalf("❌ Ошибка парсинга JSON: %v", err)
		}
		resp.Body.Close()

		for _, obj := range apiResponse.Results {
			allObjectIDs = append(allObjectIDs, obj.ID)
			if objectIDSet[obj.ID] {
				// Это дубликат!
				if len(allObjectIDs) <= 100 || (len(allObjectIDs)-len(objectIDSet)) <= 10 {
					fmt.Printf("   ⚠️ Дубликат найден: Object ID %d уже встречался ранее (страница %d)\n", obj.ID, page)
				}
			}
			objectIDSet[obj.ID] = true
		}

		if apiResponse.Next == nil || *apiResponse.Next == "" {
			break
		}

		nextURL = *apiResponse.Next
		page++
		time.Sleep(200 * time.Millisecond)
	}

	fmt.Printf("✅ Загружено из API: %d объектов\n", len(allObjectIDs))
	fmt.Printf("✅ Уникальных external_object_id в API: %d\n\n", len(objectIDSet))

	// Проверяем, какие объекты есть в БД
	fmt.Println("🔍 Проверяем объекты в БД...")
	var dbObjectIDs []int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Pluck("external_object_id", &dbObjectIDs)

	dbObjectIDSet := make(map[int64]bool)
	for _, id := range dbObjectIDs {
		dbObjectIDSet[id] = true
	}

	fmt.Printf("✅ Объектов в БД: %d\n\n", len(dbObjectIDs))

	// Находим объекты, которые есть в API, но отсутствуют в БД
	missingInDB := make([]int, 0)
	for _, apiID := range allObjectIDs {
		if !dbObjectIDSet[int64(apiID)] {
			missingInDB = append(missingInDB, apiID)
		}
	}

	fmt.Printf("❌ Объектов в API, но отсутствующих в БД: %d\n", len(missingInDB))

	if len(missingInDB) > 0 {
		fmt.Printf("\n📋 Первые 20 отсутствующих объектов:\n")
		for i, id := range missingInDB {
			if i >= 20 {
				break
			}
			fmt.Printf("   %d. Object ID: %d\n", i+1, id)
		}

		// Проверяем, может быть эти объекты были удалены
		var deletedCount int64
		tenantDB.Model(&models.AxentaObjectSnapshot{}).
			Where("external_object_id IN ? AND deleted_at IS NOT NULL", missingInDB[:min(100, len(missingInDB))]).
			Count(&deletedCount)
		fmt.Printf("\n💡 Проверка: удаленных объектов среди отсутствующих: %d\n", deletedCount)
	}

	// Проверяем объекты, которые есть в БД, но отсутствуют в API
	missingInAPI := make([]int64, 0)
	for dbID := range dbObjectIDSet {
		if !objectIDSet[int(dbID)] {
			missingInAPI = append(missingInAPI, dbID)
		}
	}

	fmt.Printf("\n📊 Объектов в БД, но отсутствующих в API: %d\n", len(missingInAPI))

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("✅ Анализ завершен")
	fmt.Println(strings.Repeat("=", 60))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
