package telegram

import (
	"fmt"

	"github.com/vitaly/secure-ask-tg-bot/internal/service"
	"github.com/vitaly/secure-ask-tg-bot/internal/storage/gen"
)

// adminPanel строит текст и клавиатуру панели /admin по роли (docs/06).
func adminPanel(viewer gen.User, st service.BotStats, watchCount int64) (string, service.Keyboard) {
	title := "🛡 Панель администратора"
	if viewer.Role == gen.UserRoleSysadmin {
		title = "🛡 Панель системного администратора"
	}
	text := fmt.Sprintf("%s\n\n👥 Пользователи: %d · ❓ Вопросов за сутки: %d", title, st.Users, st.QuestionsToday)

	kb := service.Keyboard{
		service.Row(service.Button{Text: "👤 Найти пользователя", Data: service.CB("a", "find")}),
	}
	if viewer.Role == gen.UserRoleSysadmin {
		kb = append(kb, service.Row(service.Button{Text: "🎖 Управление ролями", Data: service.CB("a", "roles")}))
	}
	kb = append(kb,
		service.Row(service.Button{Text: fmt.Sprintf("👁 Мои подписки (%d)", watchCount), Data: service.CB("a", "watches")}),
		service.Row(service.Button{Text: "🚫 Забаненные", Data: service.CB("a", "banned")}),
	)
	if viewer.Role == gen.UserRoleSysadmin {
		kb = append(kb, service.Row(service.Button{Text: "📜 Журнал действий", Data: service.CB("a", "audit")}))
	}
	kb = append(kb, service.Row(service.Button{Text: "📊 Статистика бота", Data: service.CB("a", "stats")}))
	return text, kb
}

// userCard строит карточку пользователя с кнопками по роли смотрящего (docs/06).
func userCard(viewer gen.User, c service.UserCard) (string, service.Keyboard) {
	u := c.User
	username := "—"
	if u.TgUsername != nil && *u.TgUsername != "" {
		username = "@" + *u.TgUsername
	}
	flags := ""
	if c.IsOwner {
		flags += " · 👑 владелец"
	}
	if u.Banned {
		flags += " · 🚫 забанен"
	}
	if !u.Accepting {
		flags += " · ⏸ на паузе"
	}
	text := fmt.Sprintf(
		"👤 %s · %s · id %d%s\nРоль: %s · Вопросов получено: %d · Зарегистрирован: %s",
		u.DisplayName, username, u.TgID, flags, u.Role, c.Received, u.CreatedAt.Time.Format("2006-01-02"),
	)

	var kb service.Keyboard
	// Роли — только sysadmin; владельца не трогаем (docs/06).
	if viewer.Role == gen.UserRoleSysadmin && !c.IsOwner {
		if u.Role == gen.UserRoleUser {
			kb = append(kb, service.Row(
				service.Button{Text: "Назначить admin", Data: service.CB("a", "grant", "admin", u.ID)},
				service.Button{Text: "Назначить sysadmin", Data: service.CB("a", "grant", "sysadmin", u.ID)},
			))
		} else {
			kb = append(kb, service.Row(service.Button{Text: "Снять роль", Data: service.CB("a", "revoke", u.ID)}))
		}
	}
	watchBtn := service.Button{Text: "👁 Подписаться", Data: service.CB("a", "watch", u.ID)}
	if c.Watched {
		watchBtn = service.Button{Text: "👁 Отписаться", Data: service.CB("a", "unwatch", u.ID)}
	}
	banBtn := service.Button{Text: "🚫 Забанить", Data: service.CB("a", "ban", u.ID)}
	if u.Banned {
		banBtn = service.Button{Text: "✅ Разбанить", Data: service.CB("a", "unban", u.ID)}
	}
	if c.IsOwner {
		kb = append(kb, service.Row(watchBtn))
	} else {
		kb = append(kb, service.Row(watchBtn, banBtn))
	}
	return text, kb
}

// confirmKeyboard — подтверждение разрушительного действия.
func confirmKeyboard(yesData string) service.Keyboard {
	return service.Keyboard{service.Row(
		service.Button{Text: "Да", Data: yesData},
		service.Button{Text: "Отмена", Data: "cancel"},
	)}
}
