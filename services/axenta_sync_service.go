package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const axentaAPIBase = "https://axenta.cloud"

// AxentaSyncService отвечает за синхронизацию данных Axenta с локальной БД
type AxentaSyncService struct {
	db         *gorm.DB
	httpClient *http.Client
}

// NewAxentaSyncService создает новый сервис синхронизации Axenta
func NewAxentaSyncService(db *gorm.DB) *AxentaSyncService {
	return &AxentaSyncService{
		db: db,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SyncAllAdmins выполняет синхронизацию для всех компаний
// Согласно требованиям: запрос происходит от имени пользователя CRM, запрашиваются все доступные объекты по иерархии
// ВАЖНО: Используем токен 186 (GLOMOS), который имеет доступ ко всей иерархии для получения всех данных
func (s *AxentaSyncService) SyncAllAdmins() {
	// Получаем все компании из основной БД
	var companies []models.Company
	if err := s.db.Table("public.companies").Where("is_active = ?", true).Find(&companies).Error; err != nil {
		log.Printf("AxentaSync: не удалось загрузить компании: %v", err)
		return
	}

	log.Printf("AxentaSync: найдено %d активных компаний для синхронизации", len(companies))

	// Получаем токен из настроек снимков (хранится в схеме первой компании с минимальным ID)
	// Ищем первую компанию из списка
	if len(companies) == 0 {
		log.Printf("AxentaSync: нет активных компаний для синхронизации")
		return
	}

	firstCompany := companies[0]
	firstCompanyTenantDB := database.GetTenantDBByID(firstCompany.ID)
	if firstCompanyTenantDB == nil {
		log.Printf("AxentaSync: не удалось получить tenant DB для первой компании (ID=%d)", firstCompany.ID)
		return
	}

	// Получаем токен из настроек снимков (с company_id = 1 - глобальные настройки)
	var snapshotSettings models.SnapshotSettings
	var syncToken string
	var adminAccountID uint = firstCompany.ID

	if err := firstCompanyTenantDB.
		Where("company_id = ? AND is_active = ?", 1, true).
		First(&snapshotSettings).Error; err != nil {
		log.Printf("AxentaSync: не найдено настроек снимков в схеме %s (company_id=1): %v", firstCompany.DatabaseSchema, err)
		log.Printf("AxentaSync: попытка найти токен в user_tokens...")

		// Fallback: пытаемся найти токен в user_tokens первой компании
		var userToken models.UserToken
		if err := firstCompanyTenantDB.
			Where("is_active = ? AND expires_at > ?", true, time.Now()).
			Order("updated_at DESC").
			First(&userToken).Error; err != nil {
			log.Printf("AxentaSync: не найдено активных токенов в схеме %s: %v", firstCompany.DatabaseSchema, err)
			return
		}

		syncToken = userToken.Token
		adminAccountID = userToken.AccountID
		if adminAccountID == 0 {
			adminAccountID = firstCompany.ID
		}
		log.Printf("AxentaSync: используем токен из user_tokens (account_id=%d) для синхронизации всех компаний", adminAccountID)

		// Сохраняем токен в настройки для будущего использования (с company_id = 1 - глобальные настройки)
		snapshotSettings = models.SnapshotSettings{
			CompanyID:   1, // Глобальные настройки с company_id = 1
			AxentaToken: syncToken,
			IsActive:    true,
		}
		if err := firstCompanyTenantDB.Save(&snapshotSettings).Error; err != nil {
			log.Printf("AxentaSync: предупреждение - не удалось сохранить токен в настройки: %v", err)
		}
	} else {
		// Используем токен из настроек
		syncToken = snapshotSettings.AxentaToken
		if syncToken == "" {
			log.Printf("AxentaSync: токен в настройках пустой, пропускаем синхронизацию")
			return
		}
		log.Printf("AxentaSync: используем токен из настроек снимков (схема: %s, company_id=1) для синхронизации всех компаний", firstCompany.DatabaseSchema)
	}

	// Для каждой компании синхронизируем данные в её tenant схему используя найденный токен
	for _, company := range companies {
		// Получаем tenant DB для компании
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			log.Printf("AxentaSync: не удалось получить tenant DB для компании %d (%s)", company.ID, company.DatabaseSchema)
			continue
		}

		log.Printf("AxentaSync: синхронизация для компании %d (%s), используя токен с admin_account_id=%d",
			company.ID, company.DatabaseSchema, adminAccountID)

		// Синхронизируем используя tenant DB и найденный токен (который имеет доступ ко всей иерархии)
		if err := s.syncAdminWithTokenAndDB(adminAccountID, syncToken, tenantDB); err != nil {
			log.Printf("AxentaSync: ошибка синхронизации для компании %d: %v", company.ID, err)
		} else {
			log.Printf("AxentaSync: успешно синхронизировано для компании %d", company.ID)
		}
	}
}

// SyncAdmin синхронизирует данные для одного администратора
func (s *AxentaSyncService) SyncAdmin(adminAccountID uint) error {
	token, err := s.getActiveTokenForAdmin(adminAccountID)
	if err != nil {
		return fmt.Errorf("не удалось получить активный токен: %w", err)
	}

	if token == "" {
		return fmt.Errorf("активный токен отсутствует")
	}

	return s.syncAdminWithToken(adminAccountID, token)
}

func (s *AxentaSyncService) syncAdminWithToken(adminAccountID uint, token string) error {
	// Используем основную БД (для обратной совместимости)
	return s.syncAdminWithTokenAndDB(adminAccountID, token, s.db)
}

// ProgressCallback функция для обновления прогресса загрузки
type ProgressCallback func(loaded int, total int, currentPage int, totalPages int, status string)

// syncAdminWithTokenAndDB синхронизирует данные используя указанную БД (tenant или основную)
func (s *AxentaSyncService) syncAdminWithTokenAndDB(adminAccountID uint, token string, db *gorm.DB) error {
	return s.syncAdminWithTokenAndDBAndProgress(adminAccountID, token, db, nil)
}

// syncAdminWithTokenAndDBAndProgress синхронизирует данные с отслеживанием прогресса
func (s *AxentaSyncService) syncAdminWithTokenAndDBAndProgress(adminAccountID uint, token string, db *gorm.DB, progressCallback ProgressCallback) error {
	log.Printf("🔄 Начинаем полную синхронизацию данных из Axenta Cloud...")
	log.Printf("   📊 Admin Account ID: %d", adminAccountID)
	log.Printf("   🕐 Время начала: %s", time.Now().Format("2006-01-02 15:04:05"))
	log.Println(strings.Repeat("-", 60))

	now := time.Now().UTC()
	overallStartTime := time.Now()

	// Шаг 1: Загрузка и сохранение аккаунтов
	log.Printf("📋 Шаг 1: Загрузка и сохранение аккаунтов...")
	accountsStartTime := time.Now()

	accounts, err := s.fetchAccounts(token)
	if err != nil {
		return fmt.Errorf("ошибка получения учетных записей: %w", err)
	}

	if err := s.storeAccountsWithDB(adminAccountID, accounts, now, db); err != nil {
		return fmt.Errorf("ошибка сохранения учетных записей: %w", err)
	}

	accountsDuration := time.Since(accountsStartTime)
	log.Printf("✅ Шаг 1 завершен: сохранено %d аккаунтов за %v", len(accounts), accountsDuration.Round(time.Second))
	log.Println(strings.Repeat("-", 60))

	// Шаг 2: Загрузка и сохранение объектов
	log.Printf("📦 Шаг 2: Загрузка и сохранение объектов...")
	objectsStartTime := time.Now()

	// Запрашиваем ВСЕ объекты по иерархии (без фильтра по accountId)
	// Это соответствует требованиям: запрашиваются все доступные объекты по иерархии
	if err := s.syncAllObjectsWithDBAndProgress(adminAccountID, token, now, db, progressCallback); err != nil {
		return fmt.Errorf("ошибка синхронизации объектов: %w", err)
	}

	objectsDuration := time.Since(objectsStartTime)
	log.Printf("✅ Шаг 2 завершен за %v", objectsDuration.Round(time.Second))
	log.Println(strings.Repeat("-", 60))

	// Шаг 3: Очистка устаревших записей
	log.Printf("🧹 Шаг 3: Очистка устаревших записей...")
	cleanupStartTime := time.Now()

	// Удаляем устаревшие записи (которые не обновлялись в этом цикле)
	cutoff := now.Add(-30 * time.Second)

	var deletedAccounts int64
	if err := db.Where("admin_account_id = ? AND last_synced_at < ?", adminAccountID, cutoff).
		Delete(&models.AxentaAccountSnapshot{}).Error; err != nil {
		log.Printf("   ⚠️ Ошибка очистки устаревших аккаунтов admin=%d: %v", adminAccountID, err)
	} else {
		// Получаем количество удаленных записей
		db.Model(&models.AxentaAccountSnapshot{}).
			Where("admin_account_id = ? AND last_synced_at < ?", adminAccountID, cutoff).
			Count(&deletedAccounts)
		log.Printf("   ✅ Удалено устаревших аккаунтов: %d", deletedAccounts)
	}

	var deletedObjects int64
	if err := db.Where("admin_account_id = ? AND last_synced_at < ?", adminAccountID, cutoff).
		Delete(&models.AxentaObjectSnapshot{}).Error; err != nil {
		log.Printf("   ⚠️ Ошибка очистки устаревших объектов admin=%d: %v", adminAccountID, err)
	} else {
		// Получаем количество удаленных записей
		db.Model(&models.AxentaObjectSnapshot{}).
			Where("admin_account_id = ? AND last_synced_at < ?", adminAccountID, cutoff).
			Count(&deletedObjects)
		log.Printf("   ✅ Удалено устаревших объектов: %d", deletedObjects)
	}

	cleanupDuration := time.Since(cleanupStartTime)
	log.Printf("✅ Шаг 3 завершен за %v", cleanupDuration.Round(time.Second))
	log.Println(strings.Repeat("-", 60))

	// Финальная статистика
	overallDuration := time.Since(overallStartTime)
	log.Printf("🎉 Полная синхронизация завершена успешно!")
	log.Printf("   📊 Итоговая статистика:")
	log.Printf("      - Аккаунтов загружено: %d", len(accounts))
	log.Printf("      - Время загрузки аккаунтов: %v", accountsDuration.Round(time.Second))
	log.Printf("      - Время загрузки объектов: %v", objectsDuration.Round(time.Second))
	log.Printf("      - Время очистки: %v", cleanupDuration.Round(time.Second))
	log.Printf("      - Общее время синхронизации: %v", overallDuration.Round(time.Second))
	log.Println(strings.Repeat("=", 60))

	if err := db.Where("admin_account_id = ? AND last_synced_at < ?", adminAccountID, cutoff).
		Delete(&models.AxentaObjectSnapshot{}).Error; err != nil {
		log.Printf("AxentaSync: ошибка очистки устаревших объектов admin=%d: %v", adminAccountID, err)
	}

	return nil
}

type partnerToken struct {
	AccountID uint
	Token     string
	UpdatedAt time.Time
}

// loadActivePartnerTokens - устаревший метод, оставлен для обратной совместимости
// Фильтрует только партнеров
func (s *AxentaSyncService) loadActivePartnerTokens() (map[uint]partnerToken, error) {
	var tokens []models.UserToken
	if err := s.db.Preload("User").
		Where("is_active = ? AND expires_at > ?", true, time.Now()).
		Find(&tokens).Error; err != nil {
		return nil, err
	}

	result := make(map[uint]partnerToken)
	for _, token := range tokens {
		if token.AccountID == 0 {
			continue
		}

		if token.User != nil && !token.User.IsPartner() {
			continue
		}

		if existing, ok := result[token.AccountID]; ok {
			if token.UpdatedAt.After(existing.UpdatedAt) {
				result[token.AccountID] = partnerToken{AccountID: token.AccountID, Token: token.Token, UpdatedAt: token.UpdatedAt}
			}
			continue
		}

		result[token.AccountID] = partnerToken{AccountID: token.AccountID, Token: token.Token, UpdatedAt: token.UpdatedAt}
	}

	return result, nil
}

// loadAllActiveTokens загружает токены для ВСЕХ компаний (не только партнеров)
// Согласно требованиям: запрос происходит от имени пользователя CRM, синхронизация для всех компаний
func (s *AxentaSyncService) loadAllActiveTokens() (map[uint]partnerToken, error) {
	var tokens []models.UserToken
	if err := s.db.Preload("User").
		Where("is_active = ? AND expires_at > ?", true, time.Now()).
		Find(&tokens).Error; err != nil {
		return nil, err
	}

	result := make(map[uint]partnerToken)
	for _, token := range tokens {
		if token.AccountID == 0 {
			continue
		}

		// Берем токены для всех пользователей CRM, не только партнеров
		// API Axenta Cloud автоматически вернет только те объекты, которые доступны по иерархии для данного токена
		if existing, ok := result[token.AccountID]; ok {
			// Если уже есть токен для этого account_id, берем более свежий
			if token.UpdatedAt.After(existing.UpdatedAt) {
				result[token.AccountID] = partnerToken{AccountID: token.AccountID, Token: token.Token, UpdatedAt: token.UpdatedAt}
			}
			continue
		}

		result[token.AccountID] = partnerToken{AccountID: token.AccountID, Token: token.Token, UpdatedAt: token.UpdatedAt}
	}

	return result, nil
}

func (s *AxentaSyncService) getActiveTokenForAdmin(adminAccountID uint) (string, error) {
	var token models.UserToken
	err := s.db.Where("account_id = ? AND is_active = ? AND expires_at > ?", adminAccountID, true, time.Now()).
		Order("updated_at DESC").
		First(&token).Error
	if err != nil {
		return "", err
	}
	return token.Token, nil
}

type axentaAccountsResponse struct {
	Count    int             `json:"count"`
	Next     *string         `json:"next"`
	Previous *string         `json:"previous"`
	Results  []axentaAccount `json:"results"`
}

type axentaAccount struct {
	ID                 int     `json:"id"`
	Name               string  `json:"name"`
	Type               string  `json:"type"`
	AdminFullname      string  `json:"adminFullname"`
	AdminID            *int    `json:"adminId"`
	AdminIsActive      bool    `json:"adminIsActive"`
	ParentAccountName  string  `json:"parentAccountName"`
	ObjectsActive      int     `json:"objectsActive"`
	ObjectsTotal       int     `json:"objectsTotal"`
	ObjectsDeleted     int     `json:"objectsDeleted"`
	Comment            *string `json:"comment"`
	IsActive           bool    `json:"isActive"`
	BlockingDatetime   *string `json:"blockingDatetime"`
	Hierarchy          string  `json:"hierarchy"`
	DaysBeforeBlocking *int    `json:"daysBeforeBlocking"`
	CreationDatetime   string  `json:"creationDatetime"`
}

func (s *AxentaSyncService) fetchAccounts(token string) ([]axentaAccount, error) {
	result := make([]axentaAccount, 0)
	// Уменьшаем размер страницы до 100 для снижения нагрузки на API
	nextURL := axentaAPIBase + "/api/cms/accounts/?per_page=100"

	log.Printf("📥 Начинаем загрузку аккаунтов из Axenta Cloud API...")
	log.Printf("   📊 Размер страницы: 100 аккаунтов")
	log.Printf("   🔗 URL первого запроса: %s", nextURL)

	page := 1
	totalCount := 0
	startTime := time.Now()

	for nextURL != "" {
		log.Printf("   📄 Страница %d: запрашиваем аккаунты...", page)

		var response axentaAccountsResponse
		requestStart := time.Now()

		// Пытаемся выполнить запрос с повторными попытками при ошибках
		var err error
		maxRetries := 3
		for attempt := 1; attempt <= maxRetries; attempt++ {
			err = s.getJSON(nextURL, token, &response)
			if err == nil {
				break
			}

			if attempt < maxRetries {
				waitTime := time.Duration(attempt) * time.Second
				log.Printf("   ⚠️ Ошибка при запросе страницы %d (попытка %d/%d): %v", page, attempt, maxRetries, err)
				log.Printf("   ⏳ Ждем %v перед повторной попыткой...", waitTime)
				time.Sleep(waitTime)
			}
		}

		if err != nil {
			return nil, fmt.Errorf("ошибка получения аккаунтов (страница %d после %d попыток): %w", page, maxRetries, err)
		}

		requestDuration := time.Since(requestStart)

		// Сохраняем общее количество аккаунтов из первого ответа
		if page == 1 {
			totalCount = response.Count
			log.Printf("   ✅ Первая страница получена успешно!")
			log.Printf("   📊 Всего аккаунтов в системе: %d", totalCount)
		}

		accountsOnPage := len(response.Results)
		result = append(result, response.Results...)

		log.Printf("   ✅ Страница %d: получено %d аккаунтов (время запроса: %v)", page, accountsOnPage, requestDuration)
		log.Printf("   📈 Прогресс: %d из %d аккаунтов загружено (%.1f%%)",
			len(result), totalCount, float64(len(result))/float64(totalCount)*100)

		if response.Next == nil || *response.Next == "" {
			log.Printf("   ✅ Достигнут конец списка аккаунтов")
			break
		}

		nextURL = s.resolveURL(*response.Next)
		page++

		// Пауза между запросами для снижения нагрузки на API
		time.Sleep(200 * time.Millisecond)
	}

	totalDuration := time.Since(startTime)
	log.Printf("✅ Загрузка аккаунтов завершена успешно!")
	log.Printf("   📊 Итого:")
	log.Printf("      - Загружено страниц: %d", page)
	log.Printf("      - Загружено аккаунтов: %d", len(result))
	log.Printf("      - Общее время: %v", totalDuration.Round(time.Second))

	return result, nil
}

type axentaObjectsResponse struct {
	Count    int            `json:"count"`
	Next     *string        `json:"next"`
	Previous *string        `json:"previous"`
	Results  []axentaObject `json:"results"`
}

type axentaObject struct {
	ID                  int    `json:"id"`
	Name                string `json:"name"`
	UniqueID            string `json:"uniqueId"`
	AccountID           int    `json:"accountId"`
	AccountName         string `json:"accountName"`
	AccountType         string `json:"accountType"`
	DeviceTypeName      string `json:"deviceTypeName"`
	LastMessageDatetime string `json:"lastMessageDatetime"`
	IsActive            bool   `json:"isActive"`
	Status              string `json:"status"`
	// Новые поля из Axenta Cloud API
	CreatorName     string   `json:"creatorName"`
	CreatorID       int      `json:"creatorId"`
	CreatorIsActive bool     `json:"creatorIsActive"`
	AccountIsActive bool     `json:"accountIsActive"`
	PhoneNumbers    []string `json:"phoneNumbers"`
	CreatedAt       string   `json:"createdAt"`
	DeletedAt       string   `json:"deletedAt"`
}

// fetchObjects получает объекты для конкретного аккаунта (используется для обратной совместимости)
func (s *AxentaSyncService) fetchObjects(token string, accountID int) ([]axentaObject, error) {
	result := make([]axentaObject, 0)
	params := url.Values{}
	params.Set("per_page", "200")
	params.Set("accountId", fmt.Sprintf("%d", accountID))
	nextURL := axentaAPIBase + "/api/cms/objects/?" + params.Encode()

	for nextURL != "" {
		var response axentaObjectsResponse
		if err := s.getJSON(nextURL, token, &response); err != nil {
			return nil, err
		}

		result = append(result, response.Results...)
		if response.Next == nil || *response.Next == "" {
			break
		}

		nextURL = s.resolveURL(*response.Next)
	}

	return result, nil
}

// fetchAllObjects получает ВСЕ объекты по иерархии (без фильтра по accountId)
// Это соответствует требованиям: запрашиваются все доступные объекты по иерархии вниз до бесконечности
func (s *AxentaSyncService) fetchAllObjects(token string) ([]axentaObject, error) {
	return s.fetchAllObjectsWithProgress(token, nil)
}

// fetchAllObjectsWithProgress получает ВСЕ объекты с отслеживанием прогресса
func (s *AxentaSyncService) fetchAllObjectsWithProgress(token string, progressCallback ProgressCallback) ([]axentaObject, error) {
	result := make([]axentaObject, 0)
	params := url.Values{}
	// Уменьшаем размер страницы до 100 для снижения нагрузки на API и избежания ошибок 500
	params.Set("per_page", "100")
	nextURL := axentaAPIBase + "/api/cms/objects/?" + params.Encode()

	log.Printf("📥 Начинаем загрузку ВСЕХ объектов из Axenta Cloud API...")
	log.Printf("   📊 Размер страницы: 100 объектов (оптимизировано для стабильности)")
	log.Printf("   🔗 URL первого запроса: %s", nextURL)

	page := 1
	totalCount := 0
	startTime := time.Now()

	for nextURL != "" {
		log.Printf("   📄 Страница %d: запрашиваем объекты...", page)

		var response axentaObjectsResponse
		requestStart := time.Now()

		// Пытаемся выполнить запрос с повторными попытками при ошибках
		var err error
		maxRetries := 3
		for attempt := 1; attempt <= maxRetries; attempt++ {
			err = s.getJSON(nextURL, token, &response)
			if err == nil {
				break
			}

			if attempt < maxRetries {
				waitTime := time.Duration(attempt) * time.Second
				log.Printf("   ⚠️ Ошибка при запросе страницы %d (попытка %d/%d): %v", page, attempt, maxRetries, err)
				log.Printf("   ⏳ Ждем %v перед повторной попыткой...", waitTime)
				time.Sleep(waitTime)
			}
		}

		if err != nil {
			return nil, fmt.Errorf("ошибка получения объектов (страница %d после %d попыток): %w", page, maxRetries, err)
		}

		requestDuration := time.Since(requestStart)

		// Сохраняем общее количество объектов из первого ответа
		if page == 1 {
			totalCount = response.Count
			log.Printf("   ✅ Первая страница получена успешно!")
			log.Printf("   📊 Всего объектов в системе: %d", totalCount)
		}

		objectsOnPage := len(response.Results)
		result = append(result, response.Results...)

		// Вычисляем общее количество страниц
		totalPages := (totalCount + 99) / 100 // Округляем вверх

		// Обновляем прогресс через callback
		if progressCallback != nil {
			progressCallback(len(result), totalCount, page, totalPages, "loading")
		}

		log.Printf("   ✅ Страница %d: получено %d объектов (время запроса: %v)", page, objectsOnPage, requestDuration)
		log.Printf("   📈 Прогресс: %d из %d объектов загружено (%.1f%%)",
			len(result), totalCount, float64(len(result))/float64(totalCount)*100)

		// Логируем каждые 5 страниц или на последней странице
		if page%5 == 0 || response.Next == nil {
			elapsed := time.Since(startTime)
			avgTimePerPage := elapsed / time.Duration(page)
			estimatedTotal := avgTimePerPage * time.Duration((totalCount+99)/100) // Округляем вверх
			remaining := estimatedTotal - elapsed

			log.Printf("   📊 Статистика загрузки:")
			log.Printf("      - Загружено страниц: %d", page)
			log.Printf("      - Загружено объектов: %d из %d", len(result), totalCount)
			log.Printf("      - Прошло времени: %v", elapsed.Round(time.Second))
			log.Printf("      - Среднее время на страницу: %v", avgTimePerPage.Round(time.Millisecond))
			if remaining > 0 {
				log.Printf("      - Осталось примерно: %v", remaining.Round(time.Second))
			}
		}

		if response.Next == nil || *response.Next == "" {
			log.Printf("   ✅ Достигнут конец списка объектов")
			break
		}

		nextURL = s.resolveURL(*response.Next)
		page++

		// Увеличиваем паузу между запросами для снижения нагрузки на API
		// Пауза 200ms между запросами помогает избежать ошибок 500
		time.Sleep(200 * time.Millisecond)
	}

	totalDuration := time.Since(startTime)
	log.Printf("✅ Загрузка объектов завершена успешно!")
	log.Printf("   📊 Итого:")
	log.Printf("      - Загружено страниц: %d", page)
	log.Printf("      - Загружено объектов: %d", len(result))
	log.Printf("      - Общее время: %v", totalDuration.Round(time.Second))
	log.Printf("      - Средняя скорость: %.1f объектов/сек", float64(len(result))/totalDuration.Seconds())

	return result, nil
}

func (s *AxentaSyncService) getJSON(rawURL, token string, v interface{}) error {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return fmt.Errorf("ошибка создания HTTP запроса: %w", err)
	}

	req.Header.Set("Authorization", "Token "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка выполнения HTTP запроса к %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Читаем тело ответа для более детальной информации об ошибке
		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)
		if len(bodyStr) > 200 {
			bodyStr = bodyStr[:200] + "..."
		}

		return fmt.Errorf("axenta API вернул статус %d (%s). Ответ: %s",
			resp.StatusCode, resp.Status, bodyStr)
	}

	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("ошибка парсинга JSON ответа от %s: %w", rawURL, err)
	}

	return nil
}

func (s *AxentaSyncService) resolveURL(raw string) string {
	if strings.HasPrefix(raw, "http") {
		return raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	base, err := url.Parse(axentaAPIBase)
	if err != nil {
		return ""
	}
	return base.ResolveReference(u).String()
}

func (s *AxentaSyncService) storeAccounts(adminAccountID uint, accounts []axentaAccount, syncedAt time.Time) error {
	return s.storeAccountsWithDB(adminAccountID, accounts, syncedAt, s.db)
}

func (s *AxentaSyncService) storeAccountsWithDB(adminAccountID uint, accounts []axentaAccount, syncedAt time.Time, db *gorm.DB) error {
	log.Printf("💾 Начинаем сохранение %d аккаунтов в базу данных...", len(accounts))

	savedCount := 0
	updatedCount := 0
	errorCount := 0

	for i, account := range accounts {
		rawPayload, _ := json.Marshal(account)
		snapshot := models.AxentaAccountSnapshot{
			AdminAccountID:    adminAccountID,
			ExternalAccountID: int64(account.ID),
			AccountName:       account.Name,
			AccountType:       account.Type,
			AdminFullname:     account.AdminFullname,
			ParentAccountName: account.ParentAccountName,
			Hierarchy:         account.Hierarchy,
			IsActive:          account.IsActive,
			ObjectsActive:     account.ObjectsActive,
			ObjectsTotal:      account.ObjectsTotal,
			LastSyncedAt:      syncedAt,
			RawPayload:        string(rawPayload),
		}

		if account.AdminID != nil {
			adminID64 := int64(*account.AdminID)
			snapshot.AdminExternalID = &adminID64
		}

		if account.Comment != nil {
			snapshot.Comment = *account.Comment
		}

		if account.BlockingDatetime != nil {
			if parsed := parseAxentaTime(*account.BlockingDatetime); parsed != nil {
				snapshot.BlockingDatetime = parsed
			}
		}

		if account.DaysBeforeBlocking != nil {
			snapshot.DaysBeforeBlocking = account.DaysBeforeBlocking
		}

		if account.CreationDatetime != "" {
			if parsed := parseAxentaTime(account.CreationDatetime); parsed != nil {
				// Используем CreationDatetime как справочную информацию
				snapshot.CreatedAt = *parsed
			}
		}

		// Проверяем, существует ли аккаунт в БД
		var existingAccount models.AxentaAccountSnapshot
		isUpdate := db.Where("admin_account_id = ? AND external_account_id = ?", adminAccountID, account.ID).First(&existingAccount).Error == nil

		if err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "admin_account_id"}, {Name: "external_account_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"account_name", "account_type", "admin_fullname", "admin_external_id", "parent_account_name", "hierarchy", "comment", "is_active", "objects_active", "objects_total", "blocking_datetime", "days_before_blocking", "last_synced_at", "raw_payload"}),
		}).Create(&snapshot).Error; err != nil {
			errorCount++
			log.Printf("   ❌ Ошибка сохранения аккаунта %d (позиция %d/%d): %v", account.ID, i+1, len(accounts), err)
			// Продолжаем обработку остальных аккаунтов вместо остановки
			continue
		}

		if isUpdate {
			updatedCount++
		} else {
			savedCount++
		}

		// Логируем прогресс каждые 50 аккаунтов или на последнем аккаунте
		if (i+1)%50 == 0 || i == len(accounts)-1 {
			progress := float64(i+1) / float64(len(accounts)) * 100
			log.Printf("   📊 Прогресс сохранения аккаунтов:")
			log.Printf("      - Обработано: %d из %d аккаунтов (%.1f%%)", i+1, len(accounts), progress)
			log.Printf("      - Сохранено новых: %d", savedCount)
			log.Printf("      - Обновлено существующих: %d", updatedCount)
			log.Printf("      - Ошибок: %d", errorCount)
		}
	}

	log.Printf("✅ Сохранение аккаунтов в БД завершено!")
	log.Printf("   📊 Итоговая статистика:")
	log.Printf("      - Всего аккаунтов обработано: %d", len(accounts))
	log.Printf("      - Сохранено новых: %d", savedCount)
	log.Printf("      - Обновлено существующих: %d", updatedCount)
	log.Printf("      - Ошибок: %d", errorCount)

	if errorCount > 0 {
		return fmt.Errorf("сохранение завершено с ошибками: %d из %d аккаунтов не сохранены", errorCount, len(accounts))
	}

	return nil
}

// syncAllObjects синхронизирует ВСЕ объекты по иерархии (без фильтра по accountId)
// Это соответствует требованиям: запрашиваются все доступные объекты по иерархии вниз до бесконечности
func (s *AxentaSyncService) syncAllObjects(adminAccountID uint, token string, syncedAt time.Time) error {
	return s.syncAllObjectsWithDB(adminAccountID, token, syncedAt, s.db)
}

// syncAllObjectsWithDB синхронизирует ВСЕ объекты используя указанную БД
func (s *AxentaSyncService) syncAllObjectsWithDB(adminAccountID uint, token string, syncedAt time.Time, db *gorm.DB) error {
	return s.syncAllObjectsWithDBAndProgress(adminAccountID, token, syncedAt, db, nil)
}

// syncAllObjectsWithDBAndProgress синхронизирует ВСЕ объекты с отслеживанием прогресса
func (s *AxentaSyncService) syncAllObjectsWithDBAndProgress(adminAccountID uint, token string, syncedAt time.Time, db *gorm.DB, progressCallback ProgressCallback) error {
	// Запрашиваем ВСЕ объекты без фильтра по accountId
	// API Axenta Cloud автоматически возвращает все объекты, доступные по иерархии для данного токена
	objects, err := s.fetchAllObjectsWithProgress(token, progressCallback)
	if err != nil {
		return fmt.Errorf("ошибка получения всех объектов: %w", err)
	}

	log.Printf("💾 Начинаем сохранение %d объектов в базу данных...", len(objects))
	log.Printf("   📊 Admin Account ID: %d", adminAccountID)
	log.Printf("   📅 Время синхронизации: %s", syncedAt.Format("2006-01-02 15:04:05"))

	// Обновляем статус: начинаем сохранение
	if progressCallback != nil {
		progressCallback(len(objects), len(objects), 0, 0, "saving")
	}

	startTime := time.Now()
	savedCount := 0
	updatedCount := 0
	errorCount := 0

	for i, obj := range objects {
		// Обновляем прогресс сохранения каждые 100 объектов
		if progressCallback != nil && (i+1)%100 == 0 {
			progress := float64(i+1) / float64(len(objects)) * 100
			progressCallback(i+1, len(objects), 0, 0, "saving")
			log.Printf("   💾 Прогресс сохранения: %d из %d объектов (%.1f%%)", i+1, len(objects), progress)
		}
		rawPayload, _ := json.Marshal(obj)
		adminAccountIDPtr := &adminAccountID
		snapshot := models.AxentaObjectSnapshot{
			// AdminAccountID устанавливаем для совместимости с БД (требуется для уникального индекса)
			AdminAccountID:    adminAccountIDPtr,
			AccountExternalID: int64(obj.AccountID),
			ExternalObjectID:  int64(obj.ID),
			ObjectName:        obj.Name,
			UniqueID:          obj.UniqueID,
			DeviceTypeName:    obj.DeviceTypeName,
			AccountName:       obj.AccountName,
			Status:            obj.Status,
			IsActive:          obj.IsActive,
			LastSyncedAt:      syncedAt,
			RawPayload:        string(rawPayload),
		}

		// Парсим последнее сообщение
		if obj.LastMessageDatetime != "" {
			if parsed := parseAxentaTime(obj.LastMessageDatetime); parsed != nil {
				snapshot.LastCommunicationAt = parsed
			}
		}

		// Новые поля из API (с проверкой на пустые значения)
		if obj.CreatorName != "" {
			snapshot.CreatorName = &obj.CreatorName
		}
		if obj.CreatorID != 0 {
			snapshot.CreatorID = &obj.CreatorID
		}
		snapshot.CreatorIsActive = &obj.CreatorIsActive
		snapshot.AccountIsActive = &obj.AccountIsActive

		// Конвертируем массив телефонов в JSON
		if len(obj.PhoneNumbers) > 0 {
			phonesJSON, _ := json.Marshal(obj.PhoneNumbers)
			phonesStr := string(phonesJSON)
			snapshot.PhoneNumbers = &phonesStr
		}

		// Парсим дату создания в Axenta
		if obj.CreatedAt != "" {
			if parsed := parseAxentaTime(obj.CreatedAt); parsed != nil {
				snapshot.AxentaCreatedAt = parsed
			}
		}

		// Парсим дату удаления в Axenta
		if obj.DeletedAt != "" {
			if parsed := parseAxentaTime(obj.DeletedAt); parsed != nil {
				snapshot.AxentaDeletedAt = parsed
			}
		}

		// Проверяем, существует ли объект в БД (по external_object_id, так как уникальный индекс только по нему)
		var existingObj models.AxentaObjectSnapshot
		isUpdate := db.Where("external_object_id = ?", obj.ID).First(&existingObj).Error == nil

		// ВАЖНО: Уникальный индекс в БД только по external_object_id (idx_axenta_object_external)
		// Поэтому используем только external_object_id в OnConflict
		// Это означает, что объект с одинаковым external_object_id будет обновляться, а не создаваться заново
		result := db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "external_object_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"admin_account_id", "account_external_id", "object_name", "unique_id", "device_type_name", "account_name",
				"status", "is_active", "last_synced_at", "raw_payload", "last_communication_at",
				// Новые поля
				"creator_name", "creator_id", "creator_is_active", "account_is_active",
				"phone_numbers", "axenta_created_at", "axenta_deleted_at",
			}),
		}).Create(&snapshot)

		if result.Error != nil {
			errorCount++
			log.Printf("   ❌ Ошибка сохранения объекта %d (позиция %d/%d): %v", obj.ID, i+1, len(objects), result.Error)
			// Продолжаем обработку остальных объектов вместо остановки
			continue
		}

		// ВАЖНО: При использовании OnConflict, RowsAffected может быть 0, если объект уже существует
		// и все поля идентичны (PostgreSQL не обновляет строку, если ничего не изменилось)
		// Это нормальное поведение, но нам нужно убедиться, что объект существует в БД
		if result.RowsAffected == 0 && !isUpdate {
			// Объект не был создан и не существовал ранее - это проблема
			errorCount++
			log.Printf("   ⚠️ Объект %d (позиция %d/%d) не был сохранен (RowsAffected=0, не существовал ранее)", obj.ID, i+1, len(objects))
			// Проверяем, действительно ли объект не существует
			var checkObj models.AxentaObjectSnapshot
			if db.Where("external_object_id = ?", obj.ID).First(&checkObj).Error != nil {
				log.Printf("   ❌ Подтверждено: объект %d действительно отсутствует в БД", obj.ID)
			} else {
				log.Printf("   ✅ Объект %d существует в БД (возможно, был создан другим процессом)", obj.ID)
			}
			continue
		}

		if isUpdate {
			updatedCount++
		} else {
			savedCount++
		}

		// Логируем прогресс каждые 100 объектов или на последнем объекте
		if (i+1)%100 == 0 || i == len(objects)-1 {
			elapsed := time.Since(startTime)
			progress := float64(i+1) / float64(len(objects)) * 100
			avgTimePerObject := elapsed / time.Duration(i+1)
			estimatedTotal := avgTimePerObject * time.Duration(len(objects))
			remaining := estimatedTotal - elapsed

			log.Printf("   📊 Прогресс сохранения:")
			log.Printf("      - Обработано: %d из %d объектов (%.1f%%)", i+1, len(objects), progress)
			log.Printf("      - Сохранено новых: %d", savedCount)
			log.Printf("      - Обновлено существующих: %d", updatedCount)
			log.Printf("      - Ошибок: %d", errorCount)
			log.Printf("      - Прошло времени: %v", elapsed.Round(time.Second))
			if remaining > 0 && i < len(objects)-1 {
				log.Printf("      - Осталось примерно: %v", remaining.Round(time.Second))
			}
		}
	}

	totalDuration := time.Since(startTime)

	// Проверяем, сколько объектов реально сохранено в БД
	var actualCount int64
	db.Model(&models.AxentaObjectSnapshot{}).Count(&actualCount)

	log.Printf("✅ Сохранение объектов в БД завершено!")
	log.Printf("   📊 Итоговая статистика:")
	log.Printf("      - Всего объектов обработано: %d", len(objects))
	log.Printf("      - Сохранено новых: %d", savedCount)
	log.Printf("      - Обновлено существующих: %d", updatedCount)
	log.Printf("      - Ошибок: %d", errorCount)
	log.Printf("      - Общее время: %v", totalDuration.Round(time.Second))
	log.Printf("      - Средняя скорость: %.1f объектов/сек", float64(len(objects))/totalDuration.Seconds())
	log.Printf("      - Реально сохранено в БД: %d объектов", actualCount)

	// Проверяем разницу
	if int(actualCount) < len(objects) {
		missing := len(objects) - int(actualCount)
		log.Printf("   ⚠️ ВНИМАНИЕ: Разница между обработанными и сохраненными: %d объектов", missing)
		log.Printf("   💡 Возможные причины:")
		log.Printf("      - Объекты с одинаковым external_object_id обновляются (не создаются заново)")
		log.Printf("      - Ошибки при сохранении (см. логи выше)")
		log.Printf("      - Объекты не прошли валидацию")
	}

	if errorCount > 0 {
		return fmt.Errorf("сохранение завершено с ошибками: %d из %d объектов не сохранены", errorCount, len(objects))
	}

	return nil
}

// syncObjectsForAccounts - устаревший метод, оставлен для обратной совместимости
// Использует старый подход: запрашивает объекты для каждого аккаунта отдельно
func (s *AxentaSyncService) syncObjectsForAccounts(adminAccountID uint, token string, accounts []axentaAccount, syncedAt time.Time) error {
	for _, account := range accounts {
		objects, err := s.fetchObjects(token, account.ID)
		if err != nil {
			log.Printf("AxentaSync: ошибка получения объектов для account=%d: %v", account.ID, err)
			continue
		}

		for _, obj := range objects {
			rawPayload, _ := json.Marshal(obj)
			adminAccountIDPtr := &adminAccountID
			snapshot := models.AxentaObjectSnapshot{
				// AdminAccountID устанавливаем для совместимости с БД (требуется для уникального индекса)
				AdminAccountID:    adminAccountIDPtr,
				AccountExternalID: int64(obj.AccountID),
				ExternalObjectID:  int64(obj.ID),
				ObjectName:        obj.Name,
				UniqueID:          obj.UniqueID,
				DeviceTypeName:    obj.DeviceTypeName,
				AccountName:       obj.AccountName,
				Status:            obj.Status,
				IsActive:          obj.IsActive,
				LastSyncedAt:      syncedAt,
				RawPayload:        string(rawPayload),
			}

			// Парсим последнее сообщение
			if obj.LastMessageDatetime != "" {
				if parsed := parseAxentaTime(obj.LastMessageDatetime); parsed != nil {
					snapshot.LastCommunicationAt = parsed
				}
			}

			// Новые поля из API (с проверкой на пустые значения)
			if obj.CreatorName != "" {
				snapshot.CreatorName = &obj.CreatorName
			}
			if obj.CreatorID != 0 {
				snapshot.CreatorID = &obj.CreatorID
			}
			snapshot.CreatorIsActive = &obj.CreatorIsActive
			snapshot.AccountIsActive = &obj.AccountIsActive

			// Конвертируем массив телефонов в JSON
			if len(obj.PhoneNumbers) > 0 {
				phonesJSON, _ := json.Marshal(obj.PhoneNumbers)
				phonesStr := string(phonesJSON)
				snapshot.PhoneNumbers = &phonesStr
			}

			// Парсим дату создания в Axenta
			if obj.CreatedAt != "" {
				if parsed := parseAxentaTime(obj.CreatedAt); parsed != nil {
					snapshot.AxentaCreatedAt = parsed
				}
			}

			// Парсим дату удаления в Axenta
			if obj.DeletedAt != "" {
				if parsed := parseAxentaTime(obj.DeletedAt); parsed != nil {
					snapshot.AxentaDeletedAt = parsed
				}
			}

			// ВАЖНО: Уникальный индекс в БД только по external_object_id
			if err := s.db.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "external_object_id"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"admin_account_id", "account_external_id", "object_name", "unique_id", "device_type_name", "account_name",
					"status", "is_active", "last_synced_at", "raw_payload", "last_communication_at",
					// Новые поля
					"creator_name", "creator_id", "creator_is_active", "account_is_active",
					"phone_numbers", "axenta_created_at", "axenta_deleted_at",
				}),
			}).Create(&snapshot).Error; err != nil {
				return fmt.Errorf("ошибка сохранения объекта %d: %w", obj.ID, err)
			}
		}
	}

	return nil
}

func parseAxentaTime(value string) *time.Time {
	if value == "" {
		return nil
	}

	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, value); err == nil {
			return &t
		}
	}

	return nil
}
