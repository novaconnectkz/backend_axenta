package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupJWTServiceTestDB создает тестовую БД (sqlite in-memory).
func setupJWTServiceTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.LocalUser{}, &models.RefreshToken{})
	require.NoError(t, err)

	database.DB = db
	return db
}

func setupJWTService(_ *testing.T, db *gorm.DB) *JWTService {
	os.Setenv("JWT_SECRET", "test-secret-key-for-testing-only")
	os.Setenv("REFRESH_PEPPER", "test-refresh-pepper-for-testing-only")
	return NewJWTService(db)
}

// makeUser создаёт и сохраняет пользователя (ValidateAccessToken делает
// lookup в БД для сверки token_version/is_active).
func makeUser(t *testing.T, db *gorm.DB) *models.LocalUser {
	u := &models.LocalUser{
		Username:     "testuser",
		PasswordHash: "x",
		CompanyID:    "123",
		Role:         "admin",
		IsActive:     true,
		TokenVersion: 1,
	}
	require.NoError(t, db.Create(u).Error)
	return u
}

func TestNewJWTService(t *testing.T) {
	db := setupJWTServiceTestDB(t)
	service := setupJWTService(t, db)

	assert.NotNil(t, service)
	assert.NotNil(t, service.secretKey)
	assert.NotEmpty(t, service.refreshPepper)
	assert.NotEmpty(t, service.accessTokenTTL)
	assert.NotEmpty(t, service.refreshTokenTTL)
}

func TestGenerateAndValidateAccessToken(t *testing.T) {
	db := setupJWTServiceTestDB(t)
	service := setupJWTService(t, db)
	user := makeUser(t, db)

	token, err := service.GenerateAccessToken(user)
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	claims, err := service.ValidateAccessToken(token)
	require.NoError(t, err)
	assert.Equal(t, user.ID, claims.UserID)
	assert.Equal(t, "123", claims.CompanyID)
	assert.Equal(t, 1, claims.TokenVersion)
}

// B7: смена TokenVersion в БД мгновенно инвалидирует уже выданный JWT.
func TestValidateAccessToken_TokenVersionMismatch(t *testing.T) {
	db := setupJWTServiceTestDB(t)
	service := setupJWTService(t, db)
	user := makeUser(t, db)

	token, err := service.GenerateAccessToken(user)
	require.NoError(t, err)

	require.NoError(t, db.Model(&models.LocalUser{}).
		Where("id = ?", user.ID).UpdateColumn("token_version", 2).Error)

	_, err = service.ValidateAccessToken(token)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "revoked")
}

// B7: неактивный пользователь — access отклоняется.
func TestValidateAccessToken_InactiveUser(t *testing.T) {
	db := setupJWTServiceTestDB(t)
	service := setupJWTService(t, db)
	user := makeUser(t, db)

	token, err := service.GenerateAccessToken(user)
	require.NoError(t, err)

	require.NoError(t, db.Model(&models.LocalUser{}).
		Where("id = ?", user.ID).UpdateColumn("is_active", false).Error)

	_, err = service.ValidateAccessToken(token)
	require.Error(t, err)
}

// B6: сырой refresh-токен в БД не хранится — только HMAC-хэш (64 hex).
func TestRefreshTokenNotStoredPlaintext(t *testing.T) {
	db := setupJWTServiceTestDB(t)
	service := setupJWTService(t, db)
	user := makeUser(t, db)

	_, refresh, err := service.GenerateTokenPair(user)
	require.NoError(t, err)
	assert.NotEmpty(t, refresh)

	var rt models.RefreshToken
	require.NoError(t, db.First(&rt).Error)
	assert.NotEqual(t, refresh, rt.TokenHash, "в БД должен лежать хэш, не сырой токен")
	assert.Len(t, rt.TokenHash, 64, "HMAC-SHA256 hex = 64 символа")
	assert.Equal(t, service.hashRefresh(refresh), rt.TokenHash)
	assert.NotEmpty(t, rt.FamilyID)
}

// Штатная ротация: новый токен валиден, старый — мёртв.
func TestRotateRefreshToken_Rotates(t *testing.T) {
	db := setupJWTServiceTestDB(t)
	service := setupJWTService(t, db)
	user := makeUser(t, db)

	_, refresh, err := service.GenerateTokenPair(user)
	require.NoError(t, err)

	newAccess, newRefresh, err := service.RotateRefreshToken(refresh)
	require.NoError(t, err)
	assert.NotEmpty(t, newAccess)
	assert.NotEmpty(t, newRefresh)
	assert.NotEqual(t, refresh, newRefresh)

	// Старый помечен ротированным+отозванным.
	var old models.RefreshToken
	require.NoError(t, db.Where("token_hash = ?", service.hashRefresh(refresh)).First(&old).Error)
	assert.True(t, old.IsRevoked)
	assert.NotNil(t, old.RotatedAt)

	// Новый валиден.
	claims, err := service.ValidateAccessToken(newAccess)
	require.NoError(t, err)
	assert.Equal(t, user.ID, claims.UserID)
}

// B1: гонка нормального клиента — повторный refresh тем же токеном в
// пределах grace-окна НЕ отзывает семью, отдаёт рабочий токен.
func TestRotateRefreshToken_GraceWindow(t *testing.T) {
	db := setupJWTServiceTestDB(t)
	service := setupJWTService(t, db)
	user := makeUser(t, db)

	_, refresh, err := service.GenerateTokenPair(user)
	require.NoError(t, err)

	_, r1, err := service.RotateRefreshToken(refresh)
	require.NoError(t, err)
	assert.NotEmpty(t, r1)

	// BLK1: догоняющий запрос тем же старым токеном в grace-окне
	// (легитимная гонка ИЛИ вор украденного R0) → ErrRefreshRace:
	// НИКАКОГО нового токена не выдаём, семью НЕ отзываем.
	_, r2, err := service.RotateRefreshToken(refresh)
	require.ErrorIs(t, err, ErrRefreshRace)
	assert.Empty(t, r2, "BLK1: в grace-окне токен предъявителю не выдаём")

	// Семья жива (нет ложного массового логаута, B1) — победивший r1
	// остаётся валиден и ротируем.
	var total, revoked int64
	db.Model(&models.RefreshToken{}).Count(&total)
	db.Model(&models.RefreshToken{}).Where("is_revoked = ?", true).Count(&revoked)
	assert.Less(t, revoked, total, "семья не должна быть отозвана на гонке")
	_, _, err = service.RotateRefreshToken(r1)
	require.NoError(t, err, "победивший токен r1 должен оставаться рабочим")
}

// BLK2: CleanupExpiredTokens НЕ удаляет отозванные-но-не-истёкшие
// (rotated-предшественники) — иначе reuse старого токена после чистки
// даёт «not found» вместо revoke-family, кража не детектится.
func TestCleanupKeepsRevokedUnexpired(t *testing.T) {
	db := setupJWTServiceTestDB(t)
	service := setupJWTService(t, db)
	user := makeUser(t, db)

	_, refresh, err := service.GenerateTokenPair(user)
	require.NoError(t, err)
	_, _, err = service.RotateRefreshToken(refresh) // refresh → revoked, не истёк
	require.NoError(t, err)

	require.NoError(t, service.CleanupExpiredTokens())

	var old models.RefreshToken
	err = db.Where("token_hash = ?", service.hashRefresh(refresh)).First(&old).Error
	require.NoError(t, err, "BLK2: отозванный-но-не-истёкший предок должен сохраниться")
	assert.True(t, old.IsRevoked)

	// И reuse после grace всё ещё детектится (предок на месте).
	past := time.Now().Add(-2 * refreshGraceWindow)
	require.NoError(t, db.Model(&models.RefreshToken{}).
		Where("token_hash = ?", service.hashRefresh(refresh)).
		UpdateColumn("rotated_at", past).Error)
	_, _, err = service.RotateRefreshToken(refresh)
	require.ErrorIs(t, err, ErrRefreshReuse)
}

// B1: предъявление мёртвого токена ВНЕ grace-окна → reuse → отзыв семьи.
func TestRotateRefreshToken_ReuseAfterGrace(t *testing.T) {
	db := setupJWTServiceTestDB(t)
	service := setupJWTService(t, db)
	user := makeUser(t, db)

	_, refresh, err := service.GenerateTokenPair(user)
	require.NoError(t, err)

	_, _, err = service.RotateRefreshToken(refresh)
	require.NoError(t, err)

	// Бэкдейтим rotated_at старого токена за пределы grace-окна.
	past := time.Now().Add(-2 * refreshGraceWindow)
	require.NoError(t, db.Model(&models.RefreshToken{}).
		Where("token_hash = ?", service.hashRefresh(refresh)).
		UpdateColumn("rotated_at", past).Error)

	_, _, err = service.RotateRefreshToken(refresh)
	require.ErrorIs(t, err, ErrRefreshReuse)

	// Вся семья отозвана.
	var total, revoked int64
	db.Model(&models.RefreshToken{}).Count(&total)
	db.Model(&models.RefreshToken{}).Where("is_revoked = ?", true).Count(&revoked)
	assert.Equal(t, total, revoked, "после reuse вся семья должна быть отозвана")
}

// BLK2: украденный R0, предъявленный ПОСЛЕ своего истечения, всё
// равно должен дать reuse-detection (revoke family), а не «expired».
func TestRotateRefreshToken_ReuseAfterExpiry(t *testing.T) {
	db := setupJWTServiceTestDB(t)
	service := setupJWTService(t, db)
	user := makeUser(t, db)

	_, refresh, err := service.GenerateTokenPair(user)
	require.NoError(t, err)
	_, _, err = service.RotateRefreshToken(refresh)
	require.NoError(t, err)

	// R0: ротирован И истёк (вне grace).
	past := time.Now().Add(-2 * refreshGraceWindow)
	expired := time.Now().Add(-time.Hour)
	require.NoError(t, db.Model(&models.RefreshToken{}).
		Where("token_hash = ?", service.hashRefresh(refresh)).
		Updates(map[string]any{"rotated_at": past, "expires_at": expired}).Error)

	_, _, err = service.RotateRefreshToken(refresh)
	require.ErrorIs(t, err, ErrRefreshReuse, "истёкший ротированный R0 → reuse, не expired")

	var total, revoked int64
	db.Model(&models.RefreshToken{}).Count(&total)
	db.Model(&models.RefreshToken{}).Where("is_revoked = ?", true).Count(&revoked)
	require.Equal(t, total, revoked, "семья отозвана")
}

// B7: RevokeAllUserTokens отзывает refresh и поднимает TokenVersion,
// делая все ранее выданные access-JWT невалидными.
func TestRevokeAllUserTokens_BumpsVersion(t *testing.T) {
	db := setupJWTServiceTestDB(t)
	service := setupJWTService(t, db)
	user := makeUser(t, db)

	access, refresh, err := service.GenerateTokenPair(user)
	require.NoError(t, err)
	_, err = service.ValidateAccessToken(access)
	require.NoError(t, err)

	require.NoError(t, service.RevokeAllUserTokens(user.ID))

	_, err = service.ValidateAccessToken(access)
	require.Error(t, err, "старый access должен отвалиться по token_version")

	_, _, err = service.RotateRefreshToken(refresh)
	require.Error(t, err, "refresh отозван")
}
