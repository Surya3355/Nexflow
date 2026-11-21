package cases

import (
	"context"
	"fmt"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/cases/casesdb"
)

// StageEvent is one dated stage move, in the order it happened (spec §4.3).
type StageEvent struct {
	ID        int64
	ReachedAt time.Time
	StageCode string
	StageName string
	ChangedBy string
}

// History returns the stage events of a case, oldest first. Earlier events are never changed.
func (s *Service) History(ctx context.Context, caseID int64) ([]StageEvent, error) {
	rows, err := s.q.CaseStageHistory(ctx, caseID)
	if err != nil {
		return nil, fmt.Errorf("cases: history: %w", err)
	}
	out := make([]StageEvent, 0, len(rows))
	for _, r := range rows {
		out = append(out, StageEvent{
			ID: r.ID, ReachedAt: r.ReachedAt.Time, StageCode: r.StageCode,
			StageName: r.StageName, ChangedBy: r.ChangedByName,
		})
	}
	return out, nil
}

// ByCustomer returns all cases of a customer, oldest first.
func (s *Service) ByCustomer(ctx context.Context, customerID int64) ([]Case, error) {
	rows, err := s.q.CasesForCustomer(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("cases: by customer: %w", err)
	}
	out := make([]Case, 0, len(rows))
	for _, r := range rows {
		out = append(out, toCase(casesdb.GetCaseRow(r)))
	}
	return out, nil
}
