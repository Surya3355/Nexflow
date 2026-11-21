package reports

import (
	"context"
	"fmt"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/reports/reportsdb"
	"github.com/jackc/pgx/v5/pgtype"
)

// RowFilter is the report filter, the same as the case list (spec §8).
type RowFilter struct {
	StageID    *int64
	BankID     *int64
	ProjectID  *int64
	AssigneeID *int64
	From       *time.Time
	To         *time.Time
}

// Rows returns the report rows for the filter. Every staff role may view and
// export (spec §6). The on-screen report and the CSV both use this function.
func (s *Service) Rows(ctx context.Context, actor access.User, f RowFilter) ([]Row, error) {
	if !access.Can(actor.Role, access.ActionViewReports) {
		return nil, access.ErrForbidden
	}
	rows, err := s.q.ReportRows(ctx, reportsdb.ReportRowsParams{
		StageID: int64Null(f.StageID), BankID: int64Null(f.BankID), ProjectID: int64Null(f.ProjectID), AssigneeID: int64Null(f.AssigneeID),
		FromTs: timeNull(f.From), ToTs: timeNull(f.To),
	})
	if err != nil {
		return nil, fmt.Errorf("reports: rows: %w", err)
	}
	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		row := Row{
			CaseID: r.ID, CustomerName: r.CustomerName, Mobile: r.Mobile, Bank: r.BankName, Project: r.ProjectName,
			Stage: r.StageName, Status: r.Status, Assignee: r.AssigneeName, LoanAmount: r.LoanAmount,
			Tracking: r.TrackingNumber, CreatedOn: istDay(r.CreatedAt.Time),
		}
		if r.FollowUpDate.Valid {
			row.FollowUpOn = r.FollowUpDate.Time.Format("2006-01-02")
		}
		out = append(out, row)
	}
	return out, nil
}

func istDay(t time.Time) string { return t.In(istZone).Format("2006-01-02") }

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
