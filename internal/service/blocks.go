package service

import (
	"context"

	"github.com/vitaly/secure-ask-tg-bot/internal/storage"
	"github.com/vitaly/secure-ask-tg-bot/internal/storage/gen"
)

// Blocks — персональные блокировки отправителей.
// Блокировка хранит только sender_hash: личность не раскрывается ни
// получателю, ни в БД (docs/05).
type Blocks struct {
	store Store
}

// NewBlocks создаёт сервис блокировок.
func NewBlocks(store Store) *Blocks {
	return &Blocks{store: store}
}

// BlockSenderOfQuestion блокирует отправителя вопроса для его получателя
// (кнопка 🚫 под вопросом). Проверяет, что жмёт именно получатель.
func (s *Blocks) BlockSenderOfQuestion(ctx context.Context, recipientTgID, questionID int64) error {
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
	return s.store.AddBlock(ctx, gen.AddBlockParams{RecipientID: recipient.ID, SenderHash: q.SenderHash})
}
