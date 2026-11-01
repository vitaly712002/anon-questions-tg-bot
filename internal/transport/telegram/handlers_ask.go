package telegram

import (
	"context"
	"errors"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/vitaly/secure-ask-tg-bot/internal/service"
)

// onAskMessage — гость в состоянии asking прислал вопрос (любого типа).
func (r *Router) onAskMessage(ctx context.Context, msg *models.Message) error {
	senderName := displayName(msg.From)
	if msg.From.Username != "" {
		senderName = "@" + msg.From.Username + " (" + senderName + ")"
	}

	err := r.svc.Questions.Submit(ctx, service.SubmitInput{
		SenderTgID:     msg.From.ID,
		SenderUsername: senderName,
		Text:           extractText(msg),
		FileID:         extractFileID(msg),
		SrcChatID:      msg.Chat.ID,
		SrcMsgID:       msg.ID,
		ContentType:    contentType(msg),
		Protected:      msg.HasProtectedContent,
	})
	if err != nil && !errors.Is(err, service.ErrSilentDrop) {
		return r.reply(ctx, msg.Chat.ID, userMessage(err), nil)
	}
	// Успех и silent drop выглядят одинаково: блокировка не раскрывается (docs/06).
	text := "✅ Вопрос отправлен анонимно!\n\n" +
		"Можете отправить ещё одно сообщение — или получить собственную ссылку " +
		"для анонимных вопросов: /link"
	return r.reply(ctx, msg.Chat.ID, text, nil)
}

func extractText(m *models.Message) string {
	if m.Text != "" {
		return m.Text
	}
	if m.Caption != "" {
		return m.Caption
	}
	return ""
}

func extractFileID(m *models.Message) string {
	switch {
	case m.Voice != nil:
		return m.Voice.FileID
	case m.VideoNote != nil:
		return m.VideoNote.FileID
	case len(m.Photo) > 0:
		return m.Photo[len(m.Photo)-1].FileID
	case m.Video != nil:
		return m.Video.FileID
	case m.Audio != nil:
		return m.Audio.FileID
	case m.Document != nil:
		return m.Document.FileID
	case m.Animation != nil:
		return m.Animation.FileID
	default:
		return ""
	}
}

// onReplyMessage — получатель в состоянии replying прислал ответ.
func (r *Router) onReplyMessage(ctx context.Context, msg *models.Message) error {
	if msg.HasProtectedContent {
		return r.reply(ctx, msg.Chat.ID, userMessage(service.ErrProtected), nil)
	}
	text := extractText(msg)
	fileID := extractFileID(msg)
	if err := r.svc.Questions.Answer(ctx, msg.From.ID, msg.ID, text, fileID); err != nil {
		return r.reply(ctx, msg.Chat.ID, userMessage(err), nil)
	}
	return r.reply(ctx, msg.Chat.ID, "✅ Ответ отправлен анонимно.", nil)
}

// onCallback — диспетчер нажатий inline-кнопок.
func (r *Router) onCallback(ctx context.Context, cb *models.CallbackQuery) error {
	prefix, args := service.ParseCB(cb.Data)

	switch prefix {
	case service.CBAnswer: // получатель отвечает на вопрос
		return r.cbAnswer(ctx, cb, args)
	case service.CBBlock: // блокировка отправителя
		return r.cbBlock(ctx, cb, args)
	case service.CBReport: // жалоба
		return r.cbReport(ctx, cb, args)
	case service.CBContinue: // отправитель продолжает диалог
		return r.cbContinue(ctx, cb, args)
	case "stop": // подтверждение /stop
		return r.cbStop(ctx, cb)
	case "cancel":
		return r.ackCallback(ctx, cb, "Отменено", false)
	case service.CBAdminPrefix:
		return r.onAdminCallback(ctx, cb, args)
	default:
		return r.ackCallback(ctx, cb, "", false)
	}
}

// cbAnswer — кнопка «💬 Ответить» под вопросом.
func (r *Router) cbAnswer(ctx context.Context, cb *models.CallbackQuery, args []string) error {
	qid, err := cbID(args)
	if err != nil {
		return r.ackCallback(ctx, cb, "", false)
	}
	if err := r.svc.Questions.StartReplying(ctx, cb.From.ID, qid); err != nil {
		return r.ackCallback(ctx, cb, userMessage(err), true)
	}
	if err := r.ackCallback(ctx, cb, "", false); err != nil {
		return err
	}
	return r.reply(ctx, cb.From.ID,
		"✍️ Пришлите ответ одним сообщением — он уйдёт отправителю анонимно.", nil)
}

// cbBlock — кнопка «🚫 Заблокировать».
func (r *Router) cbBlock(ctx context.Context, cb *models.CallbackQuery, args []string) error {
	qid, err := cbID(args)
	if err != nil {
		return r.ackCallback(ctx, cb, "", false)
	}
	if err := r.svc.Blocks.BlockSenderOfQuestion(ctx, cb.From.ID, qid); err != nil {
		return r.ackCallback(ctx, cb, userMessage(err), true)
	}
	return r.ackCallback(ctx, cb,
		"Отправитель заблокирован. Он не узнает об этом — его сообщения будут молча отклоняться.", true)
}

// cbReport — кнопка «⚠️ Пожаловаться».
func (r *Router) cbReport(ctx context.Context, cb *models.CallbackQuery, args []string) error {
	qid, err := cbID(args)
	if err != nil {
		return r.ackCallback(ctx, cb, "", false)
	}
	if err := r.svc.Questions.Report(ctx, cb.From.ID, qid); err != nil {
		return r.ackCallback(ctx, cb, userMessage(err), true)
	}
	return r.ackCallback(ctx, cb, "Жалоба отправлена модераторам. Спасибо!", true)
}

// cbContinue — кнопка «💬 Ответить» под ответом (у отправителя вопроса).
func (r *Router) cbContinue(ctx context.Context, cb *models.CallbackQuery, args []string) error {
	qid, err := cbID(args)
	if err != nil {
		return r.ackCallback(ctx, cb, "", false)
	}
	if err := r.svc.Questions.StartContinue(ctx, cb.From.ID, qid); err != nil {
		return r.ackCallback(ctx, cb, userMessage(err), true)
	}
	if err := r.ackCallback(ctx, cb, "", false); err != nil {
		return err
	}
	return r.reply(ctx, cb.From.ID, "✍️ Пришлите сообщение — оно уйдёт анонимно.", nil)
}

// cbStop — подтверждение полной отписки.
func (r *Router) cbStop(ctx context.Context, cb *models.CallbackQuery) error {
	user, err := r.svc.Users.Get(ctx, cb.From.ID)
	if err != nil {
		return r.ackCallback(ctx, cb, userMessage(err), true)
	}
	if err := r.svc.Users.Stop(ctx, user.ID); err != nil {
		return err
	}
	if err := r.ackCallback(ctx, cb, "", false); err != nil {
		return err
	}
	return r.reply(ctx, cb.From.ID,
		"Приём вопросов отключён, ссылка деактивирована.\nВернуть всё: /resume", nil)
}

// ackCallback подтверждает callback (убирает «часики» на кнопке).
func (r *Router) ackCallback(ctx context.Context, cb *models.CallbackQuery, text string, alert bool) error {
	_, err := r.tg.b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: cb.ID,
		Text:            text,
		ShowAlert:       alert,
	})
	return err
}

// cbID разбирает единственный числовой аргумент callback data.
func cbID(args []string) (int64, error) {
	if len(args) != 1 {
		return 0, errInvalidCB
	}
	return service.CBInt64(args[0])
}

var errInvalidCB = errors.New("некорректный callback")
