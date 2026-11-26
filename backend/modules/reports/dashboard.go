// Package reports builds the dashboard and CSV exports (spec §9).
package reports

import (
	"context"
	"fmt"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/reports/reportsdb"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var istZone = time.FixedZone("IST", 5*60*60+30*60)

// Period is a half-open time range [From, To) for stage-reached counts.
type Period struct {
	From time.Time
	To   time.Time
	// BankID, when set, limits case counts to one bank. Customer counts ignore it.
	BankID *int64
	// ProjectID, when set, limits case counts to one project, like BankID.
	ProjectID *int64
}

// StageCount is the number of cases that reached a stage in a period, with
// the sum of their amounts. Dropped counts those now dropped (marked, still counted).
type StageCount struct {
	Count   int
	Amount  string
	Dropped int
}

// StageTotal is the number of open cases of one bank at one of its stages.
type StageTotal struct {
	BankID    *int64
	BankName  string
	StageID   int64
	StageCode string
	StageName string
	SortOrder int
	Total     int
}

// DashboardData is the first-version dashboard (spec §9).
type DashboardData struct {
	NewCustomersToday int
	NewCasesToday     int
	NewCustomers      int // created in the period
	NewCases          int // created in the period
	StageMovesToday   int // stage events today (IST), not counting the Received a case opens with
	CasesByStage      []StageTotal
	Sanctions         StageCount
	Disbursements     StageCount
}

// Service builds reports from the database.
type Service struct {
	pool *pgxpool.Pool
	q    *reportsdb.Queries
}

// NewService returns a reports service on pool.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: reportsdb.New(pool)}
}

// TodayIST returns the start (inclusive) and end (exclusive) of the India
// calendar day that contains now (spec §9, plan Task 27).
func TodayIST(now time.Time) (time.Time, time.Time) {
	local := now.In(istZone)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, istZone)
	return start, start.AddDate(0, 0, 1)
}

// Dashboard computes the counts for now and the period. "Today" is the IST day.
func (s *Service) Dashboard(ctx context.Context, now time.Time, p Period) (DashboardData, error) {
	start, end := TodayIST(now)
	var d DashboardData

	n, err := s.q.CountNewCustomers(ctx, reportsdb.CountNewCustomersParams{CreatedAt: ts(start), CreatedAt_2: ts(end)})
	if err != nil {
		return d, fmt.Errorf("reports: customers today: %w", err)
	}
	d.NewCustomersToday = int(n)

	bank := pgtype.Int8{}
	if p.BankID != nil {
		bank = pgtype.Int8{Int64: *p.BankID, Valid: true}
	}
	project := pgtype.Int8{}
	if p.ProjectID != nil {
		project = pgtype.Int8{Int64: *p.ProjectID, Valid: true}
	}

	n, err = s.q.CountNewCases(ctx, reportsdb.CountNewCasesParams{FromTs: ts(start), ToTs: ts(end), BankID: bank, ProjectID: project})
	if err != nil {
		return d, fmt.Errorf("reports: cases today: %w", err)
	}
	d.NewCasesToday = int(n)

	n, err = s.q.CountNewCustomers(ctx, reportsdb.CountNewCustomersParams{CreatedAt: ts(p.From), CreatedAt_2: ts(p.To)})
	if err != nil {
		return d, fmt.Errorf("reports: customers in period: %w", err)
	}
	d.NewCustomers = int(n)

	n, err = s.q.CountNewCases(ctx, reportsdb.CountNewCasesParams{FromTs: ts(p.From), ToTs: ts(p.To), BankID: bank, ProjectID: project})
	if err != nil {
		return d, fmt.Errorf("reports: cases in period: %w", err)
	}
	d.NewCases = int(n)

	n, err = s.q.CountStageMoves(ctx, reportsdb.CountStageMovesParams{FromTs: ts(start), ToTs: ts(end), BankID: bank, ProjectID: project})
	if err != nil {
		return d, fmt.Errorf("reports: stage moves today: %w", err)
	}
	d.StageMovesToday = int(n)

	rows, err := s.q.CasesByStage(ctx, reportsdb.CasesByStageParams{BankID: bank, ProjectID: project})
	if err != nil {
		return d, fmt.Errorf("reports: by stage: %w", err)
	}
	d.CasesByStage = make([]StageTotal, 0, len(rows))
	for _, r := range rows {
		id := r.BankID
		d.CasesByStage = append(d.CasesByStage, StageTotal{BankID: &id, BankName: r.BankName, StageID: r.StageID,
			StageCode: r.StageCode, StageName: r.StageName, SortOrder: int(r.SortOrder), Total: int(r.Total)})
	}

	// Sanctions count the stage with the sanction role, whatever the bank calls it.
	sh, err := s.q.SanctionHits(ctx, reportsdb.SanctionHitsParams{FromTs: ts(p.From), ToTs: ts(p.To), BankID: bank, ProjectID: project})
	if err != nil {
		return d, fmt.Errorf("reports: sanctions: %w", err)
	}
	d.Sanctions = StageCount{Count: int(sh.Cases), Amount: sh.Amount, Dropped: int(sh.Dropped)}
	// Disbursements count cases paid in the period and the money paid in it.
	dh, err := s.q.DisbursementHits(ctx, reportsdb.DisbursementHitsParams{FromTs: ts(p.From), ToTs: ts(p.To), BankID: bank, ProjectID: project})
	if err != nil {
		return d, fmt.Errorf("reports: disbursements: %w", err)
	}
	d.Disbursements = StageCount{Count: int(dh.Cases), Amount: dh.Amount, Dropped: int(dh.Dropped)}
	return d, nil
}

func ts(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}
