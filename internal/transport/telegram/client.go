// Package telegram — транспортный слой: приём обновлений Telegram,
// маршрутизация по хендлерам, построение клавиатур и отправка ответов.
package telegram

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/vitaly/secure-ask-tg-bot/internal/service"
)

// Client реализует service.TG поверх *bot.Bot: ретраи с экспоненциальной
// задержкой и уважением retry_after из ответа 429 (docs/03).
type Client struct {
	b *bot.Bot
}

// NewClient создаёт обёртку Telegram-клиента для сервисного слоя.
func NewClient(b *bot.Bot) *Client { return &Client{b: b} }

const sendAttempts = 3

// withRetry выполняет вызов Telegram API с ретраями.
// 429 — ждём retry_after; прочие сетевые ошибки — экспоненциальный backoff.
// Ошибки вида «bot was blocked by the user» не ретраятся — они постоянные.
func withRetry[T any](ctx context.Context, call func() (T, error)) (T, error) {
	var zero T
	backoff := 500 * time.Millisecond
	for attempt := 1; ; attempt++ {
		res, err := call()
		if err == nil {
			return res, nil
		}
		if attempt >= sendAttempts || isPermanentTGError(err) {
			return zero, err
		}
		wait := backoff
		var tooMany *bot.TooManyRequestsError
		if bot.IsTooManyRequestsError(err) {
			if ok := asTooMany(err, &tooMany); ok {
				wait = time.Duration(tooMany.RetryAfter) * time.Second
			}
		}
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(wait):
		}
		backoff *= 2
	}
}

// asTooMany извлекает *bot.TooManyRequestsError (библиотека не оборачивает
// ошибки, поэтому обычный type assert).
func asTooMany(err error, target **bot.TooManyRequestsError) bool {
	t, ok := err.(*bot.TooManyRequestsError) //nolint:errorlint // библиотека не использует wrapping
	if ok {
		*target = t
	}
	return ok
}

// isPermanentTGError — ошибки, которые ретраить бессмысленно.
func isPermanentTGError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "blocked by the user") ||
		strings.Contains(msg, "chat not found") ||
		strings.Contains(msg, "user is deactivated") ||
		strings.Contains(msg, "message can't be copied") ||
		strings.Contains(msg, "bot can't initiate conversation")
}

// SendMessage отправляет текст с опциональной inline-клавиатурой.
func (c *Client) SendMessage(ctx context.Context, chatID int64, text string, kb service.Keyboard) (int, error) {
	msg, err := withRetry(ctx, func() (*models.Message, error) {
		return c.b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:      chatID,
			Text:        text,
			ReplyMarkup: toMarkup(kb),
		})
	})
	if err != nil {
		return 0, fmt.Errorf("sendMessage: %w", err)
	}
	return msg.ID, nil
}

// CopyMessage копирует сообщение (любой тип контента, без метки «Forwarded from»).
func (c *Client) CopyMessage(ctx context.Context, toChatID, fromChatID int64, messageID int, kb service.Keyboard) (int, error) {
	res, err := withRetry(ctx, func() (*models.MessageID, error) {
		return c.b.CopyMessage(ctx, &bot.CopyMessageParams{
			ChatID:      toChatID,
			FromChatID:  fromChatID,
			MessageID:   messageID,
			ReplyMarkup: toMarkup(kb),
		})
	})
	if err != nil {
		return 0, fmt.Errorf("copyMessage: %w", err)
	}
	return int(res.ID), nil
}

// GetChatInfo возвращает актуальные имя и username чата (для карточки отправителя).
func (c *Client) GetChatInfo(ctx context.Context, chatID int64) (service.ChatInfo, error) {
	chat, err := withRetry(ctx, func() (*models.ChatFullInfo, error) {
		return c.b.GetChat(ctx, &bot.GetChatParams{ChatID: chatID})
	})
	if err != nil {
		return service.ChatInfo{}, fmt.Errorf("getChat: %w", err)
	}
	name := strings.TrimSpace(chat.FirstName + " " + chat.LastName)
	if name == "" {
		name = chat.Title
	}
	return service.ChatInfo{Name: name, Username: chat.Username}, nil
}

// toMarkup конвертирует транспортно-независимую клавиатуру в модель Telegram.
func toMarkup(kb service.Keyboard) models.ReplyMarkup {
	if len(kb) == 0 {
		return nil
	}
	rows := make([][]models.InlineKeyboardButton, len(kb))
	for i, row := range kb {
		rows[i] = make([]models.InlineKeyboardButton, len(row))
		for j, btn := range row {
			rows[i][j] = models.InlineKeyboardButton{Text: btn.Text, CallbackData: btn.Data}
		}
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}
