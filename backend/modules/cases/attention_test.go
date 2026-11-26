package cases

import (
	"context"
	"testing"
	"time"
)

func backdateStage(t *testing.T, f fixture, caseID int64, days int) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(),
		`UPDATE case_stage_events SET reached_at = now() - make_interval(days => $2) WHERE case_id = $1`, caseID, days); err != nil {
		t.Fatalf("backdate: %v", err)
	}
}

func attentionFor(t *testing.T, f fixture, assignee *int64) map[int64]AttentionItem {
	t.Helper()
	items, err := f.svc.Attention(context.Background(), assignee, time.Now())
	if err != nil {
		t.Fatalf("attention: %v", err)
	}
	m := map[int64]AttentionItem{}
	for _, it := range items {
		m[it.ID] = it
	}
	return m
}

func TestAttentionMissingDocument(t *testing.T) {
	f := newFixture(t)
	c := newCase(t, f)
	if _, err := f.pool.Exec(context.Background(), `UPDATE stages SET requires_document = true WHERE code = 'received' AND bank_id IS NULL`); err != nil {
		t.Fatalf("require: %v", err)
	}
	it, ok := attentionFor(t, f, nil)[c.ID]
	if !ok || it.Reason != "No documents at this stage" {
		t.Fatalf("got %+v, want missing-document reason", it)
	}
}

func TestAttentionStuckSevenDays(t *testing.T) {
	f := newFixture(t)
	c := newCase(t, f)
	backdateStage(t, f, c.ID, 6)
	if _, ok := attentionFor(t, f, nil)[c.ID]; ok {
		t.Fatal("6 days should not need attention")
	}
	backdateStage(t, f, c.ID, 7)
	it, ok := attentionFor(t, f, nil)[c.ID]
	if !ok || it.Reason != "In Received for 7 days" {
		t.Fatalf("got %+v, want stuck reason", it)
	}
}

func TestAttentionSkipsDroppedAndDisbursed(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	dropped := newCase(t, f)
	var reason int64
	mustScan(t, f.pool.QueryRow(ctx, `SELECT id FROM dropdown_values WHERE category = 'drop_reason' AND value <> 'Other' ORDER BY sort_order LIMIT 1`), &reason)
	if _, err := f.svc.Drop(ctx, f.actor, dropped.ID, DropInput{ReasonID: reason}); err != nil {
		t.Fatalf("drop: %v", err)
	}
	backdateStage(t, f, dropped.ID, 30)
	done := newCase(t, f)
	if _, err := f.pool.Exec(ctx, `UPDATE cases SET current_stage_id = $2 WHERE id = $1`, done.ID, stageID(t, f, "disbursed")); err != nil {
		t.Fatalf("disburse: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE case_stage_events SET stage_id = $2 WHERE case_id = $1`, done.ID, stageID(t, f, "disbursed")); err != nil {
		t.Fatalf("disburse event: %v", err)
	}
	backdateStage(t, f, done.ID, 30)
	m := attentionFor(t, f, nil)
	if _, ok := m[dropped.ID]; ok {
		t.Fatal("dropped case listed")
	}
	if _, ok := m[done.ID]; ok {
		t.Fatal("disbursed case listed")
	}
}

func TestAttentionFiltersByAssignee(t *testing.T) {
	f := newFixture(t)
	a, b, _, other := twoCases(t, f)
	backdateStage(t, f, a.ID, 10)
	backdateStage(t, f, b.ID, 10)
	m := attentionFor(t, f, &other)
	if _, ok := m[a.ID]; ok {
		t.Fatal("other assignee's case listed")
	}
	if _, ok := m[b.ID]; !ok {
		t.Fatal("colleague's case missing")
	}
}

func TestAttentionStageOrderIsPosition(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var bank int64
	mustScan(t, f.pool.QueryRow(ctx, `INSERT INTO banks (name) VALUES ('GapsA') RETURNING id`), &bank)
	if _, err := f.pool.Exec(ctx, `INSERT INTO stages (bank_id, code, name, sort_order, role) VALUES
		($1, 'received', 'Received', 10, 'start'), ($1, 'disbursed', 'Disbursed', 20, 'disbursement')`, bank); err != nil {
		t.Fatalf("stages: %v", err)
	}
	c, err := f.svc.Create(ctx, f.actor, NewCase{ProjectID: f.naProject, CustomerID: f.customer, BankID: bank, LoanAmount: "500000", AssigneeID: f.actor.ID})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	backdateStage(t, f, c.ID, 8)
	if it, ok := attentionFor(t, f, nil)[c.ID]; !ok || it.StageOrder != 1 || it.StageCount != 2 {
		t.Fatalf("got %+v, want stage 1/2", it)
	}
}
