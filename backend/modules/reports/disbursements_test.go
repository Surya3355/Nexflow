package reports

import (
	"context"
	"testing"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/cases"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/Surya3355/nexflow/backend/modules/workflows"
)

// byCode sums the cases-by-stage totals by stage code, across banks.
func byCode(d DashboardData) map[string]int {
	m := map[string]int{}
	for _, s := range d.CasesByStage {
		m[s.StageCode] += s.Total
	}
	return m
}

func hdfc(t *testing.T, f fixture) int64 {
	t.Helper()
	ctx := context.Background()
	admin := access.User{ID: f.actor.ID, Role: access.RoleAdmin, IsActive: true}
	wf := workflows.NewService(f.pool, history.Nop{})
	b, err := wf.CreateBank(ctx, admin, "HDFC Ltd")
	if err != nil {
		t.Fatal(err)
	}
	if err := wf.SetBankStageList(ctx, admin, b.ID, workflows.StageListHDFC); err != nil {
		t.Fatal(err)
	}
	return b.ID
}

func bankStage(t *testing.T, f fixture, bank int64, code string) int64 {
	t.Helper()
	var id int64
	mustScan(t, f.pool.QueryRow(context.Background(), `SELECT id FROM stages WHERE bank_id = $1 AND code = $2`, bank, code), &id)
	return id
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func nov() Period {
	return Period{From: time.Date(2026, 11, 1, 0, 0, 0, 0, istZone), To: time.Date(2026, 12, 1, 0, 0, 0, 0, istZone)}
}

// hdfcCase opens an HDFC case for a new customer, sanctions it and pays parts on the given days.
func hdfcCase(t *testing.T, f fixture, bank int64, mobile, sanctioned string, parts map[time.Time]string) int64 {
	t.Helper()
	ctx := context.Background()
	var cust int64
	mustScan(t, f.pool.QueryRow(ctx, `INSERT INTO customers (name, mobile) VALUES ('Anita', $1) RETURNING id`, mobile), &cust)
	c, err := f.cases.Create(ctx, f.actor, cases.NewCase{ProjectID: naProjectID(t, f.pool), CustomerID: cust, BankID: bank, LoanAmount: "5000000", AssigneeID: f.actor.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.cases.ChangeStage(ctx, f.actor, c.ID, cases.StageMove{ToStageID: bankStage(t, f, bank, "sanctioned"),
		Sanction: &cases.SanctionDetails{Amount: sanctioned, RatePercent: "8.5"}}); err != nil {
		t.Fatal(err)
	}
	first := true
	for d, amt := range parts {
		p := cases.PaymentInput{Amount: amt, Date: d, Kind: cases.PaymentPart}
		if first {
			if _, err := f.cases.ChangeStage(ctx, f.actor, c.ID, cases.StageMove{ToStageID: bankStage(t, f, bank, "disbursement"), Payment: &p}); err != nil {
				t.Fatal(err)
			}
			first = false
			continue
		}
		if _, err := f.cases.AddPayment(ctx, f.actor, c.ID, p); err != nil {
			t.Fatal(err)
		}
	}
	return c.ID
}

func TestSanctionsCountHDFCStage(t *testing.T) {
	f := newFixture(t)
	bank := hdfc(t, f)
	hdfcCase(t, f, bank, "9000000001", "4500000", nil)
	newCaseAt(t, f, "100000", "SAN1", "logged_in", "under_sanction", "sanctioned")
	now := time.Now()
	d, err := NewService(f.pool).Dashboard(context.Background(), now, Period{From: now.Add(-time.Hour), To: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if d.Sanctions.Count != 2 || d.Sanctions.Amount != "4600000.00" {
		t.Fatalf("sanctions = %+v, want 2 cases, 4600000.00", d.Sanctions)
	}
}

func TestDisbursementTileSumsPaymentsInPeriod(t *testing.T) {
	f := newFixture(t)
	bank := hdfc(t, f)
	hdfcCase(t, f, bank, "9000000001", "3500000", map[time.Time]string{day(2026, 11, 5): "1000000", day(2026, 11, 18): "1500000"})
	hdfcCase(t, f, bank, "9000000002", "3500000", map[time.Time]string{day(2026, 10, 30): "700000"})
	d, err := NewService(f.pool).Dashboard(context.Background(), time.Now(), nov())
	if err != nil {
		t.Fatal(err)
	}
	if d.Disbursements.Count != 1 || d.Disbursements.Amount != "2500000.00" {
		t.Fatalf("disbursements = %+v, want 1 case, 2500000.00", d.Disbursements)
	}
}

func TestDisbursementRowsOneRowPerCase(t *testing.T) {
	f := newFixture(t)
	bank := hdfc(t, f)
	id := hdfcCase(t, f, bank, "9000000001", "3500000", map[time.Time]string{day(2026, 10, 20): "500000", day(2026, 11, 5): "1000000", day(2026, 11, 18): "1500000"})
	rows, err := NewService(f.pool).DisbursementRows(context.Background(), nov())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	r := rows[0]
	if r.CaseID != id || r.PaidInPeriod != "2500000.00" || r.TotalPaid != "3000000.00" || r.Remaining != "500000.00" ||
		r.Sanctioned != "3500000.00" || r.Kind != "Part" || r.LastPaidOn != "2026-11-18" || r.Bank != "HDFC Ltd" || r.Dropped {
		t.Fatalf("row = %+v", r)
	}
}

func TestDisbursementRowFullAfterFullPayment(t *testing.T) {
	f := newFixture(t)
	bank := hdfc(t, f)
	id := hdfcCase(t, f, bank, "9000000001", "3500000", map[time.Time]string{day(2026, 11, 5): "1000000"})
	if _, err := f.cases.AddPayment(context.Background(), f.actor, id, cases.PaymentInput{Amount: "2500000", Date: day(2026, 11, 20), Kind: cases.PaymentFull}); err != nil {
		t.Fatal(err)
	}
	rows, _ := NewService(f.pool).DisbursementRows(context.Background(), nov())
	if len(rows) != 1 || rows[0].Kind != "Full" || rows[0].Remaining != "0.00" || rows[0].Status != "completed" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestDroppedCaseWithPaymentCountedAndMarked(t *testing.T) {
	f := newFixture(t)
	bank := hdfc(t, f)
	id := hdfcCase(t, f, bank, "9000000001", "3500000", map[time.Time]string{day(2026, 11, 5): "1000000"})
	if _, err := f.cases.Drop(context.Background(), f.actor, id, cases.DropInput{ReasonID: reasonID(t, f, "Rejected by bank")}); err != nil {
		t.Fatal(err)
	}
	d, _ := NewService(f.pool).Dashboard(context.Background(), time.Now(), nov())
	if d.Disbursements.Count != 1 || d.Disbursements.Dropped != 1 {
		t.Fatalf("disbursements = %+v", d.Disbursements)
	}
	rows, _ := NewService(f.pool).DisbursementRows(context.Background(), nov())
	if len(rows) != 1 || !rows[0].Dropped {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestDisbursementRowsBankFilter(t *testing.T) {
	f := newFixture(t)
	bank := hdfc(t, f)
	hdfcCase(t, f, bank, "9000000001", "3500000", map[time.Time]string{day(2026, 11, 5): "1000000"})
	p := nov()
	p.BankID = &f.bank
	rows, _ := NewService(f.pool).DisbursementRows(context.Background(), p)
	if len(rows) != 0 {
		t.Fatalf("other bank rows = %d, want 0", len(rows))
	}
}

func TestStageMovesSkipFirstEvent(t *testing.T) {
	f := newFixture(t)
	bank := hdfc(t, f)
	hdfcCase(t, f, bank, "9000000001", "3500000", nil) // opens at Link sent, then Sanctioned
	now := time.Now()
	d, _ := NewService(f.pool).Dashboard(context.Background(), now, Period{From: now.Add(-time.Hour), To: now.Add(time.Hour)})
	if d.StageMovesToday != 1 {
		t.Fatalf("stage moves = %d, want 1 (Link sent on open is not a move)", d.StageMovesToday)
	}
}

func TestCasesByStageGroupedByBank(t *testing.T) {
	f := newFixture(t)
	bank := hdfc(t, f)
	hdfcCase(t, f, bank, "9000000001", "3500000", nil)
	newCaseAt(t, f, "100000", "GRP1")
	now := time.Now()
	d, _ := NewService(f.pool).Dashboard(context.Background(), now, Period{From: now.Add(-time.Hour), To: now.Add(time.Hour)})
	if len(d.CasesByStage) != 2 {
		t.Fatalf("by stage = %+v", d.CasesByStage)
	}
	// Ordered by bank name: "HDFC" (standard stages) before "HDFC Ltd".
	a, b := d.CasesByStage[0], d.CasesByStage[1]
	if a.BankName != "HDFC" || a.StageName != "Received" || a.BankID == nil || *a.BankID != f.bank || a.Total != 1 {
		t.Errorf("first = %+v", a)
	}
	if b.BankName != "HDFC Ltd" || b.StageName != "Sanctioned" || b.SortOrder != 6 || b.Total != 1 {
		t.Errorf("second = %+v", b)
	}
}
