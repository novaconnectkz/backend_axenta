package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// AccountHierarchyService управляет иерархией аккаунтов Axenta Cloud
type AccountHierarchyService struct {
	cache      map[int64]*AxentaAccount
	cacheMutex sync.RWMutex
	token      string
}

// AxentaAccount представляет аккаунт из Axenta Cloud API
type AxentaAccount struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	Type              string `json:"type"` // "partner", "client", "dealer"
	ParentAccountName string `json:"parentAccountName"`
	ParentAccountID   *int64 `json:"parentAccountId,omitempty"`
	Hierarchy         string `json:"hierarchy"`
	IsActive          bool   `json:"isActive"`
	ObjectsActive     int    `json:"objectsActive"`
	ObjectsTotal      int    `json:"objectsTotal"`
}

// NewAccountHierarchyService создаёт новый сервис
func NewAccountHierarchyService(token string) *AccountHierarchyService {
	return &AccountHierarchyService{
		cache: make(map[int64]*AxentaAccount),
		token: token,
	}
}

// LoadAllAccounts загружает все аккаунты из Axenta Cloud API
func (s *AccountHierarchyService) LoadAllAccounts() error {
	client := &http.Client{Timeout: 60 * time.Second}

	page := 1
	perPage := 1000
	totalLoaded := 0

	log.Printf("📥 Начинаем загрузку всех аккаунтов из Axenta Cloud...")

	for {
		url := fmt.Sprintf("https://axenta.cloud/api/cms/accounts/?page=%d&per_page=%d", page, perPage)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return fmt.Errorf("ошибка создания запроса: %w", err)
		}

		req.Header.Set("Authorization", "Token "+s.token)

		_ = waitAxentaToken(context.Background(), s.token) // zone#2: shared per-token rate-limit
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("ошибка запроса к Axenta Cloud: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return fmt.Errorf("Axenta Cloud вернул статус %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			return fmt.Errorf("ошибка чтения ответа: %w", err)
		}

		var response struct {
			Count   int             `json:"count"`
			Results []AxentaAccount `json:"results"`
			Next    *string         `json:"next"`
		}

		if err := json.Unmarshal(body, &response); err != nil {
			return fmt.Errorf("ошибка парсинга JSON: %w", err)
		}

		// Сохраняем в кэш
		s.cacheMutex.Lock()
		for i := range response.Results {
			s.cache[response.Results[i].ID] = &response.Results[i]
		}
		s.cacheMutex.Unlock()

		totalLoaded += len(response.Results)
		log.Printf("📦 Страница %d: загружено %d аккаунтов (всего %d из %d)",
			page, len(response.Results), totalLoaded, response.Count)

		if len(response.Results) < perPage || response.Next == nil {
			break
		}

		page++
	}

	log.Printf("✅ Загружено %d аккаунтов", totalLoaded)

	// Построим обратные связи (parent -> children)
	s.buildParentChildRelations()

	return nil
}

// buildParentChildRelations строит связи родитель-дети через парсинг hierarchy
func (s *AccountHierarchyService) buildParentChildRelations() {
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()

	// Создаём индекс по именам для быстрого поиска
	nameToID := make(map[string]int64)
	for id, acc := range s.cache {
		nameToID[acc.Name] = id
	}

	// Устанавливаем ParentAccountID на основе ParentAccountName
	for _, acc := range s.cache {
		if acc.ParentAccountName != "" && acc.ParentAccountName != "GLOMOS" {
			if parentID, exists := nameToID[acc.ParentAccountName]; exists {
				acc.ParentAccountID = &parentID
			}
		}
	}
}

// FindPartnerInHierarchy ищет партнёра в иерархии аккаунта
// Возвращает ID партнёра или 0 если партнёра нет
func (s *AccountHierarchyService) FindPartnerInHierarchy(accountID int64) int64 {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()

	visited := make(map[int64]bool)
	currentID := accountID

	// Поднимаемся по иерархии вверх до GLOMOS
	for currentID != 0 {
		// Защита от бесконечного цикла
		if visited[currentID] {
			log.Printf("⚠️ Обнаружен цикл в иерархии аккаунта %d", accountID)
			return 0
		}
		visited[currentID] = true

		acc, exists := s.cache[currentID]
		if !exists {
			// Аккаунт не найден в кэше
			return 0
		}

		// Если это партнёр - возвращаем его ID
		if acc.Type == "partner" || acc.Type == "Partner" {
			return acc.ID
		}

		// Если дошли до GLOMOS - партнёра нет
		if acc.Name == "GLOMOS" || acc.ParentAccountName == "" {
			return 0
		}

		// Переходим к родителю
		if acc.ParentAccountID != nil {
			currentID = *acc.ParentAccountID
		} else {
			// Нет родителя - это прямой клиент GLOMOS
			return 0
		}
	}

	return 0
}

// GetAccount возвращает аккаунт по ID
func (s *AccountHierarchyService) GetAccount(accountID int64) (*AxentaAccount, bool) {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()

	acc, exists := s.cache[accountID]
	return acc, exists
}

// GetAllAccounts возвращает все аккаунты из кэша
func (s *AccountHierarchyService) GetAllAccounts() []AxentaAccount {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()

	accounts := make([]AxentaAccount, 0, len(s.cache))
	for _, acc := range s.cache {
		accounts = append(accounts, *acc)
	}

	return accounts
}

// GetDirectGLOMOSClients возвращает список аккаунтов напрямую под GLOMOS (не через партнёров)
func (s *AccountHierarchyService) GetDirectGLOMOSClients() []AxentaAccount {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()

	var direct []AxentaAccount

	for _, acc := range s.cache {
		// Проверяем что это не партнёр и нет партнёра в иерархии
		if s.FindPartnerInHierarchy(acc.ID) == 0 && acc.Name != "GLOMOS" {
			direct = append(direct, *acc)
		}
	}

	return direct
}

// GetPartnerHierarchy возвращает полную иерархию для аккаунта
func (s *AccountHierarchyService) GetPartnerHierarchy(accountID int64) []string {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()

	var hierarchy []string
	visited := make(map[int64]bool)
	currentID := accountID

	for currentID != 0 {
		if visited[currentID] {
			break
		}
		visited[currentID] = true

		acc, exists := s.cache[currentID]
		if !exists {
			break
		}

		hierarchy = append([]string{fmt.Sprintf("%s (%d, %s)", acc.Name, acc.ID, acc.Type)}, hierarchy...)

		if acc.Name == "GLOMOS" || acc.ParentAccountID == nil {
			break
		}

		currentID = *acc.ParentAccountID
	}

	return hierarchy
}

// DistributeObjectsByPartner загружает ВСЕ объекты и распределяет их по партнёрам
// Возвращает map[partnerID]objectCount без дублей
func (s *AccountHierarchyService) DistributeObjectsByPartner(token string, snapshotDate time.Time) (map[int64]*PartnerObjectStats, error) {
	client := &http.Client{Timeout: 300 * time.Second}

	log.Printf("📥 Начинаем загрузку ВСЕХ объектов для точного распределения...")

	// Загружаем все объекты
	var allObjects []struct {
		ID        int64  `json:"id"`
		AccountID int64  `json:"accountId"`
		IsActive  bool   `json:"isActive"`
		CreatedAt string `json:"createdAt"`
		DeletedAt string `json:"deletedAt"`
	}

	page := 1
	perPage := 1000
	totalLoaded := 0

	for {
		url := fmt.Sprintf("https://axenta.cloud/api/cms/objects/?page=%d&per_page=%d", page, perPage)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("ошибка создания запроса: %w", err)
		}

		req.Header.Set("Authorization", "Token "+token)

		_ = waitAxentaToken(context.Background(), token) // zone#2: shared per-token rate-limit
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("ошибка запроса: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("API вернул статус %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
		}

		var response struct {
			Count   int `json:"count"`
			Results []struct {
				ID        int64  `json:"id"`
				AccountID int64  `json:"accountId"`
				IsActive  bool   `json:"isActive"`
				CreatedAt string `json:"createdAt"`
				DeletedAt string `json:"deletedAt"`
			} `json:"results"`
			Next *string `json:"next"`
		}

		if err := json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("ошибка парсинга JSON: %w", err)
		}

		allObjects = append(allObjects, response.Results...)
		totalLoaded += len(response.Results)

		if page%5 == 0 {
			log.Printf("📦 Загружено %d объектов из %d...", totalLoaded, response.Count)
		}

		if len(response.Results) < perPage || response.Next == nil {
			break
		}

		page++
		time.Sleep(100 * time.Millisecond) // Небольшая пауза между запросами
	}

	log.Printf("✅ Загружено %d объектов. Начинаем распределение по партнёрам...", totalLoaded)

	if totalLoaded == 0 {
		log.Printf("⚠️ ВНИМАНИЕ: Загружено 0 объектов! Возможно, проблема с токеном или API.")
		return make(map[int64]*PartnerObjectStats), nil // Возвращаем пустую map, но не ошибку
	}

	// Распределяем объекты по партнёрам
	partnerStats := make(map[int64]*PartnerObjectStats)
	snapshotEndOfDay := time.Date(snapshotDate.Year(), snapshotDate.Month(), snapshotDate.Day(), 23, 59, 59, 0, time.UTC)

	for i, obj := range allObjects {
		if i > 0 && i%1000 == 0 {
			log.Printf("📊 Обработано %d из %d объектов...", i, len(allObjects))
		}

		// Проверяем дату создания
		var createdAt time.Time
		if obj.CreatedAt != "" {
			formats := []string{
				time.RFC3339,
				"2006-01-02T15:04:05.000Z",
				"2006-01-02T15:04:05Z",
				"2006-01-02 15:04:05",
			}
			for _, format := range formats {
				if t, err := time.Parse(format, obj.CreatedAt); err == nil {
					createdAt = t
					break
				}
			}
		}

		// Пропускаем объекты созданные после даты снимка
		if !createdAt.IsZero() && createdAt.After(snapshotEndOfDay) {
			continue
		}

		// Проверяем дату удаления
		var deletedAt time.Time
		if obj.DeletedAt != "" {
			formats := []string{
				time.RFC3339,
				"2006-01-02T15:04:05.000Z",
				"2006-01-02T15:04:05Z",
			}
			for _, format := range formats {
				if t, err := time.Parse(format, obj.DeletedAt); err == nil {
					deletedAt = t
					break
				}
			}
		}

		// Пропускаем удалённые до или в день снимка (до конца дня снимка)
		if !deletedAt.IsZero() && (deletedAt.Before(snapshotEndOfDay) || deletedAt.Equal(snapshotEndOfDay)) {
			continue
		}

		// Находим партнёра для аккаунта объекта
		partnerID := s.FindPartnerInHierarchy(obj.AccountID)

		// Если партнёра нет - это "Объекты наших клиентов", используем ID=186 (GLOMOS)
		if partnerID == 0 {
			partnerID = 186 // GLOMOS = наши клиенты
		}

		// Инициализируем статистику если нужно
		if partnerStats[partnerID] == nil {
			partnerStats[partnerID] = &PartnerObjectStats{
				PartnerID: partnerID,
			}
		}

		// Увеличиваем счётчики
		partnerStats[partnerID].TotalObjects++
		if obj.IsActive {
			partnerStats[partnerID].ActiveObjects++
		}
	}

	log.Printf("✅ Распределение завершено. Найдено %d партнёров/групп", len(partnerStats))

	return partnerStats, nil
}

// PartnerObjectStats хранит статистику объектов партнёра
type PartnerObjectStats struct {
	PartnerID     int64
	TotalObjects  int
	ActiveObjects int
}
