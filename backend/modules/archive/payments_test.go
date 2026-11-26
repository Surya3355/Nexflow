package archive

import (
	"context"
	"testing"

	"github.com/Surya3355/nexflow/backend/modules/access"
)

func addPayment(t *testing.T, f fixture, kind string) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO case_disbursements (case_id, amount, paid_on, kind)
		VALUES ($1, 250000, '2026-11-05', $2)`, f.caseID, kind); err != nil {
		t.Fatal(err)
	}
}

func TestArchiveRestoreKeepsPayments(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	addPayment(t, f, "part")
	if err := f.svc.DeleteCase(ctx, f.manager, f.caseID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	var n int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM case_disbursements WHERE case_id = $1`, f.caseID).Scan(&n)
	if n != 0 {
		t.Fatalf("payments left after archive = %d", n)
	}
	admin := seedUser(t, f.pool, "root", access.RoleAdmin)
	if _, err := f.svc.Restore(ctx, admin, archiveID(t, f, "case", f.caseID), reason(t, f, "Not eligible")); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	var amount, kind string
	if err := f.pool.QueryRow(ctx, `SELECT amount::text, kind FROM case_disbursements WHERE case_id = $1`, f.caseID).Scan(&amount, &kind); err != nil {
		t.Fatalf("restored payment: %v", err)
	}
	if amount != "250000.00" || kind != "part" {
		t.Fatalf("restored payment = %s %s", amount, kind)
	}
}

func TestRestoreOldSnapshotWithoutPayments(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.svc.DeleteCase(ctx, f.manager, f.caseID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE archive SET snapshot = jsonb_set(snapshot, '{children}', (snapshot->'children') - 'payments') WHERE entity_type='case'`); err != nil {
		t.Fatal(err)
	}
	admin := seedUser(t, f.pool, "root", access.RoleAdmin)
	if _, err := f.svc.Restore(ctx, admin, archiveID(t, f, "case", f.caseID), reason(t, f, "Not eligible")); err != nil {
		t.Fatalf("restore old snapshot: %v", err)
	}
}
