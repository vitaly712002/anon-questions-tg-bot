// Package service — бизнес-логика бота (ядро приложения).
//
// Правило зависимостей (docs/03): service не импортирует ни библиотеку бота,
// ни драйвер pgx — только собственные интерфейсы, объявленные в этом файле.
// Типы строк БД берутся из сгенерированного sqlc-пакета gen (он тянет за
// собой типы pgtype в сигнатурах — это цена типобезопасности sqlc).
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/vitaly/secure-ask-tg-bot/internal/storage/gen"
)

// Querier — подмножество запросов, которое нужно бизнес-логике.
// *gen.Queries реализует его автоматически; в тестах подменяется фейком.
type Querier interface {
	// users
	UpsertUser(ctx context.Context, arg gen.UpsertUserParams) (gen.User, error)
	GetUserByTgID(ctx context.Context, tgID int64) (gen.User, error)
	GetUserByID(ctx context.Context, id int64) (gen.User, error)
	GetUserByUsername(ctx context.Context, username string) (gen.User, error)
	SetUserRole(ctx context.Context, arg gen.SetUserRoleParams) error
	EnsureOwnerSysadmin(ctx context.Context, tgID int64) error
	SetUserAccepting(ctx context.Context, arg gen.SetUserAcceptingParams) error
	SetUserBanned(ctx context.Context, arg gen.SetUserBannedParams) error
	ListAdmins(ctx context.Context) ([]gen.User, error)
	ListBannedUsers(ctx context.Context) ([]gen.User, error)
	CountUsers(ctx context.Context) (int64, error)

	// links
	CreateLink(ctx context.Context, arg gen.CreateLinkParams) (gen.Link, error)
	GetActiveLinkByUserID(ctx context.Context, userID int64) (gen.Link, error)
	GetLinkByToken(ctx context.Context, token string) (gen.Link, error)
	GetLinkByAlias(ctx context.Context, alias string) (gen.Link, error)
	GetLinkByID(ctx context.Context, id int64) (gen.Link, error)
	SetLinkAlias(ctx context.Context, arg gen.SetLinkAliasParams) error
	SetLinksActiveByUser(ctx context.Context, arg gen.SetLinksActiveByUserParams) error
	IncrementLinkClicks(ctx context.Context, id int64) error

	// dialog states
	SetDialogState(ctx context.Context, arg gen.SetDialogStateParams) error
	GetDialogState(ctx context.Context, tgID int64) (gen.DialogState, error)
	ClearDialogState(ctx context.Context, tgID int64) error

	// questions
	InsertQuestion(ctx context.Context, arg gen.InsertQuestionParams) (int64, error)
	UpdateQuestionSenderEnc(ctx context.Context, arg gen.UpdateQuestionSenderEncParams) error
	GetQuestion(ctx context.Context, id int64) (gen.Question, error)
	MarkQuestionDelivered(ctx context.Context, arg gen.MarkQuestionDeliveredParams) error
	WipeQuestionSrc(ctx context.Context, id int64) error
	MarkQuestionAnswered(ctx context.Context, id int64) error
	CountQuestionsBySenderSince(ctx context.Context, arg gen.CountQuestionsBySenderSinceParams) (int64, error)
	CountQuestionsFromSenderToRecipient(ctx context.Context, arg gen.CountQuestionsFromSenderToRecipientParams) (int64, error)
	CountQuestionsFromSenderToRecipientSince(ctx context.Context, arg gen.CountQuestionsFromSenderToRecipientSinceParams) (int64, error)
	CountQuestionsToRecipient(ctx context.Context, recipientID int64) (int64, error)
	CountQuestionsAnsweredByRecipient(ctx context.Context, recipientID int64) (int64, error)
	CountQuestionsSince(ctx context.Context, since pgtype.Timestamptz) (int64, error)
	CountQuestions(ctx context.Context) (int64, error)

	// blocks
	AddBlock(ctx context.Context, arg gen.AddBlockParams) error
	IsBlocked(ctx context.Context, arg gen.IsBlockedParams) (bool, error)
	CountBlocksByRecipient(ctx context.Context, recipientID int64) (int64, error)

	// admin
	AddWatch(ctx context.Context, arg gen.AddWatchParams) error
	RemoveWatch(ctx context.Context, arg gen.RemoveWatchParams) error
	ListWatchesByAdmin(ctx context.Context, adminID int64) ([]gen.User, error)
	ListWatcherTgIDs(ctx context.Context, targetID int64) ([]gen.ListWatcherTgIDsRow, error)
	CountWatchesByAdmin(ctx context.Context, adminID int64) (int64, error)
	InsertAudit(ctx context.Context, arg gen.InsertAuditParams) error
	ListAuditRecent(ctx context.Context, limit int32) ([]gen.ListAuditRecentRow, error)
}

// Store — Querier + транзакции. Реализуется адаптером над storage.Storage.
type Store interface {
	Querier
	WithTx(ctx context.Context, fn func(q Querier) error) error
	SaveQuestionContent(ctx context.Context, questionID int64, textEnc, senderUserEnc []byte, fileID string) error
	GetBotSettings(ctx context.Context) (map[string]string, error)
}

// Button и Keyboard — транспортно-независимое описание inline-клавиатуры.
// Транспортный слой конвертирует их в модели Telegram.
type Button struct {
	Text string
	Data string // callback data
}

// Keyboard — ряды кнопок.
type Keyboard [][]Button

// Row — вспомогательный конструктор ряда кнопок.
func Row(buttons ...Button) []Button { return buttons }

// ChatInfo — актуальные данные чата/пользователя из Telegram (getChat).
type ChatInfo struct {
	Name     string
	Username string // без @, может быть пустым
}

// TG — минимальный интерфейс Telegram-клиента для бизнес-логики.
// Реализация (с ретраями и уважением retry_after) — в transport/telegram.
type TG interface {
	SendMessage(ctx context.Context, chatID int64, text string, kb Keyboard) (msgID int, err error)
	CopyMessage(ctx context.Context, toChatID, fromChatID int64, messageID int, kb Keyboard) (msgID int, err error)
	GetChatInfo(ctx context.Context, chatID int64) (ChatInfo, error)
}

// BurstLimiter — антифлуд в памяти (см. internal/ratelimit).
type BurstLimiter interface {
	Allow(key string) bool
}

// Ошибки бизнес-логики. Транспорт превращает их в вежливые ответы пользователю.
var (
	ErrNotFound      = errors.New("не найдено")
	ErrLinkInactive  = errors.New("ссылка не активна")
	ErrNotAccepting  = errors.New("получатель приостановил приём вопросов")
	ErrSilentDrop    = errors.New("сообщение молча отклонено")                // блокировка или бан — отправителю не сообщаем
	ErrRateLimited   = errors.New("слишком много вопросов, попробуйте позже") // burst-лимит
	ErrDailyLimit    = errors.New("превышен суточный лимит вопросов")
	ErrProtected     = errors.New("контент защищён от копирования")
	ErrDelivery      = errors.New("не удалось доставить сообщение")
	ErrAliasTaken    = errors.New("алиас занят")
	ErrAliasInvalid  = errors.New("недопустимый алиас")
	ErrAliasReserved = errors.New("алиас зарезервирован")
	ErrForbidden     = errors.New("недостаточно прав")
	ErrOwnerChange   = errors.New("владельца бота нельзя разжаловать")
	ErrNoDialog      = errors.New("нет активного диалога")
)

// Состояния диалога (dialog_states.state).
const (
	StateAsking        = "asking"          // гость пишет вопрос
	StateReplying      = "replying"        // получатель пишет ответ на вопрос
	StateSettingAlias  = "setting_alias"   // пользователь вводит алиас
	StateAdminFind     = "admin_find"      // админ ищет пользователя (карточка)
	StateAdminRoleFind = "admin_role_find" // sysadmin ищет пользователя (роли)
)

// DialogPayload — полезная нагрузка состояния диалога (dialog_states.payload).
type DialogPayload struct {
	LinkID     int64  `json:"link_id,omitempty"`
	QuestionID int64  `json:"question_id,omitempty"`
	ReplyTo    *int64 `json:"reply_to,omitempty"`
}

// MarshalPayload сериализует полезную нагрузку состояния в JSONB.
func MarshalPayload(p DialogPayload) []byte {
	b, err := json.Marshal(p)
	if err != nil { // недостижимо для этой структуры
		return []byte("{}")
	}
	return b
}

// UnmarshalPayload разбирает полезную нагрузку состояния.
func UnmarshalPayload(raw []byte) (DialogPayload, error) {
	var p DialogPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return DialogPayload{}, fmt.Errorf("payload: %w", err)
	}
	return p, nil
}
