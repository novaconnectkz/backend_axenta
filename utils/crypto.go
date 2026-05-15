// Package utils — общие helper'ы.
//
// crypto.go — симметричное шифрование AES-256-GCM для секретов в БД (пароли
// внешних сервисов, токены). Ключ берётся из config.GetConfig().Axenta.EncryptionKey
// (ENV ENCRYPTION_KEY, ≥32 символа).
//
// Формат хранения: base64(nonce || ciphertext || authTag). Префикс "enc:v1:" —
// маркер чтобы отличать зашифрованные значения от plaintext (для постепенной
// миграции существующих рядов).
package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"backend_axenta/config"
)

const encPrefix = "enc:v1:"

// EncryptString шифрует plaintext в "enc:v1:<base64(nonce|ciphertext)>".
// Если уже зашифрован (есть префикс) — возвращает as-is (идемпотентно).
func EncryptString(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if strings.HasPrefix(plaintext, encPrefix) {
		return plaintext, nil
	}

	key, err := encryptionKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("aes.NewCipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("cipher.NewGCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("nonce read: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// DecryptString расшифровывает "enc:v1:..." в plaintext.
// Если префикса нет — возвращает значение as-is (legacy plaintext, до миграции).
func DecryptString(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if !strings.HasPrefix(stored, encPrefix) {
		return stored, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, encPrefix))
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}
	key, err := encryptionKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("aes.NewCipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("cipher.NewGCM: %w", err)
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("ciphertext короче nonce — повреждён")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("gcm.Open: %w (возможно ENCRYPTION_KEY изменился)", err)
	}
	return string(plain), nil
}

// IsEncrypted проверяет наличие префикса (для миграции — не трогать уже
// зашифрованные ряды).
func IsEncrypted(stored string) bool {
	return strings.HasPrefix(stored, encPrefix)
}

// encryptionKey возвращает 32-байтовый ключ из config. AES-256 требует ровно 32 байта.
// Если ENCRYPTION_KEY длиннее — берём первые 32; если короче — error.
func encryptionKey() ([]byte, error) {
	cfg := config.GetConfig()
	raw := cfg.Axenta.EncryptionKey
	if len(raw) < 32 {
		return nil, fmt.Errorf("ENCRYPTION_KEY должен быть ≥32 символа (текущая длина %d)", len(raw))
	}
	return []byte(raw)[:32], nil
}
