package bankqueries

import (
	"context"
	"errors"
	"testing"
)

// Fixes from the final review of bank queries (deferred minors, 2026-11-26).

func TestAddWithoutAssigneeAsksToChoose(t *testing.T) {
	f := setup(t, nil)
	_, err := f.svc.Add(context.Background(), f.staff, f.caseID, NewQuery{Body: "PAN", RaisedOn: today()})
	if !errors.Is(err, ErrAssigneeRequired) {
		t.Fatalf("err = %v, want ErrAssigneeRequired", err)
	}
}

func TestEditWithNoChangeRecordsNothing(t *testing.T) {
	f := setup(t, nil)
	ctx := context.Background()
	q := f.add(t, "PAN")
	got, err := f.svc.Edit(ctx, f.staff, q.ID, NewQuery{Body: " PAN ", RaisedOn: q.RaisedOn, AssigneeID: q.AssigneeID})
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if len(got.Events) != 1 {
		t.Fatalf("events = %+v, want only raised", got.Events)
	}
	var n int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action = 'query.edited'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("query.edited audits = %d (%v), want 0", n, err)
	}
}

// Two people on one query: the second sees what actually happened to it.
func TestStaleActionsExplainTheQueryState(t *testing.T) {
	f := setup(t, nil)
	ctx := context.Background()
	q := f.add(t, "PAN")
	if _, err := f.svc.Send(ctx, f.staff, q.ID, "sent", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Send(ctx, f.staff, q.ID, "again", nil); !errors.Is(err, ErrAlreadySent) {
		t.Fatalf("send twice err = %v, want ErrAlreadySent", err)
	}
	if _, err := f.svc.Accept(ctx, f.staff, q.ID); err != nil {
		t.Fatal(err)
	}
	for name, run := range map[string]func() error{
		"remove proof": func() error { _, err := f.svc.RemoveProof(ctx, f.staff, q.ID); return err },
		"reject":       func() error { _, err := f.svc.Reject(ctx, f.staff, q.ID, "no"); return err },
		"accept":       func() error { _, err := f.svc.Accept(ctx, f.staff, q.ID); return err },
		"send":         func() error { _, err := f.svc.Send(ctx, f.staff, q.ID, "x", nil); return err },
	} {
		if err := run(); !errors.Is(err, ErrQueryResolved) {
			t.Errorf("%s after accept err = %v, want ErrQueryResolved", name, err)
		}
	}
}

func TestPurgedProofFileShowsAsDeleted(t *testing.T) {
	f := setup(t, nil)
	ctx := context.Background()
	q := f.add(t, "Salary slips")
	sent, err := f.svc.Send(ctx, f.staff, q.ID, "", pdf())
	if err != nil {
		t.Fatal(err)
	}
	if sent.Proof == nil || sent.Proof.FileDeleted {
		t.Fatalf("fresh proof = %+v, want a live file", sent.Proof)
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM documents WHERE id = $1`, *sent.Proof.DocumentID); err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.get(ctx, f.caseID, q.ID)
	if err != nil {
		t.Fatal(err)
	}
	p := got.Proof
	if p == nil || p.DocumentID != nil || !p.FileDeleted || p.FileName != "proof.pdf" {
		t.Fatalf("proof after purge = %+v, want file proof.pdf marked deleted", p)
	}
	last := got.Events[len(got.Events)-1]
	if last.Kind != "sent" || !last.FileDeleted {
		t.Fatalf("sent event = %+v, want FileDeleted", last)
	}
	if got.Events[0].FileDeleted {
		t.Fatalf("raised event marked as deleted file: %+v", got.Events[0])
	}
}

func TestLoadMissingCase(t *testing.T) {
	f := setup(t, nil)
	if _, _, err := f.svc.Load(context.Background(), 99999); !errors.Is(err, ErrCaseNotFound) {
		t.Fatalf("err = %v, want ErrCaseNotFound", err)
	}
	qs, evs, err := f.svc.Load(context.Background(), f.caseID)
	if err != nil || len(qs) != 0 || len(evs) != 0 {
		t.Fatalf("empty case = %v %v %v", qs, evs, err)
	}
}
