// Package config читает и валидирует конфигурацию приложения из переменных окружения.
package config

import (
	"encoding/base64"
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

// Mode — режим получения обновлений от Telegram.
type Mode string

const (
	ModePolling Mode = "polling" // локальная разработка
	ModeWebhook Mode = "webhook" // прод
)

// Config — все настройки приложения. Источник — только env (12-factor).
type Config struct {
	Mode          Mode   `env:"MODE" envDefault:"polling"`
	BotToken      string `env:"BOT_TOKEN,required"`
	WebhookURL    string `env:"WEBHOOK_URL"`    // https://bot.example.com/webhook/<суффикс>
	WebhookSecret string `env:"WEBHOOK_SECRET"` // проверка X-Telegram-Bot-Api-Secret-Token
	ListenAddr    string `env:"LISTEN_ADDR" envDefault:":8080"`

	OwnerTgID int64  `env:"OWNER_TG_ID,required"` // владелец бота — sysadmin автоматически
	DBURL     string `env:"DB_URL,required"`

	EncryptionKeyB64 string `env:"ENCRYPTION_KEY,required"` // 32 байта base64 — AES-256-GCM
	HMACKeyB64       string `env:"HMAC_KEY,required"`       // 32 байта base64 — HMAC-SHA256

	// Антифлуд: burst в памяти + суточный лимит по sender_hash.
	RateBurstPerHour int `env:"RATE_BURST_PER_HOUR" envDefault:"20"`
	RateDailyLimit   int `env:"RATE_DAILY_LIMIT" envDefault:"100"`

	// Воркер-пул: 0 = GOMAXPROCS*4.
	Workers int `env:"WORKERS" envDefault:"0"`

	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"15s"`

	// Расшифрованные ключи (заполняются в Load, в env их нет).
	EncryptionKey []byte `env:"-"`
	HMACKey       []byte `env:"-"`
}

// Load читает конфиг из окружения и валидирует его.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}

	switch cfg.Mode {
	case ModePolling, ModeWebhook:
	default:
		return nil, fmt.Errorf("MODE должен быть polling или webhook, получено %q", cfg.Mode)
	}
	if cfg.Mode == ModeWebhook {
		if cfg.WebhookURL == "" {
			return nil, fmt.Errorf("WEBHOOK_URL обязателен в режиме webhook")
		}
		if len(cfg.WebhookSecret) < 32 {
			return nil, fmt.Errorf("WEBHOOK_SECRET обязателен в режиме webhook (32+ символа)")
		}
	}

	var err error
	if cfg.EncryptionKey, err = decodeKey(cfg.EncryptionKeyB64, "ENCRYPTION_KEY"); err != nil {
		return nil, err
	}
	if cfg.HMACKey, err = decodeKey(cfg.HMACKeyB64, "HMAC_KEY"); err != nil {
		return nil, err
	}
	return cfg, nil
}

// decodeKey декодирует base64-ключ и проверяет длину 32 байта.
func decodeKey(b64, name string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("%s: некорректный base64: %w", name, err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("%s: ожидается 32 байта, получено %d", name, len(key))
	}
	return key, nil
}
