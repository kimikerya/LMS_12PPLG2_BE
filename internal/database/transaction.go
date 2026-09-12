package database

import (
	"context"
	"database/sql"
)

// Querier lets repositories use a connection pool or an existing transaction.
type Querier interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// WithTx is for data changes (DML), never for MySQL schema migrations (DDL).
func WithTx(ctx context.Context, db *sql.DB, fn func(Querier) error) error {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
