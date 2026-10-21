package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/vitaly/secure-ask-tg-bot/internal/service"
	"github.com/vitaly/secure-ask-tg-bot/internal/storage/gen"
)

// cmdAdmin — вход в админ-панель. Для обычного пользователя команда
// «не существует» (не палим наличие панели).
func (r *Router) cmdAdmin(ctx context.Context, msg *models.Message) error {
	viewer, err := r.requireAdmin(ctx, msg.From.ID)
	if err != nil {
		return r.reply(ctx, msg.Chat.ID, "Неизвестная команда. Доступные команды — в меню.", nil)
	}
	return r.sendAdminPanel(ctx, viewer)
}

// sendAdminPanel отправляет панель по роли.
func (r *Router) sendAdminPanel(ctx context.Context, viewer gen.User) error {
	st, err := r.svc.Admin.Stats(ctx, viewer)
	if err != nil {
		return err
	}
	watches, err := r.svc.Store.CountWatchesByAdmin(ctx, viewer.ID)
	if err != nil {
		return err
	}
	text, kb := adminPanel(viewer, st, watches)
	return r.reply(ctx, viewer.TgID, text, kb)
}

// onAdminCallback — все кнопки с префиксом "a:". Роль перечитывается из БД
// на каждое нажатие (docs/05): callback мог быть подделан или «протухнуть».
func (r *Router) onAdminCallback(ctx context.Context, cb *models.CallbackQuery, args []string) error {
	viewer, err := r.requireAdmin(ctx, cb.From.ID)
	if err != nil {
		return r.ackCallback(ctx, cb, userMessage(service.ErrForbidden), true)
	}
	if len(args) == 0 {
		return r.ackCallback(ctx, cb, "", false)
	}
	action, rest := args[0], args[1:]

	switch action {
	case "find":
		return r.cbAdminFind(ctx, cb, viewer, service.StateAdminFind)
	case "roles":
		if viewer.Role != gen.UserRoleSysadmin {
			return r.ackCallback(ctx, cb, userMessage(service.ErrForbidden), true)
		}
		return r.cbAdminFind(ctx, cb, viewer, service.StateAdminRoleFind)
	case "watches":
		return r.cbAdminWatches(ctx, cb, viewer)
	case "banned":
		return r.cbAdminBanned(ctx, cb, viewer)
	case "audit":
		return r.cbAdminAudit(ctx, cb, viewer)
	case "stats":
		return r.cbAdminStats(ctx, cb, viewer)
	case "grant": // a:grant:<role>:<uid> → подтверждение
		return r.cbAdminGrantConfirm(ctx, cb, viewer, rest)
	case "grantc": // подтверждено
		return r.cbAdminGrant(ctx, cb, viewer, rest)
	case "revoke": // a:revoke:<uid> → подтверждение
		return r.cbAdminRevokeConfirm(ctx, cb, viewer, rest)
	case "revokec":
		return r.cbAdminRevoke(ctx, cb, viewer, rest)
	case "watch", "unwatch":
		return r.cbAdminWatchToggle(ctx, cb, viewer, action, rest)
	case "ban": // a:ban:<uid> → подтверждение
		return r.cbAdminBanConfirm(ctx, cb, viewer, rest)
	case "banc":
		return r.cbAdminBan(ctx, cb, viewer, rest, true)
	case "unban":
		return r.cbAdminBan(ctx, cb, viewer, rest, false)
	case "banq": // из карточки отправителя: подтверждение глобального бана
		return r.cbAdminBanQConfirm(ctx, cb, rest)
	case "banqc":
		return r.cbAdminBanQ(ctx, cb, viewer, rest)
	case "watchq": // подписка на отправителя вопроса
		return r.cbAdminWatchQ(ctx, cb, viewer, rest)
	default:
		return r.ackCallback(ctx, cb, "", false)
	}
}

// cbAdminFind — запросить у админа идентификатор пользователя.
func (r *Router) cbAdminFind(ctx context.Context, cb *models.CallbackQuery, viewer gen.User, state string) error {
	if err := r.setState(ctx, viewer.TgID, state, service.DialogPayload{}); err != nil {
		return err
	}
	if err := r.ackCallback(ctx, cb, "", false); err != nil {
		return err
	}
	return r.reply(ctx, viewer.TgID,
		"Пришлите @username, Telegram ID или перешлите сообщение пользователя.", nil)
}

// onAdminFindMessage — админ прислал идентификатор в состоянии поиска.
func (r *Router) onAdminFindMessage(ctx context.Context, msg *models.Message, state string) error {
	var viewer gen.User
	var err error
	if state == service.StateAdminRoleFind {
		viewer, err = r.requireSysadmin(ctx, msg.From.ID)
	} else {
		viewer, err = r.requireAdmin(ctx, msg.From.ID)
	}
	if err != nil {
		// Роль отозвали, пока состояние висело — молча сбрасываем.
		return r.svc.Store.ClearDialogState(ctx, msg.From.ID)
	}

	var target gen.User
	// Пересланное сообщение — самый надёжный способ (если пользователь не скрыл аккаунт).
	if msg.ForwardOrigin != nil && msg.ForwardOrigin.MessageOriginUser != nil {
		target, err = r.svc.Admin.FindUser(ctx, itoa(msg.ForwardOrigin.MessageOriginUser.SenderUser.ID))
	} else if msg.ForwardOrigin != nil {
		return r.reply(ctx, msg.Chat.ID, "Автор скрыл аккаунт в настройках пересылки. Пришлите @username или ID.", nil)
	} else if strings.TrimSpace(msg.Text) != "" {
		target, err = r.svc.Admin.FindUser(ctx, msg.Text)
	} else {
		return r.reply(ctx, msg.Chat.ID, "Пришлите @username, ID или пересланное сообщение.", nil)
	}
	if err != nil {
		return r.reply(ctx, msg.Chat.ID, "Пользователь не найден среди зарегистрированных в боте.", nil)
	}
	if err := r.svc.Store.ClearDialogState(ctx, msg.From.ID); err != nil {
		return err
	}
	return r.sendUserCard(ctx, viewer, target.ID)
}

// sendUserCard отправляет карточку пользователя админу.
func (r *Router) sendUserCard(ctx context.Context, viewer gen.User, targetID int64) error {
	card, err := r.svc.Admin.Card(ctx, viewer, targetID)
	if err != nil {
		return r.reply(ctx, viewer.TgID, userMessage(err), nil)
	}
	text, kb := userCard(viewer, card)
	return r.reply(ctx, viewer.TgID, text, kb)
}

// cbAdminWatches — список подписок с кнопками отписки.
func (r *Router) cbAdminWatches(ctx context.Context, cb *models.CallbackQuery, viewer gen.User) error {
	watches, err := r.svc.Admin.ListWatches(ctx, viewer)
	if err != nil {
		return r.ackCallback(ctx, cb, userMessage(err), true)
	}
	if err := r.ackCallback(ctx, cb, "", false); err != nil {
		return err
	}
	if len(watches) == 0 {
		return r.reply(ctx, viewer.TgID, "Подписок нет. Подписаться можно из карточки пользователя.", nil)
	}
	var kb service.Keyboard
	for _, w := range watches {
		kb = append(kb, service.Row(service.Button{
			Text: fmt.Sprintf("❌ %s (id %d)", w.DisplayName, w.ID),
			Data: service.CB("a", "unwatch", w.ID),
		}))
	}
	return r.reply(ctx, viewer.TgID, "👁 Ваши подписки — нажмите, чтобы отписаться:", kb)
}

// cbAdminBanned — список забаненных с кнопками разбана.
func (r *Router) cbAdminBanned(ctx context.Context, cb *models.CallbackQuery, viewer gen.User) error {
	banned, err := r.svc.Admin.ListBanned(ctx, viewer)
	if err != nil {
		return r.ackCallback(ctx, cb, userMessage(err), true)
	}
	if err := r.ackCallback(ctx, cb, "", false); err != nil {
		return err
	}
	if len(banned) == 0 {
		return r.reply(ctx, viewer.TgID, "Забаненных нет.", nil)
	}
	var kb service.Keyboard
	for _, u := range banned {
		kb = append(kb, service.Row(service.Button{
			Text: fmt.Sprintf("✅ Разбанить %s (id %d)", u.DisplayName, u.ID),
			Data: service.CB("a", "unban", u.ID),
		}))
	}
	return r.reply(ctx, viewer.TgID, "🚫 Забаненные пользователи:", kb)
}

// cbAdminAudit — последние записи журнала (только sysadmin).
func (r *Router) cbAdminAudit(ctx context.Context, cb *models.CallbackQuery, viewer gen.User) error {
	rows, err := r.svc.Admin.Audit(ctx, viewer, 20)
	if err != nil {
		return r.ackCallback(ctx, cb, userMessage(err), true)
	}
	if err := r.ackCallback(ctx, cb, "", false); err != nil {
		return err
	}
	if len(rows) == 0 {
		return r.reply(ctx, viewer.TgID, "Журнал пуст.", nil)
	}
	var b strings.Builder
	b.WriteString("📜 Последние действия:\n")
	for _, row := range rows {
		target := ""
		if row.TargetName != nil && *row.TargetName != "" {
			target = " → " + *row.TargetName
		}
		fmt.Fprintf(&b, "· %s — %s%s (%s)\n",
			row.CreatedAt.Time.Format("01-02 15:04"), row.Action, target, row.ActorName)
	}
	return r.reply(ctx, viewer.TgID, b.String(), nil)
}

// cbAdminStats — сводная статистика бота.
func (r *Router) cbAdminStats(ctx context.Context, cb *models.CallbackQuery, viewer gen.User) error {
	st, err := r.svc.Admin.Stats(ctx, viewer)
	if err != nil {
		return r.ackCallback(ctx, cb, userMessage(err), true)
	}
	if err := r.ackCallback(ctx, cb, "", false); err != nil {
		return err
	}
	text := fmt.Sprintf("📊 Статистика бота:\n├ Пользователей: %d\n├ Вопросов всего: %d\n└ Вопросов за сутки: %d",
		st.Users, st.Questions, st.QuestionsToday)
	return r.reply(ctx, viewer.TgID, text, nil)
}

// cbAdminGrantConfirm — «Точно выдать роль? [Да] [Отмена]».
func (r *Router) cbAdminGrantConfirm(ctx context.Context, cb *models.CallbackQuery, viewer gen.User, rest []string) error {
	if viewer.Role != gen.UserRoleSysadmin || len(rest) != 2 {
		return r.ackCallback(ctx, cb, userMessage(service.ErrForbidden), true)
	}
	role, uid := rest[0], rest[1]
	if err := r.ackCallback(ctx, cb, "", false); err != nil {
		return err
	}
	return r.reply(ctx, viewer.TgID,
		fmt.Sprintf("Точно выдать роль %s пользователю id %s?", role, uid),
		confirmKeyboard(service.CB("a", "grantc", role, uid)))
}

// cbAdminGrant — выдача роли после подтверждения.
func (r *Router) cbAdminGrant(ctx context.Context, cb *models.CallbackQuery, viewer gen.User, rest []string) error {
	if len(rest) != 2 {
		return r.ackCallback(ctx, cb, "", false)
	}
	role := gen.UserRole(rest[0])
	if role != gen.UserRoleAdmin && role != gen.UserRoleSysadmin {
		return r.ackCallback(ctx, cb, "", false)
	}
	uid, err := service.CBInt64(rest[1])
	if err != nil {
		return r.ackCallback(ctx, cb, "", false)
	}
	if err := r.svc.Admin.SetRole(ctx, viewer, uid, role); err != nil {
		return r.ackCallback(ctx, cb, userMessage(err), true)
	}
	r.syncCommandsForUser(ctx, uid, true)
	if err := r.ackCallback(ctx, cb, "Роль выдана ✅", false); err != nil {
		return err
	}
	return r.sendUserCard(ctx, viewer, uid)
}

// cbAdminRevokeConfirm — подтверждение снятия роли.
func (r *Router) cbAdminRevokeConfirm(ctx context.Context, cb *models.CallbackQuery, viewer gen.User, rest []string) error {
	if viewer.Role != gen.UserRoleSysadmin || len(rest) != 1 {
		return r.ackCallback(ctx, cb, userMessage(service.ErrForbidden), true)
	}
	if err := r.ackCallback(ctx, cb, "", false); err != nil {
		return err
	}
	return r.reply(ctx, viewer.TgID,
		fmt.Sprintf("Точно снять административную роль с пользователя id %s?", rest[0]),
		confirmKeyboard(service.CB("a", "revokec", rest[0])))
}

// cbAdminRevoke — снятие роли после подтверждения.
func (r *Router) cbAdminRevoke(ctx context.Context, cb *models.CallbackQuery, viewer gen.User, rest []string) error {
	uid, err := cbID(rest)
	if err != nil {
		return r.ackCallback(ctx, cb, "", false)
	}
	if err := r.svc.Admin.SetRole(ctx, viewer, uid, gen.UserRoleUser); err != nil {
		return r.ackCallback(ctx, cb, userMessage(err), true)
	}
	r.syncCommandsForUser(ctx, uid, false)
	if err := r.ackCallback(ctx, cb, "Роль снята", false); err != nil {
		return err
	}
	return r.sendUserCard(ctx, viewer, uid)
}

// cbAdminWatchToggle — подписка/отписка из карточки или списка подписок.
func (r *Router) cbAdminWatchToggle(ctx context.Context, cb *models.CallbackQuery, viewer gen.User, action string, rest []string) error {
	uid, err := cbID(rest)
	if err != nil {
		return r.ackCallback(ctx, cb, "", false)
	}
	if action == "watch" {
		err = r.svc.Admin.Watch(ctx, viewer, uid)
	} else {
		err = r.svc.Admin.Unwatch(ctx, viewer, uid)
	}
	if err != nil {
		return r.ackCallback(ctx, cb, userMessage(err), true)
	}
	if action == "watch" {
		return r.ackCallback(ctx, cb, "Подписка оформлена 👁", false)
	}
	return r.ackCallback(ctx, cb, "Подписка снята", false)
}

// cbAdminBanConfirm — подтверждение бана из карточки пользователя.
func (r *Router) cbAdminBanConfirm(ctx context.Context, cb *models.CallbackQuery, viewer gen.User, rest []string) error {
	if len(rest) != 1 {
		return r.ackCallback(ctx, cb, "", false)
	}
	if err := r.ackCallback(ctx, cb, "", false); err != nil {
		return err
	}
	return r.reply(ctx, viewer.TgID,
		fmt.Sprintf("Точно забанить пользователя id %s во всём боте?", rest[0]),
		confirmKeyboard(service.CB("a", "banc", rest[0])))
}

// cbAdminBan — бан/разбан по id пользователя.
func (r *Router) cbAdminBan(ctx context.Context, cb *models.CallbackQuery, viewer gen.User, rest []string, banned bool) error {
	uid, err := cbID(rest)
	if err != nil {
		return r.ackCallback(ctx, cb, "", false)
	}
	if err := r.svc.Admin.SetBanned(ctx, viewer, uid, banned); err != nil {
		return r.ackCallback(ctx, cb, userMessage(err), true)
	}
	if banned {
		return r.ackCallback(ctx, cb, "Пользователь забанен 🚫", false)
	}
	return r.ackCallback(ctx, cb, "Пользователь разбанен ✅", false)
}

// cbAdminBanQConfirm — подтверждение глобального бана отправителя вопроса.
func (r *Router) cbAdminBanQConfirm(ctx context.Context, cb *models.CallbackQuery, rest []string) error {
	if len(rest) != 1 {
		return r.ackCallback(ctx, cb, "", false)
	}
	if err := r.ackCallback(ctx, cb, "", false); err != nil {
		return err
	}
	return r.reply(ctx, cb.From.ID,
		"Точно забанить отправителя этого вопроса во всём боте?",
		confirmKeyboard(service.CB("a", "banqc", rest[0])))
}

// cbAdminBanQ — глобальный бан отправителя вопроса (с раскрытием и аудитом).
func (r *Router) cbAdminBanQ(ctx context.Context, cb *models.CallbackQuery, viewer gen.User, rest []string) error {
	qid, err := cbID(rest)
	if err != nil {
		return r.ackCallback(ctx, cb, "", false)
	}
	if err := r.svc.Admin.BanSenderOfQuestion(ctx, viewer, qid); err != nil {
		return r.ackCallback(ctx, cb, userMessage(err), true)
	}
	return r.ackCallback(ctx, cb, "Отправитель забанен во всём боте 🚫", true)
}

// cbAdminWatchQ — подписка на отправителя вопроса (с раскрытием и аудитом).
func (r *Router) cbAdminWatchQ(ctx context.Context, cb *models.CallbackQuery, viewer gen.User, rest []string) error {
	qid, err := cbID(rest)
	if err != nil {
		return r.ackCallback(ctx, cb, "", false)
	}
	if err := r.svc.Admin.WatchSenderOfQuestion(ctx, viewer, qid); err != nil {
		return r.ackCallback(ctx, cb, userMessage(err), true)
	}
	return r.ackCallback(ctx, cb, "Подписка на отправителя оформлена 👁", true)
}

// syncCommandsForUser обновляет меню команд чата после смены роли:
// админам видна /admin, у остальных — базовый набор.
func (r *Router) syncCommandsForUser(ctx context.Context, userID int64, isAdmin bool) {
	target, err := r.svc.Store.GetUserByID(ctx, userID)
	if err != nil {
		r.log.Warn("syncCommands: пользователь не найден", "err", err)
		return
	}
	cmds := UserCommands()
	if isAdmin {
		cmds = append(cmds, models.BotCommand{Command: "admin", Description: "Панель администрирования"})
	}
	_, err = r.tg.b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: cmds,
		Scope:    &models.BotCommandScopeChat{ChatID: target.TgID},
	})
	if err != nil {
		r.log.Warn("syncCommands: setMyCommands", "err", err)
	}
}

// UserCommands — базовый набор команд (default scope задаётся в main).
func UserCommands() []models.BotCommand {
	return []models.BotCommand{
		{Command: "start", Description: "Регистрация и ваша ссылка"},
		{Command: "link", Description: "Показать вашу ссылку"},
		{Command: "alias", Description: "Задать красивый адрес ссылки"},
		{Command: "pause", Description: "Приостановить приём вопросов"},
		{Command: "resume", Description: "Возобновить приём вопросов"},
		{Command: "stats", Description: "Личная статистика"},
		{Command: "stop", Description: "Отключить приём вопросов"},
		{Command: "privacy", Description: "Как устроена анонимность"},
	}
}
