package testutils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupTestDB(t *testing.T) {
	db, err := SetupTestDB()
	require.NoError(t, err, "Should setup test database without error")
	require.NotNil(t, db, "Database should not be nil")

	// Проверяем, что таблицы созданы
	var tableCount int64
	err = db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table'").Scan(&tableCount).Error
	require.NoError(t, err, "Should be able to query sqlite_master")
	assert.Greater(t, tableCount, int64(0), "Should have created some tables")

	// Очищаем
	CleanupTestDB(db)
}

func TestCreateTestCompany(t *testing.T) {
	db, err := SetupTestDB()
	require.NoError(t, err)
	defer CleanupTestDB(db)

	company := CreateTestCompany(db)
	require.NotNil(t, company, "Should create test company")
	assert.Equal(t, "Test Company", company.Name)
	assert.Equal(t, "tenant_test", company.DatabaseSchema)
	assert.True(t, company.IsActive)
	assert.NotEmpty(t, company.ID, "Company ID should be generated")
}

func TestCreateTestUser(t *testing.T) {
	db, err := SetupTestDB()
	require.NoError(t, err)
	defer CleanupTestDB(db)

	company := CreateTestCompany(db)
	require.NotNil(t, company)

	user := CreateTestUser(db, company.ID)
	require.NotNil(t, user, "Should create test user")
	assert.Equal(t, "testuser", user.Username)
	assert.Equal(t, "test@example.com", user.Email)
	assert.Equal(t, company.ID, user.CompanyID)
	assert.True(t, user.IsActive)
}

func TestCreateTestRole(t *testing.T) {
	db, err := SetupTestDB()
	require.NoError(t, err)
	defer CleanupTestDB(db)

	role := CreateTestRole(db)
	require.NotNil(t, role, "Should create test role")
	assert.Equal(t, "test_role", role.Name)
	assert.Equal(t, "Role for testing", role.Description)
	assert.False(t, role.IsSystem)
}

func TestCreateTestPermission(t *testing.T) {
	db, err := SetupTestDB()
	require.NoError(t, err)
	defer CleanupTestDB(db)

	permission := CreateTestPermission(db)
	require.NotNil(t, permission, "Should create test permission")
	assert.Equal(t, "test.permission", permission.Name)
	assert.Equal(t, "Permission for testing", permission.Description)
	assert.Equal(t, "test", permission.Resource)
	assert.Equal(t, "read", permission.Action)
}
