package documents

import (
	"context"
	"testing"

	"github.com/Surya3355/nexflow/backend/modules/cases"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/Surya3355/nexflow/backend/modules/workflows"
)

func setRequiresDocument(t *testing.T, f fixture, code string, required bool) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(),
		`UPDATE stages SET requires_document = $2 WHERE code = $1 AND bank_id IS NULL`, code, required); err != nil {
		t.Fatalf("set requires_document: %v", err)
	}
}

func TestStageNotRequiringDocumentsHasNoWarning(t *testing.T) {
	f := newFixture(t)
	st, err := f.svc.RequirementStatus(context.Background(), f.caseID)
	if err != nil {
		t.Fatalf("RequirementStatus: %v", err)
	}
	if st.Required || st.Warning != "" {
		t.Fatalf("status = %+v, want no requirement and no warning", st)
	}
}

func TestRequiredStageWithoutDocumentWarns(t *testing.T) {
	f := newFixture(t)
	setRequiresDocument(t, f, "received", true)
	st, err := f.svc.RequirementStatus(context.Background(), f.caseID)
	if err != nil {
		t.Fatalf("RequirementStatus: %v", err)
	}
	if !st.Required || st.Satisfied || st.Warning == "" {
		t.Fatalf("status = %+v, want required, not satisfied, with a warning", st)
	}
}

func TestRequiredStageWithDocumentIsSatisfied(t *testing.T) {
	f := newFixture(t)
	setRequiresDocument(t, f, "received", true)
	if _, err := f.svc.Upload(context.Background(), f.actor, f.caseID, "Form", UploadedFile{Name: "f.pdf", Content: samplePDF(100)}); err != nil {
		t.Fatalf("upload: %v", err)
	}
	st, err := f.svc.RequirementStatus(context.Background(), f.caseID)
	if err != nil {
		t.Fatalf("RequirementStatus: %v", err)
	}
	if !st.Required || !st.Satisfied || st.Warning != "" {
		t.Fatalf("status = %+v, want required and satisfied", st)
	}
}

// Spec §4.4 and round 11: uploads are optional at every stage, and a missing
// file never blocks a stage move. Covers plan Task 22's optional-upload test
// and Task 24's non-blocking test.
func TestMissingDocumentDoesNotBlockStageMove(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	setRequiresDocument(t, f, "under_sanction", true)
	casesSvc := cases.NewService(f.pool, workflows.NewService(f.pool, history.Nop{}), history.Nop{})

	var loggedIn, underSanction int64
	mustScan(t, f.pool.QueryRow(ctx, `SELECT id FROM stages WHERE code='logged_in' AND bank_id IS NULL`), &loggedIn)
	mustScan(t, f.pool.QueryRow(ctx, `SELECT id FROM stages WHERE code='under_sanction' AND bank_id IS NULL`), &underSanction)
	tn := "NODOC1"
	if _, err := casesSvc.ChangeStage(ctx, f.actor, f.caseID, cases.StageMove{ToStageID: loggedIn, TrackingNumber: &tn}); err != nil {
		t.Fatalf("to logged_in: %v", err)
	}
	if _, err := casesSvc.ChangeStage(ctx, f.actor, f.caseID, cases.StageMove{ToStageID: underSanction}); err != nil {
		t.Fatalf("missing document blocked the move: %v", err)
	}
	st, _ := f.svc.RequirementStatus(ctx, f.caseID)
	if !st.Required || st.Satisfied || st.Warning == "" {
		t.Fatalf("after move status = %+v, want a warning, not a block", st)
	}
}
