package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSimpleModels тестирует базовую функциональность с совместимыми моделями
func TestSimpleModels(t *testing.T) {
	db := setupTestDBWithCompatibleModels(t)

	t.Run("Создание компании", func(t *testing.T) {
		company := TestCompany{
			Name:           "Test Company",
			DatabaseSchema: "test_schema",
			AxetnaLogin:    "test_login",
			AxetnaPassword: "test_password",
		}

		err := db.Create(&company).Error
		require.NoError(t, err)
		assert.NotZero(t, company.ID)
		assert.Equal(t, "Test Company", company.Name)
	})

	t.Run("Создание пользователя", func(t *testing.T) {
		// Сначала создаем компанию
		company := TestCompany{
			Name:           "User Company",
			DatabaseSchema: "user_schema",
			AxetnaLogin:    "user_login",
			AxetnaPassword: "user_password",
		}
		err := db.Create(&company).Error
		require.NoError(t, err)

		user := TestUser{
			Username:  "testuser",
			Email:     "test@example.com",
			Password:  "hashedpassword",
			FirstName: "Test",
			LastName:  "User",
			CompanyID: company.ID,
		}

		err = db.Create(&user).Error
		require.NoError(t, err)
		assert.NotZero(t, user.ID)
		assert.Equal(t, "testuser", user.Username)
		assert.Equal(t, company.ID, user.CompanyID)
	})

	t.Run("Создание объекта", func(t *testing.T) {
		// Сначала создаем компанию
		company := TestCompany{
			Name:           "Object Company",
			DatabaseSchema: "object_schema",
			AxetnaLogin:    "object_login",
			AxetnaPassword: "object_password",
		}
		err := db.Create(&company).Error
		require.NoError(t, err)

		object := TestObject{
			Name:      "Test Object",
			Type:      "vehicle",
			Status:    "active",
			CompanyID: company.ID,
		}

		err = db.Create(&object).Error
		require.NoError(t, err)
		assert.NotZero(t, object.ID)
		assert.Equal(t, "Test Object", object.Name)
		assert.Equal(t, company.ID, object.CompanyID)
	})

	t.Run("Создание договора", func(t *testing.T) {
		// Сначала создаем компанию
		company := TestCompany{
			Name:           "Contract Company",
			DatabaseSchema: "contract_schema",
			AxetnaLogin:    "contract_login",
			AxetnaPassword: "contract_password",
		}
		err := db.Create(&company).Error
		require.NoError(t, err)

		contract := TestContract{
			Number:     "TEST-001",
			Title:      "Test Contract",
			CompanyID:  company.ID,
			ClientName: "Test Client",
			MonthlyFee: "1000.00",
		}

		err = db.Create(&contract).Error
		require.NoError(t, err)
		assert.NotZero(t, contract.ID)
		assert.Equal(t, "TEST-001", contract.Number)
		assert.Equal(t, company.ID, contract.CompanyID)
	})

	t.Run("Связи между моделями", func(t *testing.T) {
		// Создаем компанию
		company := TestCompany{
			Name:           "Relations Company",
			DatabaseSchema: "relations_schema",
			AxetnaLogin:    "relations_login",
			AxetnaPassword: "relations_password",
		}
		err := db.Create(&company).Error
		require.NoError(t, err)

		// Создаем пользователя
		user := TestUser{
			Username:  "relationuser",
			Email:     "relation@example.com",
			Password:  "password",
			CompanyID: company.ID,
		}
		err = db.Create(&user).Error
		require.NoError(t, err)

		// Создаем объект
		object := TestObject{
			Name:      "Relation Object",
			Type:      "vehicle",
			CompanyID: company.ID,
		}
		err = db.Create(&object).Error
		require.NoError(t, err)

		// Проверяем что все связано с одной компанией
		var foundUsers []TestUser
		err = db.Where("company_id = ?", company.ID).Find(&foundUsers).Error
		require.NoError(t, err)
		assert.Len(t, foundUsers, 1)
		assert.Equal(t, user.Username, foundUsers[0].Username)

		var foundObjects []TestObject
		err = db.Where("company_id = ?", company.ID).Find(&foundObjects).Error
		require.NoError(t, err)
		assert.Len(t, foundObjects, 1)
		assert.Equal(t, object.Name, foundObjects[0].Name)
	})
}
