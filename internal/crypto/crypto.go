// Package crypto инкапсулирует всю криптографию проекта:
// шифрование Telegram ID отправителей (AES-256-GCM), детерминированные
// поисковые хэши (HMAC-SHA256) и генерацию токенов ссылок (crypto/rand).
//
// Схема описана в docs/05-security.md. Ключи шифрования и HMAC — разные секреты.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
)

const (
	nonceSize = 12 // стандартный nonce GCM
	tokenLen  = 16 // 128 бит энтропии для токена ссылки

	// TokenPrefix отличает токен ссылки от алиаса в /start-аргументе.
	TokenPrefix = "q_"
)

// Codec шифрует/хэширует ID отправителей. Создаётся один раз в main.
type Codec struct {
	aead    cipher.AEAD
	hmacKey []byte
}

// NewCodec создаёт Codec из 32-байтных ключей (AES-256 и HMAC-SHA256).
func NewCodec(encryptionKey, hmacKey []byte) (*Codec, error) {
	if len(encryptionKey) != 32 {
		return nil, fmt.Errorf("encryption key: ожидается 32 байта, получено %d", len(encryptionKey))
	}
	if len(hmacKey) != 32 {
		return nil, fmt.Errorf("hmac key: ожидается 32 байта, получено %d", len(hmacKey))
	}
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("aes: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	return &Codec{aead: aead, hmacKey: hmacKey}, nil
}

// EncryptSender шифрует Telegram ID отправителя.
// AAD — id вопроса: привязывает шифротекст к строке БД, «пересадить»
// отправителя из одной записи в другую нельзя.
// Формат результата: nonce ‖ ciphertext.
func (c *Codec) EncryptSender(senderTgID, questionID int64) ([]byte, error) {
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}
	plaintext := int64BE(senderTgID)
	ciphertext := c.aead.Seal(nonce, nonce, plaintext, int64BE(questionID))
	return ciphertext, nil
}

// DecryptSender расшифровывает Telegram ID отправителя.
//
// ВНИМАНИЕ: эта функция сама по себе не показывает личность человеку —
// она нужна и для технической доставки анонимных ответов. Показ личности
// администратору идёт только через service (RevealSender) с записью в audit_log.
func (c *Codec) DecryptSender(senderEnc []byte, questionID int64) (int64, error) {
	if len(senderEnc) < nonceSize+8 {
		return 0, fmt.Errorf("шифротекст слишком короткий: %d байт", len(senderEnc))
	}
	nonce, ciphertext := senderEnc[:nonceSize], senderEnc[nonceSize:]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, int64BE(questionID))
	if err != nil {
		return 0, fmt.Errorf("расшифровка: %w", err)
	}
	if len(plaintext) != 8 {
		return 0, fmt.Errorf("неожиданная длина открытого текста: %d", len(plaintext))
	}
	return int64(binary.BigEndian.Uint64(plaintext)), nil
}

// SenderHash — детерминированный HMAC-SHA256 от Telegram ID.
// По нему работают блокировки и суточные лимиты без расшифровки личности.
func (c *Codec) SenderHash(senderTgID int64) []byte {
	mac := hmac.New(sha256.New, c.hmacKey)
	mac.Write(int64BE(senderTgID))
	return mac.Sum(nil)
}

// NewLinkToken генерирует токен персональной ссылки: q_<22 символа base64url>.
// 128 бит из crypto/rand — подбор невозможен, из Telegram ID не выводится.
func NewLinkToken() (string, error) {
	raw := make([]byte, tokenLen)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("token: %w", err)
	}
	return TokenPrefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

// int64BE кодирует int64 в 8 байт big-endian.
func int64BE(v int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}
