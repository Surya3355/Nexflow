package documents

import (
	"context"
	"testing"

	"github.com/Surya3355/nexflow/backend/internal/platform/testdb"
	"github.com/Surya3355/nexflow/backend/migrations"
	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	svc     *Service
	pool    *pgxpool.Pool
	store   *MemoryStore
	actor   access.User
	caseID  int64
	stageID int64
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	pool := testdb.New(t)
	if err := migrations.Up(pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	var f fixture
	f.store = NewMemoryStore()
	f.svc = NewService(pool, f.store, history.NewPgRecorder(pool))
	f.pool = pool
	f.actor = access.User{Username: "staff", Role: access.RoleEmployee, IsActive: true}
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO users (username, full_name, password_hash, role)
		VALUES ('staff', 'Staff', 'x', 'employee') RETURNING id`), &f.actor.ID)
	var bankID, customerID int64
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO banks (name) VALUES ('HDFC') RETURNING id`), &bankID)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO customers (name, mobile) VALUES ('Ravi', '9876543210') RETURNING id`), &customerID)
	mustScan(t, pool.QueryRow(ctx, `SELECT id FROM stages WHERE code='received' AND bank_id IS NULL`), &f.stageID)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO cases (customer_id, bank_id, loan_amount, current_stage_id, assignee_id)
		VALUES ($1, $2, 100000, $3, $4) RETURNING id`, customerID, bankID, f.stageID, f.actor.ID), &f.caseID)
	return f
}

func mustScan(t *testing.T, row interface{ Scan(...any) error }, dest ...any) {
	t.Helper()
	if err := row.Scan(dest...); err != nil {
		t.Fatalf("scan: %v", err)
	}
}

func TestUploadRecordsStageAtUpload(t *testing.T) {
	f := newFixture(t)
	doc, err := f.svc.Upload(context.Background(), f.actor, f.caseID, "PAN card", UploadedFile{Name: "pan.pdf", Content: samplePDF(200)})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if doc.StageID != f.stageID {
		t.Fatalf("stage_id = %d, want case's current stage %d", doc.StageID, f.stageID)
	}
}

func TestSameLabelReplacesFileAndDeletesOldObject(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	first, err := f.svc.Upload(ctx, f.actor, f.caseID, "Bank statement", UploadedFile{Name: "a.pdf", Content: samplePDF(100)})
	if err != nil {
		t.Fatalf("first upload: %v", err)
	}
	second, err := f.svc.Upload(ctx, f.actor, f.caseID, "Bank statement", UploadedFile{Name: "b.pdf", Content: samplePDF(150)})
	if err != nil {
		t.Fatalf("second upload: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("same label created a second row: %d vs %d", second.ID, first.ID)
	}
	if n := count(t, f.pool, `SELECT count(*) FROM documents WHERE case_id=$1`, f.caseID); n != 1 {
		t.Fatalf("document rows = %d, want 1 (latest version only)", n)
	}
	if f.store.Has(first.StorageKey) {
		t.Fatal("old stored file was not deleted")
	}
	if !f.store.Has(second.StorageKey) {
		t.Fatal("new stored file is missing")
	}
}

func TestLabelIsFreeText(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.Upload(context.Background(), f.actor, f.caseID, "Anything at all: ITR 2025-26 (copy)", UploadedFile{Name: "x.png", Content: samplePNG(120)}); err != nil {
		t.Fatalf("free-text label refused: %v", err)
	}
}

func TestEmployeeCanUpload(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.Upload(context.Background(), f.actor, f.caseID, "Salary slip", UploadedFile{Name: "s.jpg", Content: sampleJPEG(120)}); err != nil {
		t.Fatalf("employee upload: %v", err)
	}
}

func count(t *testing.T, pool *pgxpool.Pool, q string, args ...any) int {
	t.Helper()
	var n int
	mustScan(t, pool.QueryRow(context.Background(), q, args...), &n)
	return n
}
