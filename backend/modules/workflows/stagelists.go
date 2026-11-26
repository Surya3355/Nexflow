package workflows

import (
	"context"
	"errors"
	"fmt"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/Surya3355/nexflow/backend/modules/workflows/workflowsdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Stage roles: what a stage does, whatever its name (spec 2026-11-25 §4 A).
const (
	RoleStart        = "start"        // new cases land here
	RoleTracking     = "tracking"     // the bank's application number is required to reach it
	RoleSanction     = "sanction"     // the sanction form; counted as a sanction in reports
	RoleDisbursement = "disbursement" // payments; a Full payment completes the case
)

// Stage lists a bank can use.
const (
	StageListStandard = "standard"
	StageListHDFC     = "hdfc"
)

// Errors for bank stage-list changes.
var (
	ErrInvalidStageList = errors.New("workflows: unknown stage list")
	ErrBankHasOwnStages = errors.New("workflows: this bank already has its own stages")
)

// Officer teams: which bank officer a stage asks staff to pick (spec 2026-11-26 §5.1).
const (
	TeamTDEP      = "tdep"
	TeamDOVR      = "dovr"
	TeamAppraisal = "appraisal"
	TeamFixing    = "fixing"
	TeamOfficer   = "officer" // the one officer of a standard-list bank (SBI)
)

var teamNames = map[string]string{
	TeamTDEP: "TDEP", TeamDOVR: "DOVR", TeamAppraisal: "Appraisal", TeamFixing: "Fixing", TeamOfficer: "Officer",
}

// TeamName is the display name of an officer team, or "" for an unknown code.
func TeamName(code string) string { return teamNames[code] }

type templateStage struct{ code, name, role, team string }

// hdfcTemplate is HDFC's stage list, approved by the owner on 2026-11-25.
var hdfcTemplate = []templateStage{
	{"received", "Received", "", ""},
	{"link_sent", "Link sent", RoleStart, TeamTDEP},
	{"application_submitted", "Application submitted", RoleTracking, ""},
	{"dovr", "DOVR", "", TeamDOVR},
	{"appraisal", "Appraisal", "", TeamAppraisal},
	{"sanctioned", "Sanctioned", RoleSanction, ""},
	{"upload_documents", "Upload documents", "", ""},
	{"under_fixing", "Under fixing", "", TeamFixing},
	{"fixed", "Fixed", "", ""},
	{"signing", "Signing", "", ""},
	{"disbursement", "Disbursement", RoleDisbursement, ""},
}

// standardToHDFC moves a case on a standard stage to the nearest HDFC stage.
var standardToHDFC = map[string]string{
	"received":       "link_sent",
	"logged_in":      "application_submitted",
	"under_sanction": "dovr",
	"sanctioned":     "sanctioned",
	"documentation":  "upload_documents",
	"disbursed":      "disbursement",
}

// StageList is one list of stages for the admin Stages screen.
type StageList struct {
	BankID *int64 // nil for the standard list
	Name   string
	Stages []Stage
}

func stageListOf(hasOwn bool) string {
	if hasOwn {
		return StageListHDFC
	}
	return StageListStandard
}

// StageWithRole returns the stage that has role, if any.
func StageWithRole(stages []Stage, role string) (Stage, bool) {
	for _, st := range stages {
		if st.Role == role {
			return st, true
		}
	}
	return Stage{}, false
}

// SetBankStageList gives a bank HDFC's stage list. Its cases move to the
// nearest HDFC stage; their history keeps the old stages. Part payments are
// switched on. Admin only; there is no way back to the standard list.
func (s *Service) SetBankStageList(ctx context.Context, actor access.User, bankID int64, list string) error {
	if !access.Can(actor.Role, access.ActionManageConfig) {
		return access.ErrForbidden
	}
	if list != StageListHDFC {
		return ErrInvalidStageList
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("workflows: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	bank, err := q.LockBank(ctx, bankID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrBankNotFound
	}
	if err != nil {
		return fmt.Errorf("workflows: lock bank: %w", err)
	}
	if bank.HasOwnStages {
		return ErrBankHasOwnStages
	}
	by := pgtype.Int8{Int64: actor.ID, Valid: true}
	newIDs := map[string]int64{}
	for i, st := range hdfcTemplate {
		id, err := q.InsertBankStage(ctx, workflowsdb.InsertBankStageParams{
			BankID: pgtype.Int8{Int64: bankID, Valid: true}, Code: st.code, Name: st.name,
			SortOrder: int32(i + 1), Role: pgtype.Text{String: st.role, Valid: st.role != ""},
			OfficerTeam: pgtype.Text{String: st.team, Valid: st.team != ""}, CreatedBy: by,
		})
		if err != nil {
			return fmt.Errorf("workflows: insert stage %s: %w", st.code, err)
		}
		newIDs[st.code] = id
	}
	std, err := q.ListStandardStages(ctx)
	if err != nil {
		return fmt.Errorf("workflows: standard stages: %w", err)
	}
	for _, st := range std {
		to, ok := newIDs[standardToHDFC[st.Code]]
		if !ok {
			to = newIDs["link_sent"] // a standard stage admin added later
		}
		if err := q.MoveBankCasesStage(ctx, workflowsdb.MoveBankCasesStageParams{
			ToStageID: to, UpdatedBy: by, BankID: bankID, FromStageID: st.ID,
		}); err != nil {
			return fmt.Errorf("workflows: move cases: %w", err)
		}
	}
	if _, err := q.SetBankPartPayments(ctx, workflowsdb.SetBankPartPaymentsParams{ID: bankID, PartPayments: true, UpdatedBy: by}); err != nil {
		return fmt.Errorf("workflows: part payments: %w", err)
	}
	id := actor.ID
	if err := s.rec.Record(history.WithTx(ctx, tx), history.Entry{
		ActorUserID: &id, Action: "bank.stage_list_changed", EntityType: "bank", EntityID: &bankID,
		Details: map[string]any{"stage_list": list},
	}); err != nil {
		return fmt.Errorf("workflows: audit: %w", err)
	}
	return tx.Commit(ctx)
}

// SetBankPartPayments switches part payments on or off for a bank. Admin only.
func (s *Service) SetBankPartPayments(ctx context.Context, actor access.User, bankID int64, on bool) error {
	if !access.Can(actor.Role, access.ActionManageConfig) {
		return access.ErrForbidden
	}
	n, err := s.q.SetBankPartPayments(ctx, workflowsdb.SetBankPartPaymentsParams{
		ID: bankID, PartPayments: on, UpdatedBy: pgtype.Int8{Int64: actor.ID, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("workflows: part payments: %w", err)
	}
	if n == 0 {
		return ErrBankNotFound
	}
	return s.audit(ctx, actor, "bank.part_payments_changed", "bank", bankID)
}

// BankPartPayments reports whether the bank pays loans in parts.
func (s *Service) BankPartPayments(ctx context.Context, bankID int64) (bool, error) {
	on, err := s.q.GetBankPartPayments(ctx, bankID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrBankNotFound
	}
	if err != nil {
		return false, fmt.Errorf("workflows: part payments: %w", err)
	}
	return on, nil
}

// StageLists returns the standard list, then each bank that has its own list.
func (s *Service) StageLists(ctx context.Context) ([]StageList, error) {
	std, err := s.StandardStages(ctx)
	if err != nil {
		return nil, err
	}
	out := []StageList{{Name: "Standard stages", Stages: std}}
	banks, err := s.q.ListBanksWithOwnStages(ctx)
	if err != nil {
		return nil, fmt.Errorf("workflows: banks with stages: %w", err)
	}
	for _, b := range banks {
		stages, err := s.StagesFor(ctx, b.ID)
		if err != nil {
			return nil, err
		}
		id := b.ID
		out = append(out, StageList{BankID: &id, Name: b.Name, Stages: stages})
	}
	return out, nil
}
