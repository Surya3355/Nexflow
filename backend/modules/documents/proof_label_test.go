package documents

import (
	"context"
	"errors"
	"testing"
)

func TestNormalUploadCantUseQueryProofLabel(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for _, label := range []string{"Query 1 proof", "query 12  PROOF", " Query 3 proof 2 "} {
		_, err := f.svc.Upload(ctx, f.actor, f.caseID, label, UploadedFile{Name: "a.pdf", Content: samplePDF(100)})
		if !errors.Is(err, ErrReservedLabel) {
			t.Errorf("label %q: err = %v, want ErrReservedLabel", label, err)
		}
	}
	// Labels that only look similar are fine.
	for _, label := range []string{"Query letter", "Proof of address", "Query 1 proof copy"} {
		if _, err := f.svc.Upload(ctx, f.actor, f.caseID, label, UploadedFile{Name: "a.pdf", Content: samplePDF(100)}); err != nil {
			t.Errorf("label %q refused: %v", label, err)
		}
	}
}

func TestNormalUploadCantReplaceProof(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := f.svc.StoreFile(f.caseID, UploadedFile{Name: "proof.pdf", Content: samplePDF(100)})
	if err != nil {
		t.Fatal(err)
	}
	proof, err := f.svc.InsertTx(ctx, tx, f.actor, f.caseID, "Query 1 proof", stored)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Upload(ctx, f.actor, f.caseID, "Query 1 proof", UploadedFile{Name: "other.pdf", Content: samplePDF(120)}); !errors.Is(err, ErrReservedLabel) {
		t.Fatalf("overwrite proof: err = %v, want ErrReservedLabel", err)
	}
	var name string
	mustScan(t, f.pool.QueryRow(ctx, `SELECT file_name FROM documents WHERE id = $1`, proof.ID), &name)
	if name != "proof.pdf" {
		t.Fatalf("proof file now %q, want proof.pdf", name)
	}
}
