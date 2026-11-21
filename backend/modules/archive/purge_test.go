package archive

import (
	"context"
	"testing"
	"time"
)

// recordingRemover records deleted storage keys.
type recordingRemover struct{ deleted []string }

func (r *recordingRemover) Delete(key string) error {
	r.deleted = append(r.deleted, key)
	return nil
}

func TestPurgeRemovesExpiredArchiveRow(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.svc.DeleteCase(ctx, f.manager, f.caseID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE archive SET purge_after = now() - interval '1 day'`); err != nil {
		t.Fatalf("expire: %v", err)
	}
	res, err := f.svc.PurgeExpired(ctx, time.Now())
	if err != nil {
		t.Fatalf("PurgeExpired: %v", err)
	}
	if res.ArchiveRows != 1 {
		t.Fatalf("archive rows purged = %d, want 1", res.ArchiveRows)
	}
	if n := count(t, f.pool, `SELECT count(*) FROM archive`); n != 0 {
		t.Fatalf("archive rows left = %d, want 0", n)
	}
}

func TestPurgeKeepsAuditEntry(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.svc.DeleteCase(ctx, f.manager, f.caseID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE archive SET purge_after = now() - interval '1 day'`); err != nil {
		t.Fatalf("expire: %v", err)
	}
	if _, err := f.svc.PurgeExpired(ctx, time.Now()); err != nil {
		t.Fatalf("PurgeExpired: %v", err)
	}
	if n := count(t, f.pool, `SELECT count(*) FROM audit_log WHERE action='case.deleted' AND entity_id=$1`, f.caseID); n != 1 {
		t.Fatalf("audit entries after purge = %d, want 1", n)
	}
}

func TestPurgeKeepsRowsBeforeExpiry(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.svc.DeleteCase(ctx, f.manager, f.caseID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	res, err := f.svc.PurgeExpired(ctx, time.Now())
	if err != nil {
		t.Fatalf("PurgeExpired: %v", err)
	}
	if res.ArchiveRows != 0 || count(t, f.pool, `SELECT count(*) FROM archive`) != 1 {
		t.Fatalf("row before expiry was purged: %+v", res)
	}
}

// Documents of a dropped or disbursed case are purged one year after that date.
func TestPurgeRemovesDocumentsOneYearAfterDisbursedOrDropped(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	uploader := f.manager
	var stageID int64
	mustScanOne(t, f, `SELECT id FROM stages WHERE code='received' AND bank_id IS NULL`, &stageID)
	if _, err := f.pool.Exec(ctx, `INSERT INTO documents (case_id, stage_id, label, file_name, content_type, size_bytes, storage_key, purge_after, uploaded_by)
		VALUES ($1, $2, 'Old', 'old.pdf', 'application/pdf', 10, 'k/old', current_date - 1, $3)`,
		f.caseID, stageID, uploader.ID); err != nil {
		t.Fatalf("insert expired document: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO documents (case_id, stage_id, label, file_name, content_type, size_bytes, storage_key, purge_after, uploaded_by)
		VALUES ($1, $2, 'New', 'new.pdf', 'application/pdf', 10, 'k/new', current_date + 30, $3)`,
		f.caseID, stageID, uploader.ID); err != nil {
		t.Fatalf("insert current document: %v", err)
	}
	rem := &recordingRemover{}
	f.svc.SetFileRemover(rem)
	res, err := f.svc.PurgeExpired(ctx, time.Now())
	if err != nil {
		t.Fatalf("PurgeExpired: %v", err)
	}
	if res.Documents != 1 || len(rem.deleted) != 1 || rem.deleted[0] != "k/old" {
		t.Fatalf("documents purged=%d files=%v, want the expired file only", res.Documents, rem.deleted)
	}
	if n := count(t, f.pool, `SELECT count(*) FROM documents WHERE storage_key='k/new'`); n != 1 {
		t.Fatal("current document was purged")
	}
}
