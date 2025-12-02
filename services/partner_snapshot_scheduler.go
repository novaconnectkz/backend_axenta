package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

// PartnerSnapshotScheduler управляет автоматическим созданием ежедневных снимков
type PartnerSnapshotScheduler struct {
	cron               *cron.Cron
	snapshotService    *PartnerSnapshotService
	axentaSyncService  *AxentaSyncService
	db                 *gorm.DB
	lastSnapshotTime   time.Time
	isRunning          bool
}

// NewPartnerSnapshotScheduler создает новый планировщик снимков
func NewPartnerSnapshotScheduler() *PartnerSnapshotScheduler {
	return &PartnerSnapshotScheduler{
		cron:              cron.New(cron.WithLocation(time.UTC)),
		snapshotService:   NewPartnerSnapshotService(),
		axentaSyncService: NewAxentaSyncService(database.DB),
		db:                database.DB,
	}
}

// Start запускает планировщик
func (s *PartnerSnapshotScheduler) Start() error {
	// Запускаем создание снимков каждый день в 00:00 UTC
	_, err := s.cron.AddFunc("0 0 * * *", func() {
		log.Println("🕐 Запуск автоматического создания ежедневных снимков (UTC 00:00)")
		s.createDailySnapshots()
	})
	
	if err != nil {
		return err
	}

	s.cron.Start()
	s.isRunning = true
	
	log.Println("✅ Планировщик ежедневных снимков запущен (каждый день в 00:00 UTC)")
	
	return nil
}

// Stop останавливает планировщик
func (s *PartnerSnapshotScheduler) Stop() {
	if s.cron != nil {
		s.cron.Stop()
		s.isRunning = false
		log.Println("🛑 Планировщик ежедневных снимков остановлен")
	}
}

// createDailySnapshots создает снимки для всех активных партнерских договоров и синхронизирует все аккаунты из Axenta Cloud
func (s *PartnerSnapshotScheduler) createDailySnapshots() {
	startTime := time.Now()
	log.Println("📸 Начало создания ежедневных снимков и синхронизации партнерских аккаунтов")

	// Получаем вчерашнюю дату (снимок создается в 00:00 UTC за предыдущий день)
	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	snapshotDate := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, time.UTC)

	log.Printf("📅 Дата снимка: %s", snapshotDate.Format("2006-01-02"))

	// Создаем запись о задаче в ОСНОВНОЙ БД в схеме public (не tenant)
	mainDB := database.DB.Exec("SET search_path TO public") // Явно устанавливаем схему public
	mainDB = database.DB // Используем основную БД для логов
	job := &models.SnapshotJob{
		JobType:    models.SnapshotJobTypeDailyAuto,
		StartedAt:  startTime,
		DateFrom:   snapshotDate,
		DateTo:     snapshotDate,
		Status:     models.SnapshotJobStatusRunning,
		TriggeredBy: "cron",
		ServerInfo: models.ServerInfo{
			Hostname:  s.getHostname(),
			Version:   "1.0.0", // TODO: получать из конфига
			GoVersion: runtime.Version(),
		},
	}

	// Сохраняем начальное состояние задачи в основную БД
	if err := mainDB.Create(job).Error; err != nil {
		log.Printf("❌ Ошибка создания записи о задаче: %v", err)
		// Продолжаем выполнение даже если не удалось создать запись
	}

	// Получаем все компании (тенанты) из схемы public
	var companies []models.Company
	if err := mainDB.Table("public.companies").Find(&companies).Error; err != nil {
		log.Printf("❌ Ошибка получения списка компаний: %v", err)
		job.FinishJob(models.SnapshotJobStatusFailed, fmt.Sprintf("Ошибка получения списка компаний: %v", err))
		mainDB.Save(job)
		return
	}

	job.TotalCompanies = len(companies)
	totalContracts := 0
	successCount := 0
	errorCount := 0
	skippedCount := 0

	// Для каждой компании (тенанта)
	for _, company := range companies {
		companyStartTime := time.Now()
		companyDetail := models.CompanyJobDetail{
			CompanyID:   company.ID,
			CompanyName: company.DatabaseSchema,
		}

		// Получаем tenant DB по ID компании
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			errMsg := fmt.Sprintf("Не удалось получить DB для тенанта %s (ID=%d)", company.DatabaseSchema, company.ID)
			log.Printf("⚠️ %s", errMsg)
			job.AddError(models.JobError{
				CompanyID:   company.ID,
				Message:     errMsg,
				ErrorType:   "db_error",
				Recoverable: false,
			})
			continue
		}

		// НОВАЯ ЛОГИКА: Синхронизируем ВСЕ партнерские аккаунты из Axenta Cloud для всех активных партнеров
		log.Printf("🔄 Синхронизация всех партнерских аккаунтов для компании %s (ID=%d)...", company.DatabaseSchema, company.ID)
		s.syncAllPartnerAccounts(tenantDB, company.ID)

		// Получаем все активные партнерские договоры
		var contracts []models.Contract
		log.Printf("🔍 Ищем партнерские договоры для компании %s (ID=%d)...", company.DatabaseSchema, company.ID)
		if err := tenantDB.
			Where("contract_type = ? AND status = ?", "partner", "active").
			Find(&contracts).Error; err != nil {
			errMsg := fmt.Sprintf("Ошибка получения договоров для %s: %v", company.DatabaseSchema, err)
			log.Printf("❌ %s", errMsg)
			job.AddError(models.JobError{
				CompanyID:   company.ID,
				Message:     errMsg,
				ErrorType:   "db_error",
				Recoverable: true,
			})
			continue
		}

		log.Printf("📋 Компания %s: найдено %d партнерских договоров", company.DatabaseSchema, len(contracts))
		companyDetail.ContractsCount = len(contracts)
		totalContracts += len(contracts)

		// Для каждого договора создаем снимок
		for _, contract := range contracts {
			contractDetail := models.ContractJobDetail{
				ContractID:     contract.ID,
				ContractNumber: contract.Number,
				CompanyID:      company.ID,
				DaysProcessed:  1, // За один день
			}

			// Получаем токен партнера для доступа к Axenta API
			token, err := s.getPartnerToken(tenantDB, contract.AdminAccountID)
			if err != nil {
				errMsg := fmt.Sprintf("Не удалось получить токен для договора %d: %v", contract.ID, err)
				log.Printf("⚠️ %s", errMsg)
				contractDetail.ErrorCount = 1
				contractDetail.ErrorMessage = errMsg
				job.AddError(models.JobError{
					CompanyID:   company.ID,
					ContractID:  contract.ID,
					Message:     errMsg,
					ErrorType:   "api_error",
					Recoverable: true,
				})
				errorCount++
				companyDetail.ErrorCount++
				job.AddContractDetail(contractDetail)
				continue
			}

			// Создаем снимок
			if err := s.snapshotService.CreateSnapshotForContractWithTokenAndDB(&contract, snapshotDate, token, tenantDB); err != nil {
				if err.Error() == "snapshot already exists" {
					skippedCount++
					contractDetail.SuccessCount = 1 // Считаем существующий снимок как успех
					log.Printf("ℹ️ Снимок для договора %d уже существует, пропускаем", contract.ID)
				} else {
					errMsg := fmt.Sprintf("Ошибка создания снимка для договора %d: %v", contract.ID, err)
					log.Printf("❌ %s", errMsg)
					contractDetail.ErrorCount = 1
					contractDetail.ErrorMessage = err.Error()
					job.AddError(models.JobError{
						CompanyID:   company.ID,
						ContractID:  contract.ID,
						Date:        snapshotDate.Format("2006-01-02"),
						Message:     err.Error(),
						ErrorType:   "api_error",
						Recoverable: true,
					})
					errorCount++
					companyDetail.ErrorCount++
				}
			} else {
				successCount++
				contractDetail.SuccessCount = 1
				companyDetail.SuccessCount++
				log.Printf("✅ Снимок создан для договора %d (компания %s)", contract.ID, company.DatabaseSchema)
			}

			job.AddContractDetail(contractDetail)
		}

		// Сохраняем время обработки компании
		companyDetail.ProcessingTimeS = int(time.Since(companyStartTime).Seconds())
		job.AddCompanyDetail(companyDetail)
	}

	// Обновляем итоговую статистику задачи
	job.TotalContracts = totalContracts
	job.TotalDaysProcessed = successCount + skippedCount // Количество успешно обработанных дней
	job.SuccessCount = successCount
	job.ErrorCount = errorCount

	// Определяем финальный статус
	if errorCount == 0 {
		job.FinishJob(models.SnapshotJobStatusCompleted, "")
	} else if successCount > 0 {
		job.FinishJob(models.SnapshotJobStatusPartial, fmt.Sprintf("Обработано с ошибками: %d из %d", errorCount, totalContracts))
	} else {
		job.FinishJob(models.SnapshotJobStatusFailed, "Все попытки создания снимков завершились с ошибками")
	}

	// Сохраняем финальное состояние задачи в основную БД
	if err := mainDB.Save(job).Error; err != nil {
		log.Printf("❌ Ошибка сохранения финального состояния задачи: %v", err)
	} else {
		log.Printf("✅ Финальное состояние задачи сохранено: ID=%d, статус=%s", job.ID, job.Status)
	}

	duration := time.Since(startTime)
	log.Printf("✅ Создание ежедневных снимков завершено за %v", duration)
	log.Printf("📊 Итого: компаний=%d, договоров=%d, успешно=%d, ошибок=%d, пропущено=%d", 
		len(companies), totalContracts, successCount, errorCount, skippedCount)

	s.lastSnapshotTime = time.Now()
}

// getHostname получает hostname сервера
func (s *PartnerSnapshotScheduler) getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

// getPartnerToken получает токен партнера для доступа к Axenta API
func (s *PartnerSnapshotScheduler) getPartnerToken(db *gorm.DB, adminAccountID uint) (string, error) {
	var token models.UserToken
	if err := db.
		Where("account_id = ?", adminAccountID).
		Order("created_at DESC").
		First(&token).Error; err != nil {
		return "", err
	}
	return token.Token, nil
}

// GetStatus возвращает статус планировщика
func (s *PartnerSnapshotScheduler) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"is_running":         s.isRunning,
		"last_snapshot_time": s.lastSnapshotTime,
		"next_run":          "00:00 UTC daily",
	}
}

// RunManualSnapshot запускает создание снимков вручную (для тестирования)
func (s *PartnerSnapshotScheduler) RunManualSnapshot() {
	log.Println("🧪 Запуск ТЕСТОВОГО создания снимков вручную")
	s.createDailySnapshots()
}

// syncAllPartnerAccounts синхронизирует все партнерские аккаунты из Axenta Cloud для данного тенанта
func (s *PartnerSnapshotScheduler) syncAllPartnerAccounts(tenantDB *gorm.DB, companyID uint) {
	// ПРИОРИТЕТ 1: Используем системный токен из переменной окружения
	systemToken := os.Getenv("AXENTA_ADMIN_TOKEN")
	if systemToken != "" {
		log.Printf("🔑 Используем системный токен AXENTA_ADMIN_TOKEN для загрузки ВСЕХ аккаунтов (компания %d)", companyID)
		
		tempSyncService := NewAxentaSyncService(tenantDB)
		
		if err := s.syncWithSystemToken(tempSyncService, systemToken, companyID); err != nil {
			log.Printf("⚠️ Ошибка синхронизации через системный токен из env (компания %d): %v", companyID, err)
			// Не возвращаемся, попробуем токены из БД
		} else {
			log.Printf("✅ Синхронизация через системный токен завершена успешно (компания %d)", companyID)
			return
		}
	}
	
	// ПРИОРИТЕТ 2: Используем любой действующий токен из БД для загрузки ВСЕХ аккаунтов
	var tokens []models.UserToken
	if err := tenantDB.
		Where("is_active = ? AND expires_at > ?", true, time.Now()).
		Order("expires_at DESC").
		Limit(1).
		Find(&tokens).Error; err != nil {
		log.Printf("⚠️ Ошибка получения токенов из БД для компании %d: %v", companyID, err)
		return
	}
	
	if len(tokens) > 0 {
		token := tokens[0]
		log.Printf("🔑 Используем токен пользователя (AccountID=%d) для загрузки ВСЕХ аккаунтов (компания %d)", token.AccountID, companyID)
		
		tempSyncService := NewAxentaSyncService(tenantDB)
		
		// Используем этот токен для загрузки ВСЕХ данных
		if err := s.syncWithSystemToken(tempSyncService, token.Token, companyID); err != nil {
			log.Printf("⚠️ Ошибка синхронизации через токен из БД (компания %d): %v", companyID, err)
		} else {
			log.Printf("✅ Синхронизация через токен из БД завершена успешно (компания %d)", companyID)
			return
		}
	}

	// Если не нашли токенов, выводим сообщение
	log.Printf("⚠️ Не найдено действующих токенов для синхронизации в компании %d", companyID)
	log.Printf("💡 Для загрузки ВСЕХ объектов:")
	log.Printf("   1. Установите AXENTA_ADMIN_TOKEN в переменные окружения")
	log.Printf("   2. Или убедитесь, что есть действующий токен в настройках Axenta Cloud API")
}

// syncWithSystemToken синхронизирует ВСЕ аккаунты и объекты используя системный токен
func (s *PartnerSnapshotScheduler) syncWithSystemToken(syncService *AxentaSyncService, token string, companyID uint) error {
	log.Printf("🌐 Загрузка ВСЕХ аккаунтов из Axenta Cloud через системный токен...")
	
	// Используем admin_account_id = 0 для системной синхронизации (все аккаунты)
	// Это специальный ID для обозначения системного уровня доступа
	systemAdminID := uint(0)
	
	// Выполняем синхронизацию через существующий метод, передавая токен напрямую
	if err := syncService.syncAdminWithToken(systemAdminID, token); err != nil {
		return fmt.Errorf("ошибка синхронизации: %w", err)
	}
	
	log.Printf("✅ Загружены ВСЕ аккаунты и объекты из Axenta Cloud для компании %d", companyID)
	return nil
}

