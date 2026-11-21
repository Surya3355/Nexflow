package alerts

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// naProjectID returns the id of the fixed NA project.
func naProjectID(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(), `SELECT id FROM projects WHERE is_system`).Scan(&id); err != nil {
		t.Fatalf("NA project: %v", err)
	}
	return id
}
