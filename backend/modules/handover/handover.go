// Package handover moves all of a leaving user's open work to another user
// when an admin switches them off (spec 2026-11-27-hardening §4.2). It runs
// inside the switch-off transaction, which it takes from the context.
package handover

import (
	"context"
	"errors"
	"fmt"

	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5"
)

// Notifier queues the "assigned" alerts. alerts.Queue provides it.
type Notifier interface {
	QueueAssigned(ctx context.Context, caseID, recipientID int64) error
	QueueLeadAssigned(ctx context.Context, leadID, recipientID int64) error
}

// Handover satisfies access.Reassigner.
type Handover struct {
	rec      history.Recorder
	notifier Notifier
}

// New returns a handover that records history to rec and queues alerts with n.
func New(rec history.Recorder, n Notifier) *Handover {
	return &Handover{rec: rec, notifier: n}
}

const reason = "handover"

// ReassignAllTx moves from's open leads, active cases and unresolved bank
// queries on active cases to to. Moved leads and cases queue the "assigned"
// alert; moved queries do not (there is no such alert for queries). Every
// move is in the history, by actorID, with reason "handover".
func (h *Handover) ReassignAllTx(ctx context.Context, actorID, from, to int64) (int, int, int, error) {
	tx, ok := history.TxFromContext(ctx)
	if !ok {
		return 0, 0, 0, errors.New("handover: must run inside a transaction")
	}
	leads, err := moveIDs(ctx, tx, `UPDATE leads SET assignee_id = $2, updated_at = now(), updated_by = $3
		WHERE assignee_id = $1 AND status = 'open' RETURNING id`, from, to, actorID)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("handover: leads: %w", err)
	}
	for _, id := range leads {
		if err := h.notifier.QueueLeadAssigned(ctx, id, to); err != nil {
			return 0, 0, 0, err
		}
		if err := h.record(ctx, actorID, "lead.assigned", "lead", id, map[string]any{"assignee_id": to, "from_user_id": from, "reason": reason}); err != nil {
			return 0, 0, 0, err
		}
	}

	cases, err := moveIDs(ctx, tx, `UPDATE cases SET assignee_id = $2, updated_at = now(), updated_by = $3
		WHERE assignee_id = $1 AND status = 'active' RETURNING id`, from, to, actorID)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("handover: cases: %w", err)
	}
	for _, id := range cases {
		if _, err := tx.Exec(ctx, `INSERT INTO case_assignment_events (case_id, from_user_id, to_user_id, changed_by, changed_at)
			VALUES ($1, $2, $3, $4, now())`, id, from, to, actorID); err != nil {
			return 0, 0, 0, fmt.Errorf("handover: case event: %w", err)
		}
		if err := h.notifier.QueueAssigned(ctx, id, to); err != nil {
			return 0, 0, 0, err
		}
		if err := h.record(ctx, actorID, "case.assigned", "case", id, map[string]any{"from_user_id": from, "to_user_id": to, "reason": reason}); err != nil {
			return 0, 0, 0, err
		}
	}

	queries, err := moveIDs(ctx, tx, `UPDATE case_queries q SET assignee_id = $2, updated_at = now(), updated_by = $3
		FROM cases c
		WHERE c.id = q.case_id AND c.status = 'active'
		  AND q.assignee_id = $1 AND q.removed_at IS NULL AND q.status <> 'resolved'
		RETURNING q.id`, from, to, actorID)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("handover: queries: %w", err)
	}
	for _, id := range queries {
		if err := h.record(ctx, actorID, "query.reassigned", "case_query", id, map[string]any{"from_user_id": from, "to_user_id": to, "reason": reason}); err != nil {
			return 0, 0, 0, err
		}
	}
	return len(leads), len(cases), len(queries), nil
}

func moveIDs(ctx context.Context, tx pgx.Tx, sql string, args ...any) ([]int64, error) {
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

func (h *Handover) record(ctx context.Context, actorID int64, action, entity string, id int64, details map[string]any) error {
	a, e := actorID, id
	if err := h.rec.Record(ctx, history.Entry{ActorUserID: &a, Action: action, EntityType: entity, EntityID: &e, Details: details}); err != nil {
		return fmt.Errorf("handover: audit: %w", err)
	}
	return nil
}
