// Package workflows holds the admin-maintained configuration: banks, stages
// and dropdown values. Stages and dropdowns are data, not code (agent.md §3).
package workflows

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/Surya3355/nexflow/backend/modules/workflows/workflowsdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DropReasonCategory is the dropdown category for dropped reasons (spec §4.2).
const DropReasonCategory = "drop_reason"

// Errors returned when renaming a bank.
var (
	ErrBankNameRequired = errors.New("workflows: bank name is required")
	ErrBankNameTaken    = errors.New("workflows: a bank with this name already exists")
	ErrBankNotFound     = errors.New("workflows: bank not found")
)

// Bank is a lender (HDFC, SBI, and others).
type Bank struct {
	ID           int64
	Name         string
	IsActive     bool
	HasOwnStages bool   // false means the bank uses the standard stages
	PartPayments bool   // the bank pays some loans in parts (spec 2026-11-25 §5.2)
	StageList    string // StageListStandard or StageListHDFC
}

// Stage is one step of the loan flow. BankID nil means a standard stage.
type Stage struct {
	ID               int64
	BankID           *int64
	Code             string
	Name             string
	SortOrder        int
	RequiresDocument bool
	Role             string // "" or one of the Role constants
}

// DropdownValue is one admin-maintained choice, such as a drop reason.
type DropdownValue struct {
	ID        int64
	Category  string
	Value     string
	SortOrder int
}

// Service runs configuration reads and admin-only changes.
type Service struct {
	pool *pgxpool.Pool
	q    *workflowsdb.Queries
	rec  history.Recorder
}

// NewService returns a workflows service on pool.
func NewService(pool *pgxpool.Pool, rec history.Recorder) *Service {
	return &Service{pool: pool, q: workflowsdb.New(pool), rec: rec}
}

// ListBanks returns the active banks, by name.
func (s *Service) ListBanks(ctx context.Context) ([]Bank, error) {
	rows, err := s.q.ListBanks(ctx)
	if err != nil {
		return nil, fmt.Errorf("workflows: list banks: %w", err)
	}
	out := make([]Bank, 0, len(rows))
	for _, r := range rows {
		out = append(out, Bank{ID: r.ID, Name: r.Name, IsActive: r.IsActive, HasOwnStages: r.HasOwnStages,
			PartPayments: r.PartPayments, StageList: stageListOf(r.HasOwnStages)})
	}
	return out, nil
}

// CreateBank adds a bank. Admin only.
func (s *Service) CreateBank(ctx context.Context, actor access.User, name string) (Bank, error) {
	if !access.Can(actor.Role, access.ActionManageConfig) {
		return Bank{}, access.ErrForbidden
	}
	row, err := s.q.InsertBank(ctx, workflowsdb.InsertBankParams{
		Name:      name,
		CreatedBy: pgtype.Int8{Int64: actor.ID, Valid: true},
	})
	if err != nil {
		return Bank{}, fmt.Errorf("workflows: insert bank: %w", err)
	}
	b := Bank{ID: row.ID, Name: row.Name, IsActive: row.IsActive, PartPayments: row.PartPayments, StageList: StageListStandard}
	if err := s.audit(ctx, actor, "bank.created", "bank", b.ID); err != nil {
		return Bank{}, err
	}
	return b, nil
}

// RenameBank changes a bank's name. Admin only. Names are trimmed and unique.
func (s *Service) RenameBank(ctx context.Context, actor access.User, id int64, name string) (Bank, error) {
	if !access.Can(actor.Role, access.ActionManageConfig) {
		return Bank{}, access.ErrForbidden
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Bank{}, ErrBankNameRequired
	}
	row, err := s.q.RenameBank(ctx, workflowsdb.RenameBankParams{
		ID: id, Name: name, UpdatedBy: pgtype.Int8{Int64: actor.ID, Valid: true},
	})
	var pgErr *pgconn.PgError
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Bank{}, ErrBankNotFound
	case errors.As(err, &pgErr) && pgErr.Code == "23505":
		return Bank{}, ErrBankNameTaken
	case err != nil:
		return Bank{}, fmt.Errorf("workflows: rename bank: %w", err)
	}
	b := Bank{ID: row.ID, Name: row.Name, IsActive: row.IsActive, PartPayments: row.PartPayments}
	if err := s.audit(ctx, actor, "bank.renamed", "bank", b.ID); err != nil {
		return Bank{}, err
	}
	return b, nil
}

// StagesFor returns the bank's own active stages, or the standard stages when
// the bank has none (spec §5, round 7).
func (s *Service) StagesFor(ctx context.Context, bankID int64) ([]Stage, error) {
	rows, err := s.q.ListStagesForBank(ctx, pgtype.Int8{Int64: bankID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("workflows: stages for bank: %w", err)
	}
	if len(rows) == 0 {
		std, err := s.q.ListStandardStages(ctx)
		if err != nil {
			return nil, fmt.Errorf("workflows: standard stages: %w", err)
		}
		out := make([]Stage, 0, len(std))
		for _, r := range std {
			out = append(out, Stage{ID: r.ID, Code: r.Code, Name: r.Name,
				SortOrder: int(r.SortOrder), RequiresDocument: r.RequiresDocument, Role: r.Role})
		}
		return out, nil
	}
	out := make([]Stage, 0, len(rows))
	for _, r := range rows {
		id := r.BankID.Int64
		out = append(out, Stage{ID: r.ID, BankID: &id, Code: r.Code, Name: r.Name,
			SortOrder: int(r.SortOrder), RequiresDocument: r.RequiresDocument, Role: r.Role})
	}
	return out, nil
}

// AddStage adds a stage. bankID nil adds a standard stage. Admin only.
func (s *Service) AddStage(ctx context.Context, actor access.User, bankID *int64, code, name string, sortOrder int, requiresDocument bool) (Stage, error) {
	if !access.Can(actor.Role, access.ActionManageConfig) {
		return Stage{}, access.ErrForbidden
	}
	bank := pgtype.Int8{}
	if bankID != nil {
		bank = pgtype.Int8{Int64: *bankID, Valid: true}
	}
	row, err := s.q.InsertStage(ctx, workflowsdb.InsertStageParams{
		BankID:           bank,
		Code:             code,
		Name:             name,
		SortOrder:        int32(sortOrder),
		RequiresDocument: requiresDocument,
		CreatedBy:        pgtype.Int8{Int64: actor.ID, Valid: true},
	})
	if err != nil {
		return Stage{}, fmt.Errorf("workflows: insert stage: %w", err)
	}
	st := Stage{ID: row.ID, Code: row.Code, Name: row.Name, SortOrder: int(row.SortOrder), RequiresDocument: row.RequiresDocument}
	if row.BankID.Valid {
		id := row.BankID.Int64
		st.BankID = &id
	}
	if err := s.audit(ctx, actor, "stage.created", "stage", st.ID); err != nil {
		return Stage{}, err
	}
	return st, nil
}

// SetStageRequiresDocument marks a stage as needing documents (round 11). Admin only.
func (s *Service) SetStageRequiresDocument(ctx context.Context, actor access.User, stageID int64, required bool) error {
	if !access.Can(actor.Role, access.ActionManageConfig) {
		return access.ErrForbidden
	}
	if _, err := s.q.SetStageRequiresDocument(ctx, workflowsdb.SetStageRequiresDocumentParams{
		ID:               stageID,
		RequiresDocument: required,
		UpdatedBy:        pgtype.Int8{Int64: actor.ID, Valid: true},
	}); err != nil {
		return fmt.Errorf("workflows: set requires_document: %w", err)
	}
	if err := s.audit(ctx, actor, "stage.requires_document_changed", "stage", stageID); err != nil {
		return err
	}
	return nil
}

// DropReasons returns the active dropped-reason list, in admin order.
func (s *Service) DropReasons(ctx context.Context) ([]DropdownValue, error) {
	return s.dropdown(ctx, DropReasonCategory)
}

// AddDropdownValue adds a value to a dropdown category. Admin only.
func (s *Service) AddDropdownValue(ctx context.Context, actor access.User, category, value string, sortOrder int) (DropdownValue, error) {
	if !access.Can(actor.Role, access.ActionManageConfig) {
		return DropdownValue{}, access.ErrForbidden
	}
	row, err := s.q.InsertDropdownValue(ctx, workflowsdb.InsertDropdownValueParams{
		Category:  category,
		Value:     value,
		SortOrder: int32(sortOrder),
		CreatedBy: pgtype.Int8{Int64: actor.ID, Valid: true},
	})
	if err != nil {
		return DropdownValue{}, fmt.Errorf("workflows: insert dropdown value: %w", err)
	}
	if err := s.audit(ctx, actor, "dropdown.created", "dropdown_value", row.ID); err != nil {
		return DropdownValue{}, err
	}
	return DropdownValue{ID: row.ID, Category: row.Category, Value: row.Value, SortOrder: int(row.SortOrder)}, nil
}

// Dropdown returns the active values of a dropdown category, in admin order.
func (s *Service) Dropdown(ctx context.Context, category string) ([]DropdownValue, error) {
	return s.dropdown(ctx, category)
}

func (s *Service) dropdown(ctx context.Context, category string) ([]DropdownValue, error) {
	rows, err := s.q.ListDropdownValues(ctx, category)
	if err != nil {
		return nil, fmt.Errorf("workflows: dropdown %s: %w", category, err)
	}
	out := make([]DropdownValue, 0, len(rows))
	for _, r := range rows {
		out = append(out, DropdownValue{ID: r.ID, Category: r.Category, Value: r.Value, SortOrder: int(r.SortOrder)})
	}
	return out, nil
}

func (s *Service) audit(ctx context.Context, actor access.User, action, entityType string, entityID int64) error {
	id := actor.ID
	eid := entityID
	if err := s.rec.Record(ctx, history.Entry{ActorUserID: &id, Action: action, EntityType: entityType, EntityID: &eid}); err != nil {
		return fmt.Errorf("workflows: audit %s: %w", action, err)
	}
	return nil
}

// StandardStages returns the standard stages every bank without its own stages uses.
func (s *Service) StandardStages(ctx context.Context) ([]Stage, error) {
	std, err := s.q.ListStandardStages(ctx)
	if err != nil {
		return nil, fmt.Errorf("workflows: standard stages: %w", err)
	}
	out := make([]Stage, 0, len(std))
	for _, r := range std {
		out = append(out, Stage{ID: r.ID, Code: r.Code, Name: r.Name,
			SortOrder: int(r.SortOrder), RequiresDocument: r.RequiresDocument, Role: r.Role})
	}
	return out, nil
}
