package service

import (
	"context"
	"crypto/hmac"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/vitaly/secure-ask-tg-bot/internal/crypto"
	"github.com/vitaly/secure-ask-tg-bot/internal/storage"
	"github.com/vitaly/secure-ask-tg-bot/internal/storage/gen"
)

// Questions — приём, доставка и ответы на анонимные вопросы (ядро бота).
type Questions struct {
	store      Store
	codec      *crypto.Codec
	tg         TG
	burst      BurstLimiter
	dailyLimit int
	log        *slog.Logger
}

// NewQuestions создаёт сервис вопросов.
func NewQuestions(store Store, codec *crypto.Codec, tg TG, burst BurstLimiter, dailyLimit int, log *slog.Logger) *Questions {
	return &Questions{store: store, codec: codec, tg: tg, burst: burst, dailyLimit: dailyLimit, log: log}
}

// StartAsking переводит гостя в состояние «пишет вопрос» по /start <токен|алиас>.
// Возвращает имя получателя для приветственного текста.
// Если гость заблокирован получателем — не сообщаем об этом (silent drop при Submit).
func (s *Questions) StartAsking(ctx context.Context, guestTgID int64, startArg string, links *Links) (string, error) {
	link, owner, err := links.Resolve(ctx, startArg)
	if err != nil {
		return "", err
	}
	if !owner.Accepting || owner.Banned {
		return "", ErrNotAccepting
	}
	if err := s.store.IncrementLinkClicks(ctx, link.ID); err != nil {
		return "", err
	}
	err = s.store.SetDialogState(ctx, gen.SetDialogStateParams{
		TgID:    guestTgID,
		State:   StateAsking,
		Payload: MarshalPayload(DialogPayload{LinkID: link.ID}),
	})
	if err != nil {
		return "", err
	}
	return owner.DisplayName, nil
}

// SubmitInput — входные данные принятого сообщения-вопроса.
type SubmitInput struct {
	SenderTgID     int64
	SenderUsername string
	Text           string
	FileID         string
	SrcChatID      int64 // чат гостя — откуда копировать
	SrcMsgID       int   // сообщение гостя
	ContentType    string
	Protected      bool // has_protected_content — скопировать не сможем
}

// Submit принимает вопрос от гостя в состоянии asking и доставляет получателю.
//
// Поток (docs/03, docs/09): проверки → tx(INSERT с placeholder → RETURNING id →
// шифрование AAD=id → UPDATE sender_enc) → copyMessage → MarkQuestionDelivered
// (затирает src_*) → авто-показ отправителя админам (получателю-админу и подписчикам).
func (s *Questions) Submit(ctx context.Context, in SubmitInput) error {
	if in.Protected {
		return ErrProtected
	}

	// Проверка режима техобслуживания
	if settings, err := s.store.GetBotSettings(ctx); err == nil && settings != nil {
		if settings["maintenance_mode"] == "true" {
			return errors.New("бот временно на техобслуживании. Пожалуйста, попробуйте позже")
		}
	}

	// 1. Состояние диалога: кому адресован вопрос.
	st, err := s.store.GetDialogState(ctx, in.SenderTgID)
	if storage.IsNotFound(err) || (err == nil && st.State != StateAsking) {
		return ErrNoDialog
	}
	if err != nil {
		return err
	}
	payload, err := UnmarshalPayload(st.Payload)
	if err != nil {
		return err
	}

	link, err := s.store.GetLinkByID(ctx, payload.LinkID)
	if storage.IsNotFound(err) {
		return ErrNoDialog
	}
	if err != nil {
		return err
	}
	if !link.IsActive {
		return ErrLinkInactive
	}
	recipient, err := s.store.GetUserByID(ctx, link.UserID)
	if err != nil {
		return err
	}
	if !recipient.Accepting || recipient.Banned {
		return ErrNotAccepting
	}

	// 2. Глобальный бан отправителя — молча отклоняем (он не должен понять, что забанен).
	if sender, err := s.store.GetUserByTgID(ctx, in.SenderTgID); err == nil {
		if sender.Banned {
			return ErrSilentDrop
		}
	} else if !storage.IsNotFound(err) {
		return err
	}

	// 3. Персональная блокировка — тоже молча (docs/06).
	senderHash := s.codec.SenderHash(in.SenderTgID)
	blocked, err := s.store.IsBlocked(ctx, gen.IsBlockedParams{RecipientID: recipient.ID, SenderHash: senderHash})
	if err != nil {
		return err
	}
	if blocked {
		return ErrSilentDrop
	}

	// 4. Антифлуд: burst в памяти + суточный лимит по БД.
	if !s.burst.Allow(hex.EncodeToString(senderHash)) {
		return ErrRateLimited
	}
	daily, err := s.store.CountQuestionsBySenderSince(ctx, gen.CountQuestionsBySenderSinceParams{
		SenderHash: senderHash,
		Since:      pgTime(time.Now().Add(-24 * time.Hour)),
	})
	if err != nil {
		return err
	}
	if daily >= int64(s.dailyLimit) {
		return ErrDailyLimit
	}

	// 5. Транзакция: INSERT с placeholder-шифротекстом → настоящий шифротекст с AAD=id.
	// AAD привязывает шифротекст к строке, а id известен только после INSERT (docs/09).
	var questionID int64
	err = s.store.WithTx(ctx, func(q Querier) error {
		questionID, err = q.InsertQuestion(ctx, gen.InsertQuestionParams{
			LinkID:       link.ID,
			RecipientID:  recipient.ID,
			ReplyTo:      payload.ReplyTo,
			SenderEnc:    []byte{0}, // placeholder до вычисления AAD
			SenderHash:   senderHash,
			ContentType:  in.ContentType,
			SrcChatID:    in.SrcChatID,
			SrcMessageID: int64(in.SrcMsgID),
		})
		if err != nil {
			return err
		}
		enc, err := s.codec.EncryptSender(in.SenderTgID, questionID)
		if err != nil {
			return err
		}
		return q.UpdateQuestionSenderEnc(ctx, gen.UpdateQuestionSenderEncParams{ID: questionID, SenderEnc: enc})
	})
	if err != nil {
		return fmt.Errorf("сохранение вопроса: %w", err)
	}

	// Сохраняем зашифрованный текст и отправителя для админ-панели (docs/05)
	var textEnc, senderUserEnc []byte
	if in.Text != "" {
		textEnc, _ = s.codec.EncryptText(in.Text)
	}
	if in.SenderUsername != "" {
		senderUserEnc, _ = s.codec.EncryptText(in.SenderUsername)
	}
	if err := s.store.SaveQuestionContent(ctx, questionID, textEnc, senderUserEnc, in.FileID); err != nil {
		s.log.Warn("сохранение контента вопроса", "err", err)
	}

	// 6. Доставка получателю: заголовок + копия контента с кнопками.
	header := "❓ Новый анонимный вопрос:"
	if payload.ReplyTo != nil {
		header = "💬 Новое сообщение в анонимном диалоге:"
	}
	kb := Keyboard{Row(
		Button{Text: "💬 Ответить", Data: CB(CBAnswer, questionID)},
		Button{Text: "🚫 Заблокировать", Data: CB(CBBlock, questionID)},
		Button{Text: "⚠️ Пожаловаться", Data: CB(CBReport, questionID)},
	)}
	if _, err := s.tg.SendMessage(ctx, recipient.TgID, header, nil); err != nil {
		s.wipeSrc(ctx, questionID)
		return fmt.Errorf("%w: %v", ErrDelivery, err)
	}
	deliveredID, err := s.tg.CopyMessage(ctx, recipient.TgID, in.SrcChatID, in.SrcMsgID, kb)
	if err != nil {
		// Получатель заблокировал бота, protected content и т.п. — вопрос не доставлен.
		s.wipeSrc(ctx, questionID)
		return fmt.Errorf("%w: %v", ErrDelivery, err)
	}

	// 7. Дублирование подписанным админам — пока src_* ещё в памяти (в БД затрутся ниже).
	s.notifyWatchers(ctx, recipient, questionID, in)

	// 8. Фиксация доставки затирает src_* (src_chat_id — это Telegram ID отправителя).
	did := int64(deliveredID)
	if err := s.store.MarkQuestionDelivered(ctx, gen.MarkQuestionDeliveredParams{ID: questionID, DeliveredMsgID: &did}); err != nil {
		return err
	}

	// 9. Если получатель admin/sysadmin — авто-карточка отправителя (docs/06).
	if recipient.Role == gen.UserRoleAdmin || recipient.Role == gen.UserRoleSysadmin {
		if err := s.SendSenderCard(ctx, recipient, questionID); err != nil {
			s.log.Warn("не удалось отправить карточку отправителя", "question_id", questionID, "err", err)
		}
	}
	return nil
}

// notifyWatchers дублирует вопрос админам, подписанным на получателя:
// копия контента + карточка отправителя. Ошибки не прерывают доставку.
func (s *Questions) notifyWatchers(ctx context.Context, recipient gen.User, questionID int64, in SubmitInput) {
	watchers, err := s.store.ListWatcherTgIDs(ctx, recipient.ID)
	if err != nil {
		s.log.Warn("не удалось получить подписчиков", "err", err)
		return
	}
	for _, w := range watchers {
		if w.TgID == recipient.TgID {
			continue // сам себе дублировать не нужно — уже получил вопрос
		}
		header := fmt.Sprintf("👁 Вопрос пользователю id %d (вы подписаны):", recipient.ID)
		if _, err := s.tg.SendMessage(ctx, w.TgID, header, nil); err != nil {
			s.log.Warn("watcher: заголовок не доставлен", "err", err)
			continue
		}
		if _, err := s.tg.CopyMessage(ctx, w.TgID, in.SrcChatID, in.SrcMsgID, nil); err != nil {
			s.log.Warn("watcher: копия не доставлена", "err", err)
			continue
		}
		watcher, err := s.store.GetUserByID(ctx, w.ID)
		if err != nil {
			s.log.Warn("watcher: пользователь не найден", "err", err)
			continue
		}
		if err := s.SendSenderCard(ctx, watcher, questionID); err != nil {
			s.log.Warn("watcher: карточка не доставлена", "err", err)
		}
	}
}

// wipeSrc затирает src_* при неудачной доставке (не оставляем Telegram ID в БД).
func (s *Questions) wipeSrc(ctx context.Context, questionID int64) {
	if err := s.store.WipeQuestionSrc(ctx, questionID); err != nil {
		s.log.Error("не удалось затереть src_* вопроса", "question_id", questionID, "err", err)
	}
}

// StartReplying переводит получателя в состояние «пишет ответ» (кнопка 💬 Ответить).
func (s *Questions) StartReplying(ctx context.Context, recipientTgID, questionID int64) error {
	recipient, err := s.store.GetUserByTgID(ctx, recipientTgID)
	if err != nil {
		return err
	}
	q, err := s.store.GetQuestion(ctx, questionID)
	if storage.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if q.RecipientID != recipient.ID {
		return ErrForbidden
	}
	return s.store.SetDialogState(ctx, gen.SetDialogStateParams{
		TgID:    recipientTgID,
		State:   StateReplying,
		Payload: MarshalPayload(DialogPayload{QuestionID: questionID}),
	})
}

// Answer доставляет ответ получателя отправителю вопроса.
// Расшифровка tg_id здесь — техническая (для доставки), личность никому
// не показывается (docs/09). Отправитель получает кнопку продолжить диалог.
func (s *Questions) Answer(ctx context.Context, recipientTgID int64, srcMsgID int) error {
	st, err := s.store.GetDialogState(ctx, recipientTgID)
	if storage.IsNotFound(err) || (err == nil && st.State != StateReplying) {
		return ErrNoDialog
	}
	if err != nil {
		return err
	}
	payload, err := UnmarshalPayload(st.Payload)
	if err != nil {
		return err
	}
	recipient, err := s.store.GetUserByTgID(ctx, recipientTgID)
	if err != nil {
		return err
	}
	q, err := s.store.GetQuestion(ctx, payload.QuestionID)
	if err != nil {
		return err
	}
	if q.RecipientID != recipient.ID {
		return ErrForbidden
	}

	senderTgID, err := s.codec.DecryptSender(q.SenderEnc, q.ID)
	if err != nil {
		return fmt.Errorf("расшифровка отправителя: %w", err)
	}

	kb := Keyboard{Row(Button{Text: "💬 Ответить", Data: CB(CBContinue, q.ID)})}
	if _, err := s.tg.SendMessage(ctx, senderTgID, "💬 Ответ на ваш вопрос:", nil); err != nil {
		return fmt.Errorf("%w: %v", ErrDelivery, err)
	}
	if _, err := s.tg.CopyMessage(ctx, senderTgID, recipientTgID, srcMsgID, kb); err != nil {
		return fmt.Errorf("%w: %v", ErrDelivery, err)
	}
	if err := s.store.MarkQuestionAnswered(ctx, q.ID); err != nil {
		return err
	}
	return s.store.ClearDialogState(ctx, recipientTgID)
}

// StartContinue переводит отправителя обратно в asking для продолжения диалога
// (кнопка 💬 Ответить под ответом). Право проверяется по sender_hash:
// продолжить может только тот, кто задал исходный вопрос.
func (s *Questions) StartContinue(ctx context.Context, senderTgID, questionID int64) error {
	q, err := s.store.GetQuestion(ctx, questionID)
	if storage.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !hmac.Equal(q.SenderHash, s.codec.SenderHash(senderTgID)) {
		return ErrForbidden
	}
	link, err := s.store.GetLinkByID(ctx, q.LinkID)
	if err != nil {
		return err
	}
	if !link.IsActive {
		return ErrLinkInactive
	}
	return s.store.SetDialogState(ctx, gen.SetDialogStateParams{
		TgID:    senderTgID,
		State:   StateAsking,
		Payload: MarshalPayload(DialogPayload{LinkID: q.LinkID, ReplyTo: &q.ID}),
	})
}

// Report — жалоба получателя (кнопка ⚠️): контент пересылается всем админам
// из чата получателя по delivered_msg_id (src_* уже затёрты — docs/09),
// плюс карточка отправителя каждому админу.
func (s *Questions) Report(ctx context.Context, reporterTgID, questionID int64) error {
	reporter, err := s.store.GetUserByTgID(ctx, reporterTgID)
	if err != nil {
		return err
	}
	q, err := s.store.GetQuestion(ctx, questionID)
	if storage.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if q.RecipientID != reporter.ID {
		return ErrForbidden
	}
	if q.DeliveredMsgID == nil {
		return ErrNotFound
	}
	if err := s.audit(ctx, reporter.ID, "question_reported", nil, map[string]any{"question_id": q.ID}); err != nil {
		return err
	}

	admins, err := s.store.ListAdmins(ctx)
	if err != nil {
		return err
	}
	for _, admin := range admins {
		if admin.ID == reporter.ID {
			continue // жалобщик-админ и так всё видел
		}
		header := fmt.Sprintf("⚠️ Жалоба на вопрос #%d (получатель id %d):", q.ID, reporter.ID)
		if _, err := s.tg.SendMessage(ctx, admin.TgID, header, nil); err != nil {
			s.log.Warn("жалоба: заголовок не доставлен админу", "err", err)
			continue
		}
		if _, err := s.tg.CopyMessage(ctx, admin.TgID, reporter.TgID, int(*q.DeliveredMsgID), nil); err != nil {
			s.log.Warn("жалоба: копия не доставлена админу", "err", err)
		}
		if err := s.SendSenderCard(ctx, admin, q.ID); err != nil {
			s.log.Warn("жалоба: карточка не доставлена админу", "err", err)
		}
	}
	return nil
}

// SendSenderCard раскрывает отправителя вопроса и отправляет карточку админу.
// Единственная точка показа личности человеку: каждый вызов пишется в audit_log
// (action=sender_revealed). Имя и username берутся свежими через getChat (docs/06).
func (s *Questions) SendSenderCard(ctx context.Context, viewer gen.User, questionID int64) error {
	q, err := s.store.GetQuestion(ctx, questionID)
	if err != nil {
		return err
	}
	senderTgID, err := s.codec.DecryptSender(q.SenderEnc, q.ID)
	if err != nil {
		return fmt.Errorf("расшифровка отправителя: %w", err)
	}
	if err := s.audit(ctx, viewer.ID, "sender_revealed", nil, map[string]any{"question_id": q.ID}); err != nil {
		return err
	}

	info, err := s.tg.GetChatInfo(ctx, senderTgID)
	if err != nil {
		info = ChatInfo{Name: "(недоступен)"}
	}
	total, err := s.store.CountQuestionsFromSenderToRecipient(ctx, gen.CountQuestionsFromSenderToRecipientParams{
		SenderHash: q.SenderHash, RecipientID: q.RecipientID,
	})
	if err != nil {
		return err
	}
	today, err := s.store.CountQuestionsFromSenderToRecipientSince(ctx, gen.CountQuestionsFromSenderToRecipientSinceParams{
		SenderHash: q.SenderHash, RecipientID: q.RecipientID,
		Since: pgTime(time.Now().Add(-24 * time.Hour)),
	})
	if err != nil {
		return err
	}

	username := "—"
	if info.Username != "" {
		username = "@" + info.Username
	}
	text := fmt.Sprintf(
		"🕵️ Отправитель:\n├ Имя: %s\n├ Username: %s\n├ TG ID: %d\n└ Вопросов этому получателю: %d (%d за сутки)",
		info.Name, username, senderTgID, total, today,
	)
	kb := Keyboard{Row(
		Button{Text: "🚫 Забанить в боте", Data: CB(CBAdminPrefix, "banq", q.ID)},
		Button{Text: "👁 Подписаться на него", Data: CB(CBAdminPrefix, "watchq", q.ID)},
	)}
	_, err = s.tg.SendMessage(ctx, viewer.TgID, text, kb)
	return err
}

// RevealSenderTgID расшифровывает отправителя вопроса для админ-действий
// (бан, подписка) с записью в audit_log.
func (s *Questions) RevealSenderTgID(ctx context.Context, actor gen.User, questionID int64, action string) (int64, error) {
	q, err := s.store.GetQuestion(ctx, questionID)
	if storage.IsNotFound(err) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	senderTgID, err := s.codec.DecryptSender(q.SenderEnc, q.ID)
	if err != nil {
		return 0, fmt.Errorf("расшифровка отправителя: %w", err)
	}
	if err := s.audit(ctx, actor.ID, "sender_revealed", nil, map[string]any{"question_id": q.ID, "reason": action}); err != nil {
		return 0, err
	}
	return senderTgID, nil
}

// audit пишет запись в журнал административных действий.
func (s *Questions) audit(ctx context.Context, actorID int64, action string, targetID *int64, details map[string]any) error {
	raw, err := json.Marshal(details)
	if err != nil {
		return err
	}
	return s.store.InsertAudit(ctx, gen.InsertAuditParams{ActorID: actorID, Action: action, TargetID: targetID, Details: raw})
}

// pgTime конвертирует time.Time в pgtype.Timestamptz.
func pgTime(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}
