package leads

import (
	"context"
	"errors"
	"fmt"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/Surya3355/nexflow/backend/modules/leads/leadsdb"
	"github.com/jackc/pgx/v5/pgtype"
)

// Builder contact errors (spec 2026-11-24 §4.3).
var (
	ErrBuilderContactRequired = errors.New("leads: choose a builder contact")
	ErrBuilderContactInvalid  = errors.New("leads: pick a current contact of this project")
)

// BuilderContact is the project contact a lead points to. Removed means
// admin took it off the project's list after it was chosen.
type BuilderContact struct {
	ID                        int64
	Name, Phone, Email, Label string
	Removed                   bool
}

// SetBuilderContact points an open lead at another active contact of its
// project. All staff may do this.
func (s *Service) SetBuilderContact(ctx context.Context, actor access.User, leadID, contactID int64) (Lead, error) {
	if !access.Can(actor.Role, access.ActionEditRecord) {
		return Lead{}, access.ErrForbidden
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Lead{}, fmt.Errorf("leads: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	txCtx := history.WithTx(ctx, tx)
	row, err := lockOpen(txCtx, q, leadID)
	if err != nil {
		return Lead{}, err
	}
	ok, err := q.ActiveContactInProject(txCtx, leadsdb.ActiveContactInProjectParams{ID: contactID, ProjectID: row.ProjectID})
	if err != nil {
		return Lead{}, fmt.Errorf("leads: check contact: %w", err)
	}
	if !ok {
		return Lead{}, ErrBuilderContactInvalid
	}
	if !row.BuilderContactID.Valid || row.BuilderContactID.Int64 != contactID {
		if err := q.SetLeadBuilderContact(txCtx, leadsdb.SetLeadBuilderContactParams{
			ID: leadID, BuilderContactID: pgtype.Int8{Int64: contactID, Valid: true}, UpdatedBy: userRef(actor),
		}); err != nil {
			return Lead{}, fmt.Errorf("leads: set contact: %w", err)
		}
		var from any
		if row.BuilderContactID.Valid {
			from = row.BuilderContactID.Int64
		}
		if err := s.audit(txCtx, actor, "lead.builder_contact_changed", leadID, map[string]any{"from": from, "to": contactID}); err != nil {
			return Lead{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Lead{}, fmt.Errorf("leads: commit: %w", err)
	}
	return s.lead(ctx, leadID)
}
