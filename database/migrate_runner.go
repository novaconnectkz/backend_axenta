package database

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	postgresdriver "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"gorm.io/gorm"
)

// RunGolangMigrate применяет SQL-миграции из ./migrations/ через golang-migrate.
//
// Стратегия безопасности:
//   - Запускается ПОСЛЕ GORM AutoMigrate (т.е. схема уже создана из моделей).
//   - Если на проде ещё нет таблицы `schema_migrations` — нужно ОДИН РАЗ
//     вручную сделать INSERT INTO schema_migrations VALUES (62, false)
//     перед стартом, чтобы migrate знал что текущие миграции уже применены.
//     Иначе он попытается применить все 62 миграции — это либо упадёт на
//     неидемпотентных DDL (3-5 файлов), либо отработает no-op для остальных
//     благодаря IF NOT EXISTS.
//   - Все ошибки логируются с warning, но не блокируют запуск сервера —
//     graceful fallback на текущую схему.
//
// Все миграции называются по соглашению NNNN_<name>.up.sql / .down.sql.
func RunGolangMigrate(db *gorm.DB) error {
	// Получаем sql.DB из GORM — переиспользуем существующий пул соединений.
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("получить sql.DB: %w", err)
	}

	driver, err := postgresdriver.WithInstance(sqlDB, &postgresdriver.Config{
		MigrationsTable: "schema_migrations",
	})
	if err != nil {
		return fmt.Errorf("postgres driver: %w", err)
	}

	migrationsPath, err := filepath.Abs("migrations")
	if err != nil {
		return fmt.Errorf("abs path: %w", err)
	}
	sourceURL := "file://" + migrationsPath

	m, err := migrate.NewWithDatabaseInstance(sourceURL, "postgres", driver)
	if err != nil {
		return fmt.Errorf("migrate.New: %w", err)
	}

	curVer, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		log.Printf("⚠️ migrate version: %v", err)
	} else {
		log.Printf("🔧 migrate: текущая версия БД = %d (dirty=%v)", curVer, dirty)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}

	finalVer, _, _ := m.Version()
	log.Printf("✅ migrate: миграции применены, текущая версия = %d", finalVer)

	return nil
}
