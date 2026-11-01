package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/vitaly/secure-ask-tg-bot/internal/storage"
	"github.com/vitaly/secure-ask-tg-bot/internal/storage/gen"
)

// Admin — административные сценарии: роли, баны, подписки, статистика, аудит.
//
// Права проверяются здесь повторно (защита в глубину, docs/05): транспортный
// middleware — первая линия, но кнопка могла «протухнуть», а callback — подделан.
type Admin struct {
	store     Store
	tg        TG
	questions *Questions // единая точка раскрытия отправителей
	ownerTgID int64
	log       *slog.Logger
}

// NewAdmin создаёт админ-сервис.
func NewAdmin(store Store, tg TG, questions *Questions, ownerTgID int64, log *slog.Logger) *Admin {
	return &Admin{store: store, tg: tg, questions: questions, ownerTgID: ownerTgID, log: log}
}

// IsAdmin — роль admin или sysadmin.
func IsAdmin(u gen.User) bool {
	return u.Role == gen.UserRoleAdmin || u.Role == gen.UserRoleSysadmin
}

// FindUser ищет пользователя по @username или числовому Telegram ID.
func (s *Admin) FindUser(ctx context.Context, query string) (gen.User, error) {
	query = strings.TrimSpace(query)
	var user gen.User
	var err error
	if id, numErr := strconv.ParseInt(query, 10, 64); numErr == nil {
		user, err = s.store.GetUserByTgID(ctx, id)
	} else {
		user, err = s.store.GetUserByUsername(ctx, strings.TrimPrefix(query, "@"))
	}
	if storage.IsNotFound(err) {
		return gen.User{}, ErrNotFound
	}
	return user, err
}

// UserCard — данные карточки пользователя для админ-панели.
type UserCard struct {
	User     gen.User
	Received int64
	Watched  bool // подписан ли текущий админ на этого пользователя
	IsOwner  bool
}

// Card собирает карточку пользователя для просмотра админом.
func (s *Admin) Card(ctx context.Context, viewer gen.User, targetID int64) (UserCard, error) {
	if !IsAdmin(viewer) {
		return UserCard{}, ErrForbidden
	}
	target, err := s.store.GetUserByID(ctx, targetID)
	if storage.IsNotFound(err) {
		return UserCard{}, ErrNotFound
	}
	if err != nil {
		return UserCard{}, err
	}
	received, err := s.store.CountQuestionsToRecipient(ctx, target.ID)
	if err != nil {
		return UserCard{}, err
	}
	watches, err := s.store.ListWatchesByAdmin(ctx, viewer.ID)
	if err != nil {
		return UserCard{}, err
	}
	watched := false
	for _, w := range watches {
		if w.ID == target.ID {
			watched = true
			break
		}
	}
	return UserCard{User: target, Received: received, Watched: watched, IsOwner: target.TgID == s.ownerTgID}, nil
}

// SetRole назначает или снимает роль (только sysadmin). Владельца бота
// разжаловать нельзя. Пользователь уведомляется, действие пишется в аудит.
func (s *Admin) SetRole(ctx context.Context, actor gen.User, targetID int64, role gen.UserRole) error {
	if actor.Role != gen.UserRoleSysadmin {
		return ErrForbidden
	}
	target, err := s.store.GetUserByID(ctx, targetID)
	if storage.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if target.TgID == s.ownerTgID {
		return ErrOwnerChange
	}
	if err := s.store.SetUserRole(ctx, gen.SetUserRoleParams{ID: target.ID, Role: role}); err != nil {
		return err
	}
	action := "role_granted"
	notice := "🎖 Вам назначена роль " + string(role) + "."
	if role == gen.UserRoleUser {
		action = "role_revoked"
		notice = "Ваша административная роль снята."
	}
	if err := s.audit(ctx, actor.ID, action, &target.ID, map[string]any{"role": role}); err != nil {
		return err
	}
	if _, err := s.tg.SendMessage(ctx, target.TgID, notice, nil); err != nil {
		s.log.Warn("уведомление о роли не доставлено", "err", err)
	}
	return nil
}

// SetBanned банит/разбанивает пользователя на уровне бота (admin+).
func (s *Admin) SetBanned(ctx context.Context, actor gen.User, targetID int64, banned bool) error {
	if !IsAdmin(actor) {
		return ErrForbidden
	}
	target, err := s.store.GetUserByID(ctx, targetID)
	if storage.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if target.TgID == s.ownerTgID {
		return ErrOwnerChange
	}
	if err := s.store.SetUserBanned(ctx, gen.SetUserBannedParams{ID: target.ID, Banned: banned}); err != nil {
		return err
	}
	action := "user_banned"
	if !banned {
		action = "user_unbanned"
	}
	return s.audit(ctx, actor.ID, action, &target.ID, nil)
}

// BanSenderOfQuestion глобально банит отправителя вопроса (кнопка в карточке
// отправителя / жалобе). Отправитель может быть не зарегистрирован — тогда
// создаётся запись users, чтобы бан было где хранить.
func (s *Admin) BanSenderOfQuestion(ctx context.Context, actor gen.User, questionID int64) error {
	if actor.Role != gen.UserRoleSysadmin {
		return ErrForbidden
	}
	senderUser, err := s.ensureSenderUser(ctx, actor, questionID, "ban")
	if err != nil {
		return err
	}
	if senderUser.TgID == s.ownerTgID {
		return ErrOwnerChange
	}
	if err := s.store.SetUserBanned(ctx, gen.SetUserBannedParams{ID: senderUser.ID, Banned: true}); err != nil {
		return err
	}
	return s.audit(ctx, actor.ID, "user_banned", &senderUser.ID, map[string]any{"question_id": questionID})
}

// WatchSenderOfQuestion подписывает админа на отправителя вопроса.
func (s *Admin) WatchSenderOfQuestion(ctx context.Context, actor gen.User, questionID int64) error {
	if actor.Role != gen.UserRoleSysadmin {
		return ErrForbidden
	}
	senderUser, err := s.ensureSenderUser(ctx, actor, questionID, "watch")
	if err != nil {
		return err
	}
	return s.watch(ctx, actor, senderUser.ID)
}

// ensureSenderUser раскрывает отправителя вопроса (с аудитом) и гарантирует,
// что для него существует строка users.
func (s *Admin) ensureSenderUser(ctx context.Context, actor gen.User, questionID int64, reason string) (gen.User, error) {
	senderTgID, err := s.questions.RevealSenderTgID(ctx, actor, questionID, reason)
	if err != nil {
		return gen.User{}, err
	}
	senderUser, err := s.store.GetUserByTgID(ctx, senderTgID)
	if storage.IsNotFound(err) {
		name := ""
		if info, infoErr := s.tg.GetChatInfo(ctx, senderTgID); infoErr == nil {
			name = info.Name
		}
		senderUser, err = s.store.UpsertUser(ctx, gen.UpsertUserParams{TgID: senderTgID, DisplayName: name})
	}
	return senderUser, err
}

// Watch подписывает админа на пользователя (карточка пользователя).
func (s *Admin) Watch(ctx context.Context, actor gen.User, targetID int64) error {
	if !IsAdmin(actor) {
		return ErrForbidden
	}
	if _, err := s.store.GetUserByID(ctx, targetID); err != nil {
		if storage.IsNotFound(err) {
			return ErrNotFound
		}
		return err
	}
	return s.watch(ctx, actor, targetID)
}

func (s *Admin) watch(ctx context.Context, actor gen.User, targetID int64) error {
	if err := s.store.AddWatch(ctx, gen.AddWatchParams{AdminID: actor.ID, TargetID: targetID}); err != nil {
		return err
	}
	return s.audit(ctx, actor.ID, "watch_added", &targetID, nil)
}

// Unwatch снимает подписку.
func (s *Admin) Unwatch(ctx context.Context, actor gen.User, targetID int64) error {
	if !IsAdmin(actor) {
		return ErrForbidden
	}
	if err := s.store.RemoveWatch(ctx, gen.RemoveWatchParams{AdminID: actor.ID, TargetID: targetID}); err != nil {
		return err
	}
	return s.audit(ctx, actor.ID, "watch_removed", &targetID, nil)
}

// ListWatches — список подписок админа.
func (s *Admin) ListWatches(ctx context.Context, actor gen.User) ([]gen.User, error) {
	if !IsAdmin(actor) {
		return nil, ErrForbidden
	}
	return s.store.ListWatchesByAdmin(ctx, actor.ID)
}

// ListBanned — список забаненных пользователей.
func (s *Admin) ListBanned(ctx context.Context, actor gen.User) ([]gen.User, error) {
	if !IsAdmin(actor) {
		return nil, ErrForbidden
	}
	return s.store.ListBannedUsers(ctx)
}

// BotStats — сводная статистика бота для админ-панели.
type BotStats struct {
	Users          int64
	Questions      int64
	QuestionsToday int64
}

// Stats собирает сводную статистику (admin+).
func (s *Admin) Stats(ctx context.Context, actor gen.User) (BotStats, error) {
	if !IsAdmin(actor) {
		return BotStats{}, ErrForbidden
	}
	var st BotStats
	var err error
	if st.Users, err = s.store.CountUsers(ctx); err != nil {
		return st, err
	}
	if st.Questions, err = s.store.CountQuestions(ctx); err != nil {
		return st, err
	}
	if st.QuestionsToday, err = s.store.CountQuestionsSince(ctx, pgTime(time.Now().Add(-24*time.Hour))); err != nil {
		return st, err
	}
	return st, nil
}

// Audit возвращает последние записи журнала (только sysadmin).
func (s *Admin) Audit(ctx context.Context, actor gen.User, limit int32) ([]gen.ListAuditRecentRow, error) {
	if actor.Role != gen.UserRoleSysadmin {
		return nil, ErrForbidden
	}
	return s.store.ListAuditRecent(ctx, limit)
}

// audit пишет запись в журнал.
func (s *Admin) audit(ctx context.Context, actorID int64, action string, targetID *int64, details map[string]any) error {
	if details == nil {
		details = map[string]any{}
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return err
	}
	return s.store.InsertAudit(ctx, gen.InsertAuditParams{ActorID: actorID, Action: action, TargetID: targetID, Details: raw})
}
