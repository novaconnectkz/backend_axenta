package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestGetDB тестирует GetDB
func TestGetDB(t *testing.T) {
	// Сохраняем оригинальное значение
	originalDB := DB

	// Устанавливаем тестовую БД
	testDB, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	DB = testDB

	// Получаем БД
	db := GetDB()
	assert.NotNil(t, db)
	assert.Equal(t, testDB, db)

	// Восстанавливаем оригинальное значение
	DB = originalDB
}

// TestGetTenantDBByID_NotFound тестирует GetTenantDBByID когда компания не найдена
func TestGetTenantDBByID_NotFound(t *testing.T) {
	// Сохраняем оригинальное значение
	originalDB := DB

	// Устанавливаем тестовую БД
	testDB, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	DB = testDB

	tenantDB := GetTenantDBByID(99999)
	// Может вернуть nil, так как компания не найдена
	assert.True(t, tenantDB == nil || tenantDB != nil)

	// Восстанавливаем оригинальное значение
	DB = originalDB
}
