package archive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Errors for restore.
var (
	ErrMobileInUse       = errors.New("archive: that mobile number is now used by another customer; restore is refused")
	ErrDuplicateTracking = errors.New("archive: that tracking number is now used by a live case; restore is refused")
	ErrCustomerArchived  = errors.New("archive: the customer of this case is archived; restore the customer first")
	ErrReasonRequired    = errors.New("archive: choose a drop reason for the restored item")
	ErrAlreadyRestored   = errors.New("archive: this item has already been restored")
	ErrInvalidReason     = errors.New("archive: choose a valid drop reason")
	ErrRemarkRequired    = errors.New("archive: a remark is required when the reason is Other")
)

// RestoredRef names the live record created by a restore.
type RestoredRef struct {
	EntityType string
	EntityID   int64
}

// DropInput is the reason to use when the snapshot has none (spec §4.5, round 6).
type DropInput struct {
	ReasonID int64
	Remark   string
}

// Restore brings an archived customer or case back as Dropped (round 6), so
// staff review it before continuing. Admin only. The archive row is marked
// restored and the audit log records it.
func (s *Service) Restore(ctx context.Context, actor access.User, archiveID int64, reason DropInput) (RestoredRef, error) {
	if !access.Can(actor.Role, access.ActionRestoreArchive) {
		return RestoredRef{}, access.ErrForbidden
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RestoredRef{}, fmt.Errorf("archive: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	txCtx := history.WithTx(ctx, tx)

	var entityType, snapshotText string
	var restored bool
	err = tx.QueryRow(txCtx,
		`SELECT entity_type, snapshot::text, restored_at IS NOT NULL FROM archive WHERE id = $1 FOR UPDATE`,
		archiveID).Scan(&entityType, &snapshotText, &restored)
	if errors.Is(err, pgx.ErrNoRows) {
		return RestoredRef{}, ErrNotFound
	}
	if err != nil {
		return RestoredRef{}, fmt.Errorf("archive: read: %w", err)
	}
	if restored {
		return RestoredRef{}, ErrAlreadyRestored
	}

	snapshot := []byte(snapshotText)
	var ref RestoredRef
	switch entityType {
	case "customer":
		ref, err = s.restoreCustomer(txCtx, tx, snapshot, reason)
	case "case":
		ref, err = s.restoreCase(txCtx, tx, snapshot, reason)
	default:
		err = fmt.Errorf("archive: unknown entity type %q", entityType)
	}
	if err != nil {
		return RestoredRef{}, err
	}

	if _, err := tx.Exec(txCtx,
		`UPDATE archive SET restored_at = now(), restored_by = $2 WHERE id = $1`, archiveID, actor.ID); err != nil {
		return RestoredRef{}, fmt.Errorf("archive: mark restored: %w", err)
	}
	actorID, id := actor.ID, ref.EntityID
	if err := s.rec.Record(txCtx, history.Entry{
		ActorUserID: &actorID, Action: "archive.restored", EntityType: ref.EntityType, EntityID: &id,
		Details: map[string]any{"archive_id": archiveID},
	}); err != nil {
		return RestoredRef{}, fmt.Errorf("archive: audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return RestoredRef{}, fmt.Errorf("archive: commit: %w", err)
	}
	return ref, nil
}

func (s *Service) restoreCustomer(ctx context.Context, tx pgx.Tx, snapshot []byte, reason DropInput) (RestoredRef, error) {
	var snap struct {
		Customer json.RawMessage   `json:"customer"`
		Leads    []json.RawMessage `json:"leads"`
	}
	if err := json.Unmarshal(snapshot, &snap); err != nil {
		return RestoredRef{}, fmt.Errorf("archive: customer snapshot: %w", err)
	}
	var head struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
		Reason *int64 `json:"dropped_reason_id"`
	}
	if err := json.Unmarshal(snap.Customer, &head); err != nil {
		return RestoredRef{}, fmt.Errorf("archive: customer json: %w", err)
	}

	_, err := tx.Exec(ctx,
		`INSERT INTO customers OVERRIDING SYSTEM VALUE
		 SELECT * FROM json_populate_record(NULL::customers, $1::json)`, string(snap.Customer))
	if isUniqueViolation(err) {
		return RestoredRef{}, ErrMobileInUse
	}
	if err != nil {
		return RestoredRef{}, fmt.Errorf("archive: insert customer: %w", err)
	}
	if head.Status != "dropped" || head.Reason == nil {
		reasonID, err := s.dropReasonFor(ctx, tx, reason, head.Reason)
		if err != nil {
			return RestoredRef{}, err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE customers SET status = 'dropped', dropped_reason_id = $2,
			 dropped_remark = $3, dropped_at = now(), dropped_by = NULL WHERE id = $1`,
			head.ID, reasonID, strings.TrimSpace(reason.Remark)); err != nil {
			return RestoredRef{}, fmt.Errorf("archive: drop restored customer: %w", err)
		}
	}
	if err := restoreLeads(ctx, tx, head.ID, snap.Leads); err != nil {
		return RestoredRef{}, err
	}
	return RestoredRef{EntityType: "customer", EntityID: head.ID}, nil
}

func (s *Service) restoreCase(ctx context.Context, tx pgx.Tx, snapshot []byte, reason DropInput) (RestoredRef, error) {
	var snap struct {
		Case     json.RawMessage `json:"case"`
		Children struct {
			StageEvents      json.RawMessage `json:"stage_events"`
			AssignmentEvents json.RawMessage `json:"assignment_events"`
			Documents        json.RawMessage `json:"documents"`
			Alerts           json.RawMessage `json:"alerts"`
			Payments         json.RawMessage `json:"payments"` // absent in snapshots before 2026-11-25
			Officers         json.RawMessage `json:"officers"` // absent in snapshots before 2026-11-26
		} `json:"children"`
	}
	if err := json.Unmarshal(snapshot, &snap); err != nil {
		return RestoredRef{}, fmt.Errorf("archive: case snapshot: %w", err)
	}
	if len(snap.Children.Payments) == 0 {
		snap.Children.Payments = json.RawMessage("[]")
	}
	if len(snap.Children.Officers) == 0 {
		snap.Children.Officers = json.RawMessage("[]")
	}
	var head struct {
		ID         int64  `json:"id"`
		CustomerID int64  `json:"customer_id"`
		Status     string `json:"status"`
		Reason     *int64 `json:"dropped_reason_id"`
	}
	if err := json.Unmarshal(snap.Case, &head); err != nil {
		return RestoredRef{}, fmt.Errorf("archive: case json: %w", err)
	}

	var customerLive bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM customers WHERE id = $1)`, head.CustomerID).Scan(&customerLive); err != nil {
		return RestoredRef{}, fmt.Errorf("archive: customer check: %w", err)
	}
	if !customerLive {
		return RestoredRef{}, ErrCustomerArchived
	}

	caseJSON, err := withDefaultProject(ctx, tx, snap.Case)
	if err != nil {
		return RestoredRef{}, err
	}
	if caseJSON, err = withUsableLead(ctx, tx, caseJSON); err != nil {
		return RestoredRef{}, err
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO cases OVERRIDING SYSTEM VALUE
		 SELECT * FROM json_populate_record(NULL::cases, $1::json)`, string(caseJSON))
	if isUniqueViolation(err) {
		return RestoredRef{}, ErrDuplicateTracking
	}
	if err != nil {
		return RestoredRef{}, fmt.Errorf("archive: insert case: %w", err)
	}

	if head.Status != "dropped" || head.Reason == nil {
		reasonID, err := s.dropReasonFor(ctx, tx, reason, head.Reason)
		if err != nil {
			return RestoredRef{}, err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE cases SET status = 'dropped', dropped_reason_id = $2, dropped_remark = $3,
			 dropped_at = now(), dropped_by = NULL, dropped_via_customer = false WHERE id = $1`,
			head.ID, reasonID, strings.TrimSpace(reason.Remark)); err != nil {
			return RestoredRef{}, fmt.Errorf("archive: drop restored case: %w", err)
		}
	}

	children := []struct {
		name  string
		query string
		doc   json.RawMessage
	}{
		{"stage events", `INSERT INTO case_stage_events OVERRIDING SYSTEM VALUE SELECT * FROM jsonb_populate_recordset(NULL::case_stage_events, $1::jsonb)`, snap.Children.StageEvents},
		{"assignment events", `INSERT INTO case_assignment_events OVERRIDING SYSTEM VALUE SELECT * FROM jsonb_populate_recordset(NULL::case_assignment_events, $1::jsonb)`, snap.Children.AssignmentEvents},
		{"documents", `INSERT INTO documents OVERRIDING SYSTEM VALUE SELECT * FROM jsonb_populate_recordset(NULL::documents, $1::jsonb)`, snap.Children.Documents},
		{"alerts", `INSERT INTO alert_log OVERRIDING SYSTEM VALUE SELECT * FROM jsonb_populate_recordset(NULL::alert_log, $1::jsonb)`, snap.Children.Alerts},
		{"payments", `INSERT INTO case_disbursements OVERRIDING SYSTEM VALUE SELECT * FROM jsonb_populate_recordset(NULL::case_disbursements, $1::jsonb)`, snap.Children.Payments},
		{"officers", `INSERT INTO case_officers OVERRIDING SYSTEM VALUE SELECT * FROM jsonb_populate_recordset(NULL::case_officers, $1::jsonb)`, snap.Children.Officers},
	}
	for _, c := range children {
		if _, err := tx.Exec(ctx, c.query, string(c.doc)); err != nil {
			return RestoredRef{}, fmt.Errorf("archive: restore %s: %w", c.name, err)
		}
	}
	return RestoredRef{EntityType: "case", EntityID: head.ID}, nil
}

// dropReasonFor returns the reason to store on a restored item: the snapshot's
// reason if it has one, else the admin's reason, which must be valid.
func (s *Service) dropReasonFor(ctx context.Context, tx pgx.Tx, in DropInput, fromSnapshot *int64) (int64, error) {
	if fromSnapshot != nil {
		return *fromSnapshot, nil
	}
	if in.ReasonID == 0 {
		return 0, ErrReasonRequired
	}
	var value string
	err := tx.QueryRow(ctx,
		`SELECT value FROM dropdown_values WHERE id = $1 AND category = 'drop_reason' AND is_active`, in.ReasonID).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrInvalidReason
	}
	if err != nil {
		return 0, fmt.Errorf("archive: reason: %w", err)
	}
	if value == "Other" && strings.TrimSpace(in.Remark) == "" {
		return 0, ErrRemarkRequired
	}
	return in.ReasonID, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// withDefaultProject sets project_id to the NA project when a case snapshot
// has none: cases archived before projects existed (spec 2026-11-21 §5).
func withDefaultProject(ctx context.Context, tx pgx.Tx, raw json.RawMessage) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("archive: case json: %w", err)
	}
	if v, ok := fields["project_id"]; ok && string(v) != "null" {
		return raw, nil
	}
	var na int64
	if err := tx.QueryRow(ctx, `SELECT id FROM projects WHERE is_system`).Scan(&na); err != nil {
		return nil, fmt.Errorf("archive: NA project: %w", err)
	}
	fields["project_id"] = json.RawMessage(fmt.Sprint(na))
	out, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("archive: case json: %w", err)
	}
	return out, nil
}

// restoreLeads puts a customer's archived leads and contact logs back. Every
// lead comes back dropped: an open or converted lead takes the restored
// customer's drop reason (spec 2026-11-23 §4.7). Old snapshots have no leads.
func restoreLeads(ctx context.Context, tx pgx.Tx, customerID int64, leadDocs []json.RawMessage) error {
	for _, doc := range leadDocs {
		var lead map[string]json.RawMessage
		if err := json.Unmarshal(doc, &lead); err != nil {
			return fmt.Errorf("archive: lead json: %w", err)
		}
		contacts := lead["contacts"]
		delete(lead, "contacts")
		row, err := json.Marshal(lead)
		if err != nil {
			return fmt.Errorf("archive: lead json: %w", err)
		}
		var id int64
		if err := tx.QueryRow(ctx,
			`INSERT INTO leads OVERRIDING SYSTEM VALUE
			 SELECT * FROM jsonb_populate_record(NULL::leads, $1::jsonb) RETURNING id`, string(row)).Scan(&id); err != nil {
			return fmt.Errorf("archive: insert lead: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE leads l SET status = 'dropped', next_call_date = NULL,
			   dropped_reason_id = c.dropped_reason_id, dropped_remark = c.dropped_remark, dropped_at = now(), dropped_by = NULL
			 FROM customers c WHERE l.id = $1 AND c.id = $2 AND l.status <> 'dropped'`, id, customerID); err != nil {
			return fmt.Errorf("archive: drop restored lead: %w", err)
		}
		if len(contacts) > 0 {
			if _, err := tx.Exec(ctx,
				`INSERT INTO lead_contacts OVERRIDING SYSTEM VALUE
				 SELECT * FROM jsonb_populate_recordset(NULL::lead_contacts, $1::jsonb)`, string(contacts)); err != nil {
				return fmt.Errorf("archive: restore lead contacts: %w", err)
			}
		}
	}
	return nil
}

// withUsableLead clears a restored case's lead_id unless that lead still
// exists and no other case uses it.
func withUsableLead(ctx context.Context, tx pgx.Tx, raw json.RawMessage) (json.RawMessage, error) {
	var row map[string]json.RawMessage
	if err := json.Unmarshal(raw, &row); err != nil {
		return nil, fmt.Errorf("archive: case json: %w", err)
	}
	var leadID *int64
	if v, ok := row["lead_id"]; ok {
		if err := json.Unmarshal(v, &leadID); err != nil {
			return nil, fmt.Errorf("archive: case lead: %w", err)
		}
	}
	if leadID == nil {
		return raw, nil
	}
	var usable bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM leads WHERE id = $1) AND NOT EXISTS (SELECT 1 FROM cases WHERE lead_id = $1)`,
		*leadID).Scan(&usable); err != nil {
		return nil, fmt.Errorf("archive: lead check: %w", err)
	}
	if usable {
		return raw, nil
	}
	row["lead_id"] = json.RawMessage("null")
	return json.Marshal(row)
}
