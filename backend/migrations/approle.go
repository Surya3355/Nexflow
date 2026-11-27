package migrations

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AppRole is the database role the running app acts as. It can read and
// change every table but may only add to and read the audit log (000012).
const AppRole = "nexflow_app"

// UseAppRole makes every connection of a pool act as AppRole. Run Up with a
// plain owner pool first; serve requests from a pool configured with this.
func UseAppRole(cfg *pgxpool.Config) {
	prev := cfg.AfterConnect
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		if prev != nil {
			if err := prev(ctx, c); err != nil {
				return err
			}
		}
		_, err := c.Exec(ctx, "SET ROLE "+AppRole)
		return err
	}
}
