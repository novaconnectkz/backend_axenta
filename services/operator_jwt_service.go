package services

import (
	"backend_axenta/models"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// OperatorJWTService — auth-токены КОНТРОЛ-ПЛЕЙНА (оператор).
//
// Криптографически изолирован от tenant-контура (Ф1 JWTService):
// ОТДЕЛЬНЫЙ секрет подписи (OPERATOR_JWT_SECRET) и pepper
// (OPERATOR_REFRESH_PEPPER). Tenant-токен подписан другим ключом →
// здесь не провалидируется, и наоборот. Плюс claim Audience=
// "control-plane" как defense-in-depth. Модель безопасности refresh —
// та же, что Ф1 (HMAC-хэш в БД, family-ротация, grace-окно,
// reuse-detection, TokenVersion-отзыв). Ошибки ErrRefreshReuse/
// ErrRefreshRace переиспользуются (семантика идентична).

const operatorAudience = "control-plane"

// OperatorClaims — claims операторского access-JWT.
type OperatorClaims struct {
	OperatorID   uint   `json:"operator_id"`
	Username     string `json:"username"`
	TokenVersion int    `json:"tv"`
	jwt.RegisteredClaims
}

type OperatorJWTService struct {
	secretKey       []byte
	refreshPepper   []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	db              *gorm.DB
}

func (j *OperatorJWTService) getPublicDB() *gorm.DB {
	db := j.db.Session(&gorm.Session{})
	if err := db.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️ OperatorJWTService: search_path public: %v", err)
	}
	return db
}

// NewOperatorJWTService — fail-fast в production при пустых секретах.
func NewOperatorJWTService(db *gorm.DB) *OperatorJWTService {
	secret := os.Getenv("OPERATOR_JWT_SECRET")
	if secret == "" {
		if isProduction() {
			log.Fatal("❌ OPERATOR_JWT_SECRET обязателен в production — отказ запуска")
		}
		secret = "acrm-operator-default-secret-change-in-production"
		log.Println("⚠️ OPERATOR_JWT_SECRET не задан — DEV-дефолт (НЕ для прода)")
	}
	pepper := os.Getenv("OPERATOR_REFRESH_PEPPER")
	if pepper == "" {
		if isProduction() {
			log.Fatal("❌ OPERATOR_REFRESH_PEPPER обязателен в production — отказ запуска")
		}
		pepper = "acrm-operator-default-pepper-change-in-production"
		log.Println("⚠️ OPERATOR_REFRESH_PEPPER не задан — DEV-дефолт (НЕ для прода)")
	}

	accessTTL := 15 * time.Minute
	if m := os.Getenv("OPERATOR_JWT_ACCESS_TTL_MIN"); m != "" {
		if v, err := strconv.Atoi(m); err == nil && v > 0 {
			accessTTL = time.Duration(v) * time.Minute
		}
	}
	refreshTTL := 24 * 7 * time.Hour
	if h := os.Getenv("OPERATOR_JWT_REFRESH_TTL"); h != "" {
		if v, err := strconv.Atoi(h); err == nil && v > 0 {
			refreshTTL = time.Duration(v) * time.Hour
		}
	}

	return &OperatorJWTService{
		secretKey:       []byte(secret),
		refreshPepper:   []byte(pepper),
		accessTokenTTL:  accessTTL,
		refreshTokenTTL: refreshTTL,
		db:              db,
	}
}

func (j *OperatorJWTService) AccessTTLSeconds() int { return int(j.accessTokenTTL.Seconds()) }

func (j *OperatorJWTService) hashRefresh(raw string) string {
	mac := hmac.New(sha256.New, j.refreshPepper)
	mac.Write([]byte(raw))
	return hex.EncodeToString(mac.Sum(nil))
}

// GenerateTokenPair — новая сессия (новая family).
func (j *OperatorJWTService) GenerateTokenPair(op *models.Operator) (string, string, error) {
	access, err := j.GenerateAccessToken(op)
	if err != nil {
		return "", "", fmt.Errorf("operator access token: %w", err)
	}
	family, err := newRawToken()
	if err != nil {
		return "", "", err
	}
	refresh, _, err := j.issueRefreshToken(j.getPublicDB(), op.ID, family, nil)
	if err != nil {
		return "", "", fmt.Errorf("operator refresh token: %w", err)
	}
	return access, refresh, nil
}

func (j *OperatorJWTService) GenerateAccessToken(op *models.Operator) (string, error) {
	now := time.Now()
	claims := OperatorClaims{
		OperatorID:   op.ID,
		Username:     op.Username,
		TokenVersion: op.TokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(j.accessTokenTTL)),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "acrm-control-plane",
			Subject:   fmt.Sprintf("operator:%d", op.ID),
			Audience:  jwt.ClaimStrings{operatorAudience},
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(j.secretKey)
}

func (j *OperatorJWTService) issueRefreshToken(db *gorm.DB, opID uint, family string, rotatedFrom *uint) (string, uint, error) {
	raw, err := newRawToken()
	if err != nil {
		return "", 0, err
	}
	rt := &models.OperatorRefreshToken{
		OperatorID:  opID,
		TokenHash:   j.hashRefresh(raw),
		FamilyID:    family,
		RotatedFrom: rotatedFrom,
		ExpiresAt:   time.Now().Add(j.refreshTokenTTL),
	}
	if err := db.Create(rt).Error; err != nil {
		return "", 0, fmt.Errorf("save operator refresh: %w", err)
	}
	return raw, rt.ID, nil
}

// ValidateAccessToken — подпись операторским ключом + aud=control-plane
// + сверка TokenVersion/active по таблице operators.
func (j *OperatorJWTService) ValidateAccessToken(tokenString string) (*OperatorClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &OperatorClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return j.secretKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse operator token: %w", err)
	}
	claims, ok := token.Claims.(*OperatorClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid operator token")
	}
	// Audience обязателен — defense-in-depth поверх раздельного секрета.
	audOK := false
	for _, a := range claims.Audience {
		if a == operatorAudience {
			audOK = true
			break
		}
	}
	if !audOK {
		return nil, fmt.Errorf("operator token: wrong audience")
	}
	if j.db == nil { // degraded (юнит-тесты с nil-db)
		return claims, nil
	}
	var op models.Operator
	if err := j.getPublicDB().Select("id", "token_version", "is_active").
		First(&op, claims.OperatorID).Error; err != nil {
		return nil, fmt.Errorf("operator lookup: %w", err)
	}
	if !op.IsActive {
		return nil, fmt.Errorf("operator inactive")
	}
	if op.TokenVersion != claims.TokenVersion {
		return nil, fmt.Errorf("operator token revoked")
	}
	return claims, nil
}

// RotateRefreshToken — ротация по cookie (та же модель, что Ф1):
// текущий → ротация; grace-окно → ErrRefreshRace; вне окна/семья
// отозвана → ErrRefreshReuse + revoke family.
func (j *OperatorJWTService) RotateRefreshToken(rawRefresh string) (string, string, error) {
	hash := j.hashRefresh(rawRefresh)
	var newAccess, newRefresh string
	var reuseDetected, raceDetected bool

	err := j.getPublicDB().Transaction(func(tx *gorm.DB) error {
		var rt models.OperatorRefreshToken
		q := tx.Where("token_hash = ?", hash)
		if tx.Dialector.Name() == "postgres" {
			q = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash = ?", hash)
		}
		if err := q.First(&rt).Error; err != nil {
			return fmt.Errorf("operator refresh not found: %w", err)
		}
		now := time.Now()
		alreadyRotated := rt.RotatedAt != nil || rt.IsRevoked
		// BLK2: reuse-detection ПЕРЕД expiry (см. jwt_service.go).
		if !alreadyRotated && now.After(rt.ExpiresAt) {
			return fmt.Errorf("operator refresh expired")
		}
		if alreadyRotated {
			if j.familyRevoked(tx, rt.FamilyID) {
				reuseDetected = true
				return nil
			}
			if rt.RotatedAt != nil && now.Sub(*rt.RotatedAt) <= refreshGraceWindow {
				raceDetected = true
				return nil
			}
			if err := j.revokeFamily(tx, rt.FamilyID); err != nil {
				return err
			}
			reuseDetected = true
			return nil
		}
		raw, access, err := j.rotateInto(tx, &rt, now)
		if err != nil {
			return err
		}
		newRefresh, newAccess = raw, access
		return nil
	})
	if err != nil {
		return "", "", err
	}
	if reuseDetected {
		return "", "", ErrRefreshReuse
	}
	if raceDetected {
		return "", "", ErrRefreshRace
	}
	return newAccess, newRefresh, nil
}

func (j *OperatorJWTService) rotateInto(tx *gorm.DB, rt *models.OperatorRefreshToken, now time.Time) (string, string, error) {
	var op models.Operator
	if err := tx.First(&op, rt.OperatorID).Error; err != nil {
		return "", "", fmt.Errorf("load operator: %w", err)
	}
	if !op.IsActive {
		return "", "", fmt.Errorf("operator inactive")
	}
	raw, newID, err := j.issueRefreshToken(tx, op.ID, rt.FamilyID, &rt.ID)
	if err != nil {
		return "", "", err
	}
	if err := tx.Model(&models.OperatorRefreshToken{}).
		Where("id = ?", rt.ID).Updates(map[string]any{
		"is_revoked":  true,
		"rotated_at":  now,
		"replaced_by": newID,
	}).Error; err != nil {
		return "", "", fmt.Errorf("mark operator token rotated: %w", err)
	}
	access, err := j.GenerateAccessToken(&op)
	if err != nil {
		return "", "", fmt.Errorf("operator access: %w", err)
	}
	return raw, access, nil
}

func (j *OperatorJWTService) familyRevoked(tx *gorm.DB, family string) bool {
	var total, revoked int64
	tx.Model(&models.OperatorRefreshToken{}).Where("family_id = ?", family).Count(&total)
	tx.Model(&models.OperatorRefreshToken{}).Where("family_id = ? AND is_revoked = true", family).Count(&revoked)
	return total > 0 && total == revoked
}

func (j *OperatorJWTService) revokeFamily(tx *gorm.DB, family string) error {
	return tx.Model(&models.OperatorRefreshToken{}).
		Where("family_id = ?", family).Update("is_revoked", true).Error
}

// RevokeRefreshToken — logout одной сессии (отзыв всей family).
func (j *OperatorJWTService) RevokeRefreshToken(rawRefresh string) error {
	hash := j.hashRefresh(rawRefresh)
	db := j.getPublicDB()
	var rt models.OperatorRefreshToken
	if err := db.Where("token_hash = ?", hash).First(&rt).Error; err != nil {
		return fmt.Errorf("operator refresh not found: %w", err)
	}
	return db.Model(&models.OperatorRefreshToken{}).
		Where("family_id = ?", rt.FamilyID).Update("is_revoked", true).Error
}

// RevokeAllOperatorTokens — отзыв всех refresh + bump TokenVersion
// (мгновенный отзыв всех access-JWT оператора).
func (j *OperatorJWTService) RevokeAllOperatorTokens(operatorID uint) error {
	db := j.getPublicDB()
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.OperatorRefreshToken{}).
			Where("operator_id = ? AND is_revoked = false", operatorID).
			Update("is_revoked", true).Error; err != nil {
			return err
		}
		return tx.Model(&models.Operator{}).
			Where("id = ?", operatorID).
			UpdateColumn("token_version", gorm.Expr("token_version + 1")).Error
	})
}
