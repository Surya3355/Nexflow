package archive

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestDeleteWaitsForConcurrentUpload: an upload that holds the case lock
// when the delete starts must end up in the archive copy, not be lost.
func TestDeleteWaitsForConcurrentUpload(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var stageID int64
	mustScanOne(t, f, `SELECT id FROM stages WHERE code='received' AND bank_id IS NULL`, &stageID)

	upload, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := upload.Exec(ctx, `SELECT id FROM cases WHERE id = $1 FOR UPDATE`, f.caseID); err != nil {
		t.Fatal(err)
	}
	if _, err := upload.Exec(ctx, `INSERT INTO documents (case_id, stage_id, label, file_name, content_type, size_bytes, storage_key, uploaded_by)
		VALUES ($1, $2, 'Late upload', 'late.pdf', 'application/pdf', 10, 'k/late', $3)`, f.caseID, stageID, f.employee.ID); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- f.svc.DeleteCase(ctx, f.manager, f.caseID) }()
	time.Sleep(300 * time.Millisecond)
	if err := upload.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("DeleteCase: %v", err)
	}
	var snap string
	mustScanOne(t, f, `SELECT snapshot::text FROM archive WHERE entity_type = 'case'`, &snap)
	if !strings.Contains(snap, "Late upload") {
		t.Fatal("archive copy is missing the document uploaded while the delete waited")
	}
}

// committedCheck is a file remover that looks, from another connection,
// whether the document row is already gone when the file is deleted.
type committedCheck struct {
	pool    *pgxpool.Pool
	key     string
	visible int
}

func (c *committedCheck) Delete(key string) error {
	if key == c.key {
		_ = c.pool.QueryRow(context.Background(), `SELECT count(*) FROM documents WHERE storage_key = $1`, key).Scan(&c.visible)
	}
	return nil
}

func TestPurgeDeletesFilesAfterCommit(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var stageID int64
	mustScanOne(t, f, `SELECT id FROM stages WHERE code='received' AND bank_id IS NULL`, &stageID)
	if _, err := f.pool.Exec(ctx, `INSERT INTO documents (case_id, stage_id, label, file_name, content_type, size_bytes, storage_key, purge_after, uploaded_by)
		VALUES ($1, $2, 'Old', 'old.pdf', 'application/pdf', 10, 'k/old', current_date - 1, $3)`, f.caseID, stageID, f.manager.ID); err != nil {
		t.Fatal(err)
	}
	rem := &committedCheck{pool: f.pool, key: "k/old", visible: -1}
	f.svc.SetFileRemover(rem)
	if _, err := f.svc.PurgeExpired(ctx, time.Now()); err != nil {
		t.Fatalf("PurgeExpired: %v", err)
	}
	if rem.visible != 0 {
		t.Fatalf("file deleted while its row was still saved (rows visible = %d); want the delete after commit", rem.visible)
	}
}
