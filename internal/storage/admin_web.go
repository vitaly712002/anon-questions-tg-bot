package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

// AdminAccount представляет учетную запись в веб-панели.
type AdminAccount struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// EnsureDefaultAdmin создает администратора по умолчанию, если таблица пуста.
func (s *Storage) EnsureDefaultAdmin(ctx context.Context, username, password string) error {
	var count int64
	err := s.Pool.QueryRow(ctx, "SELECT count(*) FROM admin_accounts").Scan(&count)
	if err != nil {
		return fmt.Errorf("count admin_accounts: %w", err)
	}
	if count > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	_, err = s.Pool.Exec(ctx,
		"INSERT INTO admin_accounts (username, password_hash, role) VALUES ($1, $2, 'sysadmin')",
		username, string(hash),
	)
	if err != nil {
		return fmt.Errorf("insert default admin: %w", err)
	}
	return nil
}

// ValidateAdmin проверяет логин и пароль администратора.
func (s *Storage) ValidateAdmin(ctx context.Context, username, password string) (*AdminAccount, error) {
	var acc AdminAccount
	var hash string
	err := s.Pool.QueryRow(ctx,
		"SELECT id, username, password_hash, role, created_at FROM admin_accounts WHERE username = $1",
		username,
	).Scan(&acc.ID, &acc.Username, &hash, &acc.Role, &acc.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errors.New("неверный логин или пароль")
	}
	if err != nil {
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return nil, errors.New("неверный логин или пароль")
	}
	return &acc, nil
}

// SaveQuestionContent сохраняет зашифрованный текст вопроса, имя отправителя и file_id.
func (s *Storage) SaveQuestionContent(ctx context.Context, questionID int64, textEnc, senderUserEnc []byte, fileID string) error {
	_, err := s.Pool.Exec(ctx,
		"UPDATE questions SET text_enc = $2, sender_username_enc = $3, file_id = $4 WHERE id = $1",
		questionID, textEnc, senderUserEnc, fileID,
	)
	return err
}

// QuestionFeedItem — строка вопроса для веб-панели.
type QuestionFeedItem struct {
	ID                int64      `json:"id"`
	LinkID            int64      `json:"link_id"`
	LinkToken         string     `json:"link_token"`
	RecipientID       int64      `json:"recipient_id"`
	RecipientTgID     int64      `json:"recipient_tg_id"`
	RecipientUsername string     `json:"recipient_username"`
	RecipientName     string     `json:"recipient_name"`
	ContentType       string     `json:"content_type"`
	FileID            *string    `json:"file_id"`
	TextEnc           []byte     `json:"-"`
	SenderUserEnc     []byte     `json:"-"`
	Answered          bool       `json:"answered"`
	AnswerTextEnc     []byte     `json:"-"`
	AnswerFileID      *string    `json:"answer_file_id"`
	AnsweredAt        *time.Time `json:"answered_at"`
	CreatedAt         time.Time  `json:"created_at"`
}

// ListQuestionsAdmin возвращает вопросы с информацией о получателе.
func (s *Storage) ListQuestionsAdmin(ctx context.Context, limit, offset int) ([]QuestionFeedItem, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT q.id, q.link_id, COALESCE(l.token, ''), q.recipient_id,
		       u.tg_id, COALESCE(u.tg_username, ''), u.display_name,
		       q.content_type, q.file_id, q.text_enc, q.sender_username_enc,
		       q.answered, q.answer_text_enc, q.answer_file_id, q.answered_at, q.created_at
		FROM questions q
		JOIN users u ON q.recipient_id = u.id
		LEFT JOIN links l ON q.link_id = l.id
		ORDER BY q.created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []QuestionFeedItem
	for rows.Next() {
		var item QuestionFeedItem
		var fileID *string
		var answerFileID *string
		err := rows.Scan(
			&item.ID, &item.LinkID, &item.LinkToken, &item.RecipientID,
			&item.RecipientTgID, &item.RecipientUsername, &item.RecipientName,
			&item.ContentType, &fileID, &item.TextEnc, &item.SenderUserEnc,
			&item.Answered, &item.AnswerTextEnc, &answerFileID, &item.AnsweredAt, &item.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		item.FileID = fileID
		item.AnswerFileID = answerFileID
		items = append(items, item)
	}
	return items, rows.Err()
}

// SaveQuestionAnswer сохраняет зашифрованный текст ответа, медиафайл и время ответа.
func (s *Storage) SaveQuestionAnswer(ctx context.Context, questionID int64, answerTextEnc []byte, answerFileID string) error {
	var fileIDVal *string
	if answerFileID != "" {
		fileIDVal = &answerFileID
	}
	_, err := s.Pool.Exec(ctx, `
		UPDATE questions
		SET answered = TRUE,
		    answer_text_enc = $2,
		    answer_file_id = $3,
		    answered_at = COALESCE(answered_at, now())
		WHERE id = $1
	`, questionID, answerTextEnc, fileIDVal)
	return err
}

// UserListItem — пользователь бота для панели управления.
type UserListItem struct {
	ID             int64     `json:"id"`
	TgID           int64     `json:"tg_id"`
	TgUsername     string    `json:"tg_username"`
	DisplayName    string    `json:"display_name"`
	Role           string    `json:"role"`
	Accepting      bool      `json:"accepting"`
	Banned         bool      `json:"banned"`
	LinkToken      string    `json:"link_token"`
	LinkAlias      string    `json:"link_alias"`
	QuestionsCount int64     `json:"questions_count"`
	CreatedAt      time.Time `json:"created_at"`
}

// ListUsersAdmin возвращает список всех пользователей бота со статистикой.
func (s *Storage) ListUsersAdmin(ctx context.Context) ([]UserListItem, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT u.id, u.tg_id, COALESCE(u.tg_username, ''), u.display_name,
		       u.role::text, u.accepting, u.banned,
		       COALESCE(l.token, ''), COALESCE(l.alias, ''),
		       (SELECT count(*) FROM questions q WHERE q.recipient_id = u.id),
		       u.created_at
		FROM users u
		LEFT JOIN links l ON l.user_id = u.id AND l.is_active = TRUE
		ORDER BY u.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []UserListItem
	for rows.Next() {
		var u UserListItem
		if err := rows.Scan(
			&u.ID, &u.TgID, &u.TgUsername, &u.DisplayName,
			&u.Role, &u.Accepting, &u.Banned,
			&u.LinkToken, &u.LinkAlias,
			&u.QuestionsCount, &u.CreatedAt,
		); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// AdminSetUserRole меняет роль пользователя.
func (s *Storage) AdminSetUserRole(ctx context.Context, userID int64, role string) error {
	_, err := s.Pool.Exec(ctx, "UPDATE users SET role = $2::user_role WHERE id = $1", userID, role)
	return err
}

// AdminSetUserBanned банит или разбанивает пользователя.
func (s *Storage) AdminSetUserBanned(ctx context.Context, userID int64, banned bool) error {
	_, err := s.Pool.Exec(ctx, "UPDATE users SET banned = $2 WHERE id = $1", userID, banned)
	return err
}

// GetBotSettings возвращает карту всех динамических настроек бота.
func (s *Storage) GetBotSettings(ctx context.Context) (map[string]string, error) {
	rows, err := s.Pool.Query(ctx, "SELECT key, value FROM bot_settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		settings[k] = v
	}
	return settings, rows.Err()
}

// SetBotSetting сохраняет настройку.
func (s *Storage) SetBotSetting(ctx context.Context, key, value string) error {
	_, err := s.Pool.Exec(ctx,
		"INSERT INTO bot_settings (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = $2",
		key, value,
	)
	return err
}

// AdminStats статистика для главной страницы дашборда.
type AdminStats struct {
	TotalUsers     int64 `json:"total_users"`
	TotalQuestions int64 `json:"total_questions"`
	AnsweredCount  int64 `json:"answered_count"`
	BannedUsers    int64 `json:"banned_users"`
}

// GetAdminStats возвращает агрегированные метрики.
func (s *Storage) GetAdminStats(ctx context.Context) (*AdminStats, error) {
	var st AdminStats
	err := s.Pool.QueryRow(ctx, `
		SELECT 
			(SELECT count(*) FROM users),
			(SELECT count(*) FROM questions),
			(SELECT count(*) FROM questions WHERE answered = TRUE),
			(SELECT count(*) FROM users WHERE banned = TRUE)
	`).Scan(&st.TotalUsers, &st.TotalQuestions, &st.AnsweredCount, &st.BannedUsers)
	if err != nil {
		return nil, err
	}
	return &st, nil
}
