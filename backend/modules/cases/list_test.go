package cases

import (
	"context"
	"testing"
	"time"
)

// twoCases returns two cases: A at Received for the fixture employee, and
// B at SBI for a second employee, which is moved to Logged in.
func twoCases(t *testing.T, f fixture) (a, b Case, sbi int64, other int64) {
	t.Helper()
	ctx := context.Background()
	a = newCase(t, f)
	mustScan(t, f.pool.QueryRow(ctx, `INSERT INTO banks (name) VALUES ('SBI-L') RETURNING id`), &sbi)
	other = seedUser(t, f, "colleague", true).ID
	var err error
	b, err = f.svc.Create(ctx, f.actor, NewCase{ProjectID: f.naProject, CustomerID: f.customer, BankID: sbi, LoanAmount: "900000", AssigneeID: other})
	if err != nil {
		t.Fatalf("second case: %v", err)
	}
	return a, b, sbi, other
}

func ids(list []Case) map[int64]bool {
	m := map[int64]bool{}
	for _, c := range list {
		m[c.ID] = true
	}
	return m
}

func TestEachFilterAlone(t *testing.T) {
	f := newFixture(t)
	a, b, sbi, other := twoCases(t, f)
	ctx := context.Background()

	byStage, err := f.svc.List(ctx, ListFilter{StageID: ptr(stageID(t, f, "received"))})
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	if !ids(byStage)[a.ID] || !ids(byStage)[b.ID] {
		t.Fatalf("stage filter (received) = %v, want both", ids(byStage))
	}

	byBank, err := f.svc.List(ctx, ListFilter{BankID: &sbi})
	if err != nil {
		t.Fatalf("bank: %v", err)
	}
	if ids(byBank)[a.ID] || !ids(byBank)[b.ID] {
		t.Fatalf("bank filter = %v, want only B", ids(byBank))
	}

	byAssignee, err := f.svc.List(ctx, ListFilter{AssigneeID: &other})
	if err != nil {
		t.Fatalf("assignee: %v", err)
	}
	if ids(byAssignee)[a.ID] || !ids(byAssignee)[b.ID] {
		t.Fatalf("assignee filter = %v, want only B", ids(byAssignee))
	}

	from := time.Now().Add(-time.Hour)
	byDate, err := f.svc.List(ctx, ListFilter{From: &from})
	if err != nil {
		t.Fatalf("date: %v", err)
	}
	if !ids(byDate)[a.ID] || !ids(byDate)[b.ID] {
		t.Fatalf("date filter = %v, want both (created just now)", ids(byDate))
	}
	future := time.Now().Add(time.Hour)
	none, _ := f.svc.List(ctx, ListFilter{From: &future})
	if len(none) != 0 {
		t.Fatalf("future date range returned %d cases, want 0", len(none))
	}
}

func TestFiltersCombine(t *testing.T) {
	f := newFixture(t)
	a, b, sbi, other := twoCases(t, f)
	ctx := context.Background()

	got, err := f.svc.List(ctx, ListFilter{BankID: &sbi, AssigneeID: &other, StageID: ptr(stageID(t, f, "received"))})
	if err != nil {
		t.Fatalf("combined: %v", err)
	}
	if !ids(got)[b.ID] || ids(got)[a.ID] || len(got) != 1 {
		t.Fatalf("combined filters = %v, want only B", ids(got))
	}
	none, err := f.svc.List(ctx, ListFilter{BankID: &sbi, AssigneeID: ptr(f.actor.ID)})
	if err != nil {
		t.Fatalf("combined none: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("contradictory filters returned %d, want 0", len(none))
	}
}

func TestDroppedItemsStillListed(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := newCase(t, f)
	if _, err := f.svc.Drop(ctx, f.actor, c.ID, DropInput{ReasonID: reasonID(t, f, "Not eligible")}); err != nil {
		t.Fatalf("drop: %v", err)
	}
	list, err := f.svc.List(ctx, ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, x := range list {
		if x.ID == c.ID {
			if x.Status != "dropped" {
				t.Fatalf("dropped case listed with status %s", x.Status)
			}
			return
		}
	}
	t.Fatal("dropped case missing from the list")
}

func ptr[T any](v T) *T { return &v }

func rowByID(t *testing.T, rows []CaseRow, id int64) CaseRow {
	t.Helper()
	for _, r := range rows {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("case %d not in rows", id)
	return CaseRow{}
}

func TestListRowsCarriesNames(t *testing.T) {
	f := newFixture(t)
	a, _, _, _ := twoCases(t, f)
	ctx := context.Background()

	rows, err := f.svc.ListRows(ctx, ListFilter{})
	if err != nil {
		t.Fatalf("list rows: %v", err)
	}
	r := rowByID(t, rows, a.ID)
	if r.CustomerName != "Ravi" || r.CustomerMobile != "9876543210" || r.BankName != "HDFC" {
		t.Fatalf("names = %q %q %q", r.CustomerName, r.CustomerMobile, r.BankName)
	}
	if r.StageName != "Received" || r.StageCode != "received" || r.StageOrder != 1 || r.StageCount != 6 {
		t.Fatalf("stage = %q %q %d/%d", r.StageName, r.StageCode, r.StageOrder, r.StageCount)
	}
	if r.AssigneeName != "Staff" {
		t.Fatalf("assignee = %q", r.AssigneeName)
	}
	var reached time.Time
	mustScan(t, f.pool.QueryRow(ctx, `SELECT max(reached_at) FROM case_stage_events WHERE case_id = $1`, a.ID), &reached)
	if !r.StageSince.Equal(reached) {
		t.Fatalf("stage since = %v, want %v", r.StageSince, reached)
	}
}

func TestListRowsUsesBankStageCount(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var bank int64
	mustScan(t, f.pool.QueryRow(ctx, `INSERT INTO banks (name) VALUES ('OwnStages') RETURNING id`), &bank)
	if _, err := f.pool.Exec(ctx, `INSERT INTO stages (bank_id, code, name, sort_order) VALUES
		($1, 'received', 'Received', 1), ($1, 'logged_in', 'Logged in', 2), ($1, 'disbursed', 'Disbursed', 3)`, bank); err != nil {
		t.Fatalf("bank stages: %v", err)
	}
	c, err := f.svc.Create(ctx, f.actor, NewCase{ProjectID: f.naProject, CustomerID: f.customer, BankID: bank, LoanAmount: "500000", AssigneeID: f.actor.ID})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	rows, err := f.svc.ListRows(ctx, ListFilter{})
	if err != nil {
		t.Fatalf("list rows: %v", err)
	}
	if r := rowByID(t, rows, c.ID); r.StageCount != 3 || r.StageOrder != 1 {
		t.Fatalf("stage %d/%d, want 1/3", r.StageOrder, r.StageCount)
	}
}

func TestListRowsNeedsDocument(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a := newCase(t, f)
	if _, err := f.pool.Exec(ctx, `UPDATE stages SET requires_document = true WHERE code = 'received' AND bank_id IS NULL`); err != nil {
		t.Fatalf("require: %v", err)
	}
	rows, err := f.svc.ListRows(ctx, ListFilter{})
	if err != nil {
		t.Fatalf("list rows: %v", err)
	}
	if !rowByID(t, rows, a.ID).NeedsDocument {
		t.Fatal("want NeedsDocument with no documents")
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO documents (case_id, stage_id, label, file_name, content_type, size_bytes, storage_key, uploaded_by)
		VALUES ($1, $2, 'PAN', 'pan.pdf', 'application/pdf', 10, 'k1', $3)`, a.ID, a.CurrentStageID, f.actor.ID); err != nil {
		t.Fatalf("doc: %v", err)
	}
	rows, err = f.svc.ListRows(ctx, ListFilter{})
	if err != nil {
		t.Fatalf("list rows: %v", err)
	}
	if rowByID(t, rows, a.ID).NeedsDocument {
		t.Fatal("want no NeedsDocument after an upload")
	}
}

func TestListRowsFilterByCustomer(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a := newCase(t, f)
	var other int64
	mustScan(t, f.pool.QueryRow(ctx, `INSERT INTO customers (name, mobile) VALUES ('Other', '9000000099') RETURNING id`), &other)
	b, err := f.svc.Create(ctx, f.actor, NewCase{ProjectID: f.naProject, CustomerID: other, BankID: f.bankID, LoanAmount: "100000", AssigneeID: f.actor.ID})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	rows, err := f.svc.ListRows(ctx, ListFilter{CustomerID: &f.customer})
	if err != nil {
		t.Fatalf("list rows: %v", err)
	}
	got := map[int64]bool{}
	for _, r := range rows {
		got[r.ID] = true
	}
	if !got[a.ID] || got[b.ID] {
		t.Fatalf("customer filter = %v, want only %d", got, a.ID)
	}
}

// Admin-set sort orders can have gaps; the row reports the stage's position.
func TestListRowsStageOrderIsPosition(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var bank int64
	mustScan(t, f.pool.QueryRow(ctx, `INSERT INTO banks (name) VALUES ('Gaps') RETURNING id`), &bank)
	if _, err := f.pool.Exec(ctx, `INSERT INTO stages (bank_id, code, name, sort_order) VALUES
		($1, 'received', 'Received', 10), ($1, 'logged_in', 'Logged in', 20), ($1, 'disbursed', 'Disbursed', 30)`, bank); err != nil {
		t.Fatalf("stages: %v", err)
	}
	c, err := f.svc.Create(ctx, f.actor, NewCase{ProjectID: f.naProject, CustomerID: f.customer, BankID: bank, LoanAmount: "500000", AssigneeID: f.actor.ID})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	rows, err := f.svc.ListRows(ctx, ListFilter{})
	if err != nil {
		t.Fatalf("list rows: %v", err)
	}
	if r := rowByID(t, rows, c.ID); r.StageOrder != 1 || r.StageCount != 3 {
		t.Fatalf("stage %d/%d, want 1/3", r.StageOrder, r.StageCount)
	}
}
