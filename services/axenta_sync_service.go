package services

import (
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

// SyncAllAdmins выполняет синхронизацию для всех активных партнеров
func (s *AxentaSyncService) SyncAllAdmins() {
	tokens, err := s.loadActivePartnerTokens()
	if err != nil {
		log.Printf("AxentaSync: не удалось загрузить токены партнеров: %v", err)
		return
	}

	for adminID := range tokens {
		if err := s.SyncAdmin(adminID); err != nil {
			log.Printf("AxentaSync: ошибка синхронизации для admin_account_id=%d: %v", adminID, err)
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
	now := time.Now().UTC()

	accounts, err := s.fetchAccounts(token)
	if err != nil {
		return fmt.Errorf("ошибка получения учетных записей: %w", err)
	}

	if err := s.storeAccounts(adminAccountID, accounts, now); err != nil {
		return fmt.Errorf("ошибка сохранения учетных записей: %w", err)
	}

	if err := s.syncObjectsForAccounts(adminAccountID, token, accounts, now); err != nil {
		return fmt.Errorf("ошибка синхронизации объектов: %w", err)
	}

	// Удаляем устаревшие записи (которые не обновлялись в этом цикле)
	cutoff := now.Add(-30 * time.Second)
	if err := s.db.Where("admin_account_id = ? AND last_synced_at < ?", adminAccountID, cutoff).
		Delete(&models.AxentaAccountSnapshot{}).Error; err != nil {
		log.Printf("AxentaSync: ошибка очистки устаревших аккаунтов admin=%d: %v", adminAccountID, err)
	}

	if err := s.db.Where("admin_account_id = ? AND last_synced_at < ?", adminAccountID, cutoff).
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
}

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

		if err := s.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "admin_account_id"}, {Name: "external_account_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"account_name", "account_type", "admin_fullname", "admin_external_id", "parent_account_name", "hierarchy", "comment", "is_active", "objects_active", "objects_total", "blocking_datetime", "days_before_blocking", "last_synced_at", "raw_payload"}),
		}).Create(&snapshot).Error; err != nil {
			return err
		}
	}

	return nil
}

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

			if obj.LastMessageDatetime != "" {
				if parsed := parseAxentaTime(obj.LastMessageDatetime); parsed != nil {
					snapshot.LastCommunicationAt = parsed
				}
			}

			if err := s.db.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "admin_account_id"}, {Name: "external_object_id"}},
				DoUpdates: clause.AssignmentColumns([]string{"account_external_id", "object_name", "unique_id", "device_type_name", "account_name", "status", "is_active", "last_synced_at", "raw_payload", "last_communication_at"}),
			}).Create(&snapshot).Error; err != nil {
				return err
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
