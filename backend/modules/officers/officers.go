// Package officers keeps each bank's list of officers per team (round 15
// tasks 6 and 7, spec 2026-11-26). Any staff member may change the list.
package officers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/customers"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/Surya3355/nexflow/backend/modules/officers/officersdb"
	"github.com/Surya3355/nexflow/backend/modules/workflows"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Errors for officer changes.
var (
	ErrNameRequired    = errors.New("officers: enter the officer's name (up to 80 characters)")
	ErrPhoneInvalid    = errors.New("officers: mobile number must be 10 digits")
	ErrPhoneTaken      = errors.New("officers: this bank already has this mobile number in that team")
	ErrEmailInvalid    = errors.New("officers: enter a valid email, like name@bank.com")
	ErrEmailTaken      = errors.New("officers: this bank already has this email in that team")
	ErrContactRequired = errors.New("officers: enter a mobile number or an email")
	ErrTeamNotForBank  = errors.New("officers: this bank's stages have no such team")
	ErrOfficerNotFound = errors.New("officers: officer not found")
)

const maxName = 80

// Officer is one bank officer in one team.
type Officer struct {
	ID       int64
	BankID   int64
	BankName string
	Team     string
	TeamName string
	Name     string
	Phone    string // "" when the officer gave only an email
	Email    string // "" when the officer gave only a mobile
	Removed  bool
}

// NewOfficer is the input for adding an officer.
type NewOfficer struct {
	BankID int64
	Team   string
	Name   string
	Phone  string // optional; Phone or Email is required
	Email  string // optional
}

// Service runs officer reads and changes.
type Service struct {
	pool *pgxpool.Pool
	q    *officersdb.Queries
	wf   *workflows.Service
	rec  history.Recorder
}

// NewService returns an officers service on pool.
func NewService(pool *pgxpool.Pool, wf *workflows.Service, rec history.Recorder) *Service {
	return &Service{pool: pool, q: officersdb.New(pool), wf: wf, rec: rec}
}

// List returns the current officers of all banks.
func (s *Service) List(ctx context.Context) ([]Officer, error) {
	rows, err := s.q.ListOfficers(ctx)
	if err != nil {
		return nil, fmt.Errorf("officers: list: %w", err)
	}
	out := make([]Officer, 0, len(rows))
	for _, r := range rows {
		out = append(out, Officer{ID: r.ID, BankID: r.BankID, BankName: r.BankName, Team: r.Team,
			TeamName: workflows.TeamName(r.Team), Name: r.Name, Phone: r.Phone, Email: r.Email})
	}
	return out, nil
}

// Get returns one officer, removed or not.
func (s *Service) Get(ctx context.Context, id int64) (Officer, error) {
	r, err := s.q.GetOfficer(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Officer{}, ErrOfficerNotFound
	}
	if err != nil {
		return Officer{}, fmt.Errorf("officers: get: %w", err)
	}
	return Officer{ID: r.ID, BankID: r.BankID, BankName: r.BankName, Team: r.Team,
		TeamName: workflows.TeamName(r.Team), Name: r.Name, Phone: r.Phone, Email: r.Email, Removed: r.Removed}, nil
}

// contact is an officer's cleaned name, mobile and email. Mobile and email
// are each optional; at least one is required (owner, 2026-11-26).
type contact struct{ name, phone, email string }

func clean(name, phone, email string) (contact, error) {
	c := contact{name: strings.TrimSpace(name), email: strings.TrimSpace(email)}
	if c.name == "" || utf8.RuneCountInString(c.name) > maxName {
		return c, ErrNameRequired
	}
	if strings.TrimSpace(phone) != "" {
		p, err := customers.NormalizeMobile(phone)
		if err != nil {
			return c, ErrPhoneInvalid
		}
		c.phone = p
	}
	if c.email != "" && !validEmail(c.email) {
		return c, ErrEmailInvalid
	}
	if c.phone == "" && c.email == "" {
		return c, ErrContactRequired
	}
	return c, nil
}

// validEmail is a light check: one @ with text on both sides, no spaces.
func validEmail(e string) bool {
	at := strings.Index(e, "@")
	return len(e) <= 254 && at > 0 && at == strings.LastIndex(e, "@") && at < len(e)-1 &&
		!strings.ContainsAny(e, " \t\r\n")
}

func optText(v string) pgtype.Text { return pgtype.Text{String: v, Valid: v != ""} }

// writeErr maps unique violations to the duplicate mobile or email message.
func writeErr(what string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		if pgErr.ConstraintName == "bank_officers_active_email_uidx" {
			return ErrEmailTaken
		}
		return ErrPhoneTaken
	}
	return fmt.Errorf("officers: %s: %w", what, err)
}

// Add puts an officer on a bank's team. The team must be one the bank's
// stages ask for.
func (s *Service) Add(ctx context.Context, actor access.User, in NewOfficer) (Officer, error) {
	if !access.Can(actor.Role, access.ActionAddRecord) {
		return Officer{}, access.ErrForbidden
	}
	c, err := clean(in.Name, in.Phone, in.Email)
	if err != nil {
		return Officer{}, err
	}
	if ok, err := s.bankHasTeam(ctx, in.BankID, in.Team); err != nil {
		return Officer{}, err
	} else if !ok {
		return Officer{}, ErrTeamNotForBank
	}
	by := pgtype.Int8{Int64: actor.ID, Valid: true}
	var id int64
	err = s.inTx(ctx, func(ctx context.Context, q *officersdb.Queries) error {
		var err error
		id, err = q.InsertOfficer(ctx, officersdb.InsertOfficerParams{BankID: in.BankID, Team: in.Team, Name: c.name,
			Phone: optText(c.phone), Email: optText(c.email), CreatedBy: by})
		if err != nil {
			return writeErr("insert", err)
		}
		return s.audit(ctx, actor, "officer.added", id, map[string]any{"bank_id": in.BankID, "team": in.Team, "name": c.name, "phone": c.phone, "email": c.email})
	})
	if err != nil {
		return Officer{}, err
	}
	return s.Get(ctx, id)
}

// Update changes a current officer's name, mobile and email. Cases that point to
// the officer show the new details.
func (s *Service) Update(ctx context.Context, actor access.User, id int64, name, phone, email string) (Officer, error) {
	if !access.Can(actor.Role, access.ActionAddRecord) {
		return Officer{}, access.ErrForbidden
	}
	c, err := clean(name, phone, email)
	if err != nil {
		return Officer{}, err
	}
	err = s.inTx(ctx, func(ctx context.Context, q *officersdb.Queries) error {
		before, err := q.LockOfficer(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOfficerNotFound
		}
		if err != nil {
			return fmt.Errorf("officers: lock: %w", err)
		}
		_, err = q.UpdateOfficer(ctx, officersdb.UpdateOfficerParams{ID: id, Name: c.name, Phone: optText(c.phone), Email: optText(c.email),
			UpdatedBy: pgtype.Int8{Int64: actor.ID, Valid: true}})
		if err != nil {
			return writeErr("update", err)
		}
		return s.audit(ctx, actor, "officer.updated", id, map[string]any{
			"before": map[string]string{"name": before.Name, "phone": before.Phone, "email": before.Email},
			"after":  map[string]string{"name": c.name, "phone": c.phone, "email": c.email},
		})
	})
	if err != nil {
		return Officer{}, err
	}
	return s.Get(ctx, id)
}

// Remove takes an officer off the list. Cases keep them.
func (s *Service) Remove(ctx context.Context, actor access.User, id int64) error {
	if !access.Can(actor.Role, access.ActionAddRecord) {
		return access.ErrForbidden
	}
	return s.inTx(ctx, func(ctx context.Context, q *officersdb.Queries) error {
		n, err := q.RemoveOfficer(ctx, officersdb.RemoveOfficerParams{ID: id, UpdatedBy: pgtype.Int8{Int64: actor.ID, Valid: true}})
		if err != nil {
			return fmt.Errorf("officers: remove: %w", err)
		}
		if n == 0 {
			return ErrOfficerNotFound
		}
		return s.audit(ctx, actor, "officer.removed", id, nil)
	})
}

// inTx runs fn in one transaction; the audit entry joins it (history.WithTx).
func (s *Service) inTx(ctx context.Context, fn func(context.Context, *officersdb.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("officers: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(history.WithTx(ctx, tx), s.q.WithTx(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("officers: commit: %w", err)
	}
	return nil
}

func (s *Service) bankHasTeam(ctx context.Context, bankID int64, team string) (bool, error) {
	if team == "" || workflows.TeamName(team) == "" {
		return false, nil
	}
	if _, err := s.wf.BankPartPayments(ctx, bankID); errors.Is(err, workflows.ErrBankNotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	stages, err := s.wf.StagesFor(ctx, bankID)
	if err != nil {
		return false, err
	}
	for _, st := range stages {
		if st.OfficerTeam == team {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) audit(ctx context.Context, actor access.User, action string, id int64, details map[string]any) error {
	actorID := actor.ID
	if err := s.rec.Record(ctx, history.Entry{ActorUserID: &actorID, Action: action, EntityType: "bank_officer", EntityID: &id, Details: details}); err != nil {
		return fmt.Errorf("officers: audit: %w", err)
	}
	return nil
}
