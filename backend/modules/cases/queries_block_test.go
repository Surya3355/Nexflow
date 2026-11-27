package cases

import (
	"context"
	"errors"
	"testing"
	"time"
)

// addQuery inserts a bank query row directly (the bankqueries module owns the real flow).
func addQuery(t *testing.T, f fixture, caseID int64, number int, status string) int64 {
	t.Helper()
	var id int64
	mustScan(t, f.pool.QueryRow(context.Background(), `INSERT INTO case_queries (case_id, number, body, raised_on, assignee_id, status, created_by, updated_by)
		VALUES ($1, $2, 'Salary slips', CURRENT_DATE - 1, $3, $4, $3, $3) RETURNING id`, caseID, number, f.actor.ID, status), &id)
	return id
}

func TestOpenQueryBlocksForwardMove(t *testing.T) {
	for _, status := range []string{"open", "sent"} {
		t.Run(status, func(t *testing.T) {
			f := newFixture(t)
			bank := hdfcBank(t, f)
			c := caseAtBank(t, f, bank, "500000")
			q := addQuery(t, f, c.ID, 1, status)
			if err := moveTo(t, f, c, bank, "dovr", 0); !errors.Is(err, ErrOpenQueries) {
				t.Fatalf("forward move err = %v, want ErrOpenQueries", err)
			}
			if err := moveTo(t, f, c, bank, "received", 0); err != nil {
				t.Fatalf("backward move: %v", err)
			}
			if _, err := f.pool.Exec(context.Background(), `UPDATE case_queries SET status = 'resolved' WHERE id = $1`, q); err != nil {
				t.Fatal(err)
			}
			if err := moveTo(t, f, c, bank, "dovr", 0); err != nil {
				t.Fatalf("move after resolve: %v", err)
			}
		})
	}
}

func TestRemovedQueryDoesNotBlock(t *testing.T) {
	f := newFixture(t)
	bank := hdfcBank(t, f)
	c := caseAtBank(t, f, bank, "500000")
	q := addQuery(t, f, c.ID, 1, "open")
	if _, err := f.pool.Exec(context.Background(), `UPDATE case_queries SET removed_at = now(), removed_by = $2 WHERE id = $1`, q, f.actor.ID); err != nil {
		t.Fatal(err)
	}
	if err := moveTo(t, f, c, bank, "dovr", 0); err != nil {
		t.Fatalf("move with removed query: %v", err)
	}
}

func TestOpenQueriesNeedAttention(t *testing.T) {
	f := newFixture(t)
	bank := hdfcBank(t, f)
	c := caseAtBank(t, f, bank, "500000")
	addQuery(t, f, c.ID, 1, "open")
	addQuery(t, f, c.ID, 2, "sent")
	addQuery(t, f, c.ID, 3, "resolved")
	items, err := f.svc.Attention(context.Background(), nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != c.ID || items[0].Reason != "2 open bank queries" || items[0].OpenQueries != 2 {
		t.Fatalf("attention = %+v, want the case with reason '2 open bank queries'", items)
	}
	rows, err := f.svc.ListRows(context.Background(), ListFilter{})
	if err != nil || len(rows) != 1 || rows[0].OpenQueries != 2 {
		t.Fatalf("rows = %+v %v, want OpenQueries 2", rows, err)
	}
	other := caseAtBank(t, f, bank, "600000")
	addQuery(t, f, other.ID, 1, "open")
	items, _ = f.svc.Attention(context.Background(), nil, time.Now())
	for _, it := range items {
		if it.ID == other.ID && it.Reason != "1 open bank query" {
			t.Fatalf("one-query reason = %q", it.Reason)
		}
	}
}

// Review fix: a Full payment must not complete a case with an open query.
func TestFullPaymentRefusedWithOpenQuery(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, c := hdfcAtDisbursement(t, f, "2500000", "1000000")
	q := addQuery(t, f, c.ID, 1, "sent")
	if _, err := f.svc.AddPayment(ctx, f.actor, c.ID, *full("1500000")); !errors.Is(err, ErrOpenQueries) {
		t.Fatalf("full payment with open query err = %v, want ErrOpenQueries", err)
	}
	if _, err := f.svc.AddPayment(ctx, f.actor, c.ID, *part("500000")); err != nil {
		t.Fatalf("part payment with open query: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE case_queries SET status = 'resolved' WHERE id = $1`, q); err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.AddPayment(ctx, f.actor, c.ID, *full("1000000"))
	if err != nil || got.Status != "completed" {
		t.Fatalf("full payment after resolve = %v %v", got.Status, err)
	}
}

// Review fix: a query raised on a completed case still needs attention.
func TestCompletedCaseWithOpenQueryNeedsAttention(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, c := hdfcAtDisbursement(t, f, "2500000", "1000000")
	if _, err := f.svc.AddPayment(ctx, f.actor, c.ID, *full("1500000")); err != nil {
		t.Fatal(err)
	}
	items, _ := f.svc.Attention(ctx, nil, time.Now())
	if len(items) != 0 {
		t.Fatalf("completed case without queries in attention: %+v", items)
	}
	addQuery(t, f, c.ID, 1, "open")
	items, err := f.svc.Attention(ctx, nil, time.Now())
	if err != nil || len(items) != 1 || items[0].Reason != "1 open bank query" {
		t.Fatalf("attention = %+v %v, want the completed case with its query", items, err)
	}
}
