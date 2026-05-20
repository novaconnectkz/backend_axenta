package services

import (
	"backend_axenta/models"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// refreshGraceWindow — окно идемпотентной ротации refresh-токена.
//
// Если уже ротированный токен предъявлен ПОВТОРНО в пределах окна —
// это гонка нормального клиента (двойной клик, retry axios, вкладки,
// reconnect мобильного), а не кража. Выдаём свежий токен той же семьи,
// семью НЕ отзываем (риск B1: без ложных массовых разлогинов).
// Предъявление ротированного токена ПОСЛЕ окна → настоящий reuse →
// отзыв всей семьи.
const refreshGraceWindow = 20 * time.Second

// ErrRefreshReuse — обнаружено повторное использование (кража) refresh-токена.
var ErrRefreshReuse = errors.New("refresh token reuse detected: family revoked")

// ErrRefreshRace — гонка легитимного клиента в grace-окне (токен уже
// ротирован, но семья жива). Не кража: семью не трогаем, токен не
// выдаём. Хендлер → HTTP 409, cookie НЕ чистим (валидный преемник уже
// в общем cookie от победившей ротации).
var ErrRefreshRace = errors.New("refresh rotation race: retry")

// JWTClaims представляет claims для JWT токена
type JWTClaims struct {
	UserID       uint   `json:"user_id"`
	CompanyID    string `json:"company_id"` // числовой ID компании строкой (источник tenant-схемы)
	Role         string `json:"role"`
	Username     string `json:"username"`
	IsSuperadmin bool   `json:"is_superadmin"`
	// TokenVersion дублирует LocalUser.TokenVersion на момент выпуска.
	// Расхождение с БД → токен отозван (logout-all/смена пароля, риск B7).
	TokenVersion int `json:"tv"`
	jwt.RegisteredClaims
}

// JWTService сервис для работы с JWT токенами
type JWTService struct {
	secretKey       []byte
	refreshPepper   []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	db              *gorm.DB
}

// getPublicDB возвращает подключение к схеме public для глобальных таблиц
func (j *JWTService) getPublicDB() *gorm.DB {
	publicDB := j.db.Session(&gorm.Session{})
	if err := publicDB.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️ JWTService: Не удалось переключиться на схему public: %v", err)
	}
	return publicDB
}

// isProduction — прод-окружение (для fail-fast по секретам).
func isProduction() bool {
	env := os.Getenv("APP_ENV")
	return env == "production" || env == "prod"
}

// NewJWTService создает новый JWT сервис.
//
// Fail-fast: в production пустой JWT_SECRET или REFRESH_PEPPER — фатально
// (нельзя подписывать токены дефолтным секретом / хранить refresh без pepper).
func NewJWTService(db *gorm.DB) *JWTService {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		if isProduction() {
			log.Fatal("❌ JWT_SECRET обязателен в production — отказ запуска")
		}
		secret = "axenta-crm-default-secret-change-in-production"
		log.Println("⚠️ JWT_SECRET не задан — используется DEV-дефолт (НЕ для прода)")
	}

	pepper := os.Getenv("REFRESH_PEPPER")
	if pepper == "" {
		if isProduction() {
			log.Fatal("❌ REFRESH_PEPPER обязателен в production — отказ запуска")
		}
		pepper = "axenta-crm-default-refresh-pepper-change-in-production"
		log.Println("⚠️ REFRESH_PEPPER не задан — используется DEV-дефолт (НЕ для прода)")
	}

	// Access TTL: по умолчанию 15 минут (короткий, риск B7).
	// Обратная совместимость со старым JWT_ACCESS_TTL (часы) сохранена;
	// JWT_ACCESS_TTL_MIN (минуты) имеет приоритет, если задан.
	accessTTL := 15 * time.Minute
	if hStr := os.Getenv("JWT_ACCESS_TTL"); hStr != "" {
		if hours, err := strconv.Atoi(hStr); err == nil && hours > 0 {
			accessTTL = time.Duration(hours) * time.Hour
		}
	}
	if mStr := os.Getenv("JWT_ACCESS_TTL_MIN"); mStr != "" {
		if mins, err := strconv.Atoi(mStr); err == nil && mins > 0 {
			accessTTL = time.Duration(mins) * time.Minute
		}
	}

	refreshTTL := 24 * 7 * time.Hour // 7 дней
	if hStr := os.Getenv("JWT_REFRESH_TTL"); hStr != "" {
		if hours, err := strconv.Atoi(hStr); err == nil && hours > 0 {
			refreshTTL = time.Duration(hours) * time.Hour
		}
	}

	return &JWTService{
		secretKey:       []byte(secret),
		refreshPepper:   []byte(pepper),
		accessTokenTTL:  accessTTL,
		refreshTokenTTL: refreshTTL,
		db:              db,
	}
}

// AccessTTLSeconds — TTL access-токена в секундах (для ответа клиенту).
func (j *JWTService) AccessTTLSeconds() int {
	return int(j.accessTokenTTL.Seconds())
}

// hashRefresh возвращает HMAC-SHA256(pepper, raw) в hex (64 символа).
// В БД хранится только это — сырой токен не персистится (риск B6).
func (j *JWTService) hashRefresh(raw string) string {
	mac := hmac.New(sha256.New, j.refreshPepper)
	mac.Write([]byte(raw))
	return hex.EncodeToString(mac.Sum(nil))
}

// newRawToken — 256-bit криптослучайный токен в base64url (риск B6).
func newRawToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("csprng failed: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// GenerateTokenPair генерирует пару access + refresh (новая login-сессия).
func (j *JWTService) GenerateTokenPair(user *models.LocalUser) (string, string, error) {
	accessToken, err := j.GenerateAccessToken(user)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate access token: %w", err)
	}

	// Новая семья = новая сессия.
	familyID, err := newRawToken()
	if err != nil {
		return "", "", err
	}

	refreshToken, _, err := j.issueRefreshToken(j.getPublicDB(), user.ID, familyID, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return accessToken, refreshToken, nil
}

// GenerateAccessToken генерирует подписанный access JWT.
func (j *JWTService) GenerateAccessToken(user *models.LocalUser) (string, error) {
	now := time.Now()
	claims := JWTClaims{
		UserID:       user.ID,
		CompanyID:    user.CompanyID,
		Role:         user.Role,
		Username:     user.Username,
		IsSuperadmin: user.IsSuperadmin,
		TokenVersion: user.TokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(j.accessTokenTTL)),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "axenta-crm",
			Subject:   fmt.Sprintf("user:%d", user.ID),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(j.secretKey)
}

// issueRefreshToken создаёт запись refresh-токена и возвращает
// (сырой токен, ID записи). Сырой токен наружу — в БД только hashRefresh().
func (j *JWTService) issueRefreshToken(db *gorm.DB, userID uint, familyID string, rotatedFrom *uint) (string, uint, error) {
	raw, err := newRawToken()
	if err != nil {
		return "", 0, err
	}

	rt := &models.RefreshToken{
		UserID:      userID,
		TokenHash:   j.hashRefresh(raw),
		FamilyID:    familyID,
		RotatedFrom: rotatedFrom,
		ExpiresAt:   time.Now().Add(j.refreshTokenTTL),
	}
	if err := db.Create(rt).Error; err != nil {
		return "", 0, fmt.Errorf("failed to save refresh token: %w", err)
	}
	return raw, rt.ID, nil
}

// ValidateAccessToken валидирует подпись/срок access-токена и сверяет
// TokenVersion с БД (риск B7: мгновенный отзыв при logout-all/смене пароля).
func (j *JWTService) ValidateAccessToken(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return j.secretKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	// Degraded-режим: сервис создан без БД (юнит-тесты с nil-db).
	// В проде main.go всегда передаёт database.DB, поэтому B7-сверка
	// ниже работает; здесь — не паникуем, отдаём claims как есть.
	if j.db == nil {
		return claims, nil
	}

	// Сверка версии токена + активности пользователя (риск B7).
	var user models.LocalUser
	if err := j.getPublicDB().Select("id", "token_version", "is_active").
		First(&user, claims.UserID).Error; err != nil {
		return nil, fmt.Errorf("user lookup failed: %w", err)
	}
	if !user.IsActive {
		return nil, fmt.Errorf("user is inactive")
	}
	if user.TokenVersion != claims.TokenVersion {
		return nil, fmt.Errorf("token version mismatch: revoked")
	}

	return claims, nil
}

// RotateRefreshToken — ротация по предъявленному сырому refresh-токену.
//
// Возвращает (newAccess, newRefresh). Логика конкурентно-безопасна
// (риск B1): вся работа в транзакции, строка токена берётся FOR UPDATE,
// поэтому параллельные /refresh с одним cookie сериализуются.
//
//   - токен текущий (не ротирован, не отозван, не истёк) → ротация:
//     старый помечается rotated+revoked, выпускается преемник той же семьи.
//   - токен уже ротирован, но в пределах grace-окна → гонка нормального
//     клиента: выдаём свежий токен той же семьи, семью НЕ трогаем.
//   - токен ротирован/отозван ВНЕ окна, либо семья уже отозвана →
//     reuse-detection: отзываем всю семью, ErrRefreshReuse.
func (j *JWTService) RotateRefreshToken(rawRefresh string) (string, string, error) {
	hash := j.hashRefresh(rawRefresh)
	var newAccess, newRefresh string
	// reuse фиксируем флагом и возвращаем ОШИБКУ ПОСЛЕ транзакции:
	// если вернуть error из callback'а — gorm сделает ROLLBACK и
	// revokeFamily откатится (риск B1: семья осталась бы живой).
	var reuseDetected, raceDetected bool

	err := j.getPublicDB().Transaction(func(tx *gorm.DB) error {
		var rt models.RefreshToken
		// SELECT ... FOR UPDATE сериализует параллельные /refresh
		// (риск B1). Только Postgres — sqlite (юнит-тесты) FOR UPDATE
		// не поддерживает, там сериализация не нужна (single-conn).
		q := tx.Where("token_hash = ?", hash)
		if tx.Dialector.Name() == "postgres" {
			q = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash = ?", hash)
		}
		if err := q.First(&rt).Error; err != nil {
			return fmt.Errorf("refresh token not found: %w", err)
		}

		now := time.Now()

		alreadyRotated := rt.RotatedAt != nil || rt.IsRevoked

		// BLK2: reuse-detection ПЕРЕД expiry. Иначе украденный R0,
		// предъявленный после своего истечения, выходил бы на «expired»
		// и НЕ отзывал семью — кража не детектится. Ротированный/
		// отозванный токен = либо гонка, либо кража, независимо от срока.
		if !alreadyRotated && now.After(rt.ExpiresAt) {
			// Свежий (не ротированный) токен просто состарился — не кража.
			return fmt.Errorf("refresh token expired")
		}

		if alreadyRotated {
			// Семья уже отозвана целиком → точно reuse.
			if j.familyRevoked(tx, rt.FamilyID) {
				reuseDetected = true
				return nil
			}
			// Ротирован, в пределах grace-окна → ГОНКА легитимного
			// клиента (двойной submit/вкладки). НЕ выдаём токен
			// (риск BLK1: иначе вор украденного R0 в окне получил бы
			// живой sibling) и НЕ жжём семью (риск B1: без ложного
			// массового логаута — победившая ротация уже выдала
			// преемника в общий cookie). Просто «постоять в стороне».
			//
			// RB2 (accepted trade-off, IETF OAuth 2.0 Security BCP
			// §4.13): если 200-ответ победившей ротации потерян в сети,
			// клиент остаётся на R0 и после grace получит reuse →
			// family-revoke → перелогин. Это fail-safe: по одному R0
			// вора и «потерял ответ» не различить, выдать токен =
			// реоткрыть BLK1. Цена — редкий перелогин при сетевом
			// сбое; мониторить долю refresh_reuse_detected в логах.
			if rt.RotatedAt != nil && now.Sub(*rt.RotatedAt) <= refreshGraceWindow {
				raceDetected = true
				return nil
			}
			// Вне окна предъявлен мёртвый токен → кража. Жжём семью
			// и КОММИТИМ отзыв (return nil), ошибку отдаём после tx.
			if err := j.revokeFamily(tx, rt.FamilyID); err != nil {
				return err
			}
			reuseDetected = true
			return nil
		}

		// Нормальная ротация текущего токена.
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

// rotateInto выпускает преемника для rt в той же семье и помечает rt
// ротированным+отозванным. Вызывается только для ТЕКУЩЕГО токена
// (ротированные обрабатываются как race/reuse выше по стеку).
func (j *JWTService) rotateInto(tx *gorm.DB, rt *models.RefreshToken, now time.Time) (string, string, error) {
	var user models.LocalUser
	if err := tx.First(&user, rt.UserID).Error; err != nil {
		return "", "", fmt.Errorf("failed to load user: %w", err)
	}
	if !user.IsActive {
		return "", "", fmt.Errorf("user is inactive")
	}

	raw, newID, err := j.issueRefreshToken(tx, user.ID, rt.FamilyID, &rt.ID)
	if err != nil {
		return "", "", err
	}

	// Помечаем предъявленный токен ротированным + отозванным.
	if err := tx.Model(&models.RefreshToken{}).
		Where("id = ?", rt.ID).Updates(map[string]any{
		"is_revoked":  true,
		"rotated_at":  now,
		"replaced_by": newID,
	}).Error; err != nil {
		return "", "", fmt.Errorf("failed to mark token rotated: %w", err)
	}

	access, err := j.GenerateAccessToken(&user)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate access token: %w", err)
	}
	return raw, access, nil
}

// familyRevoked — есть ли в семье хоть один отозванный-по-reuse корень.
// Признак: запись семьи с пометкой revoked И без rotated_at
// (revokeFamily ставит is_revoked всем, не выставляя rotated_at тем,
// кто не ротировался штатно). Достаточно проверки «вся семья revoked».
func (j *JWTService) familyRevoked(tx *gorm.DB, familyID string) bool {
	var total, revoked int64
	tx.Model(&models.RefreshToken{}).Where("family_id = ?", familyID).Count(&total)
	tx.Model(&models.RefreshToken{}).Where("family_id = ? AND is_revoked = true", familyID).Count(&revoked)
	return total > 0 && total == revoked
}

// revokeFamily отзывает все токены семьи (reuse-detection, риск B1).
func (j *JWTService) revokeFamily(tx *gorm.DB, familyID string) error {
	return tx.Model(&models.RefreshToken{}).
		Where("family_id = ?", familyID).
		Update("is_revoked", true).Error
}

// RevokeRefreshToken отзывает всю семью токена (logout одной сессии).
func (j *JWTService) RevokeRefreshToken(rawRefresh string) error {
	hash := j.hashRefresh(rawRefresh)
	publicDB := j.getPublicDB()

	var rt models.RefreshToken
	if err := publicDB.Where("token_hash = ?", hash).First(&rt).Error; err != nil {
		return fmt.Errorf("refresh token not found: %w", err)
	}
	return publicDB.Model(&models.RefreshToken{}).
		Where("family_id = ?", rt.FamilyID).
		Update("is_revoked", true).Error
}

// RevokeAllUserTokens отзывает все refresh-токены пользователя и
// инкрементирует TokenVersion (мгновенный отзыв всех access-JWT, риск B7).
func (j *JWTService) RevokeAllUserTokens(userID uint) error {
	publicDB := j.getPublicDB()
	return publicDB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.RefreshToken{}).
			Where("user_id = ? AND is_revoked = false", userID).
			Update("is_revoked", true).Error; err != nil {
			return err
		}
		return tx.Model(&models.LocalUser{}).
			Where("id = ?", userID).
			UpdateColumn("token_version", gorm.Expr("token_version + 1")).Error
	})
}

// CleanupExpiredTokens удаляет ТОЛЬКО истёкшие по сроку refresh-токены.
//
// Отозванные-но-не-истёкшие (rotated-предшественники) НЕ трогаем
// (риск BLK2): они нужны reuse-detection весь refresh-TTL — иначе
// удалённый предок при replay даёт «not found» вместо revoke-family,
// и кража не фиксируется. По истечении ExpiresAt — естественная чистка.
func (j *JWTService) CleanupExpiredTokens() error {
	publicDB := j.getPublicDB()
	return publicDB.Where("expires_at < ?", time.Now()).
		Delete(&models.RefreshToken{}).Error
}

// GetTokenInfo парсит claims без проверки подписи (для диагностики).
func (j *JWTService) GetTokenInfo(tokenString string) (*JWTClaims, error) {
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, &JWTClaims{})
	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}
	if claims, ok := token.Claims.(*JWTClaims); ok {
		return claims, nil
	}
	return nil, fmt.Errorf("invalid token claims")
}
