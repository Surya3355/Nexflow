package documents

import (
	"context"
	"fmt"
)

// RequirementWarning is shown when a stage needs documents and has none (round 12).
const RequirementWarning = "This stage normally needs at least one document. None is uploaded yet."

// Status is the document requirement of a case's current stage.
// A missing document is a warning only, never a block (round 12).
type Status struct {
	Required  bool
	Satisfied bool
	Warning   string
}

// RequirementStatus reports whether the case's current stage requires
// documents, and whether the case has at least one (spec §5, round 11-12).
func (s *Service) RequirementStatus(ctx context.Context, caseID int64) (Status, error) {
	row, err := s.q.GetCaseRequirement(ctx, caseID)
	if err != nil {
		return Status{}, fmt.Errorf("documents: requirement: %w", err)
	}
	st := Status{Required: row.RequiresDocument, Satisfied: row.DocumentCount > 0}
	if st.Required && !st.Satisfied {
		st.Warning = RequirementWarning
	}
	return st, nil
}
