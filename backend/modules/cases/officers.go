package cases

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/cases/casesdb"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/Surya3355/nexflow/backend/modules/workflows"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Officer pick errors (spec 2026-11-26 §5.5).
var (
	ErrOfficerInvalid  = errors.New("cases: pick a current officer of this bank and team")
	ErrTeamNotOnStages = errors.New("cases: this case's bank stages have no such officer team")
)

// Officer is the bank officer a case points to for one team.
type Officer struct {
	ID      int64
	Name    string
	Phone   string // "" when the officer has only an email
	Email   string // "" when the officer has only a mobile
	Removed bool // taken off the bank's list; the case keeps them
}

// CaseOfficer is one team the case has reached and its officer (nil = not set).
type CaseOfficer struct {
	Team     string
	TeamName string
	Officer  *Officer
}

// OfficerEvent is one officer pick, for the case history.
type OfficerEvent struct {
	At          time.Time
	Team        string
	TeamName    string
	OfficerName string
	ChangedBy   string
}

// pickOfficerTx records officerID as the case's officer for team, inside the
// caller's transaction, dated at (the same time as the move it comes with). The officer must be current and of the case's bank
// and that team. Picking the current officer again records nothing.
func (s *Service) pickOfficerTx(ctx context.Context, q *casesdb.Queries, actor access.User, caseID, bankID int64, team string, officerID int64, at time.Time) error {
	if officerID <= 0 {
		return ErrOfficerInvalid
	}
	o, err := q.GetBankOfficerForPick(ctx, officerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrOfficerInvalid
	}
	if err != nil {
		return fmt.Errorf("cases: officer: %w", err)
	}
	if o.Removed || o.BankID != bankID || o.Team != team {
		return ErrOfficerInvalid
	}
	var from any
	cur, err := q.LatestCaseOfficer(ctx, casesdb.LatestCaseOfficerParams{CaseID: caseID, Team: team})
	switch {
	case err == nil && cur == officerID:
		return nil
	case err == nil:
		from = cur
	case !errors.Is(err, pgx.ErrNoRows):
		return fmt.Errorf("cases: current officer: %w", err)
	}
	if err := q.InsertCaseOfficer(ctx, casesdb.InsertCaseOfficerParams{CaseID: caseID, Team: team, OfficerID: officerID, PickedAt: pgtype.Timestamptz{Time: at, Valid: true}, PickedBy: actor.ID}); err != nil {
		return fmt.Errorf("cases: pick officer: %w", err)
	}
	return s.audit(ctx, actor, "case.officer_changed", caseID, map[string]any{"team": team, "from": from, "to": officerID})
}

// SetOfficer changes the case's officer for one team of its bank's stages.
// Whoever can edit the case may do this; a dropped case cannot change.
func (s *Service) SetOfficer(ctx context.Context, actor access.User, caseID int64, team string, officerID int64) (Case, error) {
	if !access.Can(actor.Role, access.ActionEditRecord) {
		return Case{}, access.ErrForbidden
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Case{}, fmt.Errorf("cases: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	txCtx := history.WithTx(ctx, tx)

	locked, err := q.LockCase(txCtx, caseID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Case{}, ErrCaseNotFound
	}
	if err != nil {
		return Case{}, fmt.Errorf("cases: lock: %w", err)
	}
	if locked.Status == "dropped" {
		return Case{}, ErrCaseDropped
	}
	stages, err := s.workflows.StagesFor(txCtx, locked.BankID)
	if err != nil {
		return Case{}, err
	}
	if !hasTeam(stages, team) {
		return Case{}, ErrTeamNotOnStages
	}
	if err := s.pickOfficerTx(txCtx, q, actor, caseID, locked.BankID, team, officerID, time.Now()); err != nil {
		return Case{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Case{}, fmt.Errorf("cases: commit: %w", err)
	}
	return s.load(ctx, caseID)
}

func hasTeam(stages []workflows.Stage, team string) bool {
	if team == "" {
		return false
	}
	for _, st := range stages {
		if st.OfficerTeam == team {
			return true
		}
	}
	return false
}

// Officers lists the teams of the case's stages that the case has reached
// (the team's stage is at or before the current one), in stage order.
func (s *Service) Officers(ctx context.Context, caseID int64) ([]CaseOfficer, error) {
	c, err := s.Get(ctx, caseID)
	if err != nil {
		return nil, err
	}
	stages, err := s.workflows.StagesFor(ctx, c.BankID)
	if err != nil {
		return nil, err
	}
	current := -1
	for _, st := range stages {
		if st.ID == c.CurrentStageID {
			current = st.SortOrder
		}
	}
	rows, err := s.q.CurrentCaseOfficers(ctx, caseID)
	if err != nil {
		return nil, fmt.Errorf("cases: officers: %w", err)
	}
	picked := map[string]*Officer{}
	for _, r := range rows {
		picked[r.Team] = &Officer{ID: r.ID, Name: r.Name, Phone: r.Phone, Email: r.Email, Removed: r.Removed}
	}
	out := []CaseOfficer{}
	for _, st := range stages {
		if st.OfficerTeam == "" || st.SortOrder > current {
			continue
		}
		out = append(out, CaseOfficer{Team: st.OfficerTeam, TeamName: workflows.TeamName(st.OfficerTeam), Officer: picked[st.OfficerTeam]})
	}
	return out, nil
}

// OfficerHistory returns every officer pick of a case, oldest first.
func (s *Service) OfficerHistory(ctx context.Context, caseID int64) ([]OfficerEvent, error) {
	rows, err := s.q.CaseOfficerHistory(ctx, caseID)
	if err != nil {
		return nil, fmt.Errorf("cases: officer history: %w", err)
	}
	out := make([]OfficerEvent, 0, len(rows))
	for _, r := range rows {
		out = append(out, OfficerEvent{At: r.PickedAt.Time, Team: r.Team, TeamName: workflows.TeamName(r.Team),
			OfficerName: r.OfficerName, ChangedBy: r.ChangedByName})
	}
	return out, nil
}
