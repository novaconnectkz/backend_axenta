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
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// PartnerSnapshotScheduler управляет автоматическим созданием ежедневных снимков
type PartnerSnapshotScheduler struct {
	cron              *cron.Cron
	snapshotService   *PartnerSnapshotService
	axentaSyncService *AxentaSyncService
	db                *gorm.DB
	lastSnapshotTime  time.Time
	isRunning         bool
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
	// Запускаем создание снимков каждый день в 21:20 UTC (00:20 MSK следующего дня)
	_, err := s.cron.AddFunc("20 21 * * *", func() {
		log.Println("🕐 Запуск автоматического создания ежедневных снимков (UTC 21:20 / MSK 00:20)")
		s.createDailySnapshots()
	})

	if err != nil {
		return err
	}

	s.cron.Start()
	s.isRunning = true

	log.Println("✅ Планировщик ежедневных снимков запущен (каждый день в 21:20 UTC / 00:20 MSK)")

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
	// Получаем вчерашнюю дату (снимок создается в 00:00 UTC за предыдущий день)
	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	snapshotDate := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, time.UTC)
	s.createDailySnapshotsForDate(snapshotDate)
}

// createDailySnapshotsForDate создает снимки за указанную дату
func (s *PartnerSnapshotScheduler) createDailySnapshotsForDate(snapshotDate time.Time) {
	startTime := time.Now()
	log.Printf("📸 Начало создания ежедневных снимков за дату: %s", snapshotDate.Format("2006-01-02"))

	log.Printf("📅 Дата снимка: %s", snapshotDate.Format("2006-01-02"))

	// Создаем запись о задаче в ОСНОВНОЙ БД в схеме public (не tenant)
	mainDB := database.DB
	job := &models.SnapshotJob{
		JobType:     models.SnapshotJobTypeDailyAuto,
		StartedAt:   startTime,
		DateFrom:    snapshotDate,
		DateTo:      snapshotDate,
		Status:      models.SnapshotJobStatusRunning,
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

		// ИСПОЛЬЗУЕМ УЖЕ СИНХРОНИЗИРОВАННЫЕ данные (синхронизация происходит отдельно каждую минуту)
		// Синхронизация занимает слишком много времени и блокирует создание снимков
		log.Printf("📊 Используем синхронизированные данные для компании %s (ID=%d)...", company.DatabaseSchema, company.ID)

		// Получаем ВСЕ партнерские аккаунты из snapshot (включая тех у кого НЕТ договоров)
		var allPartnerAccounts []models.AxentaAccountSnapshot
		log.Printf("🔍 Ищем ВСЕ партнерские аккаунты в снимках для компании %s (ID=%d)...", company.DatabaseSchema, company.ID)
		if err := tenantDB.
			Where("account_type = ? OR account_type = ?", "partner", "Partner").
			Find(&allPartnerAccounts).Error; err != nil {
			errMsg := fmt.Sprintf("Ошибка получения партнерских аккаунтов для %s: %v", company.DatabaseSchema, err)
			log.Printf("❌ %s", errMsg)
			job.AddError(models.JobError{
				CompanyID:   company.ID,
				Message:     errMsg,
				ErrorType:   "db_error",
				Recoverable: true,
			})
			continue
		}

		log.Printf("📋 Компания %s: найдено %d партнерских аккаунтов в Axenta Cloud", company.DatabaseSchema, len(allPartnerAccounts))

		// Если нет партнеров, пропускаем компанию
		if len(allPartnerAccounts) == 0 {
			log.Printf("ℹ️ Компания %s: нет партнерских аккаунтов, пропускаем", company.DatabaseSchema)
			companyDetail.ProcessingTimeS = int(time.Since(companyStartTime).Seconds())
			job.AddCompanyDetail(companyDetail)
			continue
		}

		log.Printf("🔑 Получаем токен для доступа к Axenta API (компания %d)...", company.ID)
		// Получаем токен для доступа к Axenta API
		token, err := s.getAnyActiveToken(tenantDB, company.ID)
		if err != nil {
			errMsg := fmt.Sprintf("Не удалось получить токен для компании %d: %v", company.ID, err)
			log.Printf("⚠️ %s", errMsg)
			job.AddError(models.JobError{
				CompanyID:   company.ID,
				Message:     errMsg,
				ErrorType:   "api_error",
				Recoverable: true,
			})
			continue
		}
		// Показываем информацию о токене (первые 10 символов для безопасности)
		tokenPreview := token
		if len(tokenPreview) > 10 {
			tokenPreview = tokenPreview[:10] + "..."
		}
		log.Printf("✅ Токен получен для компании %d: %s (длина: %d символов)", company.ID, tokenPreview, len(token))
		
		// Получаем общую статистику объектов из /stats/ (как на странице /objects)
		totalFromStats, activeFromStats, statsErr := s.snapshotService.getTotalObjectsFromStats(token)
		if statsErr != nil {
			log.Printf("⚠️ Не удалось получить статистику из /stats/ для компании %d: %v", company.ID, statsErr)
			// Продолжаем без этой статистики
		} else {
			log.Printf("📊 Компания %d: общая статистика из /stats/: всего=%d, активных=%d (как на /objects)", 
				company.ID, totalFromStats, activeFromStats)
		}
		
		// Сохраняем реальное количество объектов из API для проверки
		companyDetail.RealTotalObjects = totalFromStats
		companyDetail.RealActiveObjects = activeFromStats
		
		// Загружаем иерархию всех аккаунтов и распределяем объекты БЕЗ ДУБЛЕЙ
		log.Printf("🌳 Загружаем иерархию всех аккаунтов...")
		hierarchyService := NewAccountHierarchyService(token)
		if err := hierarchyService.LoadAllAccounts(); err != nil {
			log.Printf("⚠️ Не удалось загрузить иерархию аккаунтов: %v", err)
			continue
		}
		
		// Загружаем ВСЕ объекты и распределяем их по партнёрам без дублей
		log.Printf("📦 Загружаем ВСЕ объекты и распределяем по партнёрам (без дублей)...")
		partnerObjectsMap, err := hierarchyService.DistributeObjectsByPartner(token, snapshotDate)
		if err != nil {
			log.Printf("⚠️ Не удалось загрузить и распределить объекты: %v", err)
			continue
		}
		
		// Выводим статистику
		totalObjects := 0
		totalActive := 0
		for _, stats := range partnerObjectsMap {
			totalObjects += stats.TotalObjects
			totalActive += stats.ActiveObjects
		}
		log.Printf("📊 Распределено %d объектов по %d партнёрам (активных: %d)", 
			totalObjects, len(partnerObjectsMap), totalActive)

		log.Printf("📋 Загружаем существующие договоры для маппинга (компания %d)...", company.ID)
		// Получаем существующие договоры для маппинга
		var contracts []models.Contract
		contractsByPartnerID := make(map[int64]*models.Contract)
		if err := tenantDB.
			Where("contract_type = ? AND status = ?", "partner", "active").
			Find(&contracts).Error; err == nil {
			for i := range contracts {
				if contracts[i].PartnerCompanyID != nil && *contracts[i].PartnerCompanyID > 0 {
					contractsByPartnerID[int64(*contracts[i].PartnerCompanyID)] = &contracts[i]
				}
			}
			log.Printf("✅ Найдено %d партнерских договоров для маппинга", len(contracts))
		} else {
			log.Printf("⚠️ Ошибка загрузки договоров: %v", err)
		}

		// Получаем дефолтный тарифный план для партнеров без договора
		var defaultPlan models.BillingPlan
		if err := tenantDB.
			Where("is_active = ? AND admin_account_id = ?", true, company.ID).
			Order("created_at DESC").
			First(&defaultPlan).Error; err != nil {
			log.Printf("⚠️ Не найден тарифный план для компании %d: %v", company.ID, err)
			// Создадим временный план с базовыми параметрами
			adminAccountIDPtr := &company.ID
			defaultPlan = models.BillingPlan{
				Name:           "Базовый партнерский план",
				Price:          decimal.NewFromFloat(70),
				BillingPeriod:  "monthly",
				AdminAccountID: *adminAccountIDPtr,
			}
		}

		companyDetail.ContractsCount = len(allPartnerAccounts) // Считаем все партнерские аккаунты
		totalContracts += len(allPartnerAccounts)

		log.Printf("🚀 Начинаем создание снимков для %d партнеров (компания %d)...", len(partnerObjectsMap), company.ID)
		// Для каждого партнера из распределённых объектов создаём снимок
		processedCount := 0
		for partnerID, objectStats := range partnerObjectsMap {
			processedCount++
			if processedCount%10 == 0 {
				log.Printf("📊 Прогресс: обработано %d из %d партнеров", processedCount, len(partnerObjectsMap))
			}
			
			contractDetail := models.ContractJobDetail{
				CompanyID:     company.ID,
				DaysProcessed: 1,
			}

			// Проверяем есть ли договор для этого партнера
			contract, hasContract := contractsByPartnerID[partnerID]
			
			// Определяем название для партнёра ID=186
			partnerName := ""
			if partnerID == 186 {
				partnerName = "Объекты наших клиентов"
				log.Printf("📄 GLOMOS (186 - %s): %d активных объектов", partnerName, objectStats.ActiveObjects)
			}
			
			// Создаём снимок напрямую с данными из распределения
			createErr := s.snapshotService.CreateSnapshotWithObjectCounts(
				company.ID,
				uint(partnerID),
				objectStats.TotalObjects,
				objectStats.ActiveObjects,
				snapshotDate,
				&defaultPlan,
				contract,
				partnerName,
				tenantDB,
			)

			if createErr != nil {
				if createErr.Error() == "snapshot already exists" {
					skippedCount++
					contractDetail.SuccessCount = 1
					log.Printf("ℹ️ Снимок для партнера %d уже существует, пропускаем", partnerID)
				} else {
					errMsg := fmt.Sprintf("Ошибка создания снимка для партнера %d: %v", partnerID, createErr)
					log.Printf("❌ %s", errMsg)
					contractDetail.ErrorCount = 1
					contractDetail.ErrorMessage = createErr.Error()
					errorCount++
					companyDetail.ErrorCount++
				}
			} else {
				successCount++
				contractDetail.SuccessCount = 1
				companyDetail.SuccessCount++
				if hasContract {
					log.Printf("✅ Снимок создан для партнера %d (договор %s): %d/%d объектов", 
						partnerID, contract.Number, objectStats.ActiveObjects, objectStats.TotalObjects)
				} else if partnerID == 186 {
					log.Printf("✅ Снимок создан для GLOMOS (186 - %s): %d/%d объектов", 
						partnerName, objectStats.ActiveObjects, objectStats.TotalObjects)
				} else {
					log.Printf("✅ Снимок создан для партнера %d (без договора): %d/%d объектов", 
						partnerID, objectStats.ActiveObjects, objectStats.TotalObjects)
				}
			}

			if hasContract {
				contractDetail.ContractID = contract.ID
				contractDetail.ContractNumber = contract.Number
			}
			job.AddContractDetail(contractDetail)
		}
		
		companyDetail.ContractsCount = len(partnerObjectsMap)
		totalContracts += len(partnerObjectsMap)

		// НЕ создаём снимок для GLOMOS (186), потому что:
		// 1. API запрос ?accountId=партнёр УЖЕ включает всех дочерних клиентов
		// 2. Прямые клиенты GLOMOS тоже включены в партнёрские запросы
		// 3. Создание отдельного снимка приведёт к двойному подсчёту
		
		if hierarchyService != nil {
			directClients := hierarchyService.GetDirectGLOMOSClients()
			if len(directClients) > 0 {
				directTotal := 0
				directActive := 0
				for _, client := range directClients {
					directTotal += client.ObjectsTotal
					directActive += client.ObjectsActive
				}
				log.Printf("ℹ️ Прямых клиентов GLOMOS: %d аккаунтов, %d активных объектов (УЖЕ учтены в партнёрских снимках)", 
					len(directClients), directActive)
			}
		}
		
		// Проверяем совпадение с общей статистикой из /stats/
		if totalFromStats > 0 {
			var sumTotal, sumActive int64
			if err := tenantDB.Model(&models.PartnerDailySnapshot{}).
				Where("DATE(snapshot_date AT TIME ZONE 'UTC') = ?", snapshotDate.Format("2006-01-02")).
				Select("COALESCE(SUM(total_objects_count), 0) as sum_total, COALESCE(SUM(active_objects_count), 0) as sum_active").
				Row().Scan(&sumTotal, &sumActive); err == nil {
				
				diffTotal := totalFromStats - int(sumTotal)
				diffActive := activeFromStats - int(sumActive)
				
				log.Printf("📊 Итоговое сравнение:")
				log.Printf("   - Общая статистика (/objects): %d всего, %d активных", totalFromStats, activeFromStats)
				log.Printf("   - Сумма партнёрских снимков:   %d всего, %d активных", int(sumTotal), int(sumActive))
				log.Printf("   - Расхождение:                 %d всего, %d активных", diffTotal, diffActive)
				
				if diffTotal == 0 && diffActive == 0 {
					log.Printf("✅ Идеальное совпадение! Все объекты учтены.")
				} else if diffTotal < 0 || diffActive < 0 {
					log.Printf("⚠️ Сумма снимков БОЛЬШЕ чем общая статистика (двойной подсчёт из-за иерархии)")
				} else {
					log.Printf("⚠️ Есть объекты не учтённые в партнёрских снимках")
				}
			}
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

	// Подсчитываем общее количество объектов из РЕАЛЬНО созданных снимков
	// Это важно, так как некоторые снимки могли быть пропущены или созданы с другими данными
	var totalObjectsSum, activeObjectsSum int64
	var realTotalFromAPI, realActiveFromAPI int
	
	// Сначала пытаемся получить реальное количество из API (из CompanyJobDetail)
	for _, companyDetail := range job.Details.Companies {
		if companyDetail.RealTotalObjects > 0 {
			realTotalFromAPI = companyDetail.RealTotalObjects
			realActiveFromAPI = companyDetail.RealActiveObjects
			log.Printf("📊 Реальное количество из API: %d объектов (активных: %d)", realTotalFromAPI, realActiveFromAPI)
			break
		}
	}
	
	// Подсчитываем из созданных снимков
	for _, company := range companies {
		tenantDB := database.DB.Session(&gorm.Session{})
		if err := tenantDB.Exec(fmt.Sprintf("SET search_path TO tenant_%d, public", company.ID)).Error; err != nil {
			log.Printf("⚠️ Не удалось переключиться на схему tenant_%d для подсчета объектов: %v", company.ID, err)
			continue
		}
		
		var companyTotal, companyActive int64
		if err := tenantDB.Model(&models.PartnerDailySnapshot{}).
			Where("DATE(snapshot_date AT TIME ZONE 'UTC') = ? AND deleted_at IS NULL", snapshotDate.Format("2006-01-02")).
			Select("COALESCE(SUM(total_objects_count), 0) as total, COALESCE(SUM(active_objects_count), 0) as active").
			Row().Scan(&companyTotal, &companyActive); err == nil {
			totalObjectsSum += companyTotal
			activeObjectsSum += companyActive
			log.Printf("📊 Компания %d: %d объектов (активных: %d) из созданных снимков", company.ID, companyTotal, companyActive)
		} else {
			log.Printf("⚠️ Ошибка подсчета объектов для компании %d: %v", company.ID, err)
		}
	}
	
	// Используем реальное количество из API, если оно доступно и отличается от суммы снимков
	// Это более точное значение, так как API возвращает точное количество объектов
	if realTotalFromAPI > 0 {
		// Если разница больше 5%, используем значение из API
		diffPercent := float64(totalObjectsSum-int64(realTotalFromAPI)) / float64(realTotalFromAPI) * 100
		if diffPercent > 5 || diffPercent < -5 {
			log.Printf("⚠️ Расхождение между API (%d) и суммой снимков (%d): %.1f%%. Используем значение из API.", 
				realTotalFromAPI, totalObjectsSum, diffPercent)
			job.TotalObjects = realTotalFromAPI
			job.ActiveObjects = realActiveFromAPI
		} else {
			log.Printf("✅ Расхождение в пределах нормы (%.1f%%). Используем сумму из снимков.", diffPercent)
			job.TotalObjects = int(totalObjectsSum)
			job.ActiveObjects = int(activeObjectsSum)
		}
	} else {
		// Если нет данных из API, используем сумму из снимков
		job.TotalObjects = int(totalObjectsSum)
		job.ActiveObjects = int(activeObjectsSum)
	}
	
	job.SkippedCount = skippedCount
	
	log.Printf("📊 ИТОГО: %d объектов (активных: %d) - %s", 
		job.TotalObjects, job.ActiveObjects, 
		map[bool]string{true: "из API", false: "из снимков"}[realTotalFromAPI > 0 && (job.TotalObjects == realTotalFromAPI)])
	
	// Устанавливаем scheduled_time для автоматических задач (21:00 UTC / 00:00 MSK следующего дня)
	// Снимок за 01.12.2025 создаётся в 00:00 MSK 02.12.2025 (т.е. в 21:00 UTC 01.12.2025)
	if job.JobType == models.SnapshotJobTypeDailyAuto {
		// scheduled_time = дата снимка + 1 день в 00:00 MSK = дата снимка в 21:00 UTC
		scheduledTime := time.Date(snapshotDate.Year(), snapshotDate.Month(), snapshotDate.Day(), 21, 0, 0, 0, time.UTC)
		job.ScheduledTime = &scheduledTime
	}
	
	// Сохраняем финальное состояние задачи в основную БД
	if err := mainDB.Save(job).Error; err != nil {
		log.Printf("❌ Ошибка сохранения финального состояния задачи: %v", err)
	} else {
		log.Printf("✅ Финальное состояние задачи сохранено: ID=%d, статус=%s, объектов=%d/%d", 
			job.ID, job.Status, job.ActiveObjects, job.TotalObjects)
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

// getAnyActiveToken получает любой активный токен для доступа к Axenta API
func (s *PartnerSnapshotScheduler) getAnyActiveToken(db *gorm.DB, companyID uint) (string, error) {
	// ПРИОРИТЕТ 1: Пробуем токен из настроек снимков (для суперадмина, ID=1)
	const superAdminCompanyID = 1
	superAdminTenantDB := database.GetTenantDBByID(superAdminCompanyID)
	if superAdminTenantDB != nil {
		var snapshotSettings models.SnapshotSettings
		if err := superAdminTenantDB.
			Where("company_id = ? AND is_active = ?", superAdminCompanyID, true).
			First(&snapshotSettings).Error; err == nil {
			if snapshotSettings.AxentaToken != "" {
				tokenPreview := snapshotSettings.AxentaToken
				if len(tokenPreview) > 10 {
					tokenPreview = tokenPreview[:10] + "..."
				}
				log.Printf("🔑 [Компания %d] Используем токен из настроек снимков: %s (длина: %d)", 
					companyID, tokenPreview, len(snapshotSettings.AxentaToken))
				return snapshotSettings.AxentaToken, nil
			} else {
				log.Printf("⚠️ [Компания %d] Настройки снимков найдены, но токен пустой", companyID)
			}
		} else {
			log.Printf("⚠️ [Компания %d] Настройки снимков не найдены: %v", companyID, err)
		}
	} else {
		log.Printf("⚠️ [Компания %d] Не удалось получить tenant DB для суперадмина (ID=1)", companyID)
	}

	// ПРИОРИТЕТ 2: Пробуем системный токен из env
	systemToken := os.Getenv("AXENTA_ADMIN_TOKEN")
	if systemToken != "" {
		tokenPreview := systemToken
		if len(tokenPreview) > 10 {
			tokenPreview = tokenPreview[:10] + "..."
		}
		log.Printf("🔑 [Компания %d] Используем системный токен из переменной окружения AXENTA_ADMIN_TOKEN: %s", 
			companyID, tokenPreview)
		return systemToken, nil
	} else {
		log.Printf("⚠️ [Компания %d] Системный токен AXENTA_ADMIN_TOKEN не установлен", companyID)
	}

	// ПРИОРИТЕТ 3: Берем любой активный токен из БД текущего тенанта
	var token models.UserToken
	if err := db.
		Where("is_active = ? AND expires_at > ?", true, time.Now()).
		Order("updated_at DESC").
		First(&token).Error; err != nil {
		log.Printf("❌ [Компания %d] Не найдено активных токенов в user_tokens: %v", companyID, err)
		return "", fmt.Errorf("не найдено активных токенов: %w", err)
	}
	tokenPreview := token.Token
	if len(tokenPreview) > 10 {
		tokenPreview = tokenPreview[:10] + "..."
	}
	log.Printf("🔑 [Компания %d] Используем токен из user_tokens (account_id=%d, username=%s): %s", 
		companyID, token.AccountID, token.Username, tokenPreview)
	return token.Token, nil
}

// GetStatus возвращает статус планировщика
func (s *PartnerSnapshotScheduler) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"is_running":         s.isRunning,
		"last_snapshot_time": s.lastSnapshotTime,
		"next_run":           "00:00 UTC daily",
	}
}

// RunManualSnapshot запускает создание снимков вручную (для тестирования)
func (s *PartnerSnapshotScheduler) RunManualSnapshot() {
	log.Println("🧪 Запуск ТЕСТОВОГО создания снимков вручную")
	s.createDailySnapshots()
}

// RunManualSnapshotForDate создает снимки за указанную дату (для ручного запуска)
func (s *PartnerSnapshotScheduler) RunManualSnapshotForDate(targetDate time.Time) {
	log.Printf("🧪 Запуск ТЕСТОВОГО создания снимков вручную за дату: %s", targetDate.Format("2006-01-02"))
	s.createDailySnapshotsForDate(targetDate)
}

// RunManualSnapshotForPeriod создает снимки за указанный период (для ручного запуска)
func (s *PartnerSnapshotScheduler) RunManualSnapshotForPeriod(dateFrom time.Time, dateTo time.Time) {
	log.Printf("🧪 Запуск создания снимков вручную за период: %s - %s", 
		dateFrom.Format("2006-01-02"), dateTo.Format("2006-01-02"))
	
	// Итерируемся по всем дням в периоде
	currentDate := dateFrom
	for !currentDate.After(dateTo) {
		s.createDailySnapshotsForDate(currentDate)
		// Переходим к следующему дню
		currentDate = currentDate.AddDate(0, 0, 1)
	}
	
	log.Printf("✅ Создание снимков за период завершено")
}

// syncAllPartnerAccounts синхронизирует все партнерские аккаунты из Axenta Cloud для данного тенанта
func (s *PartnerSnapshotScheduler) syncAllPartnerAccounts(tenantDB *gorm.DB, companyID uint) error {
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
			return nil
		}
	}

	// ПРИОРИТЕТ 2: Используем любой действующий токен из БД для загрузки ВСЕХ аккаунтов
	var tokens []models.UserToken
	if err := tenantDB.
		Where("is_active = ? AND expires_at > ?", true, time.Now()).
		Order("expires_at DESC").
		Limit(1).
		Find(&tokens).Error; err != nil {
		errMsg := fmt.Sprintf("ошибка получения токенов из БД для компании %d: %v", companyID, err)
		log.Printf("⚠️ %s", errMsg)
		return fmt.Errorf(errMsg)
	}

	if len(tokens) > 0 {
		token := tokens[0]
		log.Printf("🔑 Используем токен пользователя (AccountID=%d) для загрузки ВСЕХ аккаунтов (компания %d)", token.AccountID, companyID)

		tempSyncService := NewAxentaSyncService(tenantDB)

		// Используем этот токен для загрузки ВСЕХ данных
		if err := s.syncWithSystemToken(tempSyncService, token.Token, companyID); err != nil {
			errMsg := fmt.Sprintf("ошибка синхронизации через токен из БД (компания %d): %v", companyID, err)
			log.Printf("⚠️ %s", errMsg)
			return fmt.Errorf(errMsg)
		} else {
			log.Printf("✅ Синхронизация через токен из БД завершена успешно (компания %d)", companyID)
			return nil
		}
	}

	// Если не нашли токенов, возвращаем ошибку
	errMsg := fmt.Sprintf("не найдено действующих токенов для синхронизации в компании %d", companyID)
	log.Printf("⚠️ %s", errMsg)
	log.Printf("💡 Для загрузки ВСЕХ объектов:")
	log.Printf("   1. Установите AXENTA_ADMIN_TOKEN в переменные окружения")
	log.Printf("   2. Или убедитесь, что есть действующий токен в настройках Axenta Cloud API")
	return fmt.Errorf(errMsg)
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
