package documents

import (
	"context"
	"errors"
	"testing"
)

func TestStoreFileRejectsBadType(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.StoreFile(f.caseID, UploadedFile{Name: "x.exe", Content: []byte("MZ not a pdf")}); err == nil {
		t.Fatal("StoreFile accepted a bad file")
	}
}

func TestInsertTxPicksFreeLabel(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	// The first proof takes the plain label; it goes in through the proof path,
	// since normal uploads can't use proof labels.
	seed, err := f.svc.StoreFile(f.caseID, UploadedFile{Name: "a.pdf", Content: samplePDF(100)})
	if err != nil {
		t.Fatal(err)
	}
	seedTx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first, err := f.svc.InsertTx(ctx, seedTx, f.actor, f.caseID, "Query 1 proof", seed)
	if err != nil {
		t.Fatalf("seed proof: %v", err)
	}
	if err := seedTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	st, err := f.svc.StoreFile(f.caseID, UploadedFile{Name: "b.pdf", Content: samplePDF(120)})
	if err != nil {
		t.Fatalf("StoreFile: %v", err)
	}
	if !f.store.Has(st.Key) {
		t.Fatal("stored file not in store")
	}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := f.svc.InsertTx(ctx, tx, f.actor, f.caseID, "Query 1 proof", st)
	if err != nil {
		t.Fatalf("InsertTx: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if doc.Label != "Query 1 proof 2" || doc.FileName != "b.pdf" || doc.StageID != f.stageID {
		t.Fatalf("doc = %+v, want label 'Query 1 proof 2', b.pdf at current stage", doc)
	}
	again, _, err := f.svc.Download(ctx, first.ID)
	if err != nil || again.FileName != "a.pdf" {
		t.Fatalf("first document changed: %+v %v", again, err)
	}
}

func TestInsertTxRollbackLeavesNoRow(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	st, err := f.svc.StoreFile(f.caseID, UploadedFile{Name: "b.pdf", Content: samplePDF(120)})
	if err != nil {
		t.Fatal(err)
	}
	tx, _ := f.pool.Begin(ctx)
	if _, err := f.svc.InsertTx(ctx, tx, f.actor, f.caseID, "Query 1 proof", st); err != nil {
		t.Fatalf("InsertTx: %v", err)
	}
	_ = tx.Rollback(ctx)
	f.svc.DiscardFile(st.Key)
	docs, _ := f.svc.ListForCase(ctx, f.caseID)
	if len(docs) != 0 || f.store.Has(st.Key) {
		t.Fatalf("after rollback docs=%d stored=%v, want none", len(docs), f.store.Has(st.Key))
	}
}

func TestDeleteTxRemovesRowAndAudits(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	doc, err := f.svc.Upload(ctx, f.actor, f.caseID, "Bank statement", UploadedFile{Name: "a.pdf", Content: samplePDF(100)})
	if err != nil {
		t.Fatal(err)
	}
	tx, _ := f.pool.Begin(ctx)
	key, err := f.svc.DeleteTx(ctx, tx, f.actor, doc.ID)
	if err != nil {
		t.Fatalf("DeleteTx: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if key != doc.StorageKey {
		t.Fatalf("key = %q, want %q", key, doc.StorageKey)
	}
	if _, _, err := f.svc.Download(ctx, doc.ID); !errors.Is(err, ErrNotFoundDoc) {
		t.Fatalf("Download after delete err = %v, want ErrNotFoundDoc", err)
	}
	var n int
	mustScan(t, f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action = 'document.removed' AND entity_id = $1`, doc.ID), &n)
	if n != 1 {
		t.Fatalf("audit rows = %d, want 1", n)
	}
}
