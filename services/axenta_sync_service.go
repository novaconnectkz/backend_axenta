package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"encoding/json"
	"fmt"
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

	// Получаем токен из настроек снимков (хранится в базе суперадмина, ID=1)
	const superAdminCompanyID = 1
	superAdminTenantDB := database.GetTenantDBByID(superAdminCompanyID)
	if superAdminTenantDB == nil {
		log.Printf("AxentaSync: не удалось получить tenant DB для суперадмина (ID=1)")
		return
	}

	// Получаем токен из настроек снимков
	var snapshotSettings models.SnapshotSettings
	var syncToken string
	var adminAccountID uint = superAdminCompanyID

	if err := superAdminTenantDB.
		Where("company_id = ? AND is_active = ?", superAdminCompanyID, true).
		First(&snapshotSettings).Error; err != nil {
		log.Printf("AxentaSync: не найдено настроек снимков в базе суперадмина (ID=1): %v", err)
		log.Printf("AxentaSync: попытка найти токен в user_tokens...")
		
		// Fallback: пытаемся найти токен в user_tokens
		var userToken models.UserToken
		if err := superAdminTenantDB.
			Where("is_active = ? AND expires_at > ?", true, time.Now()).
			Order("updated_at DESC").
			First(&userToken).Error; err != nil {
			log.Printf("AxentaSync: не найдено активных токенов в базе суперадмина (ID=1): %v", err)
			return
		}
		
		syncToken = userToken.Token
		adminAccountID = userToken.AccountID
		if adminAccountID == 0 {
			adminAccountID = superAdminCompanyID
		}
		log.Printf("AxentaSync: используем токен из user_tokens (account_id=%d) для синхронизации всех компаний", adminAccountID)
		
		// Сохраняем токен в настройки для будущего использования
		snapshotSettings = models.SnapshotSettings{
			CompanyID:   superAdminCompanyID,
			AxentaToken: syncToken,
			IsActive:    true,
		}
		if err := superAdminTenantDB.Save(&snapshotSettings).Error; err != nil {
			log.Printf("AxentaSync: предупреждение - не удалось сохранить токен в настройки: %v", err)
		}
	} else {
		// Используем токен из настроек
		syncToken = snapshotSettings.AxentaToken
		if syncToken == "" {
			log.Printf("AxentaSync: токен в настройках пустой, пропускаем синхронизацию")
			return
		}
		log.Printf("AxentaSync: используем токен из настроек снимков (ID=1) для синхронизации всех компаний")
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

// syncAdminWithTokenAndDB синхронизирует данные используя указанную БД (tenant или основную)
func (s *AxentaSyncService) syncAdminWithTokenAndDB(adminAccountID uint, token string, db *gorm.DB) error {
	now := time.Now().UTC()

	accounts, err := s.fetchAccounts(token)
	if err != nil {
		return fmt.Errorf("ошибка получения учетных записей: %w", err)
	}

	if err := s.storeAccountsWithDB(adminAccountID, accounts, now, db); err != nil {
		return fmt.Errorf("ошибка сохранения учетных записей: %w", err)
	}

	// Запрашиваем ВСЕ объекты по иерархии (без фильтра по accountId)
	// Это соответствует требованиям: запрашиваются все доступные объекты по иерархии
	if err := s.syncAllObjectsWithDB(adminAccountID, token, now, db); err != nil {
		return fmt.Errorf("ошибка синхронизации объектов: %w", err)
	}

	// Удаляем устаревшие записи (которые не обновлялись в этом цикле)
	cutoff := now.Add(-30 * time.Second)
	if err := db.Where("admin_account_id = ? AND last_synced_at < ?", adminAccountID, cutoff).
		Delete(&models.AxentaAccountSnapshot{}).Error; err != nil {
		log.Printf("AxentaSync: ошибка очистки устаревших аккаунтов admin=%d: %v", adminAccountID, err)
	}

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
	nextURL := axentaAPIBase + "/api/cms/accounts/?per_page=200"

	for nextURL != "" {
		var response axentaAccountsResponse
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
	result := make([]axentaObject, 0)
	params := url.Values{}
	params.Set("per_page", "1000") // Увеличиваем размер страницы для эффективности
	nextURL := axentaAPIBase + "/api/cms/objects/?" + params.Encode()

	log.Printf("AxentaSync: начинаем загрузку ВСЕХ объектов по иерархии...")

	page := 1
	for nextURL != "" {
		var response axentaObjectsResponse
		if err := s.getJSON(nextURL, token, &response); err != nil {
			return nil, fmt.Errorf("ошибка получения объектов (страница %d): %w", page, err)
		}

		result = append(result, response.Results...)

		if page%10 == 0 {
			log.Printf("AxentaSync: загружено %d объектов из %d...", len(result), response.Count)
		}

		if response.Next == nil || *response.Next == "" {
			break
		}

		nextURL = s.resolveURL(*response.Next)
		page++

		// Небольшая пауза между запросами для избежания перегрузки API
		time.Sleep(100 * time.Millisecond)
	}

	log.Printf("AxentaSync: загружено всего %d объектов", len(result))
	return result, nil
}

func (s *AxentaSyncService) getJSON(rawURL, token string, v interface{}) error {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Token "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("axenta API returned status %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(v)
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
	for _, account := range accounts {
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

		if err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "admin_account_id"}, {Name: "external_account_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"account_name", "account_type", "admin_fullname", "admin_external_id", "parent_account_name", "hierarchy", "comment", "is_active", "objects_active", "objects_total", "blocking_datetime", "days_before_blocking", "last_synced_at", "raw_payload"}),
		}).Create(&snapshot).Error; err != nil {
			return err
		}
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
	// Запрашиваем ВСЕ объекты без фильтра по accountId
	// API Axenta Cloud автоматически возвращает все объекты, доступные по иерархии для данного токена
	objects, err := s.fetchAllObjects(token)
	if err != nil {
		return fmt.Errorf("ошибка получения всех объектов: %w", err)
	}

	log.Printf("AxentaSync: синхронизируем %d объектов для admin_account_id=%d", len(objects), adminAccountID)

	for _, obj := range objects {
		rawPayload, _ := json.Marshal(obj)
		snapshot := models.AxentaObjectSnapshot{
			AdminAccountID:    adminAccountID,
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

		if err := db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "admin_account_id"}, {Name: "external_object_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"account_external_id", "object_name", "unique_id", "device_type_name", "account_name",
				"status", "is_active", "last_synced_at", "raw_payload", "last_communication_at",
				// Новые поля
				"creator_name", "creator_id", "creator_is_active", "account_is_active",
				"phone_numbers", "axenta_created_at", "axenta_deleted_at",
			}),
		}).Create(&snapshot).Error; err != nil {
			return fmt.Errorf("ошибка сохранения объекта %d: %w", obj.ID, err)
		}
	}

	log.Printf("AxentaSync: успешно синхронизировано %d объектов для admin_account_id=%d", len(objects), adminAccountID)
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
			snapshot := models.AxentaObjectSnapshot{
				AdminAccountID:    adminAccountID,
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

			if err := s.db.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "admin_account_id"}, {Name: "external_object_id"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"account_external_id", "object_name", "unique_id", "device_type_name", "account_name",
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
