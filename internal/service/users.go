package service

import (
	"context"
	"fmt"

	"github.com/vitaly/secure-ask-tg-bot/internal/crypto"
	"github.com/vitaly/secure-ask-tg-bot/internal/storage"
	"github.com/vitaly/secure-ask-tg-bot/internal/storage/gen"
)

// Users — регистрация и настройки пользователей.
type Users struct {
	store     Store
	ownerTgID int64
}

// NewUsers создаёт сервис пользователей.
func NewUsers(store Store, ownerTgID int64) *Users {
	return &Users{store: store, ownerTgID: ownerTgID}
}

// Register регистрирует (или обновляет) пользователя по /start и гарантирует,
// что у него есть активная персональная ссылка. Владелец бота получает
// sysadmin автоматически (docs/06).
func (s *Users) Register(ctx context.Context, tgID int64, username *string, displayName string) (gen.User, gen.Link, error) {
	user, err := s.store.UpsertUser(ctx, gen.UpsertUserParams{
		TgID:        tgID,
		TgUsername:  username,
		DisplayName: displayName,
	})
	if err != nil {
		return gen.User{}, gen.Link{}, fmt.Errorf("upsert user: %w", err)
	}

	if tgID == s.ownerTgID && user.Role != gen.UserRoleSysadmin {
		if err := s.store.EnsureOwnerSysadmin(ctx, tgID); err != nil {
			return gen.User{}, gen.Link{}, fmt.Errorf("ensure owner sysadmin: %w", err)
		}
		user.Role = gen.UserRoleSysadmin
	}

	link, err := s.store.GetActiveLinkByUserID(ctx, user.ID)
	if storage.IsNotFound(err) {
		link, err = s.createLink(ctx, user.ID)
	}
	if err != nil {
		return gen.User{}, gen.Link{}, fmt.Errorf("link: %w", err)
	}
	return user, link, nil
}

// createLink создаёт ссылку с новым случайным токеном.
// Коллизия токена (2^128) практически исключена, но на всякий случай — ретрай.
func (s *Users) createLink(ctx context.Context, userID int64) (gen.Link, error) {
	for attempt := 0; attempt < 3; attempt++ {
		token, err := crypto.NewLinkToken()
		if err != nil {
			return gen.Link{}, err
		}
		link, err := s.store.CreateLink(ctx, gen.CreateLinkParams{UserID: userID, Token: token})
		if storage.IsUniqueViolation(err) {
			continue
		}
		return link, err
	}
	return gen.Link{}, fmt.Errorf("не удалось создать уникальный токен ссылки")
}

// Get возвращает пользователя по Telegram ID.
func (s *Users) Get(ctx context.Context, tgID int64) (gen.User, error) {
	user, err := s.store.GetUserByTgID(ctx, tgID)
	if storage.IsNotFound(err) {
		return gen.User{}, ErrNotFound
	}
	return user, err
}

// SetAccepting включает/выключает приём вопросов (/pause, /resume).
// При возобновлении также реактивируется ссылка (симметрично Stop).
func (s *Users) SetAccepting(ctx context.Context, userID int64, accepting bool) error {
	if err := s.store.SetUserAccepting(ctx, gen.SetUserAcceptingParams{ID: userID, Accepting: accepting}); err != nil {
		return err
	}
	if accepting {
		return s.store.SetLinksActiveByUser(ctx, gen.SetLinksActiveByUserParams{UserID: userID, IsActive: true})
	}
	return nil
}

// Stop — полная отписка (/stop): приём выключается, ссылки деактивируются.
// Повторный /resume вернёт всё обратно.
func (s *Users) Stop(ctx context.Context, userID int64) error {
	if err := s.store.SetUserAccepting(ctx, gen.SetUserAcceptingParams{ID: userID, Accepting: false}); err != nil {
		return err
	}
	return s.store.SetLinksActiveByUser(ctx, gen.SetLinksActiveByUserParams{UserID: userID, IsActive: false})
}

// UserStats — личная статистика для /stats.
type UserStats struct {
	Received int64
	Answered int64
	Blocked  int64
	Clicks   int64
}

// Stats собирает личную статистику пользователя.
func (s *Users) Stats(ctx context.Context, userID int64) (UserStats, error) {
	var st UserStats
	var err error
	if st.Received, err = s.store.CountQuestionsToRecipient(ctx, userID); err != nil {
		return st, err
	}
	if st.Answered, err = s.store.CountQuestionsAnsweredByRecipient(ctx, userID); err != nil {
		return st, err
	}
	if st.Blocked, err = s.store.CountBlocksByRecipient(ctx, userID); err != nil {
		return st, err
	}
	if link, err := s.store.GetActiveLinkByUserID(ctx, userID); err == nil {
		st.Clicks = link.Clicks
	} else if !storage.IsNotFound(err) {
		return st, err
	}
	return st, nil
}
