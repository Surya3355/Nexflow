package archive

import (
	"context"
	"errors"
	"testing"

	"github.com/Surya3355/nexflow/backend/modules/access"
)

func archiveID(t *testing.T, f fixture, entityType string, id int64) int64 {
	t.Helper()
	var aid int64
	if err := f.pool.QueryRow(context.Background(),
		`SELECT id FROM archive WHERE entity_type=$1 AND entity_id=$2 AND restored_at IS NULL`, entityType, id).Scan(&aid); err != nil {
		t.Fatalf("archive row for %s %d: %v", entityType, id, err)
	}
	return aid
}

func reason(t *testing.T, f fixture, value string) DropInput {
	t.Helper()
	var id int64
	if err := f.pool.QueryRow(context.Background(),
		`SELECT id FROM dropdown_values WHERE category='drop_reason' AND value=$1`, value).Scan(&id); err != nil {
		t.Fatalf("reason %s: %v", value, err)
	}
	return DropInput{ReasonID: id}
}

func TestOnlyAdminCanRestore(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.svc.DeleteCase(ctx, f.manager, f.caseID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	aid := archiveID(t, f, "case", f.caseID)
	admin := seedUser(t, f.pool, "root", access.RoleAdmin)

	if _, err := f.svc.Restore(ctx, f.manager, aid, reason(t, f, "Not eligible")); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("manager restore err = %v, want ErrForbidden", err)
	}
	if _, err := f.svc.Restore(ctx, f.employee, aid, reason(t, f, "Not eligible")); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("employee restore err = %v, want ErrForbidden", err)
	}
	if _, err := f.svc.Restore(ctx, admin, aid, reason(t, f, "Not eligible")); err != nil {
		t.Fatalf("admin restore: %v", err)
	}
}

func TestRestoredItemComesBackDropped(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	admin := seedUser(t, f.pool, "root", access.RoleAdmin)
	if err := f.svc.DeleteCase(ctx, f.manager, f.caseID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	ref, err := f.svc.Restore(ctx, admin, archiveID(t, f, "case", f.caseID), reason(t, f, "Not eligible"))
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if ref.EntityType != "case" || ref.EntityID != f.caseID {
		t.Fatalf("ref = %+v", ref)
	}
	var status string
	var reasonSet bool
	if err := f.pool.QueryRow(ctx,
		`SELECT status, dropped_reason_id IS NOT NULL FROM cases WHERE id=$1`, f.caseID).Scan(&status, &reasonSet); err != nil {
		t.Fatalf("read restored case: %v", err)
	}
	if status != "dropped" || !reasonSet {
		t.Fatalf("restored case status=%s reason_set=%v, want dropped with a reason", status, reasonSet)
	}
}

func TestRestoreMarksArchiveRowRestored(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	admin := seedUser(t, f.pool, "root", access.RoleAdmin)
	if err := f.svc.DeleteCase(ctx, f.manager, f.caseID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	aid := archiveID(t, f, "case", f.caseID)
	if _, err := f.svc.Restore(ctx, admin, aid, reason(t, f, "Not eligible")); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	var restoredBy int64
	var restoredAt bool
	if err := f.pool.QueryRow(ctx,
		`SELECT restored_by, restored_at IS NOT NULL FROM archive WHERE id=$1`, aid).Scan(&restoredBy, &restoredAt); err != nil {
		t.Fatalf("read archive: %v", err)
	}
	if restoredBy != admin.ID || !restoredAt {
		t.Fatalf("restored_by=%d restored_at_set=%v", restoredBy, restoredAt)
	}
}

// Owner answer B: restore is refused when the mobile is now in use.
func TestRestoredMobileClashIsRefused(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	admin := seedUser(t, f.pool, "root", access.RoleAdmin)
	if err := f.svc.DeleteCase(ctx, f.manager, f.caseID); err != nil {
		t.Fatalf("archive case: %v", err)
	}
	if err := f.svc.DeleteCustomer(ctx, f.manager, f.customer); err != nil {
		t.Fatalf("archive customer: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO customers (name, mobile) VALUES ('New', '9876543210')`); err != nil {
		t.Fatalf("new customer with same mobile: %v", err)
	}
	_, err := f.svc.Restore(ctx, admin, archiveID(t, f, "customer", f.customer), reason(t, f, "Not eligible"))
	if !errors.Is(err, ErrMobileInUse) {
		t.Fatalf("err = %v, want ErrMobileInUse", err)
	}
}

func TestTrackingNumberClashOnRestoreIsRefused(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	admin := seedUser(t, f.pool, "root", access.RoleAdmin)
	if _, err := f.pool.Exec(ctx, `UPDATE cases SET tracking_number='CLASH-1' WHERE id=$1`, f.caseID); err != nil {
		t.Fatalf("set tracking: %v", err)
	}
	if err := f.svc.DeleteCase(ctx, f.manager, f.caseID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	var bankID, stageID int64
	mustScanOne(t, f, `SELECT id FROM banks LIMIT 1`, &bankID)
	mustScanOne(t, f, `SELECT id FROM stages WHERE code='received' AND bank_id IS NULL`, &stageID)
	if _, err := f.pool.Exec(ctx, `INSERT INTO cases (customer_id, bank_id, loan_amount, current_stage_id, assignee_id, tracking_number)
		VALUES ($1, $2, 1000, $3, $4, 'CLASH-1')`, f.customer, bankID, stageID, f.employee.ID); err != nil {
		t.Fatalf("live case with same tracking: %v", err)
	}
	_, err := f.svc.Restore(ctx, admin, archiveID(t, f, "case", f.caseID), reason(t, f, "Not eligible"))
	if !errors.Is(err, ErrDuplicateTracking) {
		t.Fatalf("err = %v, want ErrDuplicateTracking", err)
	}
}

// Completes plan Task 13: an archived case does not block its tracking number.
func TestArchivedCaseDoesNotBlockTrackingNumber(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE cases SET tracking_number='REUSE-1' WHERE id=$1`, f.caseID); err != nil {
		t.Fatalf("set tracking: %v", err)
	}
	if err := f.svc.DeleteCase(ctx, f.manager, f.caseID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	var bankID, stageID int64
	mustScanOne(t, f, `SELECT id FROM banks LIMIT 1`, &bankID)
	mustScanOne(t, f, `SELECT id FROM stages WHERE code='received' AND bank_id IS NULL`, &stageID)
	if _, err := f.pool.Exec(ctx, `INSERT INTO cases (customer_id, bank_id, loan_amount, current_stage_id, assignee_id, tracking_number)
		VALUES ($1, $2, 1000, $3, $4, 'REUSE-1')`, f.customer, bankID, stageID, f.employee.ID); err != nil {
		t.Fatalf("archived case blocked the tracking number: %v", err)
	}
}

func TestRestoreCaseRefusedWhileCustomerArchived(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	admin := seedUser(t, f.pool, "root", access.RoleAdmin)
	if err := f.svc.DeleteCase(ctx, f.manager, f.caseID); err != nil {
		t.Fatalf("archive case: %v", err)
	}
	if err := f.svc.DeleteCustomer(ctx, f.manager, f.customer); err != nil {
		t.Fatalf("archive customer: %v", err)
	}
	_, err := f.svc.Restore(ctx, admin, archiveID(t, f, "case", f.caseID), reason(t, f, "Not eligible"))
	if !errors.Is(err, ErrCustomerArchived) {
		t.Fatalf("err = %v, want ErrCustomerArchived", err)
	}
}

func mustScanOne(t *testing.T, f fixture, query string, dest ...any) {
	t.Helper()
	if err := f.pool.QueryRow(context.Background(), query).Scan(dest...); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
}

// A case archived before projects existed has no project_id in its snapshot.
func TestRestoreOldCaseGetsNA(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	admin := seedUser(t, f.pool, "root", access.RoleAdmin)
	if err := f.svc.DeleteCase(ctx, f.manager, f.caseID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	id := archiveID(t, f, "case", f.caseID)
	if _, err := f.pool.Exec(ctx, `UPDATE archive SET snapshot = snapshot #- '{case,project_id}' WHERE id = $1`, id); err != nil {
		t.Fatalf("strip project: %v", err)
	}
	if _, err := f.svc.Restore(ctx, admin, id, reason(t, f, "Not eligible")); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	var system bool
	if err := f.pool.QueryRow(ctx, `SELECT p.is_system FROM cases c JOIN projects p ON p.id = c.project_id WHERE c.id = $1`, f.caseID).Scan(&system); err != nil || !system {
		t.Fatalf("restored case project is NA = %v (%v), want true", system, err)
	}
}
