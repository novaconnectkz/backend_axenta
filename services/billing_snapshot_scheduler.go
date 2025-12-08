package services

import (
	"log"
	"time"

	"github.com/robfig/cron/v3"
)

// BillingSnapshotScheduler управляет автоматическим расчетом billing snapshots
type BillingSnapshotScheduler struct {
	cron                   *cron.Cron
	billingSnapshotService *BillingSnapshotService
	isRunning              bool
}

// NewBillingSnapshotScheduler создает новый планировщик billing snapshots
func NewBillingSnapshotScheduler() *BillingSnapshotScheduler {
	// Создаем cron с обработкой ошибок и паник
	c := cron.New(
		cron.WithLocation(time.UTC),
		cron.WithChain(
			cron.Recover(cron.DefaultLogger), // Встроенная обработка паник в cron
		),
	)

	return &BillingSnapshotScheduler{
		cron:                   c,
		billingSnapshotService: NewBillingSnapshotService(),
	}
}

// Start запускает планировщик
func (s *BillingSnapshotScheduler) Start() error {
	// Запускаем расчет billing snapshots каждый день в 01:00 UTC (04:00 MSK)
	_, err := s.cron.AddFunc("0 1 * * *", func() {
		// Обработка паник, чтобы планировщик не останавливался при ошибках
		defer func() {
			if r := recover(); r != nil {
				log.Printf("❌ ПАНИКА в планировщике billing snapshots: %v", r)
				log.Printf("❌ Стек паники: %+v", r)
				// Планировщик продолжит работу после паники
			}
		}()

		log.Println("🕐 Запуск автоматического расчета billing snapshots (UTC 01:00 / MSK 04:00)")
		s.runDailyBillingSnapshot()
	})

	if err != nil {
		return err
	}

	s.cron.Start()
	s.isRunning = true

	log.Println("✅ Планировщик billing snapshots запущен (каждый день в 01:00 UTC / 04:00 MSK)")

	return nil
}

// Stop останавливает планировщик
func (s *BillingSnapshotScheduler) Stop() {
	if s.cron != nil {
		s.cron.Stop()
		s.isRunning = false
		log.Println("🛑 Планировщик billing snapshots остановлен")
	}
}

// runDailyBillingSnapshot выполняет ежедневный расчет billing snapshots
func (s *BillingSnapshotScheduler) runDailyBillingSnapshot() {
	// Обработка паник, чтобы планировщик не останавливался при ошибках
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ ПАНИКА в runDailyBillingSnapshot: %v", r)
			log.Printf("❌ Стек паники: %+v", r)
		}
	}()

	startTime := time.Now()
	log.Printf("📊 Начало расчета billing snapshots")

	if s.billingSnapshotService != nil {
		if result, err := s.billingSnapshotService.RunDailySnapshot(); err != nil {
			log.Printf("⚠️ Billing daily snapshots: ошибка обновления: %v", err)
		} else if len(result.ProcessedDates) > 0 {
			duration := time.Since(startTime)
			log.Printf("✅ Billing daily snapshots: создано %d записей за %v, итоговое количество объектов=%d",
				len(result.ProcessedDates), duration, result.FinalTotal)
		} else {
			log.Printf("ℹ️ Billing daily snapshots: новые даты отсутствуют")
		}
	} else {
		log.Printf("⚠️ BillingSnapshotService не инициализирован")
	}
}

// IsRunning возвращает статус работы планировщика
func (s *BillingSnapshotScheduler) IsRunning() bool {
	return s.isRunning
}
