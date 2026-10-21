package service

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/vitaly/secure-ask-tg-bot/internal/crypto"
	"github.com/vitaly/secure-ask-tg-bot/internal/storage/gen"
)

// --- фейки -----------------------------------------------------------------

// fakeStore — in-memory реализация нужных тестам методов Store.
// Встроенный nil-интерфейс Store паникует при вызове незамоканного метода —
// тест сразу покажет, какой метод нужно добавить.
type fakeStore struct {
	Store
	users     map[int64]gen.User // по внутреннему id
	links     map[int64]gen.Link
	states    map[int64]gen.DialogState
	questions map[int64]gen.Question
	blocked   bool
	daily     int64
	watchers  []gen.ListWatcherTgIDsRow
	audits    []gen.InsertAuditParams
	nextQID   int64
	wiped     []int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		users:     map[int64]gen.User{},
		links:     map[int64]gen.Link{},
		states:    map[int64]gen.DialogState{},
		questions: map[int64]gen.Question{},
	}
}

func (f *fakeStore) WithTx(ctx context.Context, fn func(q Querier) error) error { return fn(f) }

func (f *fakeStore) GetDialogState(_ context.Context, tgID int64) (gen.DialogState, error) {
	st, ok := f.states[tgID]
	if !ok {
		return gen.DialogState{}, pgx.ErrNoRows
	}
	return st, nil
}

func (f *fakeStore) SetDialogState(_ context.Context, arg gen.SetDialogStateParams) error {
	f.states[arg.TgID] = gen.DialogState{TgID: arg.TgID, State: arg.State, Payload: arg.Payload}
	return nil
}

func (f *fakeStore) ClearDialogState(_ context.Context, tgID int64) error {
	delete(f.states, tgID)
	return nil
}

func (f *fakeStore) GetLinkByID(_ context.Context, id int64) (gen.Link, error) {
	l, ok := f.links[id]
	if !ok {
		return gen.Link{}, pgx.ErrNoRows
	}
	return l, nil
}

func (f *fakeStore) GetUserByID(_ context.Context, id int64) (gen.User, error) {
	u, ok := f.users[id]
	if !ok {
		return gen.User{}, pgx.ErrNoRows
	}
	return u, nil
}

func (f *fakeStore) GetUserByTgID(_ context.Context, tgID int64) (gen.User, error) {
	for _, u := range f.users {
		if u.TgID == tgID {
			return u, nil
		}
	}
	return gen.User{}, pgx.ErrNoRows
}

func (f *fakeStore) IsBlocked(_ context.Context, _ gen.IsBlockedParams) (bool, error) {
	return f.blocked, nil
}

func (f *fakeStore) CountQuestionsBySenderSince(_ context.Context, _ gen.CountQuestionsBySenderSinceParams) (int64, error) {
	return f.daily, nil
}

func (f *fakeStore) InsertQuestion(_ context.Context, arg gen.InsertQuestionParams) (int64, error) {
	f.nextQID++
	f.questions[f.nextQID] = gen.Question{
		ID: f.nextQID, LinkID: arg.LinkID, RecipientID: arg.RecipientID, ReplyTo: arg.ReplyTo,
		SenderEnc: arg.SenderEnc, SenderHash: arg.SenderHash, ContentType: arg.ContentType,
		SrcChatID: arg.SrcChatID, SrcMessageID: arg.SrcMessageID,
	}
	return f.nextQID, nil
}

func (f *fakeStore) UpdateQuestionSenderEnc(_ context.Context, arg gen.UpdateQuestionSenderEncParams) error {
	q := f.questions[arg.ID]
	q.SenderEnc = arg.SenderEnc
	f.questions[arg.ID] = q
	return nil
}

func (f *fakeStore) GetQuestion(_ context.Context, id int64) (gen.Question, error) {
	q, ok := f.questions[id]
	if !ok {
		return gen.Question{}, pgx.ErrNoRows
	}
	return q, nil
}

func (f *fakeStore) MarkQuestionDelivered(_ context.Context, arg gen.MarkQuestionDeliveredParams) error {
	q := f.questions[arg.ID]
	q.DeliveredMsgID = arg.DeliveredMsgID
	q.SrcChatID, q.SrcMessageID = 0, 0
	f.questions[arg.ID] = q
	return nil
}

func (f *fakeStore) WipeQuestionSrc(_ context.Context, id int64) error {
	f.wiped = append(f.wiped, id)
	q := f.questions[id]
	q.SrcChatID, q.SrcMessageID = 0, 0
	f.questions[id] = q
	return nil
}

func (f *fakeStore) MarkQuestionAnswered(_ context.Context, id int64) error {
	q := f.questions[id]
	q.Answered = true
	f.questions[id] = q
	return nil
}

func (f *fakeStore) ListWatcherTgIDs(_ context.Context, _ int64) ([]gen.ListWatcherTgIDsRow, error) {
	return f.watchers, nil
}

func (f *fakeStore) InsertAudit(_ context.Context, arg gen.InsertAuditParams) error {
	f.audits = append(f.audits, arg)
	return nil
}

func (f *fakeStore) CountQuestionsFromSenderToRecipient(_ context.Context, _ gen.CountQuestionsFromSenderToRecipientParams) (int64, error) {
	return 1, nil
}

func (f *fakeStore) CountQuestionsFromSenderToRecipientSince(_ context.Context, _ gen.CountQuestionsFromSenderToRecipientSinceParams) (int64, error) {
	return 1, nil
}

func (f *fakeStore) SetUserRole(_ context.Context, arg gen.SetUserRoleParams) error {
	u := f.users[arg.ID]
	u.Role = arg.Role
	f.users[arg.ID] = u
	return nil
}

// sent — записанное исходящее сообщение фейкового Telegram.
type sent struct {
	chatID int64
	text   string
	kb     Keyboard
	copied bool
	from   int64
	msgID  int
}

type fakeTG struct {
	sent    []sent
	failAll bool
	nextID  int
}

func (f *fakeTG) SendMessage(_ context.Context, chatID int64, text string, kb Keyboard) (int, error) {
	if f.failAll {
		return 0, errors.New("forbidden: bot was blocked by the user")
	}
	f.nextID++
	f.sent = append(f.sent, sent{chatID: chatID, text: text, kb: kb})
	return f.nextID, nil
}

func (f *fakeTG) CopyMessage(_ context.Context, toChatID, fromChatID int64, messageID int, kb Keyboard) (int, error) {
	if f.failAll {
		return 0, errors.New("forbidden: bot was blocked by the user")
	}
	f.nextID++
	f.sent = append(f.sent, sent{chatID: toChatID, copied: true, from: fromChatID, msgID: messageID, kb: kb})
	return f.nextID, nil
}

func (f *fakeTG) GetChatInfo(_ context.Context, _ int64) (ChatInfo, error) {
	return ChatInfo{Name: "Петя Иванов", Username: "petya"}, nil
}

type fakeLimiter struct{ allow bool }

func (f fakeLimiter) Allow(string) bool { return f.allow }

// --- вспомогательные -------------------------------------------------------

func testCodec(t *testing.T) *crypto.Codec {
	t.Helper()
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	key2[0] = 1
	c, err := crypto.NewCodec(key1, key2)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

const (
	guestTgID     = int64(777)
	recipientTgID = int64(555)
)

// setupSubmit готовит стандартную сцену: получатель id=1 со ссылкой id=10,
// гость guestTgID в состоянии asking.
func setupSubmit(t *testing.T) (*fakeStore, *fakeTG, *Questions) {
	t.Helper()
	store := newFakeStore()
	store.users[1] = gen.User{ID: 1, TgID: recipientTgID, DisplayName: "Вася", Accepting: true}
	store.links[10] = gen.Link{ID: 10, UserID: 1, Token: "q_test", IsActive: true}
	store.states[guestTgID] = gen.DialogState{
		TgID: guestTgID, State: StateAsking,
		Payload: MarshalPayload(DialogPayload{LinkID: 10}),
	}
	tg := &fakeTG{}
	q := NewQuestions(store, testCodec(t), tg, fakeLimiter{allow: true}, 20, slog.Default())
	return store, tg, q
}

func submitInput() SubmitInput {
	return SubmitInput{SenderTgID: guestTgID, SrcChatID: guestTgID, SrcMsgID: 42, ContentType: "text"}
}

// --- тесты -----------------------------------------------------------------

func TestNormalizeAlias(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr error
	}{
		{"vasya", "vasya", nil},
		{"  @Vasya_123 ", "vasya_123", nil},
		{"ab", "", ErrAliasInvalid},                                 // короткий
		{"кириллица", "", ErrAliasInvalid},                          // не latin
		{"with space", "", ErrAliasInvalid},                         //
		{"admin", "", ErrAliasReserved},                             // стоп-слово
		{"q_abcdef", "", ErrAliasReserved},                          // маскируется под токен
		{"a234567890123456789012345678901234", "", ErrAliasInvalid}, // 34 символа
	}
	for _, c := range cases {
		got, err := NormalizeAlias(c.in)
		if !errors.Is(err, c.wantErr) {
			t.Errorf("NormalizeAlias(%q): err = %v, want %v", c.in, err, c.wantErr)
		}
		if got != c.want {
			t.Errorf("NormalizeAlias(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSubmit_OK(t *testing.T) {
	store, tg, svc := setupSubmit(t)

	if err := svc.Submit(context.Background(), submitInput()); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	q, ok := store.questions[1]
	if !ok {
		t.Fatal("вопрос не сохранён")
	}
	// Отправитель расшифровывается с AAD = question_id.
	senderTgID, err := testCodec(t).DecryptSender(q.SenderEnc, q.ID)
	if err != nil {
		t.Fatalf("DecryptSender: %v", err)
	}
	if senderTgID != guestTgID {
		t.Errorf("sender = %d, want %d", senderTgID, guestTgID)
	}
	// src_* затёрты после доставки.
	if q.SrcChatID != 0 || q.SrcMessageID != 0 {
		t.Errorf("src_* не затёрты: %d/%d", q.SrcChatID, q.SrcMessageID)
	}
	if q.DeliveredMsgID == nil {
		t.Error("delivered_msg_id не записан")
	}
	// Доставка: заголовок + копия с тремя кнопками.
	if len(tg.sent) != 2 {
		t.Fatalf("отправлено %d сообщений, ожидалось 2", len(tg.sent))
	}
	if tg.sent[0].chatID != recipientTgID || tg.sent[0].text == "" {
		t.Errorf("заголовок не получателю: %+v", tg.sent[0])
	}
	cp := tg.sent[1]
	if !cp.copied || cp.from != guestTgID || cp.msgID != 42 {
		t.Errorf("копия неверная: %+v", cp)
	}
	if len(cp.kb) != 1 || len(cp.kb[0]) != 3 {
		t.Errorf("ожидались 3 кнопки, клавиатура: %+v", cp.kb)
	}
}

func TestSubmit_SilentDrops(t *testing.T) {
	t.Run("персональная блокировка", func(t *testing.T) {
		store, tg, svc := setupSubmit(t)
		store.blocked = true
		if err := svc.Submit(context.Background(), submitInput()); !errors.Is(err, ErrSilentDrop) {
			t.Fatalf("err = %v, want ErrSilentDrop", err)
		}
		if len(store.questions) != 0 || len(tg.sent) != 0 {
			t.Error("вопрос не должен сохраняться и доставляться")
		}
	})
	t.Run("глобальный бан отправителя", func(t *testing.T) {
		store, tg, svc := setupSubmit(t)
		store.users[2] = gen.User{ID: 2, TgID: guestTgID, Banned: true}
		if err := svc.Submit(context.Background(), submitInput()); !errors.Is(err, ErrSilentDrop) {
			t.Fatalf("err = %v, want ErrSilentDrop", err)
		}
		if len(tg.sent) != 0 {
			t.Error("доставки быть не должно")
		}
	})
}

func TestSubmit_Limits(t *testing.T) {
	t.Run("burst", func(t *testing.T) {
		store, _, _ := setupSubmit(t)
		svc := NewQuestions(store, testCodec(t), &fakeTG{}, fakeLimiter{allow: false}, 20, slog.Default())
		if err := svc.Submit(context.Background(), submitInput()); !errors.Is(err, ErrRateLimited) {
			t.Fatalf("err = %v, want ErrRateLimited", err)
		}
	})
	t.Run("суточный", func(t *testing.T) {
		store, _, svc := setupSubmit(t)
		store.daily = 20
		if err := svc.Submit(context.Background(), submitInput()); !errors.Is(err, ErrDailyLimit) {
			t.Fatalf("err = %v, want ErrDailyLimit", err)
		}
	})
}

func TestSubmit_Rejections(t *testing.T) {
	t.Run("protected content", func(t *testing.T) {
		_, _, svc := setupSubmit(t)
		in := submitInput()
		in.Protected = true
		if err := svc.Submit(context.Background(), in); !errors.Is(err, ErrProtected) {
			t.Fatalf("err = %v, want ErrProtected", err)
		}
	})
	t.Run("получатель на паузе", func(t *testing.T) {
		store, _, svc := setupSubmit(t)
		u := store.users[1]
		u.Accepting = false
		store.users[1] = u
		if err := svc.Submit(context.Background(), submitInput()); !errors.Is(err, ErrNotAccepting) {
			t.Fatalf("err = %v, want ErrNotAccepting", err)
		}
	})
	t.Run("нет состояния диалога", func(t *testing.T) {
		store, _, svc := setupSubmit(t)
		delete(store.states, guestTgID)
		if err := svc.Submit(context.Background(), submitInput()); !errors.Is(err, ErrNoDialog) {
			t.Fatalf("err = %v, want ErrNoDialog", err)
		}
	})
}

func TestSubmit_DeliveryFailureWipesSrc(t *testing.T) {
	store, tg, svc := setupSubmit(t)
	tg.failAll = true

	err := svc.Submit(context.Background(), submitInput())
	if !errors.Is(err, ErrDelivery) {
		t.Fatalf("err = %v, want ErrDelivery", err)
	}
	if len(store.wiped) != 1 {
		t.Errorf("src_* должны быть затёрты при неудачной доставке, wiped=%v", store.wiped)
	}
}

func TestSubmit_AdminRecipientGetsSenderCard(t *testing.T) {
	store, tg, svc := setupSubmit(t)
	u := store.users[1]
	u.Role = gen.UserRoleAdmin
	store.users[1] = u

	if err := svc.Submit(context.Background(), submitInput()); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	// заголовок + копия + карточка отправителя
	if len(tg.sent) != 3 {
		t.Fatalf("отправлено %d сообщений, ожидалось 3", len(tg.sent))
	}
	card := tg.sent[2]
	if card.chatID != recipientTgID || card.copied {
		t.Errorf("карточка неверная: %+v", card)
	}
	// Раскрытие зафиксировано в аудите.
	found := false
	for _, a := range store.audits {
		if a.Action == "sender_revealed" {
			found = true
		}
	}
	if !found {
		t.Error("sender_revealed не записан в audit_log")
	}
}

func TestSubmit_WatchersGetCopy(t *testing.T) {
	store, tg, svc := setupSubmit(t)
	store.users[3] = gen.User{ID: 3, TgID: 999, Role: gen.UserRoleAdmin}
	store.watchers = []gen.ListWatcherTgIDsRow{{TgID: 999, ID: 3}}

	if err := svc.Submit(context.Background(), submitInput()); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	// получателю: заголовок + копия; наблюдателю: заголовок + копия + карточка
	var toWatcher int
	for _, m := range tg.sent {
		if m.chatID == 999 {
			toWatcher++
		}
	}
	if toWatcher != 3 {
		t.Errorf("наблюдателю отправлено %d сообщений, ожидалось 3", toWatcher)
	}
}

func TestAnswer_DeliversToSenderAnonymously(t *testing.T) {
	store, tg, svc := setupSubmit(t)
	if err := svc.Submit(context.Background(), submitInput()); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	tg.sent = nil

	if err := svc.StartReplying(context.Background(), recipientTgID, 1); err != nil {
		t.Fatalf("StartReplying: %v", err)
	}
	if err := svc.Answer(context.Background(), recipientTgID, 100); err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if len(tg.sent) != 2 {
		t.Fatalf("отправлено %d сообщений, ожидалось 2", len(tg.sent))
	}
	if tg.sent[0].chatID != guestTgID {
		t.Errorf("ответ ушёл не отправителю вопроса: %+v", tg.sent[0])
	}
	if !store.questions[1].Answered {
		t.Error("вопрос не помечен отвеченным")
	}
	if _, ok := store.states[recipientTgID]; ok {
		t.Error("состояние replying не очищено")
	}
}

func TestStartContinue_OnlyOriginalSender(t *testing.T) {
	_, _, svc := setupSubmit(t)
	if err := svc.Submit(context.Background(), submitInput()); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	// Чужой пользователь не может продолжить чужой диалог.
	if err := svc.StartContinue(context.Background(), 12345, 1); !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
	// Автор вопроса — может.
	if err := svc.StartContinue(context.Background(), guestTgID, 1); err != nil {
		t.Fatalf("StartContinue автором: %v", err)
	}
}

func TestSetRole_Permissions(t *testing.T) {
	store := newFakeStore()
	owner := gen.User{ID: 1, TgID: 1000, Role: gen.UserRoleSysadmin}
	admin := gen.User{ID: 2, TgID: 2000, Role: gen.UserRoleAdmin}
	target := gen.User{ID: 3, TgID: 3000, Role: gen.UserRoleUser}
	store.users[1], store.users[2], store.users[3] = owner, admin, target

	tg := &fakeTG{}
	q := NewQuestions(store, testCodec(t), tg, fakeLimiter{allow: true}, 20, slog.Default())
	svc := NewAdmin(store, tg, q, 1000, slog.Default())

	// admin не может раздавать роли.
	if err := svc.SetRole(context.Background(), admin, 3, gen.UserRoleAdmin); !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
	// sysadmin может.
	if err := svc.SetRole(context.Background(), owner, 3, gen.UserRoleAdmin); err != nil {
		t.Fatalf("SetRole: %v", err)
	}
	if store.users[3].Role != gen.UserRoleAdmin {
		t.Error("роль не назначена")
	}
	// Владельца разжаловать нельзя.
	if err := svc.SetRole(context.Background(), owner, 1, gen.UserRoleUser); !errors.Is(err, ErrOwnerChange) {
		t.Fatalf("err = %v, want ErrOwnerChange", err)
	}
	// Аудит записан.
	if len(store.audits) == 0 || store.audits[0].Action != "role_granted" {
		t.Errorf("аудит: %+v", store.audits)
	}
}

// Проверка совместимости фейка: убеждаемся, что *gen.Queries реализует Querier
// (компилируется — значит, интерфейс синхронизирован с генерацией sqlc).
var _ Querier = (*gen.Queries)(nil)

// Заглушка, чтобы линтер не ругался на неиспользуемый pgtype в этом файле.
var _ = pgtype.Timestamptz{}
