package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupOperatorDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Operator{}, &models.OperatorRefreshToken{},
		&models.LocalUser{}, &models.RefreshToken{},
	))
	database.DB = db
	return db
}

func setupOperatorJWT(db *gorm.DB) *OperatorJWTService {
	os.Setenv("OPERATOR_JWT_SECRET", "operator-secret-distinct")
	os.Setenv("OPERATOR_REFRESH_PEPPER", "operator-pepper-distinct")
	return NewOperatorJWTService(db)
}

func mkOperator(t *testing.T, db *gorm.DB) *models.Operator {
	o := &models.Operator{Username: "owner", PasswordHash: "x", IsActive: true, TokenVersion: 1}
	require.NoError(t, db.Create(o).Error)
	return o
}

func TestOperatorJWT_GenerateValidate(t *testing.T) {
	db := setupOperatorDB(t)
	svc := setupOperatorJWT(db)
	op := mkOperator(t, db)

	access, err := svc.GenerateAccessToken(op)
	require.NoError(t, err)
	claims, err := svc.ValidateAccessToken(access)
	require.NoError(t, err)
	require.Equal(t, op.ID, claims.OperatorID)
	require.Contains(t, claims.Audience, operatorAudience)
}

// КЛЮЧЕВОЙ ТЕСТ ИЗОЛЯЦИИ: tenant-токен ↔ operator-токен взаимно
// невалидны (разные секреты подписи + audience).
func TestOperatorTenantTokenIsolation(t *testing.T) {
	db := setupOperatorDB(t)
	opSvc := setupOperatorJWT(db)

	os.Setenv("JWT_SECRET", "tenant-secret-distinct")
	os.Setenv("REFRESH_PEPPER", "tenant-pepper-distinct")
	tenantSvc := NewJWTService(db)

	op := mkOperator(t, db)
	lu := &models.LocalUser{Username: "u", PasswordHash: "x", CompanyID: "1",
		Role: "admin", IsActive: true, TokenVersion: 1}
	require.NoError(t, db.Create(lu).Error)

	opTok, err := opSvc.GenerateAccessToken(op)
	require.NoError(t, err)
	tnTok, err := tenantSvc.GenerateAccessToken(lu)
	require.NoError(t, err)

	// operator-токен НЕ принимается tenant-сервисом
	_, err = tenantSvc.ValidateAccessToken(opTok)
	require.Error(t, err, "tenant не должен валидировать operator-токен")

	// tenant-токен НЕ принимается operator-сервисом
	_, err = opSvc.ValidateAccessToken(tnTok)
	require.Error(t, err, "operator не должен валидировать tenant-токен")

	// каждый валидирует свой
	_, err = opSvc.ValidateAccessToken(opTok)
	require.NoError(t, err)
	_, err = tenantSvc.ValidateAccessToken(tnTok)
	require.NoError(t, err)
}

func TestOperatorJWT_TokenVersionRevoke(t *testing.T) {
	db := setupOperatorDB(t)
	svc := setupOperatorJWT(db)
	op := mkOperator(t, db)

	tok, _ := svc.GenerateAccessToken(op)
	require.NoError(t, db.Model(&models.Operator{}).Where("id = ?", op.ID).
		UpdateColumn("token_version", 2).Error)
	_, err := svc.ValidateAccessToken(tok)
	require.Error(t, err)
}

func TestOperatorRefresh_RotateReuseRace(t *testing.T) {
	db := setupOperatorDB(t)
	svc := setupOperatorJWT(db)
	op := mkOperator(t, db)

	_, refresh, err := svc.GenerateTokenPair(op)
	require.NoError(t, err)

	// штатная ротация
	_, r1, err := svc.RotateRefreshToken(refresh)
	require.NoError(t, err)
	require.NotEmpty(t, r1)

	// повтор старого в grace → RACE, без выдачи токена
	_, r2, err := svc.RotateRefreshToken(refresh)
	require.ErrorIs(t, err, ErrRefreshRace)
	require.Empty(t, r2)

	// победивший r1 живой
	_, _, err = svc.RotateRefreshToken(r1)
	require.NoError(t, err)

	// reuse вне grace → revoke family
	db.Model(&models.OperatorRefreshToken{}).
		Where("token_hash = ?", svc.hashRefresh(refresh)).
		UpdateColumn("rotated_at", time.Now().Add(-2*refreshGraceWindow))
	_, _, err = svc.RotateRefreshToken(refresh)
	require.ErrorIs(t, err, ErrRefreshReuse)
}

func TestOperatorRevokeAll_BumpsVersion(t *testing.T) {
	db := setupOperatorDB(t)
	svc := setupOperatorJWT(db)
	op := mkOperator(t, db)

	access, refresh, _ := svc.GenerateTokenPair(op)
	_, err := svc.ValidateAccessToken(access)
	require.NoError(t, err)

	require.NoError(t, svc.RevokeAllOperatorTokens(op.ID))
	_, err = svc.ValidateAccessToken(access)
	require.Error(t, err, "access отбит по token_version")
	_, _, err = svc.RotateRefreshToken(refresh)
	require.Error(t, err, "refresh отозван")
}
