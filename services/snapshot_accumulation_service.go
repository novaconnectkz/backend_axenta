package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SnapshotAccumulationService реализует новый подход к загрузке снимков:
// 1. Разовая загрузка всех текущих объектов из Axenta в БД
// 2. Определение даты первого объекта (axenta_created_at) - это стартовая дата биллинга
// 3. Ежедневное накопление: каждый день добавляются только новые объекты, созданные в этот день
// 4. Автоматическое суммирование: система сама вычисляет общее количество объектов на каждую дату
type SnapshotAccumulationService struct {
	db *gorm.DB
}

// NewSnapshotAccumulationService создает новый сервис накопительных снимков
func NewSnapshotAccumulationService() *SnapshotAccumulationService {
	return &SnapshotAccumulationService{
		db: database.DB,
	}
}

// LoadProgressCallback функция для обновления прогресса загрузки
// Используем тот же тип, что и ProgressCallback из axenta_sync_service.go
type LoadProgressCallback func(loaded int, total int, currentPage int, totalPages int, status string)

// LoadAllCurrentObjects выполняет разовую загрузку всех текущих объектов из Axenta в БД
// Использует существующий сервис синхронизации Axenta
func (s *SnapshotAccumulationService) LoadAllCurrentObjects(token string) error {
	return s.LoadAllCurrentObjectsWithProgress(token, nil)
}

// LoadAllCurrentObjectsWithProgress выполняет разовую загрузку всех текущих объектов с отслеживанием прогресса
func (s *SnapshotAccumulationService) LoadAllCurrentObjectsWithProgress(token string, progressCallback LoadProgressCallback) error {
	log.Printf("🔄 Начинаем разовую загрузку всех текущих объектов из Axenta...")

	// Используем существующий сервис синхронизации
	syncService := NewAxentaSyncService(s.db)

	// Получаем первую компанию для определения admin_account_id
	var firstCompany models.Company
	if err := s.db.Table("public.companies").Where("is_active = ?", true).Order("id ASC").First(&firstCompany).Error; err != nil {
		return fmt.Errorf("не найдено активных компаний: %w", err)
	}

	// Получаем tenant DB для первой компании
	tenantDB := database.GetTenantDBByID(firstCompany.ID)
	if tenantDB == nil {
		return fmt.Errorf("не удалось получить tenant DB для компании %d", firstCompany.ID)
	}

	// Используем admin_account_id = 1 (глобальный) или ID первой компании
	adminAccountID := uint(1)
	if firstCompany.ID > 0 {
		adminAccountID = firstCompany.ID
	}

	// Синхронизируем все объекты с отслеживанием прогресса
	// Преобразуем LoadProgressCallback в ProgressCallback (типы совместимы, просто разные имена)
	var syncProgressCallback ProgressCallback
	if progressCallback != nil {
		// Создаем обертку для преобразования типа
		syncProgressCallback = func(loaded int, total int, currentPage int, totalPages int, status string) {
			progressCallback(loaded, total, currentPage, totalPages, status)
		}
	}
	if err := syncService.syncAdminWithTokenAndDBAndProgress(adminAccountID, token, tenantDB, syncProgressCallback); err != nil {
		return fmt.Errorf("ошибка синхронизации объектов: %w", err)
	}

	log.Printf("✅ Разовая загрузка всех текущих объектов завершена")
	return nil
}

// GetBillingStartDate определяет стартовую дату биллинга на основе самого раннего axenta_created_at
// Ищет самый первый созданный объект в БД и использует дату его создания как начало биллинга
func (s *SnapshotAccumulationService) GetBillingStartDate(tenantDB *gorm.DB) (*time.Time, *models.AxentaObjectSnapshot, error) {
	log.Printf("📅 Определяем стартовую дату биллинга из БД...")

	// Сначала проверяем общее количество объектов в БД
	var totalCount int64
	if err := tenantDB.Model(&models.AxentaObjectSnapshot{}).Count(&totalCount).Error; err != nil {
		return nil, nil, fmt.Errorf("ошибка подсчета объектов в БД: %w", err)
	}

	log.Printf("📊 Всего объектов в БД: %d", totalCount)

	if totalCount == 0 {
		log.Printf("⚠️ В БД нет объектов, используем текущую дату как стартовую")
		now := time.Now().UTC()
		nowStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		// Записываем в историю с нулевым количеством объектов
		if err := s.SaveBillingStartDateToHistory(tenantDB, nowStart, 0, 0, nil); err != nil {
			log.Printf("❌ Ошибка записи в историю (нет объектов): %v", err)
		}
		return &nowStart, nil, nil
	}

	// Ищем самый первый созданный объект в БД (не удаленный)
	// Приоритет: объекты с axenta_created_at, которые не удалены
	var earliestObject models.AxentaObjectSnapshot
	query := tenantDB.
		Where("axenta_created_at IS NOT NULL").
		Where("(axenta_deleted_at IS NULL OR axenta_deleted_at > axenta_created_at)").
		Order("axenta_created_at ASC").
		Limit(1)

	if err := query.First(&earliestObject).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// Если не нашли объекты с axenta_created_at, пробуем найти любые объекты
			log.Printf("⚠️ Не найдено объектов с axenta_created_at, ищем любые объекты...")

			var anyObject models.AxentaObjectSnapshot
			if err := tenantDB.Order("created_at ASC").First(&anyObject).Error; err != nil {
				log.Printf("⚠️ В БД нет объектов, используем текущую дату")
				now := time.Now().UTC()
				nowStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
				// Записываем в историю с нулевым количеством объектов
				if err := s.SaveBillingStartDateToHistory(tenantDB, nowStart, 0, 0, nil); err != nil {
					log.Printf("❌ Ошибка записи в историю (нет объектов): %v", err)
				}
				return &nowStart, nil, nil
			}

			// Используем дату создания записи в БД как fallback
			log.Printf("⚠️ Используем дату создания записи в БД: %s", anyObject.CreatedAt.Format("2006-01-02"))
			startDate := time.Date(
				anyObject.CreatedAt.Year(),
				anyObject.CreatedAt.Month(),
				anyObject.CreatedAt.Day(),
				0, 0, 0, 0,
				time.UTC,
			)
			// Подсчитываем количество объектов на стартовую дату
			totalObjects, activeObjects, _ := s.CalculateObjectsCountForDate(startDate, tenantDB)
			// Записываем в историю (без информации о первом объекте, так как используем fallback)
			if err := s.SaveBillingStartDateToHistory(tenantDB, startDate, totalObjects, activeObjects, &anyObject); err != nil {
				log.Printf("❌ Ошибка записи в историю (fallback): %v", err)
			}
			return &startDate, &anyObject, nil
		}
		return nil, nil, fmt.Errorf("ошибка поиска самого раннего объекта: %w", err)
	}

	if earliestObject.AxentaCreatedAt == nil {
		log.Printf("⚠️ У найденного объекта нет axenta_created_at, используем текущую дату")
		now := time.Now().UTC()
		nowStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		// Подсчитываем количество объектов и записываем в историю
		totalObjects, activeObjects, _ := s.CalculateObjectsCountForDate(nowStart, tenantDB)
		if err := s.SaveBillingStartDateToHistory(tenantDB, nowStart, totalObjects, activeObjects, &earliestObject); err != nil {
			log.Printf("❌ Ошибка записи в историю (нет axenta_created_at): %v", err)
		}
		return &nowStart, &earliestObject, nil
	}

	// Округляем до начала дня
	startDate := time.Date(
		earliestObject.AxentaCreatedAt.Year(),
		earliestObject.AxentaCreatedAt.Month(),
		earliestObject.AxentaCreatedAt.Day(),
		0, 0, 0, 0,
		time.UTC,
	)

	log.Printf("✅ Найден самый первый объект в БД:")
	log.Printf("   - ID объекта: %d", earliestObject.ExternalObjectID)
	log.Printf("   - Название: %s", earliestObject.ObjectName)
	log.Printf("   - Дата создания в Axenta: %s", earliestObject.AxentaCreatedAt.Format("2006-01-02 15:04:05"))
	if earliestObject.AxentaDeletedAt != nil {
		log.Printf("   - Дата удаления: %s", earliestObject.AxentaDeletedAt.Format("2006-01-02 15:04:05"))
	}
	log.Printf("   - Стартовая дата биллинга: %s", startDate.Format("2006-01-02"))

	// Подсчитываем количество объектов на стартовую дату
	totalObjects, activeObjects, err := s.CalculateObjectsCountForDate(startDate, tenantDB)
	if err != nil {
		log.Printf("⚠️ Ошибка подсчета объектов на стартовую дату: %v", err)
		// Продолжаем без записи в историю, но возвращаем дату
		return &startDate, &earliestObject, nil
	}

	log.Printf("📊 Количество объектов на стартовую дату: всего=%d, активных=%d", totalObjects, activeObjects)

	// Записываем в историю автоматических снимков
	if err := s.SaveBillingStartDateToHistory(tenantDB, startDate, totalObjects, activeObjects, &earliestObject); err != nil {
		log.Printf("❌ ОШИБКА записи стартовой даты биллинга в историю: %v", err)
		log.Printf("   Попробуйте вызвать endpoint еще раз или проверьте логи БД")
		// Не прерываем выполнение, просто логируем ошибку
	} else {
		log.Printf("✅ Стартовая дата биллинга успешно записана в историю снимков")
	}

	return &startDate, &earliestObject, nil
}

// GetBillingStartDateSimple возвращает только стартовую дату (для обратной совместимости)
func (s *SnapshotAccumulationService) GetBillingStartDateSimple(tenantDB *gorm.DB) (*time.Time, error) {
	startDate, _, err := s.GetBillingStartDate(tenantDB)
	return startDate, err
}

// SaveBillingStartDateToHistory сохраняет стартовую дату биллинга в таблицу истории автоматических снимков
// ВАЖНО: Таблица snapshot_jobs находится в схеме public, а не в tenant схеме
func (s *SnapshotAccumulationService) SaveBillingStartDateToHistory(
	tenantDB *gorm.DB, // Используется только для подсчета объектов, запись идет в public схему
	billingStartDate time.Time,
	totalObjects int,
	activeObjects int,
	earliestObject *models.AxentaObjectSnapshot,
) error {
	log.Printf("📝 Сохранение стартовой даты биллинга в историю снимков (схема public)...")
	log.Printf("   Дата: %s, Объектов: всего=%d, активных=%d", billingStartDate.Format("2006-01-02"), totalObjects, activeObjects)

	// Таблица snapshot_jobs находится в схеме public (глобальная)
	// Переключаемся на схему public для записи
	publicDB := database.DB.Session(&gorm.Session{})
	if err := publicDB.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("❌ Ошибка переключения на схему public: %v", err)
		return fmt.Errorf("ошибка переключения на схему public: %w", err)
	}
	log.Printf("   ✅ Переключились на схему public")

	// Проверяем, не записана ли уже эта дата
	// Используем сравнение дат без времени для точного поиска
	var existingJob models.SnapshotJob
	dateStr := billingStartDate.Format("2006-01-02")
	checkQuery := publicDB.
		Where("job_type = ?", models.SnapshotJobTypeBillingStart).
		Where("date_from::date = ?::date", dateStr)

	log.Printf("   Проверяем существующую запись для даты: %s", dateStr)
	if err := checkQuery.First(&existingJob).Error; err == nil {
		log.Printf("ℹ️ Стартовая дата биллинга уже записана в историю (ID: %d, дата: %s)",
			existingJob.ID, existingJob.DateFrom.Format("2006-01-02"))
		// Обновляем существующую запись
		existingJob.TotalObjects = totalObjects
		existingJob.ActiveObjects = activeObjects
		existingJob.FinishJob(models.SnapshotJobStatusCompleted, "")
		if err := publicDB.Save(&existingJob).Error; err != nil {
			log.Printf("❌ Ошибка обновления существующей записи: %v", err)
			return fmt.Errorf("ошибка обновления записи истории: %w", err)
		}
		log.Printf("✅ Обновлена запись истории снимков (ID: %d)", existingJob.ID)
		return nil
	} else if err != gorm.ErrRecordNotFound {
		log.Printf("⚠️ Ошибка при проверке существующей записи: %v (тип ошибки: %T)", err, err)
		// Продолжаем создание новой записи
	} else {
		log.Printf("   ✅ Запись для даты %s не найдена, создаем новую", dateStr)
	}

	// Создаем новую запись
	now := time.Now()
	// Убеждаемся, что даты установлены правильно (без времени, только дата)
	dateFromOnly := time.Date(billingStartDate.Year(), billingStartDate.Month(), billingStartDate.Day(), 0, 0, 0, 0, time.UTC)
	dateToOnly := time.Date(billingStartDate.Year(), billingStartDate.Month(), billingStartDate.Day(), 0, 0, 0, 0, time.UTC)

	job := models.SnapshotJob{
		JobType:            models.SnapshotJobTypeBillingStart,
		StartedAt:          now,
		DateFrom:           dateFromOnly,
		DateTo:             dateToOnly, // Для стартовой даты from и to одинаковые
		Status:             models.SnapshotJobStatusCompleted,
		TotalObjects:       totalObjects,
		ActiveObjects:      activeObjects,
		SuccessCount:       1, // Одна дата обработана
		TotalDaysProcessed: 1,
		TriggeredBy:        "system",
	}

	log.Printf("   Подготовка записи: DateFrom=%s, DateTo=%s (обе даты должны быть одинаковыми)",
		job.DateFrom.Format("2006-01-02"), job.DateTo.Format("2006-01-02"))

	// Завершаем задачу сразу (она уже выполнена)
	job.FinishJob(models.SnapshotJobStatusCompleted, "")

	log.Printf("   Подготовлена запись: JobType=%s, DateFrom=%s, Status=%s, TotalObjects=%d, ActiveObjects=%d, DurationSeconds=%v",
		job.JobType, job.DateFrom.Format("2006-01-02"), job.Status, job.TotalObjects, job.ActiveObjects, job.DurationSeconds)

	// Добавляем информацию о первом объекте в детали
	if earliestObject != nil {
		job.Details = models.SnapshotJobDetails{
			Errors: []models.JobError{
				{
					Timestamp: now,
					Message: fmt.Sprintf("Стартовая дата биллинга определена на основе первого объекта: ID=%d, Name=%s, CreatedAt=%s",
						earliestObject.ExternalObjectID,
						earliestObject.ObjectName,
						earliestObject.AxentaCreatedAt.Format("2006-01-02 15:04:05")),
					ErrorType: "info", // Используем как информационное сообщение
				},
			},
		}
	}

	log.Printf("   Создаем запись в таблице snapshot_jobs...")
	log.Printf("   Данные записи: JobType=%s, DateFrom=%s, TotalObjects=%d, ActiveObjects=%d",
		job.JobType, job.DateFrom.Format("2006-01-02"), job.TotalObjects, job.ActiveObjects)

	// Проверяем, что таблица существует
	if !publicDB.Migrator().HasTable(&models.SnapshotJob{}) {
		log.Printf("❌ Таблица snapshot_jobs не существует в схеме public!")
		return fmt.Errorf("таблица snapshot_jobs не существует в схеме public")
	}
	log.Printf("   ✅ Таблица snapshot_jobs существует")

	if err := publicDB.Create(&job).Error; err != nil {
		log.Printf("❌ Ошибка создания записи в БД: %v", err)
		log.Printf("   Тип задачи: %s", job.JobType)
		log.Printf("   Дата: %s", job.DateFrom.Format("2006-01-02"))
		log.Printf("   Статус: %s", job.Status)
		log.Printf("   Объектов: всего=%d, активных=%d", job.TotalObjects, job.ActiveObjects)
		return fmt.Errorf("ошибка создания записи истории: %w", err)
	}

	log.Printf("✅ Стартовая дата биллинга записана в историю снимков (ID: %d, дата: %s, объектов: %d/%d)",
		job.ID, billingStartDate.Format("2006-01-02"), totalObjects, activeObjects)

	// Проверяем, что запись действительно создана и доступна через API
	var verifyJob models.SnapshotJob
	if err := publicDB.First(&verifyJob, job.ID).Error; err != nil {
		log.Printf("⚠️ ВНИМАНИЕ: Запись создана, но не найдена при проверке (ID: %d): %v", job.ID, err)
	} else {
		log.Printf("✅ Запись подтверждена в БД: ID=%d, JobType=%s, DateFrom=%s, Status=%s",
			verifyJob.ID, verifyJob.JobType, verifyJob.DateFrom.Format("2006-01-02"), verifyJob.Status)

		// Проверяем, что запись видна через обычный запрос (как в GetSnapshotJobs)
		var visibleJob models.SnapshotJob
		if err := publicDB.
			Order("started_at DESC").
			Where("id = ?", job.ID).
			First(&visibleJob).Error; err != nil {
			log.Printf("⚠️ Запись не видна через обычный запрос: %v", err)
		} else {
			log.Printf("✅ Запись видна через обычный запрос API (должна отображаться в списке)")
			log.Printf("   Детали: ID=%d, JobType=%s, DateFrom=%s, StartedAt=%s",
				visibleJob.ID, visibleJob.JobType, visibleJob.DateFrom.Format("2006-01-02"), visibleJob.StartedAt.Format("2006-01-02 15:04:05"))
		}

		// Проверяем позицию записи в общем списке (для понимания, почему она может не отображаться)
		var position int64
		if err := publicDB.Model(&models.SnapshotJob{}).
			Where("started_at > ?", job.StartedAt).
			Count(&position).Error; err == nil {
			log.Printf("   Позиция в списке (по started_at DESC): %d (0 = первая запись)", position)
			if position >= 10 {
				log.Printf("   ⚠️ Запись находится за пределами первой страницы (позиция %d)", position)
				log.Printf("   💡 Используйте пагинацию или увеличьте лимит для просмотра")
			}
		}
	}

	log.Printf("   Проверьте запись через API:")
	log.Printf("   - GET /api/auth/snapshot-jobs?job_type=billing_start")
	log.Printf("   - GET /api/auth/snapshots/check-billing-start-in-history")
	log.Printf("   - GET /api/auth/snapshot-jobs (все записи, должна быть видна в списке)")
	log.Printf("   - GET /api/auth/snapshot-jobs?limit=100 (увеличьте лимит, если запись не видна)")
	log.Printf("   - GET /api/auth/snapshot-jobs?limit=100&offset=0 (первая страница с большим лимитом)")

	return nil
}

// LoadNewObjectsForDate загружает только новые объекты, созданные в указанный день
func (s *SnapshotAccumulationService) LoadNewObjectsForDate(date time.Time, token string, tenantDB *gorm.DB) error {
	log.Printf("📥 Загрузка новых объектов за %s...", date.Format("2006-01-02"))

	// Начало и конец дня
	dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	dayEnd := dayStart.Add(24 * time.Hour).Add(-1 * time.Second)

	// Проверяем, какие объекты уже загружены за этот день
	var existingObjects []models.AxentaObjectSnapshot
	if err := tenantDB.
		Where("DATE(axenta_created_at AT TIME ZONE 'UTC') = ?", date.Format("2006-01-02")).
		Find(&existingObjects).Error; err != nil && err != gorm.ErrRecordNotFound {
		return fmt.Errorf("ошибка проверки существующих объектов: %w", err)
	}

	existingIDs := make(map[int64]bool)
	for _, obj := range existingObjects {
		existingIDs[obj.ExternalObjectID] = true
	}

	// Загружаем объекты из Axenta API
	objects, err := s.fetchObjectsForDateRange(dayStart, dayEnd, token)
	if err != nil {
		return fmt.Errorf("ошибка загрузки объектов из Axenta: %w", err)
	}

	// Фильтруем только новые объекты (созданные в этот день)
	newObjects := make([]axentaObject, 0)
	for _, obj := range objects {
		// Парсим дату создания
		if obj.CreatedAt == "" {
			continue
		}

		createdAt, err := parseAxentaTimeString(obj.CreatedAt)
		if err != nil || createdAt == nil {
			continue
		}

		// Проверяем, что объект создан в этот день
		if createdAt.Before(dayStart) || createdAt.After(dayEnd) {
			continue
		}

		// Проверяем, что объект еще не загружен
		if existingIDs[int64(obj.ID)] {
			continue
		}

		newObjects = append(newObjects, obj)
	}

	log.Printf("📊 Найдено новых объектов за %s: %d (из %d загруженных)", date.Format("2006-01-02"), len(newObjects), len(objects))

	// Сохраняем новые объекты
	if err := s.saveObjects(newObjects, tenantDB); err != nil {
		return fmt.Errorf("ошибка сохранения новых объектов: %w", err)
	}

	log.Printf("✅ Загружено %d новых объектов за %s", len(newObjects), date.Format("2006-01-02"))
	return nil
}

// CalculateObjectsCountForDate вычисляет общее количество объектов на указанную дату
// Учитывает все объекты, которые существовали на эту дату (созданы до или в этот день, не удалены)
func (s *SnapshotAccumulationService) CalculateObjectsCountForDate(date time.Time, tenantDB *gorm.DB) (total int, active int, err error) {
	log.Printf("📊 Вычисляем количество объектов на дату %s...", date.Format("2006-01-02"))

	// Конец дня для проверки
	dayEnd := time.Date(date.Year(), date.Month(), date.Day(), 23, 59, 59, 0, time.UTC)

	// Подсчитываем объекты, которые существовали на эту дату:
	// - axenta_created_at <= dayEnd (создан до или в этот день)
	// - (axenta_deleted_at IS NULL OR axenta_deleted_at > dayEnd) (не удален или удален после этого дня)
	var countResult struct {
		Total  int64
		Active int64
	}

	// Общее количество объектов
	if err := tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Where("(axenta_created_at IS NULL OR axenta_created_at <= ?) AND (axenta_deleted_at IS NULL OR axenta_deleted_at > ?)",
			dayEnd, dayEnd).
		Count(&countResult.Total).Error; err != nil {
		return 0, 0, fmt.Errorf("ошибка подсчета общего количества объектов: %w", err)
	}

	// Количество активных объектов
	if err := tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Where("(axenta_created_at IS NULL OR axenta_created_at <= ?) AND (axenta_deleted_at IS NULL OR axenta_deleted_at > ?) AND is_active = ?",
			dayEnd, dayEnd, true).
		Count(&countResult.Active).Error; err != nil {
		return 0, 0, fmt.Errorf("ошибка подсчета активных объектов: %w", err)
	}

	total = int(countResult.Total)
	active = int(countResult.Active)

	log.Printf("✅ На дату %s: всего объектов=%d, активных=%d", date.Format("2006-01-02"), total, active)
	return total, active, nil
}

// ProcessDailyAccumulation обрабатывает ежедневное накопление объектов
// Загружает новые объекты за текущий день и обновляет счетчики
func (s *SnapshotAccumulationService) ProcessDailyAccumulation(token string, tenantDB *gorm.DB) error {
	today := time.Now().UTC()
	todayStart := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)

	log.Printf("🔄 Обработка ежедневного накопления за %s...", todayStart.Format("2006-01-02"))

	// Загружаем новые объекты за сегодня
	if err := s.LoadNewObjectsForDate(todayStart, token, tenantDB); err != nil {
		return fmt.Errorf("ошибка загрузки новых объектов: %w", err)
	}

	// Вычисляем общее количество объектов на сегодня
	total, active, err := s.CalculateObjectsCountForDate(todayStart, tenantDB)
	if err != nil {
		return fmt.Errorf("ошибка вычисления количества объектов: %w", err)
	}

	log.Printf("✅ Ежедневное накопление завершено: всего объектов=%d, активных=%d", total, active)
	return nil
}

// fetchObjectsForDateRange загружает объекты из Axenta API за указанный период
func (s *SnapshotAccumulationService) fetchObjectsForDateRange(startDate, endDate time.Time, token string) ([]axentaObject, error) {
	client := &http.Client{Timeout: 180 * time.Second}
	var allObjects []axentaObject

	// Уменьшаем размер страницы до 100 для снижения нагрузки на API
	page := 1
	perPage := 100

	log.Printf("📥 Начинаем загрузку объектов за период %s - %s...",
		startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	log.Printf("   📊 Размер страницы: 100 объектов")

	for {
		// Запрос к Axenta Cloud API
		axentaURL := fmt.Sprintf("https://axenta.cloud/api/cms/objects/?page=%d&per_page=%d", page, perPage)
		log.Printf("   📄 Страница %d: запрашиваем объекты...", page)

		req, err := http.NewRequest("GET", axentaURL, nil)
		if err != nil {
			return nil, fmt.Errorf("ошибка создания запроса: %w", err)
		}

		req.Header.Set("Authorization", "Token "+token)

		_ = waitAxentaToken(context.Background(), token) // zone#2: shared per-token rate-limit
		requestStart := time.Now()
		resp, err := client.Do(req)
		requestDuration := time.Since(requestStart)

		if err != nil {
			return nil, fmt.Errorf("ошибка запроса к Axenta Cloud: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("axenta Cloud вернул статус %d", resp.StatusCode)
		}

		var axentaResponse struct {
			Results []axentaObject `json:"results"`
			Count   int            `json:"count"`
			Next    *string        `json:"next"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&axentaResponse); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("ошибка парсинга ответа: %w", err)
		}
		resp.Body.Close()

		// Сохраняем общее количество объектов из первого ответа
		if page == 1 {
			log.Printf("   ✅ Первая страница получена успешно!")
			log.Printf("   📊 Всего объектов в системе: %d", axentaResponse.Count)
		}

		// Фильтруем объекты по дате создания
		objectsInRange := 0
		for _, obj := range axentaResponse.Results {
			if obj.CreatedAt != "" {
				createdAt, err := parseAxentaTimeString(obj.CreatedAt)
				if err == nil && createdAt != nil {
					// Проверяем, попадает ли дата создания в диапазон
					if (createdAt.After(startDate) || createdAt.Equal(startDate)) && createdAt.Before(endDate.Add(time.Second)) {
						allObjects = append(allObjects, obj)
						objectsInRange++
					}
				}
			}
		}

		log.Printf("   ✅ Страница %d: получено %d объектов, из них %d в диапазоне дат (время запроса: %v)",
			page, len(axentaResponse.Results), objectsInRange, requestDuration)

		if len(axentaResponse.Results) < perPage || axentaResponse.Next == nil {
			log.Printf("   ✅ Достигнут конец списка объектов")
			break
		}

		page++

		// Пауза между запросами для снижения нагрузки на API
		time.Sleep(200 * time.Millisecond)
	}

	log.Printf("✅ Загрузка объектов за период завершена!")
	log.Printf("   📊 Итого объектов в диапазоне дат: %d", len(allObjects))

	return allObjects, nil
}

// saveObjects сохраняет объекты в БД
func (s *SnapshotAccumulationService) saveObjects(objects []axentaObject, tenantDB *gorm.DB) error {
	// Получаем adminAccountID из первой компании
	var firstCompany models.Company
	if err := s.db.Table("public.companies").Where("is_active = ?", true).Order("id ASC").First(&firstCompany).Error; err != nil {
		return fmt.Errorf("не найдено активных компаний для определения admin_account_id: %w", err)
	}
	adminAccountID := uint(1)
	if firstCompany.ID > 0 {
		adminAccountID = firstCompany.ID
	}
	adminAccountIDPtr := &adminAccountID

	now := time.Now().UTC()
	stmtTimeout := snapshotDBStmtTimeout() // zone#1: per-row deadline против hang на PG-локе
	timeoutCount := 0
	for _, obj := range objects {
		rawPayload, _ := json.Marshal(obj)
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
			LastSyncedAt:      now,
			RawPayload:        string(rawPayload),
		}

		// Парсим последнее сообщение
		if obj.LastMessageDatetime != "" {
			if parsed := parseAxentaTime(obj.LastMessageDatetime); parsed != nil {
				snapshot.LastCommunicationAt = parsed
			}
		}

		// Новые поля из API
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

		// Сохраняем с обработкой конфликтов
		// ВАЖНО: Уникальный индекс в БД только по external_object_id (idx_axenta_object_external)
		// Поэтому используем только external_object_id в OnConflict
		// Per-row deadline (zone#1): аборт застрявшего на PG-локе UPSERT вместо вечного hang.
		ctx, cancel := context.WithTimeout(context.Background(), stmtTimeout)
		err := tenantDB.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "external_object_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"admin_account_id", "account_external_id", "object_name", "unique_id", "device_type_name", "account_name",
				"status", "is_active", "last_synced_at", "raw_payload", "last_communication_at",
				"creator_name", "creator_id", "creator_is_active", "account_is_active",
				"phone_numbers", "axenta_created_at", "axenta_deleted_at",
			}),
		}).Create(&snapshot).Error
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				timeoutCount++
				log.Printf("⛔ Timeout (PG-lock?) сохранения объекта %d: %v", obj.ID, err)
			} else {
				log.Printf("⚠️ Ошибка сохранения объекта %d: %v", obj.ID, err)
			}
			continue
		}
	}

	// Codex#2: timeout-аборт = неполный кэш axenta_object_snapshots → биллинг-DB-count
	// недостоверен. Возвращаем ошибку (вызывающий accumulation пробрасывает её выше).
	if timeoutCount > 0 {
		return fmt.Errorf("кэш объектов неполон: %d аборчено по timeout (PG-lock, zone#1)", timeoutCount)
	}
	return nil
}

// parseAxentaTimeString парсит строку времени из Axenta API
func parseAxentaTimeString(value string) (*time.Time, error) {
	if value == "" {
		return nil, fmt.Errorf("пустая строка")
	}

	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, value); err == nil {
			return &t, nil
		}
	}

	return nil, fmt.Errorf("не удалось распарсить дату: %s", value)
}
