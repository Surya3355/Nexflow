package reports

import (
	"context"
	"testing"
	"time"

	"github.com/Surya3355/nexflow/backend/internal/platform/testdb"
	"github.com/Surya3355/nexflow/backend/migrations"
	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/cases"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/Surya3355/nexflow/backend/modules/workflows"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	pool     *pgxpool.Pool
	cases    *cases.Service
	actor    access.User
	customer int64
	bank     int64
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	pool := testdb.New(t)
	if err := migrations.Up(pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	var f fixture
	f.pool = pool
	f.cases = cases.NewService(pool, workflows.NewService(pool, history.Nop{}), history.NewPgRecorder(pool))
	f.actor = access.User{Username: "staff", Role: access.RoleEmployee, IsActive: true}
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO users (username, full_name, password_hash, role)
		VALUES ('staff', 'Staff', 'x', 'employee') RETURNING id`), &f.actor.ID)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO banks (name) VALUES ('HDFC') RETURNING id`), &f.bank)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO customers (name, mobile) VALUES ('Ravi', '9876543210') RETURNING id`), &f.customer)
	return f
}

func mustScan(t *testing.T, row interface{ Scan(...any) error }, dest ...any) {
	t.Helper()
	if err := row.Scan(dest...); err != nil {
		t.Fatalf("scan: %v", err)
	}
}

func stage(t *testing.T, f fixture, code string) int64 {
	t.Helper()
	var id int64
	mustScan(t, f.pool.QueryRow(context.Background(), `SELECT id FROM stages WHERE code=$1 AND bank_id IS NULL`, code), &id)
	return id
}

// newCaseAt opens a case and walks it through the given stage codes in order.
func newCaseAt(t *testing.T, f fixture, amount string, tracking string, through ...string) int64 {
	t.Helper()
	ctx := context.Background()
	c, err := f.cases.Create(ctx, f.actor, cases.NewCase{ProjectID: naProjectID(t, f.pool), CustomerID: f.customer, BankID: f.bank, LoanAmount: amount, AssigneeID: f.actor.ID})
	if err != nil {
		t.Fatalf("create case: %v", err)
	}
	for _, code := range through {
		var tn *string
		if code == "logged_in" {
			tn = &tracking
		}
		var sanction *cases.SanctionDetails
		if code == "sanctioned" {
			sanction = &cases.SanctionDetails{Amount: amount, RatePercent: "9.25", InterestType: "floating", TenureMonths: 240}
		}
		var disb *cases.PaymentInput
		if code == "disbursed" {
			disb = &cases.PaymentInput{Amount: amount, Kind: cases.PaymentFull}
		}
		if _, err := f.cases.ChangeStage(ctx, f.actor, c.ID, cases.StageMove{ToStageID: stage(t, f, code), TrackingNumber: tn, Sanction: sanction, Payment: disb}); err != nil {
			t.Fatalf("move to %s: %v", code, err)
		}
	}
	return c.ID
}

// TestTodayUsesIndiaTime: 19:00 UTC on 12 Nov is 00:30 IST on 13 Nov.
func TestTodayUsesIndiaTime(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE customers SET created_at = '2026-11-12 19:00:00+00'`); err != nil {
		t.Fatalf("set created_at: %v", err)
	}
	ist := time.FixedZone("IST", 5*60*60+30*60)

	nov13 := time.Date(2026, 11, 13, 12, 0, 0, 0, ist)
	d, err := NewService(f.pool).Dashboard(ctx, nov13, Period{From: nov13.AddDate(0, -1, 0), To: nov13})
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if d.NewCustomersToday != 1 {
		t.Fatalf("on 13 Nov IST new customers today = %d, want 1", d.NewCustomersToday)
	}

	nov12 := time.Date(2026, 11, 12, 23, 0, 0, 0, ist)
	d, err = NewService(f.pool).Dashboard(ctx, nov12, Period{From: nov12.AddDate(0, -1, 0), To: nov12})
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if d.NewCustomersToday != 0 {
		t.Fatalf("on 12 Nov IST new customers today = %d, want 0 (created 00:30 IST on 13 Nov)", d.NewCustomersToday)
	}
}

func TestSanctionCountsCaseThatMovedOn(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	newCaseAt(t, f, "1000000", "SAN1", "logged_in", "under_sanction", "sanctioned", "documentation", "disbursed")
	now := time.Now()
	d, err := NewService(f.pool).Dashboard(ctx, now, Period{From: now.Add(-time.Hour), To: now.Add(time.Hour)})
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if d.Sanctions.Count != 1 || d.Sanctions.Amount != "1000000.00" {
		t.Fatalf("sanctions = %+v, want 1 case, 1000000.00", d.Sanctions)
	}
	if d.Disbursements.Count != 1 {
		t.Fatalf("disbursements = %+v, want 1", d.Disbursements)
	}
}

func TestDroppedAfterSanctionCountedAndMarked(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	id := newCaseAt(t, f, "500000", "DRPSAN", "logged_in", "under_sanction", "sanctioned")
	if _, err := f.cases.Drop(ctx, f.actor, id, cases.DropInput{ReasonID: reasonID(t, f, "Rejected by bank")}); err != nil {
		t.Fatalf("drop: %v", err)
	}
	now := time.Now()
	d, err := NewService(f.pool).Dashboard(ctx, now, Period{From: now.Add(-time.Hour), To: now.Add(time.Hour)})
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if d.Sanctions.Count != 1 || d.Sanctions.Dropped != 1 {
		t.Fatalf("sanctions = %+v, want 1 counted and marked dropped", d.Sanctions)
	}
}

func TestCasesByStageCountsCurrentStageOnly(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	newCaseAt(t, f, "100000", "STG1")
	newCaseAt(t, f, "100000", "STG2", "logged_in")
	dropped := newCaseAt(t, f, "100000", "STG3", "logged_in")
	if _, err := f.cases.Drop(ctx, f.actor, dropped, cases.DropInput{ReasonID: reasonID(t, f, "Not eligible")}); err != nil {
		t.Fatalf("drop: %v", err)
	}
	now := time.Now()
	d, err := NewService(f.pool).Dashboard(ctx, now, Period{From: now.Add(-time.Hour), To: now.Add(time.Hour)})
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if byCode(d)["received"] != 1 || byCode(d)["logged_in"] != 1 {
		t.Fatalf("cases by stage = %v, want received=1 logged_in=1 (dropped excluded)", d.CasesByStage)
	}
}

func reasonID(t *testing.T, f fixture, value string) int64 {
	t.Helper()
	var id int64
	mustScan(t, f.pool.QueryRow(context.Background(),
		`SELECT id FROM dropdown_values WHERE category='drop_reason' AND value=$1`, value), &id)
	return id
}

func TestDashboardPeriodCounts(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	newCaseAt(t, f, "100000", "PER1")
	if _, err := f.pool.Exec(ctx, `UPDATE customers SET created_at = now() - interval '10 days'`); err != nil {
		t.Fatalf("backdate customer: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE cases SET created_at = now() - interval '10 days'`); err != nil {
		t.Fatalf("backdate case: %v", err)
	}
	now := time.Now()
	d, err := NewService(f.pool).Dashboard(ctx, now, Period{From: now.AddDate(0, 0, -30), To: now.Add(time.Hour)})
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if d.NewCustomers != 1 || d.NewCases != 1 {
		t.Fatalf("30-day period: customers %d cases %d, want 1 and 1", d.NewCustomers, d.NewCases)
	}
	d, err = NewService(f.pool).Dashboard(ctx, now, Period{From: now.AddDate(0, 0, -5), To: now.Add(time.Hour)})
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if d.NewCustomers != 0 || d.NewCases != 0 {
		t.Fatalf("5-day period: customers %d cases %d, want 0 and 0", d.NewCustomers, d.NewCases)
	}
}

func TestDashboardStageMovesToday(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	newCaseAt(t, f, "100000", "MOV1", "logged_in")
	now := time.Now()
	d, err := NewService(f.pool).Dashboard(ctx, now, Period{From: now.Add(-time.Hour), To: now.Add(time.Hour)})
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if d.StageMovesToday != 1 {
		t.Fatalf("stage moves today = %d, want 1 (Received on open is not a move)", d.StageMovesToday)
	}
}

func TestDashboardBankFilter(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	newCaseAt(t, f, "100000", "BNK1", "logged_in", "under_sanction", "sanctioned")
	var sbi int64
	mustScan(t, f.pool.QueryRow(ctx, `INSERT INTO banks (name) VALUES ('SBI') RETURNING id`), &sbi)
	if _, err := f.cases.Create(ctx, f.actor, cases.NewCase{ProjectID: naProjectID(t, f.pool), CustomerID: f.customer, BankID: sbi, LoanAmount: "200000", AssigneeID: f.actor.ID}); err != nil {
		t.Fatalf("sbi case: %v", err)
	}
	now := time.Now()
	d, err := NewService(f.pool).Dashboard(ctx, now, Period{From: now.Add(-time.Hour), To: now.Add(time.Hour), BankID: &sbi})
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if d.NewCases != 1 || d.NewCasesToday != 1 || d.Sanctions.Count != 0 || d.StageMovesToday != 0 {
		t.Fatalf("SBI only: %+v", d)
	}
	if byCode(d)["received"] != 1 || byCode(d)["sanctioned"] != 0 {
		t.Fatalf("SBI by stage = %v", d.CasesByStage)
	}
	if d.NewCustomers != 1 {
		t.Fatalf("customers ignore the bank filter: got %d", d.NewCustomers)
	}
}
