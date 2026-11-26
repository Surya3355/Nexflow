package bankqueries

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Surya3355/nexflow/backend/modules/documents"
	"github.com/Surya3355/nexflow/backend/modules/history"
)

func pdf() *documents.UploadedFile {
	b := append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("0"), 200)...)
	return &documents.UploadedFile{Name: "proof.pdf", Content: b}
}

// failOn records through the real recorder but fails for one action.
type failOn struct {
	inner  history.Recorder
	action string
}

func (r failOn) Record(ctx context.Context, e history.Entry) error {
	if e.Action == r.action {
		return errors.New("audit down")
	}
	return r.inner.Record(ctx, e)
}

func docCount(t *testing.T, f fixture) int {
	t.Helper()
	docs, err := f.docs.ListForCase(context.Background(), f.caseID)
	if err != nil {
		t.Fatal(err)
	}
	return len(docs)
}

func TestSendNeedsNoteOrFile(t *testing.T) {
	f := setup(t, nil)
	ctx := context.Background()
	q := f.add(t, "Salary slips")
	if _, err := f.svc.Send(ctx, f.staff, q.ID, "  ", nil); !errors.Is(err, ErrProofRequired) {
		t.Fatalf("neither err = %v, want ErrProofRequired", err)
	}
	if _, err := f.svc.Send(ctx, f.staff, q.ID, strings.Repeat("x", 501), nil); !errors.Is(err, ErrNoteTooLong) {
		t.Fatalf("long note err = %v", err)
	}
	got, err := f.svc.Send(ctx, f.staff, q.ID, "Uploaded on portal", nil)
	if err != nil {
		t.Fatalf("note only: %v", err)
	}
	if got.Status != StatusSent || got.Proof == nil || got.Proof.Note != "Uploaded on portal" || got.Proof.DocumentID != nil {
		t.Fatalf("after note-only send = %+v proof %+v", got, got.Proof)
	}
	if _, err := f.svc.Send(ctx, f.staff, q.ID, "again", nil); !errors.Is(err, ErrAlreadySent) {
		t.Fatalf("send twice err = %v, want ErrAlreadySent", err)
	}
}

func TestSendWithFileAddsCaseDocument(t *testing.T) {
	f := setup(t, nil)
	ctx := context.Background()
	q := f.add(t, "NOC")
	got, err := f.svc.Send(ctx, f.staff, q.ID, "", pdf())
	if err != nil {
		t.Fatalf("file only: %v", err)
	}
	if got.Proof == nil || got.Proof.DocumentID == nil || got.Proof.DocumentLabel != "Query 1 proof" || got.Proof.FileName != "proof.pdf" {
		t.Fatalf("proof = %+v", got.Proof)
	}
	if docCount(t, f) != 1 {
		t.Fatalf("documents = %d, want 1", docCount(t, f))
	}
	// Rejected, then sent again with note and file: the old file stays, the new one is "proof 2".
	if _, err := f.svc.Reject(ctx, f.staff, q.ID, "  "); !errors.Is(err, ErrReasonRequired) {
		t.Fatalf("reject without reason err = %v", err)
	}
	back, err := f.svc.Reject(ctx, f.staff, q.ID, "Needs letterhead")
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if back.Status != StatusOpen || back.Proof != nil || back.Events[len(back.Events)-1].Note != "Needs letterhead" {
		t.Fatalf("after reject = %+v", back)
	}
	again, err := f.svc.Send(ctx, f.staff, q.ID, "On letterhead now", pdf())
	if err != nil {
		t.Fatalf("resend: %v", err)
	}
	if again.Proof.DocumentLabel != "Query 1 proof 2" || again.Proof.Note != "On letterhead now" || docCount(t, f) != 2 {
		t.Fatalf("resend proof = %+v, docs %d", again.Proof, docCount(t, f))
	}
}

func TestSendDiscardsFileWhenAuditFails(t *testing.T) {
	f := setup(t, nil)
	f.svc.rec = failOn{inner: history.NewPgRecorder(f.pool), action: "query.sent"}
	ctx := context.Background()
	q := f.add(t, "NOC")
	if _, err := f.svc.Send(ctx, f.staff, q.ID, "x", pdf()); err == nil {
		t.Fatal("Send succeeded with a failing audit")
	}
	if docCount(t, f) != 0 || f.store.live() != 0 {
		t.Fatalf("after failed send docs=%d files=%d, want 0 and 0", docCount(t, f), f.store.live())
	}
	list, _ := f.svc.List(ctx, f.caseID)
	if list[0].Status != StatusOpen {
		t.Fatalf("status = %s, want open", list[0].Status)
	}
}

func TestAcceptOnlyAfterSent(t *testing.T) {
	f := setup(t, nil)
	ctx := context.Background()
	q := f.add(t, "PAN")
	if _, err := f.svc.Accept(ctx, f.staff, q.ID); !errors.Is(err, ErrNotSent) {
		t.Fatalf("accept open err = %v, want ErrNotSent", err)
	}
	if _, err := f.svc.Reject(ctx, f.staff, q.ID, "no"); !errors.Is(err, ErrNotSent) {
		t.Fatalf("reject open err = %v, want ErrNotSent", err)
	}
	if _, err := f.svc.Send(ctx, f.staff, q.ID, "sent", nil); err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.Accept(ctx, f.staff, q.ID)
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if got.Status != StatusResolved || got.Proof != nil || got.Events[len(got.Events)-1].Kind != "accepted" {
		t.Fatalf("after accept = %+v", got)
	}
	if _, err := f.svc.Remove(ctx, f.staff, q.ID); !errors.Is(err, ErrQueryResolved) {
		t.Fatalf("remove resolved err = %v", err)
	}
	if _, err := f.svc.RemoveProof(ctx, f.staff, q.ID); !errors.Is(err, ErrQueryResolved) {
		t.Fatalf("remove proof of resolved err = %v", err)
	}
}

func TestRemoveProofDeletesFileAndReopens(t *testing.T) {
	f := setup(t, nil)
	ctx := context.Background()
	q := f.add(t, "NOC")
	if _, err := f.svc.RemoveProof(ctx, f.staff, q.ID); !errors.Is(err, ErrNotSent) {
		t.Fatalf("remove proof of open err = %v", err)
	}
	if _, err := f.svc.Send(ctx, f.staff, q.ID, "wrong file", pdf()); err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.RemoveProof(ctx, f.staff, q.ID)
	if err != nil {
		t.Fatalf("RemoveProof: %v", err)
	}
	if got.Status != StatusOpen || got.Events[len(got.Events)-1].Kind != "proof_removed" {
		t.Fatalf("after remove proof = %+v", got)
	}
	if docCount(t, f) != 0 || f.store.live() != 0 {
		t.Fatalf("docs=%d files=%d, want 0 and 0", docCount(t, f), f.store.live())
	}
	// A note-only proof can be removed too.
	if _, err := f.svc.Send(ctx, f.staff, q.ID, "note only", nil); err != nil {
		t.Fatal(err)
	}
	if got, err := f.svc.RemoveProof(ctx, f.staff, q.ID); err != nil || got.Status != StatusOpen {
		t.Fatalf("remove note proof: %+v %v", got, err)
	}
}

func TestRemoveHidesQueryAndKeepsDocument(t *testing.T) {
	f := setup(t, nil)
	ctx := context.Background()
	q := f.add(t, "NOC")
	if _, err := f.svc.Send(ctx, f.staff, q.ID, "", pdf()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Remove(ctx, f.staff, q.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	list, _ := f.svc.List(ctx, f.caseID)
	if len(list) != 0 || docCount(t, f) != 1 {
		t.Fatalf("list=%d docs=%d, want 0 and 1", len(list), docCount(t, f))
	}
	evs, _ := f.svc.Events(ctx, f.caseID)
	if evs[len(evs)-1].Kind != "removed" {
		t.Fatalf("last event = %+v, want removed", evs[len(evs)-1])
	}
	if _, err := f.svc.Remove(ctx, f.staff, q.ID); !errors.Is(err, ErrQueryNotFound) {
		t.Fatalf("remove twice err = %v", err)
	}
}

func TestDroppedCaseRefusesProofActions(t *testing.T) {
	f := setup(t, nil)
	ctx := context.Background()
	q := f.add(t, "NOC")
	dropCase(t, f.pool, f.caseID)
	if _, err := f.svc.Send(ctx, f.staff, q.ID, "x", nil); !errors.Is(err, ErrCaseDropped) {
		t.Fatalf("send on dropped err = %v", err)
	}
	if _, err := f.svc.Remove(ctx, f.staff, q.ID); !errors.Is(err, ErrCaseDropped) {
		t.Fatalf("remove on dropped err = %v", err)
	}
}

// Review fix: once the proof is saved, a failed read-back must not delete the file.
func TestSendKeepsFileWhenReadBackFails(t *testing.T) {
	f := setup(t, nil)
	ctx := context.Background()
	q := f.add(t, "NOC")
	f.svc.readBack = func(context.Context, int64, int64) (Query, error) { return Query{}, errors.New("connection closed") }
	if _, err := f.svc.Send(ctx, f.staff, q.ID, "", pdf()); err == nil {
		t.Fatal("Send hid the read-back error")
	}
	if docCount(t, f) != 1 || f.store.live() != 1 {
		t.Fatalf("after committed send docs=%d files=%d, want 1 and 1", docCount(t, f), f.store.live())
	}
}
