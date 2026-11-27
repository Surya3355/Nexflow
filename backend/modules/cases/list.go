package cases

import (
	"context"
	"fmt"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/cases/casesdb"
	"github.com/jackc/pgx/v5/pgtype"
)

// ListFilter holds the optional list filters (spec §8). Nil means no filter.
// The date range applies to the created date.
type ListFilter struct {
	StageID    *int64
	BankID     *int64
	ProjectID  *int64
	AssigneeID *int64
	From       *time.Time
	To         *time.Time
	CustomerID *int64 // used by ListRows only
}

// List returns cases matching every filter that is set. Dropped cases appear
// with their status (spec §8, round 3).
func (s *Service) List(ctx context.Context, f ListFilter) ([]Case, error) {
	rows, err := s.q.ListCases(ctx, casesdb.ListCasesParams{
		StageID:    int64Null(f.StageID),
		BankID:     int64Null(f.BankID),
		ProjectID:  int64Null(f.ProjectID),
		AssigneeID: int64Null(f.AssigneeID),
		FromTs:     timeNull(f.From),
		ToTs:       timeNull(f.To),
	})
	if err != nil {
		return nil, fmt.Errorf("cases: list: %w", err)
	}
	out := make([]Case, 0, len(rows))
	for _, r := range rows {
		out = append(out, toCase(casesdb.GetCaseRow(r)))
	}
	return out, nil
}

func int64Null(p *int64) pgtype.Int8 {
	if p == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *p, Valid: true}
}

func timeNull(p *time.Time) pgtype.Timestamptz {
	if p == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *p, Valid: true}
}

// CaseRow is a case with the names and stage progress the case list shows.
type CaseRow struct {
	Case
	CustomerName   string
	CustomerMobile string
	BankName       string
	ProjectName    string
	StageName      string
	StageCode      string
	StageOrder     int
	StageCount     int // stages in the bank's list (own stages, else standard)
	AssigneeName   string
	StageSince     time.Time // when the current stage was reached
	NeedsDocument  bool      // current stage requires a document and the case has none
	OpenQueries    int       // bank queries not yet resolved
}

// ListRows is List with names and stage progress, for the case list screen.
func (s *Service) ListRows(ctx context.Context, f ListFilter) ([]CaseRow, error) {
	rows, err := s.q.ListCaseRows(ctx, casesdb.ListCaseRowsParams{
		StageID:    int64Null(f.StageID),
		BankID:     int64Null(f.BankID),
		ProjectID:  int64Null(f.ProjectID),
		AssigneeID: int64Null(f.AssigneeID),
		FromTs:     timeNull(f.From),
		ToTs:       timeNull(f.To),
		CustomerID: int64Null(f.CustomerID),
	})
	if err != nil {
		return nil, fmt.Errorf("cases: list rows: %w", err)
	}
	out := make([]CaseRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, toCaseRow(r))
	}
	return out, nil
}

func toCaseRow(r casesdb.ListCaseRowsRow) CaseRow {
	return CaseRow{
		Case: toCase(casesdb.GetCaseRow{
			ID: r.ID, CustomerID: r.CustomerID, BankID: r.BankID, LoanAmount: r.LoanAmount,
			TrackingNumber: r.TrackingNumber, CurrentStageID: r.CurrentStageID, AssigneeID: r.AssigneeID,
			Status: r.Status, CreatedAt: r.CreatedAt, FollowUpDate: r.FollowUpDate, ProjectID: r.ProjectID,
			LeadID: r.LeadID, UnitNumber: r.UnitNumber, BuilderContactID: r.BuilderContactID,
		}),
		CustomerName: r.CustomerName, CustomerMobile: r.CustomerMobile, BankName: r.BankName, ProjectName: r.ProjectName,
		StageName: r.StageName, StageCode: r.StageCode, StageOrder: int(r.StageOrder), StageCount: int(r.StageCount),
		AssigneeName: r.AssigneeName, StageSince: r.StageSince.Time, NeedsDocument: r.NeedsDocument,
		OpenQueries: int(r.OpenQueries),
	}
}
