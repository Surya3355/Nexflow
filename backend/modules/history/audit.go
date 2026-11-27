// Package history writes the permanent audit log (spec §4.6). Business code
// calls Recorder.Record; the Postgres recorder (Task 16) stores the row.
package history

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Entry is one audit row: who did what to which record, with small details.
type Entry struct {
	ActorUserID *int64
	Action      string
	EntityType  string
	EntityID    *int64
	Details     any
}

// Recorder writes audit entries.
type Recorder interface {
	Record(ctx context.Context, e Entry) error
}

// Nop discards entries. Used in tests that do not check the audit log.
type Nop struct{}

// Record does nothing and returns nil.
func (Nop) Record(context.Context, Entry) error { return nil }

type txKey struct{}

// WithTx returns a context that carries an open transaction. The Postgres
// recorder writes audit rows on that transaction, so the audit row commits
// or rolls back together with the change it describes.
func WithTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// TxFromContext returns the transaction stored by WithTx, if any.
func TxFromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	return tx, ok
}
