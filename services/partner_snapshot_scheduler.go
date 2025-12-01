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
	db                 *gorm.DB
	lastSnapshotTime   time.Time
	isRunning          bool
}

// NewPartnerSnapshotScheduler создает новый планировщик снимков
func NewPartnerSnapshotScheduler() *PartnerSnapshotScheduler {
	return &PartnerSnapshotScheduler{
		cron:            cron.New(cron.WithLocation(time.UTC)),
		snapshotService: NewPartnerSnapshotService(),
		db:              database.DB,
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

// createDailySnapshots создает снимки для всех активных партнерских договоров
func (s *PartnerSnapshotScheduler) createDailySnapshots() {
	startTime := time.Now()
	log.Println("📸 Начало создания ежедневных снимков для всех партнерских договоров")

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

