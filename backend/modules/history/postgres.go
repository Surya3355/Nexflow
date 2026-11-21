package history

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgRecorder writes audit rows to audit_log. It uses the transaction in the
// context when there is one (see WithTx), so the row commits with its change.
type PgRecorder struct {
	pool *pgxpool.Pool
}

// NewPgRecorder returns a recorder that writes to pool.
func NewPgRecorder(pool *pgxpool.Pool) *PgRecorder {
	return &PgRecorder{pool: pool}
}

// Record inserts one audit row.
func (r *PgRecorder) Record(ctx context.Context, e Entry) error {
	var details []byte
	if e.Details != nil {
		b, err := json.Marshal(e.Details)
		if err != nil {
			return fmt.Errorf("audit: details: %w", err)
		}
		details = b
	}
	args := []any{
		nullableInt(e.ActorUserID), e.Action, e.EntityType, nullableInt(e.EntityID), details,
	}
	const q = `INSERT INTO audit_log (actor_user_id, action, entity_type, entity_id, details)
	           VALUES ($1, $2, $3, $4, $5)`
	if tx, ok := TxFromContext(ctx); ok {
		if _, err := tx.Exec(ctx, q, args...); err != nil {
			return fmt.Errorf("audit: insert: %w", err)
		}
		return nil
	}
	if _, err := r.pool.Exec(ctx, q, args...); err != nil {
		return fmt.Errorf("audit: insert: %w", err)
	}
	return nil
}

func nullableInt(p *int64) pgtype.Int8 {
	if p == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *p, Valid: true}
}
