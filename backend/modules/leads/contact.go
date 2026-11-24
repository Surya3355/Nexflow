package leads

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/Surya3355/nexflow/backend/modules/leads/leadsdb"
	"github.com/jackc/pgx/v5/pgtype"
)

// Contact log errors.
var (
	ErrInvalidResponse = errors.New("leads: choose a response")
	ErrRemarkRequired  = errors.New("leads: write a remark")
)

// Responses a contact can record.
var responses = map[string]bool{"interested": true, "call_back": true, "not_reachable": true, "not_interested": true}

// Contact is one logged call or meeting with a lead's customer.
type Contact struct {
	ID           int64
	Response     string
	Remark       string
	NextCallDate *time.Time
	CreatedAt    time.Time
	CreatedBy    string
}

// ContactIn is what staff record for a contact.
type ContactIn struct {
	Response     string
	Remark       string
	NextCallDate *time.Time
}

// LogContact adds a contact to an open lead and sets the lead's next call
// date to the contact's (cleared when the contact has none). "Not
// interested" does not drop the lead; staff decide that separately.
func (s *Service) LogContact(ctx context.Context, actor access.User, leadID int64, in ContactIn) (Lead, error) {
	return s.LogContactAt(ctx, actor, leadID, in, time.Now())
}

// LogContactAt is LogContact with the current time given, which decides
// whether the next call is today and its reminder is queued at once.
func (s *Service) LogContactAt(ctx context.Context, actor access.User, leadID int64, in ContactIn, now time.Time) (Lead, error) {
	if !access.Can(actor.Role, access.ActionAddRecord) {
		return Lead{}, access.ErrForbidden
	}
	if !responses[in.Response] {
		return Lead{}, ErrInvalidResponse
	}
	remark := strings.TrimSpace(in.Remark)
	if remark == "" {
		return Lead{}, ErrRemarkRequired
	}
	var next pgtype.Date
	if in.NextCallDate != nil {
		next = dateRef(*in.NextCallDate)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Lead{}, fmt.Errorf("leads: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	txCtx := history.WithTx(ctx, tx)
	if _, err := lockOpen(txCtx, q, leadID); err != nil {
		return Lead{}, err
	}
	if err := q.InsertContact(txCtx, leadsdb.InsertContactParams{
		LeadID: leadID, Response: in.Response, Remark: remark, NextCallDate: next, CreatedBy: userRef(actor),
	}); err != nil {
		return Lead{}, fmt.Errorf("leads: insert contact: %w", err)
	}
	if err := q.SetLeadNextCall(txCtx, leadsdb.SetLeadNextCallParams{ID: leadID, NextCallDate: next, UpdatedBy: userRef(actor)}); err != nil {
		return Lead{}, fmt.Errorf("leads: next call: %w", err)
	}
	if err := s.audit(txCtx, actor, "lead.contacted", leadID, map[string]any{"response": in.Response}); err != nil {
		return Lead{}, err
	}
	if err := queueDueToday(txCtx, q, leadID, now); err != nil {
		return Lead{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Lead{}, fmt.Errorf("leads: commit: %w", err)
	}
	return s.lead(ctx, leadID)
}

// Contacts returns a lead's contact log, newest first.
func (s *Service) Contacts(ctx context.Context, leadID int64) ([]Contact, error) {
	rows, err := s.q.ListContacts(ctx, leadID)
	if err != nil {
		return nil, fmt.Errorf("leads: contacts: %w", err)
	}
	out := make([]Contact, 0, len(rows))
	for _, r := range rows {
		c := Contact{ID: r.ID, Response: r.Response, Remark: r.Remark, CreatedAt: r.CreatedAt.Time, CreatedBy: r.CreatedBy}
		if r.NextCallDate.Valid {
			d := r.NextCallDate.Time
			c.NextCallDate = &d
		}
		out = append(out, c)
	}
	return out, nil
}
