// Package storage — доступ к PostgreSQL: пул соединений, транзакции,
// применение встроенных миграций. Запросы генерируются sqlc в подпакете gen.
package storage

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/vitaly/secure-ask-tg-bot/internal/storage/gen"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Storage объединяет пул соединений и sqlc-запросы.
type Storage struct {
	Pool *pgxpool.Pool
	*gen.Queries
}

// New подключается к Postgres и применяет миграции.
func New(ctx context.Context, dbURL string) (*Storage, error) {
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil, fmt.Errorf("pgxpool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	if err := migrate(pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Storage{Pool: pool, Queries: gen.New(pool)}, nil
}

// migrate применяет goose-миграции из embed. Advisory lock внутри goose
// гарантирует, что два инстанса не применят одну миграцию дважды.
func migrate(pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, sub)
	if err != nil {
		return err
	}
	_, err = provider.Up(context.Background())
	return err
}

// WithTx выполняет fn в транзакции, отдавая ему sqlc-запросы поверх tx.
func (s *Storage) WithTx(ctx context.Context, fn func(q *gen.Queries) error) error {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback после commit — no-op

	if err := fn(s.Queries.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Close закрывает пул соединений.
func (s *Storage) Close() { s.Pool.Close() }

// IsUniqueViolation сообщает, вызвана ли ошибка нарушением уникального
// ограничения (SQLSTATE 23505). Используется сервисным слоем, чтобы
// корректно отвечать «алиас занят» при гонке, не импортируя pgx.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// IsNotFound сообщает, что запрос не нашёл ни одной строки.
func IsNotFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
