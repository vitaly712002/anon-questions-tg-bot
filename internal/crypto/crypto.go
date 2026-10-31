// Package crypto инкапсулирует криптографию проекта: шифрование ID отправителей, HMAC-хэши и токены ссылок.
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
	nonceSize = 12
	tokenLen  = 16 // 128 бит энтропии для токена ссылки

	// TokenPrefix отличает токен ссылки от алиаса в /start-аргументе.
	TokenPrefix = "q_"
)

type Codec struct {
	aead    cipher.AEAD
	hmacKey []byte
}

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

func (c *Codec) EncryptText(plaintext string) ([]byte, error) {
	if plaintext == "" {
		return nil, nil
	}
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}
	ciphertext := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return ciphertext, nil
}

func (c *Codec) DecryptText(ciphertext []byte) (string, error) {
	if len(ciphertext) < nonceSize {
		return "", nil
	}
	nonce, data := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := c.aead.Open(nil, nonce, data, nil)
	if err != nil {
		return "", fmt.Errorf("расшифровка текста: %w", err)
	}
	return string(plaintext), nil
}

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

// По нему работают блокировки и суточные лимиты без расшифровки личности.
func (c *Codec) SenderHash(senderTgID int64) []byte {
	mac := hmac.New(sha256.New, c.hmacKey)
	mac.Write(int64BE(senderTgID))
	return mac.Sum(nil)
}

// 128 бит из crypto/rand — подбор невозможен, из Telegram ID не выводится.
func NewLinkToken() (string, error) {
	raw := make([]byte, tokenLen)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("token: %w", err)
	}
	return TokenPrefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

func int64BE(v int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}
