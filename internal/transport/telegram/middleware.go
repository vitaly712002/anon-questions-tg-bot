package telegram

import (
	"context"
	"errors"
	"strconv"

	"github.com/go-telegram/bot/models"

	"github.com/vitaly/secure-ask-tg-bot/internal/service"
	"github.com/vitaly/secure-ask-tg-bot/internal/storage/gen"
)

// requireAdmin перечитывает роль из БД (не из кэша и не из callback data):
// кнопка могла «протухнуть», роль — быть отозванной (docs/05).
// Возвращает ErrForbidden для обычных пользователей.
func (r *Router) requireAdmin(ctx context.Context, tgID int64) (gen.User, error) {
	user, err := r.svc.Users.Get(ctx, tgID)
	if err != nil {
		return gen.User{}, err
	}
	if !service.IsAdmin(user) {
		return gen.User{}, service.ErrForbidden
	}
	return user, nil
}

// requireSysadmin — то же, но только для sysadmin.
func (r *Router) requireSysadmin(ctx context.Context, tgID int64) (gen.User, error) {
	user, err := r.svc.Users.Get(ctx, tgID)
	if err != nil {
		return gen.User{}, err
	}
	if user.Role != gen.UserRoleSysadmin {
		return gen.User{}, service.ErrForbidden
	}
	return user, nil
}

// userMessage переводит ошибку сервиса в вежливый ответ пользователю.
// Пустая строка — отвечать не нужно (или нужно по-особому, см. вызывающий код).
func userMessage(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, service.ErrSilentDrop):
		// Блокировка/бан не раскрываются: делаем вид, что всё отправлено.
		return "✅ Вопрос отправлен анонимно!"
	case errors.Is(err, service.ErrNotAccepting):
		return "Этот пользователь сейчас не принимает вопросы."
	case errors.Is(err, service.ErrLinkInactive), errors.Is(err, service.ErrNotFound):
		return "Ссылка не найдена или больше не действует."
	case errors.Is(err, service.ErrRateLimited):
		return "Слишком много вопросов подряд. Передохните немного и попробуйте снова."
	case errors.Is(err, service.ErrDailyLimit):
		return "Вы исчерпали суточный лимит вопросов. Возвращайтесь завтра!"
	case errors.Is(err, service.ErrProtected):
		return "Это сообщение защищено от копирования — бот не может его переслать. Отправьте другое."
	case errors.Is(err, service.ErrDelivery):
		return "Не удалось доставить сообщение: возможно, получатель заблокировал бота."
	case errors.Is(err, service.ErrAliasInvalid):
		return "Алиас должен состоять из 3–32 символов: строчные латинские буквы, цифры, подчёркивание."
	case errors.Is(err, service.ErrAliasReserved):
		return "Этот алиас зарезервирован, выберите другой."
	case errors.Is(err, service.ErrAliasTaken):
		return "Этот алиас уже занят, выберите другой."
	case errors.Is(err, service.ErrNoDialog):
		return "Не понимаю, к чему это. Откройте чью-нибудь ссылку, чтобы задать вопрос, или /link — чтобы получить свою."
	case errors.Is(err, service.ErrForbidden):
		return "Недостаточно прав."
	default:
		return "Что-то пошло не так, попробуйте ещё раз."
	}
}

// contentType определяет тип контента сообщения для questions.content_type.
func contentType(m *models.Message) string {
	switch {
	case m.Text != "":
		return "text"
	case len(m.Photo) > 0:
		return "photo"
	case m.Video != nil:
		return "video"
	case m.VideoNote != nil:
		return "video_note"
	case m.Voice != nil:
		return "voice"
	case m.Audio != nil:
		return "audio"
	case m.Sticker != nil:
		return "sticker"
	case m.Animation != nil:
		return "animation"
	case m.Document != nil:
		return "document"
	default:
		return "other"
	}
}

// displayName собирает имя пользователя Telegram.
func displayName(u *models.User) string {
	name := u.FirstName
	if u.LastName != "" {
		name += " " + u.LastName
	}
	return name
}

// usernamePtr — *string для nullable-колонки tg_username.
func usernamePtr(u *models.User) *string {
	if u.Username == "" {
		return nil
	}
	un := u.Username
	return &un
}

// itoa — краткий помощник для ключей лимитера.
func itoa(v int64) string { return strconv.FormatInt(v, 10) }
