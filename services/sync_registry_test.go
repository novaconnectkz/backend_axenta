package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSetAxentaSyncScheduler тестирует SetAxentaSyncScheduler
func TestSetAxentaSyncScheduler(t *testing.T) {
	// Сохраняем оригинальное значение
	originalScheduler := axentaSyncScheduler

	// Создаем тестовый планировщик
	testScheduler := &AxentaSyncScheduler{}

	// Устанавливаем планировщик
	SetAxentaSyncScheduler(testScheduler)

	// Проверяем, что планировщик установлен
	retrievedScheduler := GetAxentaSyncScheduler()
	assert.NotNil(t, retrievedScheduler)
	assert.Equal(t, testScheduler, retrievedScheduler)

	// Восстанавливаем оригинальное значение
	SetAxentaSyncScheduler(originalScheduler)
}

// TestGetAxentaSyncScheduler_NotSet тестирует GetAxentaSyncScheduler когда планировщик не установлен
func TestGetAxentaSyncScheduler_NotSet(t *testing.T) {
	// Сохраняем оригинальное значение
	originalScheduler := axentaSyncScheduler

	// Устанавливаем nil
	SetAxentaSyncScheduler(nil)

	// Проверяем, что возвращается nil
	retrievedScheduler := GetAxentaSyncScheduler()
	assert.Nil(t, retrievedScheduler)

	// Восстанавливаем оригинальное значение
	SetAxentaSyncScheduler(originalScheduler)
}

// TestGetAxentaSyncScheduler тестирует GetAxentaSyncScheduler
func TestGetAxentaSyncScheduler(t *testing.T) {
	// Сохраняем оригинальное значение
	originalScheduler := axentaSyncScheduler

	// Создаем тестовый планировщик
	testScheduler := &AxentaSyncScheduler{}

	// Устанавливаем и получаем планировщик
	SetAxentaSyncScheduler(testScheduler)
	retrievedScheduler := GetAxentaSyncScheduler()

	assert.NotNil(t, retrievedScheduler)
	assert.Equal(t, testScheduler, retrievedScheduler)

	// Восстанавливаем оригинальное значение
	SetAxentaSyncScheduler(originalScheduler)
}
