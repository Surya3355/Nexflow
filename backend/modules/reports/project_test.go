package reports

import (
	"bytes"
	"context"
	"encoding/csv"
	"testing"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/cases"
)

func TestCSVHasProjectColumn(t *testing.T) {
	if Header[3] != "Bank" || Header[4] != "Project" {
		t.Fatalf("header = %v, want Project right after Bank", Header)
	}
	r := Row{CaseID: 1, Bank: "HDFC", Project: "Prestige"}
	if got := r.Fields()[4]; got != "Prestige" {
		t.Fatalf("field 4 = %q, want Prestige", got)
	}
	var buf bytes.Buffer
	if err := ExportCSV(&buf, []Row{r}); err != nil {
		t.Fatal(err)
	}
	recs, err := csv.NewReader(bytes.NewReader(buf.Bytes()[3:])).ReadAll()
	if err != nil || len(recs) != 2 || recs[1][4] != "Prestige" {
		t.Fatalf("csv = %v %v", recs, err)
	}
}

func TestRowsAndDashboardProjectFilter(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var prestige int64
	mustScan(t, f.pool.QueryRow(ctx, `INSERT INTO projects (name) VALUES ('Prestige') RETURNING id`), &prestige)
	newCaseAt(t, f, "100000", "") // NA case
	if _, err := f.cases.Create(ctx, f.actor, cases.NewCase{ProjectID: prestige, CustomerID: f.customer, BankID: f.bank, LoanAmount: "200000", AssigneeID: f.actor.ID}); err != nil {
		t.Fatalf("prestige case: %v", err)
	}

	rows, err := NewService(f.pool).Rows(ctx, f.actor, RowFilter{ProjectID: &prestige})
	if err != nil || len(rows) != 1 || rows[0].Project != "Prestige" {
		t.Fatalf("rows = %+v %v, want one Prestige row", rows, err)
	}
	all, _ := NewService(f.pool).Rows(ctx, f.actor, RowFilter{})
	if len(all) != 2 {
		t.Fatalf("unfiltered rows = %d, want 2", len(all))
	}

	now := time.Now()
	d, err := NewService(f.pool).Dashboard(ctx, now, Period{From: now.Add(-time.Hour), To: now.Add(time.Hour), ProjectID: &prestige})
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if d.NewCases != 1 || d.NewCasesToday != 1 || byCode(d)["received"] != 1 {
		t.Fatalf("Prestige only: %+v", d)
	}
}
