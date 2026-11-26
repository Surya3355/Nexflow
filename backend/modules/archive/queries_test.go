package archive

import (
	"context"
	"testing"

	"github.com/Surya3355/nexflow/backend/modules/access"
)

func TestArchiveRestoreKeepsBankQueries(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var doc, q int64
	if err := f.pool.QueryRow(ctx, `INSERT INTO documents (case_id, stage_id, label, file_name, content_type, size_bytes, storage_key, uploaded_by)
		SELECT id, current_stage_id, 'Query 1 proof', 'p.pdf', 'application/pdf', 10, 'k-q1', $2 FROM cases WHERE id = $1 RETURNING id`,
		f.caseID, f.manager.ID).Scan(&doc); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO case_queries (case_id, number, body, raised_on, assignee_id, status, created_by, updated_by)
		VALUES ($1, 1, 'NOC', CURRENT_DATE - 1, $2, 'sent', $2, $2) RETURNING id`, f.caseID, f.manager.ID).Scan(&q); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO case_query_events (query_id, kind, note, document_id, by_user) VALUES ($1, 'sent', 'sent it', $2, $3)`,
		q, doc, f.manager.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteCase(ctx, f.manager, f.caseID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	var n int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM case_queries WHERE case_id = $1`, f.caseID).Scan(&n)
	if n != 0 {
		t.Fatalf("queries left after archive = %d", n)
	}
	admin := seedUser(t, f.pool, "root", access.RoleAdmin)
	if _, err := f.svc.Restore(ctx, admin, archiveID(t, f, "case", f.caseID), reason(t, f, "Not eligible")); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	var status, note string
	var gotDoc int64
	if err := f.pool.QueryRow(ctx, `SELECT q.status, e.note, e.document_id FROM case_queries q JOIN case_query_events e ON e.query_id = q.id WHERE q.id = $1`, q).
		Scan(&status, &note, &gotDoc); err != nil {
		t.Fatalf("restored query: %v", err)
	}
	if status != "sent" || note != "sent it" || gotDoc != doc {
		t.Fatalf("restored = %s %q %d, want sent, 'sent it', %d", status, note, gotDoc, doc)
	}
}

func TestRestoreOldSnapshotWithoutQueries(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.svc.DeleteCase(ctx, f.manager, f.caseID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE archive SET snapshot = jsonb_set(snapshot, '{children}', (snapshot->'children') - 'queries' - 'query_events') WHERE entity_type='case'`); err != nil {
		t.Fatal(err)
	}
	admin := seedUser(t, f.pool, "root", access.RoleAdmin)
	if _, err := f.svc.Restore(ctx, admin, archiveID(t, f, "case", f.caseID), reason(t, f, "Not eligible")); err != nil {
		t.Fatalf("restore old snapshot: %v", err)
	}
}
