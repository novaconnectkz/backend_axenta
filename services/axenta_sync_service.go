package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
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
	svc := &AxentaSyncService{
		db: db,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	if globalAxentaSyncService == nil {
		globalAxentaSyncService = svc
	}
	return svc
}

// globalAxentaSyncService — для доступа из API-handlers без injection.
// Регистрируется в первом NewAxentaSyncService (main.go).
var globalAxentaSyncService *AxentaSyncService

// GetAxentaSyncService возвращает глобальный экземпляр (может быть nil).
func GetAxentaSyncService() *AxentaSyncService {
	return globalAxentaSyncService
}

// SyncAllAdmins выполняет синхронизацию для всех компаний
// Согласно требованиям: запрос происходит от имени пользователя CRM, запрашиваются все доступные объекты по иерархии
// ВАЖНО: Используем токен 186 (GLOMOS), который имеет доступ ко всей иерархии для получения всех данных
func (s *AxentaSyncService) SyncAllAdmins() {
	// Получаем все компании из основной БД
	var companies []models.Company
	// Ф3-F-followup/Codex Q1: ORDER BY id ASC ОБЯЗАТЕЛЕН для детерминизма
	// firstCompany = companies[0] (см. ниже adminAccountID = firstCompany.ID).
	// Без ORDER Find возвращает heap-order → snapshotAdminKey в read-path
	// (api/partner_snapshots.go MIN(id)) мог бы не совпасть с тем что сюда
	// пишется. Edge: fallback на userToken.AccountID (стр.~99) пишет
	// другое значение — отдельная политика, prod main-path использует
	// firstCompany.ID (snapshot.admin_account_id=6=MIN active id ✓).
	if err := s.db.Table("public.companies").Where("is_active = ?", true).Order("id ASC").Find(&companies).Error; err != nil {
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
	successCount := 0
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
			successCount++
		}
	}

	// После успешной синхронизации хотя бы одной компании запускаем автоматический расчет биллинга
	if successCount > 0 {
		log.Printf("AxentaSync: синхронизация завершена для %d компаний, запускаем автоматический расчет биллинга...", successCount)
		AutoCalculateBillingForAllPartners()
	}
}

// RefreshAccountsOnlyForTenant — lightweight inline-refresh для UI: тянет
// аккаунты из Axenta и пишет только axenta_account_snapshots. НЕ трогает
// partner_snapshots / object_snapshots / user_snapshots / accounts —
// это ответственность cron'а SyncAllAdmins. См. план:
// memory/project_acrm_axenta_sync_refactor_plan.md (вариант C+B1).
func (s *AxentaSyncService) RefreshAccountsOnlyForTenant(adminAccountID uint, token string, db *gorm.DB) (int, error) {
	accounts, err := s.fetchAccountsParallel(token)
	if err != nil {
		return 0, fmt.Errorf("ошибка получения аккаунтов: %w", err)
	}
	return len(accounts), s.storeRefreshedAccounts(adminAccountID, accounts, db, time.Now().UTC())
}

// storeRefreshedAccounts — общий хвост: write snapshot'ов + per-admin cleanup.
// Используется RefreshAccountsOnlyForTenant и RefreshAccountsOnlyAllTenants
// (последний вызывает напрямую чтобы не дублировать fetch для каждого тенанта).
func (s *AxentaSyncService) storeRefreshedAccounts(adminAccountID uint, accounts []axentaAccount, db *gorm.DB, now time.Time) error {
	if err := s.storeAccountsWithDB(adminAccountID, accounts, now, db); err != nil {
		return fmt.Errorf("ошибка сохранения аккаунтов: %w", err)
	}
	cutoff := now.Add(-30 * time.Second)
	if err := db.Where("admin_account_id = ? AND last_synced_at < ?", adminAccountID, cutoff).
		Delete(&models.AxentaAccountSnapshot{}).Error; err != nil {
		log.Printf("RefreshAccountsOnly: предупреждение — ошибка очистки устаревших snapshot'ов admin=%d: %v", adminAccountID, err)
	}
	return nil
}

// RefreshAccountsOnlyAllTenants — обёртка-аналог SyncAllAdmins для inline-refresh:
// одинаковая логика получения токена (snapshot_settings → user_tokens fallback),
// проход по всем активным компаниям с вызовом RefreshAccountsOnlyForTenant.
// Возвращает total accounts-count (сумма по всем тенантам) и err первой
// фатальной ошибки получения токена; per-tenant ошибки логируются и не
// прерывают цикл (consistency с SyncAllAdmins).
func (s *AxentaSyncService) RefreshAccountsOnlyAllTenants() (int, error) {
	var companies []models.Company
	if err := s.db.Table("public.companies").Where("is_active = ?", true).Order("id ASC").Find(&companies).Error; err != nil {
		return 0, fmt.Errorf("не удалось загрузить компании: %w", err)
	}
	if len(companies) == 0 {
		return 0, fmt.Errorf("нет активных компаний")
	}

	firstCompany := companies[0]
	firstCompanyTenantDB := database.GetTenantDBByID(firstCompany.ID)
	if firstCompanyTenantDB == nil {
		return 0, fmt.Errorf("не удалось получить tenant DB для первой компании (ID=%d)", firstCompany.ID)
	}

	// Логика добычи токена идентична SyncAllAdmins.
	var snapshotSettings models.SnapshotSettings
	var syncToken string
	var adminAccountID uint = firstCompany.ID

	if err := firstCompanyTenantDB.
		Where("company_id = ? AND is_active = ?", 1, true).
		First(&snapshotSettings).Error; err != nil {
		var userToken models.UserToken
		if err := firstCompanyTenantDB.
			Where("is_active = ? AND expires_at > ?", true, time.Now()).
			Order("updated_at DESC").
			First(&userToken).Error; err != nil {
			return 0, fmt.Errorf("не найдено активных токенов: %w", err)
		}
		syncToken = userToken.Token
		adminAccountID = userToken.AccountID
		if adminAccountID == 0 {
			adminAccountID = firstCompany.ID
		}
	} else {
		syncToken = snapshotSettings.AxentaToken
		if syncToken == "" {
			return 0, fmt.Errorf("токен в snapshot_settings пустой")
		}
	}

	// Один fetch на всех — токен один (GLOMOS), иерархия одна, accounts
	// идентичны для всех тенантов. Дублировать fetch per-tenant = пустой
	// расход 500 req/min лимита и времени.
	accounts, err := s.fetchAccountsParallel(syncToken)
	if err != nil {
		return 0, fmt.Errorf("ошибка получения аккаунтов: %w", err)
	}
	now := time.Now().UTC()

	total := 0
	for _, company := range companies {
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			log.Printf("RefreshAccountsOnly: не удалось получить tenant DB для компании %d (%s)", company.ID, company.DatabaseSchema)
			continue
		}
		if err := s.storeRefreshedAccounts(adminAccountID, accounts, tenantDB, now); err != nil {
			log.Printf("RefreshAccountsOnly: ошибка для компании %d: %v", company.ID, err)
			continue
		}
		total += len(accounts)
	}

	return total, nil
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

	err = s.syncAdminWithToken(adminAccountID, token)
	if err == nil {
		// После успешной синхронизации проверяем, нужно ли запустить автоматический расчет биллинга
		// Функция сама проверит, есть ли уже снимки, и запустится только если их нет
		AutoCalculateBillingForAllPartners()
	}
	return err
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

	// Шаг 1.5: Загрузка и сохранение пользователей (для read-path /unified/users)
	log.Printf("👥 Шаг 1.5: Загрузка и сохранение пользователей...")
	usersStartTime := time.Now()

	users, err := s.fetchUsers(token)
	if err != nil {
		log.Printf("⚠️ Ошибка загрузки пользователей (некритично, продолжаем): %v", err)
	} else if err := s.storeUsersWithDB(adminAccountID, users, now, db); err != nil {
		log.Printf("⚠️ Ошибка сохранения пользователей (некритично): %v", err)
	} else {
		usersDuration := time.Since(usersStartTime)
		log.Printf("✅ Шаг 1.5 завершен: сохранено %d пользователей за %v", len(users), usersDuration.Round(time.Second))
	}
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

	var deletedUsers int64
	if err := db.Where("admin_account_id = ? AND last_synced_at < ?", adminAccountID, cutoff).
		Delete(&models.AxentaUserSnapshot{}).Error; err != nil {
		log.Printf("   ⚠️ Ошибка очистки устаревших пользователей admin=%d: %v", adminAccountID, err)
	} else {
		db.Model(&models.AxentaUserSnapshot{}).
			Where("admin_account_id = ? AND last_synced_at < ?", adminAccountID, cutoff).
			Count(&deletedUsers)
		log.Printf("   ✅ Удалено устаревших пользователей: %d", deletedUsers)
	}

	// Global cleanup: orphan-snapshots от старых admin_account_id (другой axetna_login
	// раньше → 2 admin'а в БД, sync новый только под одним → старые orphan не очищаются
	// per-admin фильтром выше). Удаляем всё что не sync'ились >7 дней.
	orphanCutoff := now.Add(-7 * 24 * time.Hour)
	var orphanObjects, orphanAccounts, orphanUsers int64
	db.Model(&models.AxentaObjectSnapshot{}).
		Where("last_synced_at < ?", orphanCutoff).Count(&orphanObjects)
	db.Model(&models.AxentaAccountSnapshot{}).
		Where("last_synced_at < ?", orphanCutoff).Count(&orphanAccounts)
	db.Model(&models.AxentaUserSnapshot{}).
		Where("last_synced_at < ?", orphanCutoff).Count(&orphanUsers)
	if orphanObjects+orphanAccounts+orphanUsers > 0 {
		log.Printf("   🧹 Global orphan cleanup (>7 дней без sync): объектов=%d, аккаунтов=%d, юзеров=%d",
			orphanObjects, orphanAccounts, orphanUsers)
		db.Where("last_synced_at < ?", orphanCutoff).Delete(&models.AxentaObjectSnapshot{})
		db.Where("last_synced_at < ?", orphanCutoff).Delete(&models.AxentaAccountSnapshot{})
		db.Where("last_synced_at < ?", orphanCutoff).Delete(&models.AxentaUserSnapshot{})
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

// RefreshAccount — точечный re-fetch одной учётки + UPSERT в snapshot.
// Используется когда мы знаем что один account изменился (например после нашего
// toggle или явного refresh-запроса от UI), и не хотим запускать тяжёлый
// SyncAdmin (700+ accounts, ~30s).
//
// adminAccountID — нужен чтобы знать в какой tenant.axenta_account_snapshots
// писать. db — tenant DB или public.
//
// Возвращает обновлённую axenta-структуру или error если Axenta вернул not found
// (= аккаунт удалён → удаляем из snapshot).
func (s *AxentaSyncService) RefreshAccount(token string, adminAccountID uint, accountID int, db *gorm.DB) (*axentaAccount, error) {
	if token == "" {
		return nil, fmt.Errorf("token пустой")
	}
	if accountID <= 0 {
		return nil, fmt.Errorf("accountID невалидный")
	}

	url := fmt.Sprintf("%s/api/cms/accounts/%d/", axentaAPIBase, accountID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Token "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("axenta http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Аккаунт удалён в Axenta — убираем из snapshot.
		//
		// Ф3-D долг #2 (закрыт «verified, без правки»): admin-фильтр здесь
		// СОХРАНЁН намеренно. Соблазн расширить до WHERE external_account_id
		// (т.к. legacy-sync пишет общий firstCompany.ID, а вызывающий передаёт
		// доверенный company.ID → admin-фильтр на не-первой компании = 0 строк)
		// ОТКЛОНЁН: unique-индекс составной (admin_account_id, external_account_id),
		// storeAccountsWithDB UPSERT'ит по той же паре — в одной схеме легально
		// сосуществуют строки с одним external_account_id и разными admin
		// (смена axetna_login → orphan от старого admin). Снятие admin-фильтра
		// удалило бы чужие строки = data-loss (Codex BLOCKER).
		//
		// Призрак не теряется: cron stale-cleanup само-согласован (пишет и
		// чистит одним adminAccountID, см. ~стр.250) → удалённый в Axenta
		// аккаунт уходит за ≤1 sync-цикл, плюс global orphan-cleanup (>7д).
		// Этот 404-DELETE — лишь latency-оптимизация (мгновенно vs ≤cron).
		// Остаточная latency на редком пути (delete-in-Axenta + ручной
		// RefreshSingleAccount до cron) — принятый cron-backed residual,
		// тот же класс что Invalidate→SyncAdmin.
		if db != nil {
			db.Where("admin_account_id = ? AND external_account_id = ?", adminAccountID, accountID).
				Delete(&models.AxentaAccountSnapshot{})
			log.Printf("🗑️ RefreshAccount: account=%d удалён в Axenta, snapshot очищен", accountID)
		}
		return nil, fmt.Errorf("account %d not found in axenta", accountID)
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("axenta status=%d body=%s", resp.StatusCode, string(body))
	}

	var acc axentaAccount
	if err := json.NewDecoder(resp.Body).Decode(&acc); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	// UPDATE существующей записи с тем же external_account_id (любой adminAccountID).
	// НЕ используем UPSERT через storeAccountsWithDB — он бы создал дубликат, потому что
	// в snapshot одна и та же Axenta-account может быть привязана к разным admin'ам
	// (multi-tenant), а у вызывающего юзера может быть свой adminID.
	if db != nil {
		updates := map[string]interface{}{
			"account_name":         acc.Name,
			"account_type":         acc.Type,
			"admin_fullname":       acc.AdminFullname,
			"parent_account_name":  acc.ParentAccountName,
			"hierarchy":            acc.Hierarchy,
			"is_active":            acc.IsActive,
			"objects_active":       acc.ObjectsActive,
			"objects_total":        acc.ObjectsTotal,
			"days_before_blocking": acc.DaysBeforeBlocking,
			"last_synced_at":       time.Now().UTC(),
		}
		if acc.AdminID != nil {
			updates["admin_external_id"] = *acc.AdminID
		}
		if acc.Comment != nil {
			updates["comment"] = *acc.Comment
		}
		if acc.BlockingDatetime != nil {
			if parsed := parseAxentaTime(*acc.BlockingDatetime); parsed != nil {
				updates["blocking_datetime"] = parsed
			}
		} else {
			updates["blocking_datetime"] = nil
		}

		res := db.Model(&models.AxentaAccountSnapshot{}).
			Where("external_account_id = ?", accountID).
			Updates(updates)
		if res.Error != nil {
			log.Printf("⚠️ RefreshAccount update: %v", res.Error)
		} else {
			log.Printf("✅ RefreshAccount: account=%d обновлено %d записей (isActive=%v)", accountID, res.RowsAffected, acc.IsActive)
		}
	}

	return &acc, nil
}

// fetchAccountsPage — один запрос с retry. Используется fetchAccountsParallel.
func (s *AxentaSyncService) fetchAccountsPage(token, pageURL string) (axentaAccountsResponse, error) {
	var response axentaAccountsResponse
	const maxRetries = 3
	var err error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err = s.getJSON(pageURL, token, &response)
		if err == nil {
			return response, nil
		}
		if attempt < maxRetries {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	return response, fmt.Errorf("после %d попыток: %w", maxRetries, err)
}

// fetchAccountsParallel — для inline-refresh (план B1). Первая страница sync
// (узнаём count), остальные через worker-pool с rate-limiter под Axenta
// 500 req/min. Используется только RefreshAccountsOnlyForTenant; cron
// SyncAllAdmins использует sequential fetchAccounts (тяжёлый full sync,
// риск contention не оправдан).
func (s *AxentaSyncService) fetchAccountsParallel(token string) ([]axentaAccount, error) {
	const (
		perPage    = 100
		workers    = 5
		ratePerSec = 7 // ~420/min, под лимит 500/min
	)

	startTime := time.Now()
	firstURL := fmt.Sprintf("%s/api/cms/accounts/?per_page=%d", axentaAPIBase, perPage)
	log.Printf("📥 [parallel] Загрузка аккаунтов: workers=%d, rate=%d req/s", workers, ratePerSec)

	first, err := s.fetchAccountsPage(token, firstURL)
	if err != nil {
		return nil, fmt.Errorf("страница 1: %w", err)
	}

	totalCount := first.Count
	totalPages := 1
	if totalCount > 0 {
		totalPages = (totalCount + perPage - 1) / perPage
	}
	log.Printf("   📊 Всего аккаунтов: %d, страниц: %d", totalCount, totalPages)

	result := make([]axentaAccount, 0, totalCount)
	result = append(result, first.Results...)

	if totalPages <= 1 || first.Next == nil || *first.Next == "" {
		log.Printf("✅ [parallel] %d аккаунтов за %v", len(result), time.Since(startTime).Round(time.Second))
		return result, nil
	}

	type pageResult struct {
		page     int
		accounts []axentaAccount
		err      error
	}

	jobs := make(chan int, totalPages-1)
	results := make(chan pageResult, totalPages-1)

	limiter := rate.NewLimiter(rate.Limit(ratePerSec), ratePerSec)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				if err := limiter.Wait(ctx); err != nil {
					results <- pageResult{p, nil, fmt.Errorf("limiter ждал: %w", err)}
					continue
				}
				url := fmt.Sprintf("%s/api/cms/accounts/?per_page=%d&page=%d", axentaAPIBase, perPage, p)
				resp, err := s.fetchAccountsPage(token, url)
				if err != nil {
					results <- pageResult{p, nil, err}
					continue
				}
				results <- pageResult{p, resp.Results, nil}
			}
		}()
	}

	for p := 2; p <= totalPages; p++ {
		jobs <- p
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
	}()

	var firstErr error
	collected := 0
	for r := range results {
		if r.err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("страница %d: %w", r.page, r.err)
			}
			continue
		}
		result = append(result, r.accounts...)
		collected++
		if collected%10 == 0 {
			log.Printf("   📈 [parallel] обработано %d/%d страниц", collected+1, totalPages)
		}
	}

	if firstErr != nil {
		return nil, firstErr
	}

	log.Printf("✅ [parallel] %d аккаунтов за %v (%d страниц)",
		len(result), time.Since(startTime).Round(time.Second), totalPages)
	return result, nil
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
		// Сбрасываем gorm soft-delete (deleted_at = NULL) при возврате объекта,
		// иначе он навсегда останется скрыт от scoped запросов даже когда
		// снова появился в Axenta Cloud.
		result := db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "external_object_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"admin_account_id":    snapshot.AdminAccountID,
				"account_external_id": snapshot.AccountExternalID,
				"object_name":         snapshot.ObjectName,
				"unique_id":           snapshot.UniqueID,
				"device_type_name":    snapshot.DeviceTypeName,
				"account_name":        snapshot.AccountName,
				"status":              snapshot.Status,
				"is_active":           snapshot.IsActive,
				"last_synced_at":      snapshot.LastSyncedAt,
				"raw_payload":         snapshot.RawPayload,
				"last_communication_at": snapshot.LastCommunicationAt,
				"creator_name":        snapshot.CreatorName,
				"creator_id":          snapshot.CreatorID,
				"creator_is_active":   snapshot.CreatorIsActive,
				"account_is_active":   snapshot.AccountIsActive,
				"phone_numbers":       snapshot.PhoneNumbers,
				"axenta_created_at":   snapshot.AxentaCreatedAt,
				"axenta_deleted_at":   snapshot.AxentaDeletedAt,
				"deleted_at":          gorm.DeletedAt{},
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

	// Помечаем как удалённые объекты, которых нет в новой выгрузке.
	// Axenta API не возвращает удалённые объекты — diff между БД и API
	// единственный способ выставить axenta_deleted_at.
	// Только при errorCount == 0, чтобы частичная выгрузка не false-positive'ила.
	if errorCount == 0 && len(objects) > 0 {
		seen := make([]int64, 0, len(objects))
		for _, obj := range objects {
			seen = append(seen, int64(obj.ID))
		}
		// Unscoped() — захватываем и soft-deleted записи (Шаг 3 cleanup ниже
		// soft-deletes исчезнувшие; для бизнес-метки axenta_deleted_at это
		// не должно быть препятствием).
		res := db.Unscoped().Model(&models.AxentaObjectSnapshot{}).
			Where("admin_account_id = ? AND axenta_deleted_at IS NULL AND external_object_id NOT IN ?", adminAccountID, seen).
			Update("axenta_deleted_at", syncedAt)
		if res.Error != nil {
			log.Printf("   ⚠️ Не удалось пометить удалённые объекты: %v", res.Error)
		} else if res.RowsAffected > 0 {
			log.Printf("   🗑 Помечено как удалённые: %d объектов (отсутствуют в новой выгрузке)", res.RowsAffected)
		}
	}

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

// axentaUser представляет пользователя в ответе Axenta /api/cms/users/
// Поля camelCase согласно реальному API.
type axentaUser struct {
	ID               int64  `json:"id"`
	Username         string `json:"username"`
	Name             string `json:"name"`
	FirstName        string `json:"first_name"`
	LastName         string `json:"last_name"`
	Email            string `json:"email"`
	AccountType      string `json:"accountType"`
	IsActive         bool   `json:"isActive"`
	CreationDatetime string `json:"creationDatetime"`
	CreatorName      string `json:"creatorName"`
	LastLogin        string `json:"lastLogin"`
}

type axentaUsersResponse struct {
	Count    int          `json:"count"`
	Next     *string      `json:"next"`
	Previous *string      `json:"previous"`
	Results  []axentaUser `json:"results"`
}

// fetchUsers загружает все страницы пользователей из Axenta Cloud.
// Использует per_page=1000 (как делал старый unified_users handler).
// Возвращает полный список — caller сохранит в snapshot.
func (s *AxentaSyncService) fetchUsers(token string) ([]axentaUser, error) {
	result := make([]axentaUser, 0)
	// ВАЖНО: ?ordering=-id обязателен. Без ordering Axenta API paginate'ит по
	// какому-то non-id полю и ИСКЛЮЧАЕТ admin-юзеров аккаунтов (у них
	// creation_datetime=null) — на проде потеряно 15 свежих admin'ов.
	// С ordering=-id они идут в page 1 и точно попадают в snapshot.
	nextURL := axentaAPIBase + "/api/cms/users/?per_page=1000&ordering=-id"

	log.Printf("👥 Начинаем загрузку пользователей Axenta...")

	page := 1
	totalCount := 0
	startTime := time.Now()

	for nextURL != "" {
		var response axentaUsersResponse
		var err error
		maxRetries := 3
		for attempt := 1; attempt <= maxRetries; attempt++ {
			err = s.getJSON(nextURL, token, &response)
			if err == nil {
				break
			}
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt) * time.Second)
			}
		}

		if err != nil {
			return nil, fmt.Errorf("ошибка получения пользователей (страница %d после %d попыток): %w", page, maxRetries, err)
		}

		if page == 1 {
			totalCount = response.Count
			log.Printf("   📊 Всего пользователей в Axenta: %d", totalCount)
		}

		result = append(result, response.Results...)
		log.Printf("   ✅ Страница %d: получено %d пользователей (итого %d/%d)",
			page, len(response.Results), len(result), totalCount)

		if response.Next == nil || *response.Next == "" {
			break
		}

		nextURL = s.resolveURL(*response.Next)
		page++
		time.Sleep(200 * time.Millisecond)
	}

	log.Printf("✅ Пользователи Axenta загружены: %d за %v", len(result), time.Since(startTime).Round(time.Second))
	return result, nil
}

// storeUsersWithDB сохраняет пользователей в snapshot-таблицу tenant'а через UPSERT.
func (s *AxentaSyncService) storeUsersWithDB(adminAccountID uint, users []axentaUser, syncedAt time.Time, db *gorm.DB) error {
	log.Printf("💾 Сохранение %d пользователей в БД...", len(users))

	// Diagnostic: какое максимальное external_user_id уже в БД? Чтобы понять
	// сколько свежих юзеров (id > maxExisting) — ожидаемое количество INSERT'ов.
	var maxExistingID int64
	db.Model(&models.AxentaUserSnapshot{}).
		Where("admin_account_id = ?", adminAccountID).
		Select("COALESCE(MAX(external_user_id), 0)").
		Scan(&maxExistingID)
	freshCount := 0
	for _, u := range users {
		if u.ID > maxExistingID {
			freshCount++
		}
	}
	log.Printf("🔍 storeUsersWithDB diag: в БД max external_user_id=%d, в Axenta-batch %d юзеров с id > max (ожидаем INSERT)", maxExistingID, freshCount)

	saved, updated, errs := 0, 0, 0

	for _, u := range users {
		// Приоритет имени: Name > FirstName + LastName > Username
		name := strings.TrimSpace(u.Name)
		if name == "" {
			name = strings.TrimSpace(u.FirstName + " " + u.LastName)
		}
		if name == "" {
			name = u.Username
		}

		raw, _ := json.Marshal(u)
		snapshot := models.AxentaUserSnapshot{
			AdminAccountID:   adminAccountID,
			ExternalUserID:   u.ID,
			Username:         u.Username,
			Name:             name,
			Email:            u.Email,
			AccountType:      u.AccountType,
			IsActive:         u.IsActive,
			CreatorName:      u.CreatorName,
			CreationDatetime: u.CreationDatetime,
			LastSyncedAt:     syncedAt,
			RawPayload:       string(raw),
		}

		if u.LastLogin != "" {
			if t := parseAxentaTime(u.LastLogin); t != nil {
				snapshot.LastLogin = t
			}
		}

		var existing models.AxentaUserSnapshot
		isUpdate := db.Where("admin_account_id = ? AND external_user_id = ?", adminAccountID, u.ID).First(&existing).Error == nil

		isFresh := u.ID > maxExistingID
		// Используем Unscoped, чтобы UPSERT работал и с soft-deleted записями.
		// Без этого: soft-deleted юзер (от cleanup) не восстанавливается ни через
		// First() (default scope скрывает), ни через UPSERT (DoUpdates не трогает
		// deleted_at). В DoUpdates явно сбрасываем deleted_at в NULL.
		result := db.Unscoped().Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "admin_account_id"}, {Name: "external_user_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"username":          snapshot.Username,
				"name":              snapshot.Name,
				"email":             snapshot.Email,
				"account_type":      snapshot.AccountType,
				"is_active":         snapshot.IsActive,
				"creator_name":      snapshot.CreatorName,
				"creation_datetime": snapshot.CreationDatetime,
				"last_login":        snapshot.LastLogin,
				"last_synced_at":    snapshot.LastSyncedAt,
				"raw_payload":       snapshot.RawPayload,
				"deleted_at":        nil,
			}),
		}).Create(&snapshot)
		if result.Error != nil {
			errs++
			log.Printf("   ❌ Ошибка сохранения пользователя %d: %v", u.ID, result.Error)
			continue
		}

		// Diagnostic per-user (только для свежих, чтобы не засорять логи)
		if isFresh {
			log.Printf("   🆕 fresh user id=%d username=%s rowsAffected=%d snapshot.ID=%d isUpdate=%v",
				u.ID, u.Username, result.RowsAffected, snapshot.ID, isUpdate)
		}

		if isUpdate {
			updated++
		} else {
			saved++
		}
	}

	log.Printf("   📊 Пользователи: новых=%d, обновлено=%d, ошибок=%d", saved, updated, errs)
	if errs > 0 {
		return fmt.Errorf("сохранение пользователей с ошибками: %d из %d", errs, len(users))
	}
	return nil
}

func parseAxentaTime(value string) *time.Time {
	if value == "" {
		return nil
	}

	layouts := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05.999999Z",
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
