package cases

import (
	"context"
	"fmt"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/cases/casesdb"
	"github.com/jackc/pgx/v5/pgtype"
)

// StuckAfter is how long a case can stay in one stage before it needs attention
// (UI redesign spec §6, owner confirmed 7 days).
const StuckAfter = 7 * 24 * time.Hour

// AttentionItem is a case that needs attention, with a one-line reason.
type AttentionItem struct {
	CaseRow
	Reason string
}

// Attention lists active cases that need attention, oldest stage first.
// A nil assigneeID means every assignee. Open bank queries outrank a missing
// document, which outranks a long wait.
func (s *Service) Attention(ctx context.Context, assigneeID *int64, now time.Time) ([]AttentionItem, error) {
	rows, err := s.q.AttentionCases(ctx, casesdb.AttentionCasesParams{
		AssigneeID:  int64Null(assigneeID),
		StuckBefore: pgtype.Timestamptz{Time: now.Add(-StuckAfter), Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("cases: attention: %w", err)
	}
	out := make([]AttentionItem, 0, len(rows))
	for _, r := range rows {
		row := toCaseRow(casesdb.ListCaseRowsRow(r))
		reason := "No documents at this stage"
		switch {
		case row.OpenQueries == 1:
			reason = "1 open bank query"
		case row.OpenQueries > 1:
			reason = fmt.Sprintf("%d open bank queries", row.OpenQueries)
		case !row.NeedsDocument:
			days := int(now.Sub(row.StageSince) / (24 * time.Hour))
			reason = fmt.Sprintf("In %s for %d days", row.StageName, days)
		}
		out = append(out, AttentionItem{CaseRow: row, Reason: reason})
	}
	return out, nil
}
