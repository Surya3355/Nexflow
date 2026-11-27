package archive

import (
	"context"
	"testing"

	"github.com/Surya3355/nexflow/backend/modules/access"
)

func TestArchiveRestoreKeepsOfficers(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var officer int64
	if err := f.pool.QueryRow(ctx, `INSERT INTO bank_officers (bank_id, team, name, phone)
		SELECT bank_id, 'officer', 'Sita', '9876543210' FROM cases WHERE id = $1 RETURNING id`, f.caseID).Scan(&officer); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO case_officers (case_id, team, officer_id, picked_by) VALUES ($1, 'officer', $2, $3)`,
		f.caseID, officer, f.manager.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteCase(ctx, f.manager, f.caseID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	var n int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM case_officers WHERE case_id = $1`, f.caseID).Scan(&n)
	if n != 0 {
		t.Fatalf("officer picks left after archive = %d", n)
	}
	admin := seedUser(t, f.pool, "root", access.RoleAdmin)
	if _, err := f.svc.Restore(ctx, admin, archiveID(t, f, "case", f.caseID), reason(t, f, "Not eligible")); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	var got int64
	if err := f.pool.QueryRow(ctx, `SELECT officer_id FROM case_officers WHERE case_id = $1`, f.caseID).Scan(&got); err != nil || got != officer {
		t.Fatalf("restored pick = %d %v, want %d", got, err, officer)
	}
}

func TestRestoreOldSnapshotWithoutOfficers(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.svc.DeleteCase(ctx, f.manager, f.caseID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE archive SET snapshot = jsonb_set(snapshot, '{children}', (snapshot->'children') - 'officers') WHERE entity_type='case'`); err != nil {
		t.Fatal(err)
	}
	admin := seedUser(t, f.pool, "root", access.RoleAdmin)
	if _, err := f.svc.Restore(ctx, admin, archiveID(t, f, "case", f.caseID), reason(t, f, "Not eligible")); err != nil {
		t.Fatalf("restore old snapshot: %v", err)
	}
}
