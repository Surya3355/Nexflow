package workflows

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/customers"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/Surya3355/nexflow/backend/modules/workflows/workflowsdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Project errors (specs 2026-11-21 §4.2 and 2026-11-24 §4.2).
var (
	ErrProjectNameRequired    = errors.New("workflows: project name is required")
	ErrProjectNameTaken       = errors.New("workflows: a project with this name already exists")
	ErrProjectNotFound        = errors.New("workflows: project not found")
	ErrProjectLocked          = errors.New("workflows: the NA project cannot be changed")
	ErrProjectTypeRequired    = errors.New("workflows: choose a project type")
	ErrProjectAddressRequired = errors.New("workflows: enter the project address")
	ErrProjectContactRequired = errors.New("workflows: add at least one contact")
	ErrContactNameRequired    = errors.New("workflows: contact name is required")
	ErrContactLabelTooLong    = errors.New("workflows: contact label can be up to 40 characters")
	ErrContactPhoneTaken      = errors.New("workflows: this number is already a contact on this project")
	ErrContactNotFound        = errors.New("workflows: contact not found on this project")
)

// ProjectTypes are the fixed project types, in the order the owner gave.
var ProjectTypes = []string{"high_rise", "apartments", "plots", "villas"}

const maxContactLabel = 40

// Project is a builder project that cases belong to. NA (IsSystem) is the
// fixed entry for loans without a builder project.
type Project struct {
	ID             int64
	Name           string
	IsSystem       bool
	OpenCases      int64
	TotalCases     int64
	Type           *string // nil for NA and for projects made before round 15
	Address        string
	ActiveContacts int64
	// Contacts holds the active contacts by id. Filled by GetProject,
	// CreateProject and SaveProject only.
	Contacts []ProjectContact
}

// ProjectContact is one person or number at the builder's office.
type ProjectContact struct {
	ID                        int64
	Name, Phone, Email, Label string
	RemovedAt                 *time.Time
}

// ProjectInput is the whole project form: details plus the full list of
// contacts to keep. A saved contact left out of Contacts is removed.
type ProjectInput struct {
	Name, Type, Address string
	Contacts            []ContactInput
}

// ContactInput is one contact row of the form. ID 0 is a new contact.
type ContactInput struct {
	ID                        int64
	Name, Phone, Email, Label string
}

// ListProjects returns every project, NA first and then by name, with case
// and active-contact counts.
func (s *Service) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := s.q.ListProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("workflows: list projects: %w", err)
	}
	out := make([]Project, 0, len(rows))
	for _, r := range rows {
		out = append(out, Project{
			ID: r.ID, Name: r.Name, IsSystem: r.IsSystem, OpenCases: r.OpenCases, TotalCases: r.TotalCases,
			Type: textPtr(r.Type), Address: r.Address, ActiveContacts: r.ActiveContacts,
		})
	}
	return out, nil
}

// GetProject returns one project with its active contacts.
func (s *Service) GetProject(ctx context.Context, id int64) (Project, error) {
	return s.getProject(ctx, s.q, id)
}

func (s *Service) getProject(ctx context.Context, q *workflowsdb.Queries, id int64) (Project, error) {
	row, err := q.GetProject(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Project{}, ErrProjectNotFound
	}
	if err != nil {
		return Project{}, fmt.Errorf("workflows: get project: %w", err)
	}
	cs, err := q.ListActiveProjectContacts(ctx, id)
	if err != nil {
		return Project{}, fmt.Errorf("workflows: list contacts: %w", err)
	}
	p := Project{ID: row.ID, Name: row.Name, IsSystem: row.IsSystem, Type: textPtr(row.Type), Address: row.Address,
		ActiveContacts: int64(len(cs)), Contacts: make([]ProjectContact, 0, len(cs))}
	for _, c := range cs {
		p.Contacts = append(p.Contacts, ProjectContact{ID: c.ID, Name: c.Name, Phone: c.Phone, Email: c.Email.String, Label: c.Label.String})
	}
	return p, nil
}

// CreateProject adds a complete project with its contacts. Admin only.
func (s *Service) CreateProject(ctx context.Context, actor access.User, in ProjectInput) (Project, error) {
	if !access.Can(actor.Role, access.ActionManageConfig) {
		return Project{}, access.ErrForbidden
	}
	in, err := cleanProject(in)
	if err != nil {
		return Project{}, err
	}
	for _, c := range in.Contacts {
		if c.ID != 0 {
			return Project{}, ErrContactNotFound
		}
	}
	var id int64
	err = s.inTx(ctx, func(txCtx context.Context, q *workflowsdb.Queries) error {
		row, err := q.InsertProject(txCtx, workflowsdb.InsertProjectParams{
			Name: in.Name, Type: pgtype.Text{String: in.Type, Valid: true}, Address: in.Address,
			CreatedBy: pgtype.Int8{Int64: actor.ID, Valid: true},
		})
		if err != nil {
			return projectWriteErr("insert project", err)
		}
		id = row.ID
		if err := s.applyContacts(txCtx, q, actor, id, nil, in.Contacts); err != nil {
			return err
		}
		return s.audit(txCtx, actor, "project.created", "project", id)
	})
	if err != nil {
		return Project{}, err
	}
	return s.GetProject(ctx, id)
}

// SaveProject replaces a project's details and contact list in one
// transaction. Admin only; NA is locked. Every save must leave the project
// complete, including old projects that had no details yet. The project row
// is locked, so two admins saving at once apply one after the other.
func (s *Service) SaveProject(ctx context.Context, actor access.User, id int64, in ProjectInput) (Project, error) {
	if !access.Can(actor.Role, access.ActionManageConfig) {
		return Project{}, access.ErrForbidden
	}
	err := s.inTx(ctx, func(txCtx context.Context, q *workflowsdb.Queries) error {
		cur, err := q.LockProject(txCtx, id)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return ErrProjectNotFound
		case err != nil:
			return fmt.Errorf("workflows: lock project: %w", err)
		case cur.IsSystem:
			return ErrProjectLocked
		}
		clean, err := cleanProject(in)
		if err != nil {
			return err
		}
		active, err := q.ListActiveProjectContacts(txCtx, id)
		if err != nil {
			return fmt.Errorf("workflows: list contacts: %w", err)
		}
		ids := make([]int64, 0, len(active))
		for _, c := range active {
			ids = append(ids, c.ID)
		}
		for _, c := range clean.Contacts {
			if c.ID != 0 && !slices.Contains(ids, c.ID) {
				return ErrContactNotFound
			}
		}
		if _, err := q.UpdateProject(txCtx, workflowsdb.UpdateProjectParams{
			ID: id, Name: clean.Name, Type: pgtype.Text{String: clean.Type, Valid: true}, Address: clean.Address,
			UpdatedBy: pgtype.Int8{Int64: actor.ID, Valid: true},
		}); err != nil {
			return projectWriteErr("update project", err)
		}
		if err := s.applyContacts(txCtx, q, actor, id, ids, clean.Contacts); err != nil {
			return err
		}
		return s.audit(txCtx, actor, "project.updated", "project", id)
	})
	if err != nil {
		return Project{}, err
	}
	return s.GetProject(ctx, id)
}

// applyContacts removes the active contacts missing from want, then updates
// the kept ones, then inserts the new ones. Removing first lets a removed
// number be added again in the same save.
func (s *Service) applyContacts(ctx context.Context, q *workflowsdb.Queries, actor access.User, projectID int64, active []int64, want []ContactInput) error {
	by := pgtype.Int8{Int64: actor.ID, Valid: true}
	for _, id := range active {
		if !slices.ContainsFunc(want, func(c ContactInput) bool { return c.ID == id }) {
			if err := q.RemoveProjectContact(ctx, workflowsdb.RemoveProjectContactParams{ID: id, ProjectID: projectID, UpdatedBy: by}); err != nil {
				return fmt.Errorf("workflows: remove contact: %w", err)
			}
		}
	}
	for _, c := range want {
		if c.ID == 0 {
			continue
		}
		if err := q.UpdateProjectContact(ctx, workflowsdb.UpdateProjectContactParams{
			ID: c.ID, ProjectID: projectID, Name: c.Name, Phone: c.Phone, Email: optText(c.Email), Label: optText(c.Label), UpdatedBy: by,
		}); err != nil {
			return projectWriteErr("update contact", err)
		}
	}
	for _, c := range want {
		if c.ID != 0 {
			continue
		}
		if err := q.InsertProjectContact(ctx, workflowsdb.InsertProjectContactParams{
			ProjectID: projectID, Name: c.Name, Phone: c.Phone, Email: optText(c.Email), Label: optText(c.Label), CreatedBy: by,
		}); err != nil {
			return projectWriteErr("insert contact", err)
		}
	}
	return nil
}

// cleanProject trims and checks the whole form before any write.
func cleanProject(in ProjectInput) (ProjectInput, error) {
	out := ProjectInput{Name: strings.TrimSpace(in.Name), Type: strings.TrimSpace(in.Type), Address: strings.TrimSpace(in.Address)}
	switch {
	case out.Name == "":
		return out, ErrProjectNameRequired
	case !slices.Contains(ProjectTypes, out.Type):
		return out, ErrProjectTypeRequired
	case out.Address == "":
		return out, ErrProjectAddressRequired
	case len(in.Contacts) == 0:
		return out, ErrProjectContactRequired
	}
	seen := map[string]bool{}
	for _, c := range in.Contacts {
		cc := ContactInput{ID: c.ID, Name: strings.TrimSpace(c.Name), Email: strings.TrimSpace(c.Email), Label: strings.TrimSpace(c.Label)}
		if cc.Name == "" {
			return out, ErrContactNameRequired
		}
		if utf8.RuneCountInString(cc.Label) > maxContactLabel {
			return out, ErrContactLabelTooLong
		}
		phone, err := customers.NormalizeMobile(c.Phone)
		if err != nil {
			return out, err
		}
		if seen[phone] {
			return out, ErrContactPhoneTaken
		}
		seen[phone] = true
		cc.Phone = phone
		out.Contacts = append(out.Contacts, cc)
	}
	return out, nil
}

// inTx runs fn in one transaction; audit rows written through txCtx join it.
func (s *Service) inTx(ctx context.Context, fn func(context.Context, *workflowsdb.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("workflows: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(history.WithTx(ctx, tx), s.q.WithTx(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("workflows: commit: %w", err)
	}
	return nil
}

// projectWriteErr maps unique violations to the form's messages.
func projectWriteErr(what string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "projects_name_uidx":
			return ErrProjectNameTaken
		case "project_contacts_active_phone_uidx":
			return ErrContactPhoneTaken
		}
	}
	return fmt.Errorf("workflows: %s: %w", what, err)
}

// ProjectExists reports whether a project with this id exists.
func (s *Service) ProjectExists(ctx context.Context, id int64) (bool, error) {
	ok, err := s.q.ProjectExists(ctx, id)
	if err != nil {
		return false, fmt.Errorf("workflows: project exists: %w", err)
	}
	return ok, nil
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

func optText(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
