package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"

	"github.com/vitaly/secure-ask-tg-bot/internal/crypto"
	"github.com/vitaly/secure-ask-tg-bot/internal/storage"
)

// Server предоставляет REST API для веб-панели управления.
type Server struct {
	store    *storage.Storage
	codec    *crypto.Codec
	tgBot    *bot.Bot
	botToken string
	hmacKey  []byte
	log      *slog.Logger
}

// NewServer создает новый экземпляр API-сервера.
func NewServer(store *storage.Storage, codec *crypto.Codec, tgBot *bot.Bot, botToken string, hmacKey []byte, log *slog.Logger) *Server {
	return &Server{
		store:    store,
		codec:    codec,
		tgBot:    tgBot,
		botToken: botToken,
		hmacKey:  hmacKey,
		log:      log,
	}
}

// RegisterRoutes регистрирует обработчики REST API на переданном ServeMux.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/auth/login", s.cors(s.handleLogin))
	mux.HandleFunc("/api/auth/me", s.cors(s.auth(s.handleMe)))
	mux.HandleFunc("/api/stats", s.cors(s.auth(s.handleStats)))
	mux.HandleFunc("/api/questions", s.cors(s.auth(s.handleQuestions)))
	mux.HandleFunc("/api/users", s.cors(s.auth(s.handleUsers)))
	mux.HandleFunc("/api/users/", s.cors(s.auth(s.handleUserAction)))
	mux.HandleFunc("/api/settings", s.cors(s.auth(s.handleSettings)))
	mux.HandleFunc("/api/media/", s.cors(s.handleMedia))
}

// TokenClaims — полезная нагрузка токена аутентификации.
type TokenClaims struct {
	UserID   int64  `json:"uid"`
	Username string `json:"user"`
	Role     string `json:"role"`
	Exp      int64  `json:"exp"`
}

func (s *Server) createToken(acc *storage.AdminAccount) (string, error) {
	claims := TokenClaims{
		UserID:   acc.ID,
		Username: acc.Username,
		Role:     acc.Role,
		Exp:      time.Now().Add(48 * time.Hour).Unix(),
	}
	data, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	b64 := base64.RawURLEncoding.EncodeToString(data)
	h := hmac.New(sha256.New, s.hmacKey)
	h.Write([]byte(b64))
	sig := hex.EncodeToString(h.Sum(nil))
	return b64 + "." + sig, nil
}

func (s *Server) verifyToken(tokenStr string) (*TokenClaims, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 2 {
		return nil, fmt.Errorf("неверный формат токена")
	}
	b64, sig := parts[0], parts[1]
	h := hmac.New(sha256.New, s.hmacKey)
	h.Write([]byte(b64))
	expectedSig := hex.EncodeToString(h.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return nil, fmt.Errorf("недействительная подпись")
	}
	raw, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	var claims TokenClaims
	if err := json.Unmarshal(raw, &claims); err != nil {
		return nil, err
	}
	if time.Now().Unix() > claims.Exp {
		return nil, fmt.Errorf("срок действия токена истек")
	}
	return &claims, nil
}

func (s *Server) cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

type contextKey string

const claimsKey contextKey = "claims"

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := s.verifyToken(token)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), claimsKey, claims)
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"некорректный json"}`, http.StatusBadRequest)
		return
	}
	acc, err := s.store.ValidateAdmin(r.Context(), req.Username, req.Password)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Неверный логин или пароль"})
		return
	}
	token, err := s.createToken(acc)
	if err != nil {
		http.Error(w, `{"error":"ошибка генерации токена"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"token": token,
		"user":  acc,
	})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	claims := r.Context().Value(claimsKey).(*TokenClaims)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(claims)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.GetAdminStats(r.Context())
	if err != nil {
		http.Error(w, `{"error":"ошибка получения статистики"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// QuestionView — расшифрованное представление вопроса для фронтенда.
type QuestionView struct {
	ID                int64      `json:"id"`
	LinkToken         string     `json:"link_token"`
	RecipientUsername string     `json:"recipient_username"`
	RecipientName     string     `json:"recipient_name"`
	RecipientTgID     int64      `json:"recipient_tg_id"`
	SenderUsername    string     `json:"sender_username"`
	SenderTgID        int64      `json:"sender_tg_id"`
	ContentType       string     `json:"content_type"`
	Text              string     `json:"text"`
	FileID            *string    `json:"file_id"`
	MediaURL          *string    `json:"media_url"`
	Answered          bool       `json:"answered"`
	AnswerText        string     `json:"answer_text,omitempty"`
	AnswerFileID      *string    `json:"answer_file_id,omitempty"`
	AnswerMediaURL    *string    `json:"answer_media_url,omitempty"`
	AnsweredAt        *time.Time `json:"answered_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

func (s *Server) handleQuestions(w http.ResponseWriter, r *http.Request) {
	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}

	items, err := s.store.ListQuestionsAdmin(r.Context(), limit, offset)
	if err != nil {
		s.log.Error("list questions", "err", err)
		http.Error(w, `{"error":"ошибка загрузки вопросов"}`, http.StatusInternalServerError)
		return
	}

	views := make([]QuestionView, 0, len(items))
	for _, it := range items {
		// Расшифровка текста вопроса
		text, _ := s.codec.DecryptText(it.TextEnc)

		// Расшифровка имени отправителя
		senderUser, _ := s.codec.DecryptText(it.SenderUserEnc)

		// Расшифровка ID отправителя
		senderTgID, _ := s.codec.DecryptSender(it.SenderUserEnc, it.ID)

		// Расшифровка текста ответа
		answerText, _ := s.codec.DecryptText(it.AnswerTextEnc)

		var mediaURL *string
		if it.FileID != nil && *it.FileID != "" {
			url := "/api/media/" + *it.FileID
			mediaURL = &url
		}

		var answerMediaURL *string
		if it.AnswerFileID != nil && *it.AnswerFileID != "" {
			url := "/api/media/" + *it.AnswerFileID
			answerMediaURL = &url
		}

		views = append(views, QuestionView{
			ID:                it.ID,
			LinkToken:         it.LinkToken,
			RecipientUsername: it.RecipientUsername,
			RecipientName:     it.RecipientName,
			RecipientTgID:     it.RecipientTgID,
			SenderUsername:    senderUser,
			SenderTgID:        senderTgID,
			ContentType:       it.ContentType,
			Text:              text,
			FileID:            it.FileID,
			MediaURL:          mediaURL,
			Answered:          it.Answered,
			AnswerText:        answerText,
			AnswerFileID:      it.AnswerFileID,
			AnswerMediaURL:    answerMediaURL,
			AnsweredAt:        it.AnsweredAt,
			CreatedAt:         it.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(views)
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListUsersAdmin(r.Context())
	if err != nil {
		s.log.Error("list users", "err", err)
		http.Error(w, `{"error":"ошибка получения пользователей"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

func (s *Server) handleUserAction(w http.ResponseWriter, r *http.Request) {
	// /api/users/{id}/role или /api/users/{id}/ban
	path := strings.TrimPrefix(r.URL.Path, "/api/users/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	userID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "некорректный id", http.StatusBadRequest)
		return
	}
	action := parts[1]

	switch action {
	case "role":
		var req struct {
			Role string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := s.store.AdminSetUserRole(r.Context(), userID, req.Role); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})

	case "ban":
		var req struct {
			Banned bool `json:"banned"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := s.store.AdminSetUserBanned(r.Context(), userID, req.Banned); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})

	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		settings, err := s.store.GetBotSettings(r.Context())
		if err != nil {
			http.Error(w, `{"error":"ошибка чтения настроек"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(settings)
		return
	}

	if r.Method == http.MethodPost {
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		for k, v := range req {
			if err := s.store.SetBotSetting(r.Context(), k, v); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) handleMedia(w http.ResponseWriter, r *http.Request) {
	fileID := strings.TrimPrefix(r.URL.Path, "/api/media/")
	if fileID == "" {
		http.NotFound(w, r)
		return
	}

	file, err := s.tgBot.GetFile(r.Context(), &bot.GetFileParams{FileID: fileID})
	if err != nil {
		s.log.Error("get file from telegram", "file_id", fileID, "err", err)
		http.Error(w, "файл не найден в Telegram", http.StatusNotFound)
		return
	}

	tgFileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", s.botToken, file.FilePath)
	resp, err := http.Get(tgFileURL)
	if err != nil {
		http.Error(w, "ошибка загрузки с серверов Telegram", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, "telegram file error", resp.StatusCode)
		return
	}

	// Копируем Content-Type из ответа Telegram (audio/ogg, video/mp4, image/jpeg)
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.Header().Set("Cache-Control", "public, max-age=86400")
	io.Copy(w, resp.Body)
}
