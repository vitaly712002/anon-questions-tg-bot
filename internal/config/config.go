// Package config читает и валидирует конфигурацию приложения из переменных окружения.
package config

import (
	"encoding/base64"
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Mode string

const (
	ModePolling Mode = "polling"
	ModeWebhook Mode = "webhook"
)

type Config struct {
	Mode          Mode   `env:"MODE" envDefault:"polling"`
	BotToken      string `env:"BOT_TOKEN,required"`
	WebhookURL    string `env:"WEBHOOK_URL"`
	WebhookSecret string `env:"WEBHOOK_SECRET"`
	ListenAddr    string `env:"LISTEN_ADDR" envDefault:":8080"`

	OwnerTgID int64  `env:"OWNER_TG_ID,required"`
	DBURL     string `env:"DB_URL,required"`

	AdminUsername string `env:"ADMIN_USERNAME" envDefault:"admin"`
	AdminPassword string `env:"ADMIN_PASSWORD,required"`

	WebDir string `env:"WEB_DIR" envDefault:""`

	EncryptionKeyB64 string `env:"ENCRYPTION_KEY,required"`
	HMACKeyB64       string `env:"HMAC_KEY,required"`

	RateBurstPerHour int `env:"RATE_BURST_PER_HOUR" envDefault:"20"`
	RateDailyLimit   int `env:"RATE_DAILY_LIMIT" envDefault:"100"`

	Workers int `env:"WORKERS" envDefault:"0"`

	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"15s"`

	EncryptionKey []byte `env:"-"`
	HMACKey       []byte `env:"-"`
}

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
