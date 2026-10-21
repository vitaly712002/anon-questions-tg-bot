//go:build integration

// Интеграционные тесты storage с реальным PostgreSQL (testcontainers).
// Запуск: make test-integration (нужен Docker).
package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/vitaly/secure-ask-tg-bot/internal/storage"
	"github.com/vitaly/secure-ask-tg-bot/internal/storage/gen"
)

// newTestStorage поднимает Postgres 16 в контейнере и применяет миграции.
func newTestStorage(t *testing.T) *storage.Storage {
	t.Helper()
	ctx := context.Background()

	pg, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("secureask_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("контейнер postgres: %v", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(context.Background()) })

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	st, err := storage.New(ctx, dsn) // применяет goose-миграции из embed
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(st.Close)
	return st
}

func strPtr(s string) *string { return &s }

func TestStorage_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	st := newTestStorage(t)
	ctx := context.Background()

	// --- users: upsert идемпотентен и обновляет профиль ---
	u1, err := st.UpsertUser(ctx, gen.UpsertUserParams{TgID: 100, TgUsername: strPtr("vasya"), DisplayName: "Вася"})
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if u1.Role != gen.UserRoleUser || !u1.Accepting {
		t.Errorf("дефолты роли/приёма неверны: %+v", u1)
	}
	u1b, err := st.UpsertUser(ctx, gen.UpsertUserParams{TgID: 100, TgUsername: strPtr("vasya_new"), DisplayName: "Вася Н."})
	if err != nil {
		t.Fatalf("повторный UpsertUser: %v", err)
	}
	if u1b.ID != u1.ID || *u1b.TgUsername != "vasya_new" {
		t.Errorf("upsert не обновил профиль: %+v", u1b)
	}

	// --- EnsureOwnerSysadmin идемпотентен ---
	for i := 0; i < 2; i++ {
		if err := st.EnsureOwnerSysadmin(ctx, 100); err != nil {
			t.Fatalf("EnsureOwnerSysadmin: %v", err)
		}
	}
	owner, _ := st.GetUserByTgID(ctx, 100)
	if owner.Role != gen.UserRoleSysadmin {
		t.Errorf("владелец не sysadmin: %+v", owner)
	}

	// --- links: токен уникален, алиас регистронезависимо уникален ---
	l1, err := st.CreateLink(ctx, gen.CreateLinkParams{UserID: u1.ID, Token: "q_token1"})
	if err != nil {
		t.Fatalf("CreateLink: %v", err)
	}
	if _, err := st.CreateLink(ctx, gen.CreateLinkParams{UserID: u1.ID, Token: "q_token1"}); !storage.IsUniqueViolation(err) {
		t.Errorf("дубликат токена: ожидалась unique violation, получено %v", err)
	}
	if err := st.SetLinkAlias(ctx, gen.SetLinkAliasParams{ID: l1.ID, Alias: strPtr("vasya")}); err != nil {
		t.Fatalf("SetLinkAlias: %v", err)
	}
	u2, _ := st.UpsertUser(ctx, gen.UpsertUserParams{TgID: 200, DisplayName: "Петя"})
	l2, _ := st.CreateLink(ctx, gen.CreateLinkParams{UserID: u2.ID, Token: "q_token2"})
	// "Vasya" и "vasya" — одна ссылка (индекс по lower(alias), docs/04).
	if err := st.SetLinkAlias(ctx, gen.SetLinkAliasParams{ID: l2.ID, Alias: strPtr("Vasya")}); !storage.IsUniqueViolation(err) {
		t.Errorf("регистронезависимая уникальность алиаса не сработала: %v", err)
	}
	byAlias, err := st.GetLinkByAlias(ctx, "VASYA")
	if err != nil || byAlias.ID != l1.ID {
		t.Errorf("GetLinkByAlias(VASYA): %v, link=%+v", err, byAlias)
	}

	// --- questions: вставка, доставка затирает src_* ---
	qid, err := st.InsertQuestion(ctx, gen.InsertQuestionParams{
		LinkID: l1.ID, RecipientID: u1.ID,
		SenderEnc: []byte{1, 2, 3}, SenderHash: []byte("hash-1"),
		ContentType: "text", SrcChatID: 777, SrcMessageID: 42,
	})
	if err != nil {
		t.Fatalf("InsertQuestion: %v", err)
	}
	delivered := int64(555)
	if err := st.MarkQuestionDelivered(ctx, gen.MarkQuestionDeliveredParams{ID: qid, DeliveredMsgID: &delivered}); err != nil {
		t.Fatalf("MarkQuestionDelivered: %v", err)
	}
	q, _ := st.GetQuestion(ctx, qid)
	if q.SrcChatID != 0 || q.SrcMessageID != 0 {
		t.Errorf("src_* не затёрты после доставки: %+v", q)
	}
	if q.DeliveredMsgID == nil || *q.DeliveredMsgID != delivered {
		t.Errorf("delivered_msg_id: %+v", q.DeliveredMsgID)
	}

	// --- суточный лимит: счётчик по sender_hash ---
	n, err := st.CountQuestionsBySenderSince(ctx, gen.CountQuestionsBySenderSinceParams{
		SenderHash: []byte("hash-1"),
		Since:      pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
	})
	if err != nil || n != 1 {
		t.Errorf("CountQuestionsBySenderSince = %d, %v; want 1", n, err)
	}

	// --- blocks ---
	if err := st.AddBlock(ctx, gen.AddBlockParams{RecipientID: u1.ID, SenderHash: []byte("hash-1")}); err != nil {
		t.Fatalf("AddBlock: %v", err)
	}
	// Повтор не падает (ON CONFLICT DO NOTHING).
	if err := st.AddBlock(ctx, gen.AddBlockParams{RecipientID: u1.ID, SenderHash: []byte("hash-1")}); err != nil {
		t.Fatalf("повторный AddBlock: %v", err)
	}
	blocked, _ := st.IsBlocked(ctx, gen.IsBlockedParams{RecipientID: u1.ID, SenderHash: []byte("hash-1")})
	if !blocked {
		t.Error("IsBlocked = false, want true")
	}

	// --- dialog_states: upsert и очистка ---
	if err := st.SetDialogState(ctx, gen.SetDialogStateParams{TgID: 777, State: "asking", Payload: []byte(`{"link_id":1}`)}); err != nil {
		t.Fatalf("SetDialogState: %v", err)
	}
	if err := st.SetDialogState(ctx, gen.SetDialogStateParams{TgID: 777, State: "replying", Payload: []byte(`{}`)}); err != nil {
		t.Fatalf("SetDialogState (upsert): %v", err)
	}
	ds, _ := st.GetDialogState(ctx, 777)
	if ds.State != "replying" {
		t.Errorf("state = %q, want replying", ds.State)
	}
	if err := st.ClearDialogState(ctx, 777); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetDialogState(ctx, 777); !storage.IsNotFound(err) {
		t.Errorf("после очистки ожидался IsNotFound, получено %v", err)
	}

	// --- admin: watch + audit ---
	if err := st.AddWatch(ctx, gen.AddWatchParams{AdminID: u1.ID, TargetID: u2.ID}); err != nil {
		t.Fatalf("AddWatch: %v", err)
	}
	rows, _ := st.ListWatcherTgIDs(ctx, u2.ID)
	if len(rows) != 1 || rows[0].TgID != 100 {
		t.Errorf("ListWatcherTgIDs: %+v", rows)
	}
	if err := st.InsertAudit(ctx, gen.InsertAuditParams{ActorID: u1.ID, Action: "sender_revealed", Details: []byte(`{"question_id":1}`)}); err != nil {
		t.Fatalf("InsertAudit: %v", err)
	}
	audit, _ := st.ListAuditRecent(ctx, 10)
	if len(audit) != 1 || audit[0].Action != "sender_revealed" {
		t.Errorf("ListAuditRecent: %+v", audit)
	}

	// --- WithTx: откат при ошибке ---
	wantErr := context.Canceled // любой сигнальный err
	err = st.WithTx(ctx, func(q *gen.Queries) error {
		if _, err := q.InsertQuestion(ctx, gen.InsertQuestionParams{
			LinkID: l1.ID, RecipientID: u1.ID,
			SenderEnc: []byte{9}, SenderHash: []byte("hash-tx"),
			ContentType: "text", SrcChatID: 1, SrcMessageID: 1,
		}); err != nil {
			return err
		}
		return wantErr
	})
	if err != wantErr {
		t.Fatalf("WithTx: err = %v, want %v", err, wantErr)
	}
	nTx, _ := st.CountQuestionsBySenderSince(ctx, gen.CountQuestionsBySenderSinceParams{
		SenderHash: []byte("hash-tx"),
		Since:      pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
	})
	if nTx != 0 {
		t.Errorf("транзакция не откатилась: найдено %d вопросов", nTx)
	}
}
