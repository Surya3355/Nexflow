package workflows

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/workflows/workflowsdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Project errors (spec 2026-11-21 §4.2).
var (
	ErrProjectNameRequired = errors.New("workflows: project name is required")
	ErrProjectNameTaken    = errors.New("workflows: a project with this name already exists")
	ErrProjectNotFound     = errors.New("workflows: project not found")
	ErrProjectLocked       = errors.New("workflows: the NA project cannot be renamed")
)

// Project is a builder project that cases belong to. NA (IsSystem) is the
// fixed entry for loans without a builder project.
type Project struct {
	ID         int64
	Name       string
	IsSystem   bool
	OpenCases  int64
	TotalCases int64
}

// ListProjects returns every project, NA first and then by name, with the
// number of open (active) and total cases.
func (s *Service) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := s.q.ListProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("workflows: list projects: %w", err)
	}
	out := make([]Project, 0, len(rows))
	for _, r := range rows {
		out = append(out, Project{ID: r.ID, Name: r.Name, IsSystem: r.IsSystem, OpenCases: r.OpenCases, TotalCases: r.TotalCases})
	}
	return out, nil
}

// CreateProject adds a project. Admin only. Names are trimmed and unique
// ignoring case.
func (s *Service) CreateProject(ctx context.Context, actor access.User, name string) (Project, error) {
	if !access.Can(actor.Role, access.ActionManageConfig) {
		return Project{}, access.ErrForbidden
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Project{}, ErrProjectNameRequired
	}
	row, err := s.q.InsertProject(ctx, workflowsdb.InsertProjectParams{
		Name: name, CreatedBy: pgtype.Int8{Int64: actor.ID, Valid: true},
	})
	if isUniqueViolation(err) {
		return Project{}, ErrProjectNameTaken
	}
	if err != nil {
		return Project{}, fmt.Errorf("workflows: insert project: %w", err)
	}
	p := Project{ID: row.ID, Name: row.Name, IsSystem: row.IsSystem}
	if err := s.audit(ctx, actor, "project.created", "project", p.ID); err != nil {
		return Project{}, err
	}
	return p, nil
}

// RenameProject changes a project's name. Admin only. NA cannot be renamed.
func (s *Service) RenameProject(ctx context.Context, actor access.User, id int64, name string) (Project, error) {
	if !access.Can(actor.Role, access.ActionManageConfig) {
		return Project{}, access.ErrForbidden
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Project{}, ErrProjectNameRequired
	}
	cur, err := s.q.GetProject(ctx, id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Project{}, ErrProjectNotFound
	case err != nil:
		return Project{}, fmt.Errorf("workflows: get project: %w", err)
	case cur.IsSystem:
		return Project{}, ErrProjectLocked
	}
	row, err := s.q.RenameProject(ctx, workflowsdb.RenameProjectParams{
		ID: id, Name: name, UpdatedBy: pgtype.Int8{Int64: actor.ID, Valid: true},
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Project{}, ErrProjectNotFound
	case isUniqueViolation(err):
		return Project{}, ErrProjectNameTaken
	case err != nil:
		return Project{}, fmt.Errorf("workflows: rename project: %w", err)
	}
	p := Project{ID: row.ID, Name: row.Name, IsSystem: row.IsSystem}
	if err := s.audit(ctx, actor, "project.renamed", "project", p.ID); err != nil {
		return Project{}, err
	}
	return p, nil
}

// ProjectExists reports whether a project with this id exists.
func (s *Service) ProjectExists(ctx context.Context, id int64) (bool, error) {
	ok, err := s.q.ProjectExists(ctx, id)
	if err != nil {
		return false, fmt.Errorf("workflows: project exists: %w", err)
	}
	return ok, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
