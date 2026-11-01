package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-telegram/bot/models"

	"github.com/vitaly/secure-ask-tg-bot/internal/service"
	"github.com/vitaly/secure-ask-tg-bot/internal/storage"
	"github.com/vitaly/secure-ask-tg-bot/internal/storage/gen"
)

// onMessage маршрутизирует входящее личное сообщение: команда или
// продолжение диалога по состоянию из dialog_states.
func (r *Router) onMessage(ctx context.Context, msg *models.Message) error {
	if strings.HasPrefix(msg.Text, "/") {
		return r.onCommand(ctx, msg)
	}
	st, err := r.svc.Store.GetDialogState(ctx, msg.From.ID)
	if storage.IsNotFound(err) {
		return r.reply(ctx, msg.Chat.ID, userMessage(service.ErrNoDialog), nil)
	}
	if err != nil {
		return err
	}
	switch st.State {
	case service.StateAsking:
		return r.onAskMessage(ctx, msg)
	case service.StateReplying:
		return r.onReplyMessage(ctx, msg)
	case service.StateSettingAlias:
		return r.onAliasMessage(ctx, msg)
	case service.StateAdminFind, service.StateAdminRoleFind:
		return r.onAdminFindMessage(ctx, msg, st.State)
	default:
		return r.reply(ctx, msg.Chat.ID, userMessage(service.ErrNoDialog), nil)
	}
}

// onCommand обрабатывает команды. Формат: /cmd [аргумент].
func (r *Router) onCommand(ctx context.Context, msg *models.Message) error {
	cmd, arg, _ := strings.Cut(strings.TrimSpace(msg.Text), " ")
	cmd = strings.ToLower(strings.TrimSuffix(cmd, "@"+r.botUsername()))
	arg = strings.TrimSpace(arg)

	switch cmd {
	case "/start":
		if arg != "" {
			return r.cmdStartAsking(ctx, msg, arg)
		}
		return r.cmdStart(ctx, msg)
	case "/link":
		return r.cmdLink(ctx, msg)
	case "/alias":
		return r.cmdAlias(ctx, msg, arg)
	case "/pause":
		return r.cmdSetAccepting(ctx, msg, false)
	case "/resume":
		return r.cmdSetAccepting(ctx, msg, true)
	case "/stop":
		return r.cmdStop(ctx, msg)
	case "/stats":
		return r.cmdStats(ctx, msg)
	case "/privacy":
		return r.reply(ctx, msg.Chat.ID, privacyText, nil)
	case "/admin":
		return r.cmdAdmin(ctx, msg)
	default:
		return r.reply(ctx, msg.Chat.ID, "Неизвестная команда. Доступные команды — в меню.", nil)
	}
}

// cmdStart — регистрация и выдача персональной ссылки.
func (r *Router) cmdStart(ctx context.Context, msg *models.Message) error {
	user, link, err := r.svc.Users.Register(ctx, msg.From.ID, usernamePtr(msg.From), displayName(msg.From))
	if err != nil {
		return err
	}
	// Пользователь вернулся по /start — старое состояние диалога больше не актуально.
	if err := r.svc.Store.ClearDialogState(ctx, msg.From.ID); err != nil {
		return err
	}
	text := fmt.Sprintf(
		"Привет, %s! 👋\n\n"+
			"Это бот анонимных вопросов. Вот ваша персональная ссылка:\n👉 %s\n\n"+
			"Опубликуйте её в профиле или сторис — и получайте вопросы от друзей и подписчиков!\n\n"+
			"🛡️ Преимущества нашего бота:\n"+
			"• 100%% честная анонимность — у нас принципиально нельзя раскрыть автора ни за Telegram Stars ⭐, ни за деньги, ни через платную подписку.\n"+
			"• Любые форматы — принимайте текст, фото, видео, голосовые и видеосообщения («кружочки»).\n"+
			"• Анонимный диалог — отвечайте на вопросы прямо в боте, собеседник получит ваш ответ также анонимно.\n\n"+
			"Команды:\n"+
			"/link — получить ссылку\n"+
			"/alias — настроить красивый адрес\n"+
			"/pause — временно приостановить приём\n"+
			"/stats — статистика просмотров и вопросов\n"+
			"/privacy — о безопасности и шифровании",
		user.DisplayName, r.svc.Links.URL(link),
	)
	return r.reply(ctx, msg.Chat.ID, text, nil)
}

// cmdStartAsking — гость открыл чью-то ссылку: /start q_<токен> или /start <алиас>.
func (r *Router) cmdStartAsking(ctx context.Context, msg *models.Message, arg string) error {
	name, err := r.svc.Questions.StartAsking(ctx, msg.From.ID, arg, r.svc.Links)
	if err != nil {
		return r.reply(ctx, msg.Chat.ID, userMessage(err), nil)
	}
	text := fmt.Sprintf(
		"✉️ Задайте анонимный вопрос для %s.\n\n"+
			"Отправьте что угодно: текст, фото, видео, голосовое, кружочек…\n\n"+
			"🔒 100%% анонимно: получатель не узнает, кто вы (раскрыть автора за звёзды ⭐ или деньги здесь невозможно).",
		name,
	)
	return r.reply(ctx, msg.Chat.ID, text, nil)
}

// cmdLink показывает персональную ссылку с текстом-приглашением.
func (r *Router) cmdLink(ctx context.Context, msg *models.Message) error {
	_, link, err := r.svc.Users.Register(ctx, msg.From.ID, usernamePtr(msg.From), displayName(msg.From))
	if err != nil {
		return err
	}
	url := r.svc.Links.URL(link)
	text := fmt.Sprintf(
		"🔗 Ваша ссылка:\n%s\n\nТекст для репоста:\n\n«Задай мне анонимный вопрос — я не узнаю, кто ты 👀\n%s»",
		url, url,
	)
	return r.reply(ctx, msg.Chat.ID, text, nil)
}

// cmdAlias — задать алиас: /alias vasya сразу, /alias без аргумента — диалог.
func (r *Router) cmdAlias(ctx context.Context, msg *models.Message, arg string) error {
	user, _, err := r.svc.Users.Register(ctx, msg.From.ID, usernamePtr(msg.From), displayName(msg.From))
	if err != nil {
		return err
	}
	if arg == "" {
		if err := r.setState(ctx, msg.From.ID, service.StateSettingAlias, service.DialogPayload{}); err != nil {
			return err
		}
		return r.reply(ctx, msg.Chat.ID,
			"Пришлите желаемый алиас: 3–32 символа, строчные латинские буквы, цифры, подчёркивание.\n"+
				"Ссылка станет вида t.me/"+r.botUsername()+"?start=<алиас>", nil)
	}
	return r.applyAlias(ctx, msg.Chat.ID, user.ID, arg)
}

// onAliasMessage — пользователь прислал алиас в состоянии setting_alias.
func (r *Router) onAliasMessage(ctx context.Context, msg *models.Message) error {
	user, err := r.svc.Users.Get(ctx, msg.From.ID)
	if err != nil {
		return err
	}
	if msg.Text == "" {
		return r.reply(ctx, msg.Chat.ID, "Алиас должен быть текстом.", nil)
	}
	if err := r.applyAlias(ctx, msg.Chat.ID, user.ID, msg.Text); err != nil {
		return err
	}
	return r.svc.Store.ClearDialogState(ctx, msg.From.ID)
}

// applyAlias устанавливает алиас и отвечает пользователю.
func (r *Router) applyAlias(ctx context.Context, chatID, userID int64, raw string) error {
	alias, err := r.svc.Links.SetAlias(ctx, userID, raw)
	if err != nil {
		return r.reply(ctx, chatID, userMessage(err), nil)
	}
	return r.reply(ctx, chatID,
		fmt.Sprintf("✅ Готово! Новая ссылка:\nhttps://t.me/%s?start=%s", r.botUsername(), alias), nil)
}

// cmdSetAccepting — /pause и /resume.
func (r *Router) cmdSetAccepting(ctx context.Context, msg *models.Message, accepting bool) error {
	user, err := r.svc.Users.Get(ctx, msg.From.ID)
	if err != nil {
		return r.reply(ctx, msg.Chat.ID, "Сначала зарегистрируйтесь: /start", nil)
	}
	if err := r.svc.Users.SetAccepting(ctx, user.ID, accepting); err != nil {
		return err
	}
	if accepting {
		return r.reply(ctx, msg.Chat.ID, "▶️ Приём вопросов возобновлён.", nil)
	}
	return r.reply(ctx, msg.Chat.ID, "⏸ Приём вопросов приостановлен. Вернуть: /resume", nil)
}

// cmdStop — полная отписка, с подтверждением (разрушительное действие).
func (r *Router) cmdStop(ctx context.Context, msg *models.Message) error {
	if _, err := r.svc.Users.Get(ctx, msg.From.ID); err != nil {
		return r.reply(ctx, msg.Chat.ID, "Вы и не подписаны. Начать: /start", nil)
	}
	kb := service.Keyboard{service.Row(
		service.Button{Text: "Да, отключить", Data: "stop:yes"},
		service.Button{Text: "Отмена", Data: "cancel"},
	)}
	return r.reply(ctx, msg.Chat.ID,
		"Точно отключить приём вопросов и деактивировать ссылку?\nВернуть всё можно командой /resume.", kb)
}

// cmdStats — личная статистика.
func (r *Router) cmdStats(ctx context.Context, msg *models.Message) error {
	user, err := r.svc.Users.Get(ctx, msg.From.ID)
	if err != nil {
		return r.reply(ctx, msg.Chat.ID, "Сначала зарегистрируйтесь: /start", nil)
	}
	st, err := r.svc.Users.Stats(ctx, user.ID)
	if err != nil {
		return err
	}
	text := fmt.Sprintf(
		"📊 Ваша статистика:\n├ Переходов по ссылке: %d\n├ Вопросов получено: %d\n├ Отвечено: %d\n└ Заблокировано отправителей: %d",
		st.Clicks, st.Received, st.Answered, st.Blocked,
	)
	return r.reply(ctx, msg.Chat.ID, text, nil)
}

// privacyText — лаконичное соглашение на обработку данных и условия сервиса.
const privacyText = "🔒 Политика конфиденциальности\n\n" +
	"Пользуясь данным ботом, вы даёте согласие на обработку персональных данных " +
	"(Telegram ID, имя пользователя, передаваемые сообщения и медиафайлы) " +
	"в целях обеспечения работы сервиса и модерации.\n\n" +
	"Получатель вопроса не видит имя и контакты отправителя. Администрация " +
	"оставляет за собой право модерации сообщений при нарушении правил."

// setState — сокращение для записи состояния диалога.
func (r *Router) setState(ctx context.Context, tgID int64, state string, p service.DialogPayload) error {
	return r.svc.Store.SetDialogState(ctx, gen.SetDialogStateParams{
		TgID: tgID, State: state, Payload: service.MarshalPayload(p),
	})
}

// reply отправляет ответ в чат (обёртка над клиентом с клавиатурой).
func (r *Router) reply(ctx context.Context, chatID int64, text string, kb service.Keyboard) error {
	_, err := r.tg.SendMessage(ctx, chatID, text, kb)
	return err
}
