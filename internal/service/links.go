package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/vitaly/secure-ask-tg-bot/internal/crypto"
	"github.com/vitaly/secure-ask-tg-bot/internal/storage"
	"github.com/vitaly/secure-ask-tg-bot/internal/storage/gen"
)

// aliasRe — допустимый формат алиаса (docs/04): латиница в нижнем регистре,
// цифры, подчёркивание, 3–32 символа.
var aliasRe = regexp.MustCompile(`^[a-z0-9_]{3,32}$`)

// reservedAliases — стоп-список: служебные слова нельзя занимать под алиас.
var reservedAliases = map[string]struct{}{
	"admin": {}, "administrator": {}, "sysadmin": {}, "moderator": {},
	"support": {}, "help": {}, "start": {}, "stop": {}, "api": {},
	"bot": {}, "telegram": {}, "link": {}, "alias": {}, "stats": {},
	"pause": {}, "resume": {}, "privacy": {}, "root": {}, "owner": {},
}

// Links — персональные ссылки и алиасы.
type Links struct {
	store       Store
	botUsername string // без @, для построения t.me-ссылок
}

// NewLinks создаёт сервис ссылок. botUsername узнаётся из getMe при старте.
func NewLinks(store Store, botUsername string) *Links {
	return &Links{store: store, botUsername: botUsername}
}

// URL строит публичную ссылку: алиас, если задан, иначе токен.
func (s *Links) URL(link gen.Link) string {
	arg := link.Token
	if link.Alias != nil && *link.Alias != "" {
		arg = *link.Alias
	}
	return fmt.Sprintf("https://t.me/%s?start=%s", s.botUsername, arg)
}

// NormalizeAlias приводит алиас к нижнему регистру и валидирует его.
func NormalizeAlias(raw string) (string, error) {
	alias := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(raw), "@"))
	if !aliasRe.MatchString(alias) {
		return "", ErrAliasInvalid
	}
	if strings.HasPrefix(alias, crypto.TokenPrefix) {
		// q_... зарезервировано под случайные токены — алиас не должен маскироваться под них
		return "", ErrAliasReserved
	}
	if _, bad := reservedAliases[alias]; bad {
		return "", ErrAliasReserved
	}
	return alias, nil
}

// SetAlias устанавливает алиас активной ссылки пользователя.
// Уникальность гарантирует частичный уникальный индекс по lower(alias):
// при гонке БД вернёт 23505, мы превратим его в ErrAliasTaken.
func (s *Links) SetAlias(ctx context.Context, userID int64, raw string) (string, error) {
	alias, err := NormalizeAlias(raw)
	if err != nil {
		return "", err
	}
	link, err := s.store.GetActiveLinkByUserID(ctx, userID)
	if storage.IsNotFound(err) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if err := s.store.SetLinkAlias(ctx, gen.SetLinkAliasParams{ID: link.ID, Alias: &alias}); err != nil {
		if storage.IsUniqueViolation(err) {
			return "", ErrAliasTaken
		}
		return "", err
	}
	return alias, nil
}

// Resolve находит ссылку по аргументу /start: токен q_<...> или алиас.
// Возвращает ссылку и её владельца (получателя вопросов).
func (s *Links) Resolve(ctx context.Context, startArg string) (gen.Link, gen.User, error) {
	var link gen.Link
	var err error
	if strings.HasPrefix(startArg, crypto.TokenPrefix) {
		link, err = s.store.GetLinkByToken(ctx, startArg)
	} else {
		link, err = s.store.GetLinkByAlias(ctx, strings.ToLower(startArg))
	}
	if storage.IsNotFound(err) {
		return gen.Link{}, gen.User{}, ErrNotFound
	}
	if err != nil {
		return gen.Link{}, gen.User{}, err
	}
	if !link.IsActive {
		return gen.Link{}, gen.User{}, ErrLinkInactive
	}
	owner, err := s.store.GetUserByID(ctx, link.UserID)
	if err != nil {
		return gen.Link{}, gen.User{}, err
	}
	return link, owner, nil
}
