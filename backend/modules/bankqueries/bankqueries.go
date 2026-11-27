// Package bankqueries records the queries a bank raises on a case and their
// proof (round 15 tasks 8 and 9, spec 2026-11-26 bank queries). A query is
// resolved only after proof was sent and the bank accepted it.
package bankqueries

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/bankqueries/bankqueriesdb"
	"github.com/Surya3355/nexflow/backend/modules/documents"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Query statuses.
const (
	StatusOpen     = "open"
	StatusSent     = "sent"
	StatusResolved = "resolved"
)

// Errors for query changes.
var (
	ErrBodyRequired     = errors.New("bankqueries: enter what the bank asked for")
	ErrBodyTooLong      = errors.New("bankqueries: what the bank asked for must be 500 characters or fewer")
	ErrRaisedInFuture   = errors.New("bankqueries: the date raised cannot be in the future")
	ErrDueBeforeRaised  = errors.New("bankqueries: the due date cannot be before the date raised")
	ErrAssigneeRequired = errors.New("bankqueries: choose who the query is assigned to")
	ErrAssigneeInactive = errors.New("bankqueries: the person you are assigning to is not active")
	ErrCaseNotFound     = errors.New("bankqueries: case not found")
	ErrCaseDropped      = errors.New("bankqueries: a dropped case cannot change its bank queries")
	ErrQueryNotFound    = errors.New("bankqueries: query not found")
	ErrQueryResolved    = errors.New("bankqueries: this query is already resolved")
	ErrNotSent          = errors.New("bankqueries: this query is not waiting on the bank; reload the page to see its latest state")
	ErrAlreadySent      = errors.New("bankqueries: proof was already sent for this query; reload the page to see it")
	ErrProofRequired    = errors.New("bankqueries: add a note or a file as proof")
	ErrNoteTooLong      = errors.New("bankqueries: the note must be 500 characters or fewer")
	ErrReasonRequired   = errors.New("bankqueries: enter what the bank said")
)

const maxText = 500

var istZone = time.FixedZone("IST", 5*60*60+30*60)

// NewQuery is the input for adding or editing a query.
type NewQuery struct {
	Body       string
	RaisedOn   time.Time
	DueOn      *time.Time
	AssigneeID int64
}

// Query is one live bank query with its history.
type Query struct {
	ID           int64
	CaseID       int64
	Number       int
	Body         string
	RaisedOn     time.Time
	DueOn        *time.Time
	AssigneeID   int64
	AssigneeName string
	Status       string
	Proof        *Proof // the proof sent, while the query is waiting for the bank
	Events       []Event
}

// Proof is what staff sent to the bank: a note, a file, or both.
type Proof struct {
	Note          string
	DocumentID    *int64 // nil when no file was sent or the file is gone
	DocumentLabel string
	FileName      string
	FileDeleted   bool // a file was sent but has since been purged or removed
	At            time.Time
	By            string
}

// Event is one step in a query's history.
type Event struct {
	QueryID    int64
	Number     int
	Body       string
	Kind       string
	Note       string
	DocumentID *int64
	// DocumentLabel and FileName describe the file while it still exists.
	DocumentLabel string
	FileName      string
	FileDeleted   bool // a file was sent but has since been purged or removed
	At            time.Time
	By            string
}

// Service runs bank query reads and changes.
type Service struct {
	pool *pgxpool.Pool
	q    *bankqueriesdb.Queries
	docs *documents.Service
	rec  history.Recorder
	// readBack loads a query after a change commits; a field so tests can make it fail.
	readBack func(ctx context.Context, caseID, id int64) (Query, error)
}

// NewService returns a bank queries service on pool. Proof files go through docs.
func NewService(pool *pgxpool.Pool, docs *documents.Service, rec history.Recorder) *Service {
	s := &Service{pool: pool, q: bankqueriesdb.New(pool), docs: docs, rec: rec}
	s.readBack = s.get
	return s
}

func todayIST() time.Time {
	n := time.Now().In(istZone)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func pgDate(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: dateOnly(*t), Valid: true}
}

func optText(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

// clean checks a query's input and returns it trimmed.
func clean(in NewQuery) (NewQuery, error) {
	in.Body = strings.TrimSpace(in.Body)
	if in.Body == "" {
		return in, ErrBodyRequired
	}
	if utf8.RuneCountInString(in.Body) > maxText {
		return in, ErrBodyTooLong
	}
	if in.AssigneeID <= 0 {
		return in, ErrAssigneeRequired
	}
	in.RaisedOn = dateOnly(in.RaisedOn)
	if in.RaisedOn.After(todayIST()) {
		return in, ErrRaisedInFuture
	}
	if in.DueOn != nil && dateOnly(*in.DueOn).Before(in.RaisedOn) {
		return in, ErrDueBeforeRaised
	}
	return in, nil
}

// inTx runs fn in one transaction with the audit entries it records.
func (s *Service) inTx(ctx context.Context, fn func(ctx context.Context, q *bankqueriesdb.Queries, tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("bankqueries: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(history.WithTx(ctx, tx), s.q.WithTx(tx), tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("bankqueries: commit: %w", err)
	}
	return nil
}

// lockCase locks a live (not dropped) case.
func lockCase(ctx context.Context, q *bankqueriesdb.Queries, caseID int64) error {
	c, err := q.LockCase(ctx, caseID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrCaseNotFound
	}
	if err != nil {
		return fmt.Errorf("bankqueries: lock case: %w", err)
	}
	if c.Status == "dropped" {
		return ErrCaseDropped
	}
	return nil
}

// lockQuery locks the query's case, then the query (always in that order).
// A removed query counts as not found.
func lockQuery(ctx context.Context, q *bankqueriesdb.Queries, id int64) (bankqueriesdb.LockQueryRow, error) {
	caseID, err := q.QueryCaseID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return bankqueriesdb.LockQueryRow{}, ErrQueryNotFound
	}
	if err != nil {
		return bankqueriesdb.LockQueryRow{}, fmt.Errorf("bankqueries: find: %w", err)
	}
	if err := lockCase(ctx, q, caseID); err != nil {
		return bankqueriesdb.LockQueryRow{}, err
	}
	row, err := q.LockQuery(ctx, id)
	if err != nil {
		return bankqueriesdb.LockQueryRow{}, fmt.Errorf("bankqueries: lock: %w", err)
	}
	if row.Removed {
		return bankqueriesdb.LockQueryRow{}, ErrQueryNotFound
	}
	return row, nil
}

func (s *Service) audit(ctx context.Context, actor access.User, action string, id int64, details map[string]any) error {
	actorID := actor.ID
	if err := s.rec.Record(ctx, history.Entry{
		ActorUserID: &actorID, Action: action, EntityType: "case_query", EntityID: &id, Details: details,
	}); err != nil {
		return fmt.Errorf("bankqueries: audit: %w", err)
	}
	return nil
}

// eventFile is the proof file an event refers to: its document and its name.
type eventFile struct {
	docID int64
	name  string
}

func event(ctx context.Context, q *bankqueriesdb.Queries, queryID int64, kind, note string, file *eventFile, by int64) error {
	p := bankqueriesdb.InsertEventParams{QueryID: queryID, Kind: kind, Note: optText(note), ByUser: by}
	if file != nil {
		p.DocumentID = pgtype.Int8{Int64: file.docID, Valid: true}
		p.FileName = optText(file.name)
	}
	if err := q.InsertEvent(ctx, p); err != nil {
		return fmt.Errorf("bankqueries: event: %w", err)
	}
	return nil
}

func checkAssignee(ctx context.Context, q *bankqueriesdb.Queries, id int64) error {
	ok, err := q.ActiveUser(ctx, id)
	if err != nil {
		return fmt.Errorf("bankqueries: assignee: %w", err)
	}
	if !ok {
		return ErrAssigneeInactive
	}
	return nil
}

// Add records a new query on a case. It is numbered after the case's last query.
func (s *Service) Add(ctx context.Context, actor access.User, caseID int64, in NewQuery) (Query, error) {
	if !access.Can(actor.Role, access.ActionEditRecord) {
		return Query{}, access.ErrForbidden
	}
	in, err := clean(in)
	if err != nil {
		return Query{}, err
	}
	var id int64
	err = s.inTx(ctx, func(ctx context.Context, q *bankqueriesdb.Queries, _ pgx.Tx) error {
		if err := lockCase(ctx, q, caseID); err != nil {
			return err
		}
		if err := checkAssignee(ctx, q, in.AssigneeID); err != nil {
			return err
		}
		n, err := q.NextNumber(ctx, caseID)
		if err != nil {
			return fmt.Errorf("bankqueries: number: %w", err)
		}
		id, err = q.InsertQuery(ctx, bankqueriesdb.InsertQueryParams{
			CaseID: caseID, Number: n, Body: in.Body, RaisedOn: pgDate(&in.RaisedOn), DueOn: pgDate(in.DueOn),
			AssigneeID: in.AssigneeID, CreatedBy: actor.ID,
		})
		if err != nil {
			return fmt.Errorf("bankqueries: insert: %w", err)
		}
		if err := event(ctx, q, id, "raised", "", nil, actor.ID); err != nil {
			return err
		}
		return s.audit(ctx, actor, "query.added", id, map[string]any{"case_id": caseID, "number": n, "body": in.Body})
	})
	if err != nil {
		return Query{}, err
	}
	return s.get(ctx, caseID, id)
}

// Edit changes what was asked, the dates and the assignee of an unresolved query.
func (s *Service) Edit(ctx context.Context, actor access.User, id int64, in NewQuery) (Query, error) {
	if !access.Can(actor.Role, access.ActionEditRecord) {
		return Query{}, access.ErrForbidden
	}
	in, err := clean(in)
	if err != nil {
		return Query{}, err
	}
	var caseID int64
	err = s.inTx(ctx, func(ctx context.Context, q *bankqueriesdb.Queries, _ pgx.Tx) error {
		row, err := lockQuery(ctx, q, id)
		if err != nil {
			return err
		}
		caseID = row.CaseID
		if row.Status == StatusResolved {
			return ErrQueryResolved
		}
		if unchanged(row, in) {
			return nil
		}
		if err := checkAssignee(ctx, q, in.AssigneeID); err != nil {
			return err
		}
		if err := q.UpdateQuery(ctx, bankqueriesdb.UpdateQueryParams{
			ID: id, Body: in.Body, RaisedOn: pgDate(&in.RaisedOn), DueOn: pgDate(in.DueOn),
			AssigneeID: in.AssigneeID, UpdatedBy: actor.ID,
		}); err != nil {
			return fmt.Errorf("bankqueries: update: %w", err)
		}
		if err := event(ctx, q, id, "edited", "", nil, actor.ID); err != nil {
			return err
		}
		return s.audit(ctx, actor, "query.edited", id, map[string]any{"case_id": caseID, "body": in.Body, "assignee_id": in.AssigneeID})
	})
	if err != nil {
		return Query{}, err
	}
	return s.get(ctx, caseID, id)
}

// unchanged reports whether an edit would leave the query as it is.
func unchanged(row bankqueriesdb.LockQueryRow, in NewQuery) bool {
	sameDue := !row.DueOn.Valid && in.DueOn == nil ||
		row.DueOn.Valid && in.DueOn != nil && row.DueOn.Time.Equal(dateOnly(*in.DueOn))
	return row.Body == in.Body && row.RaisedOn.Time.Equal(in.RaisedOn) && sameDue && row.AssigneeID == in.AssigneeID
}

// OpenCount is the number of the case's queries not yet resolved (open or sent).
func (s *Service) OpenCount(ctx context.Context, caseID int64) (int, error) {
	n, err := s.q.OpenCount(ctx, caseID)
	if err != nil {
		return 0, fmt.Errorf("bankqueries: count: %w", err)
	}
	return int(n), nil
}

// List returns the case's live queries, unresolved first, each with its events.
func (s *Service) List(ctx context.Context, caseID int64) ([]Query, error) {
	qs, _, err := s.Load(ctx, caseID)
	return qs, err
}

// Load returns the case's live queries (as List) and every event of the
// case's queries, removed ones included (as Events), reading the events once.
func (s *Service) Load(ctx context.Context, caseID int64) ([]Query, []Event, error) {
	ok, err := s.q.CaseExists(ctx, caseID)
	if err != nil {
		return nil, nil, fmt.Errorf("bankqueries: case: %w", err)
	}
	if !ok {
		return nil, nil, ErrCaseNotFound
	}
	rows, err := s.q.ListQueries(ctx, caseID)
	if err != nil {
		return nil, nil, fmt.Errorf("bankqueries: list: %w", err)
	}
	events, err := s.Events(ctx, caseID)
	if err != nil {
		return nil, nil, err
	}
	byQuery := map[int64][]Event{}
	for _, e := range events {
		byQuery[e.QueryID] = append(byQuery[e.QueryID], e)
	}
	out := make([]Query, 0, len(rows))
	for _, r := range rows {
		qr := Query{ID: r.ID, CaseID: r.CaseID, Number: int(r.Number), Body: r.Body, RaisedOn: r.RaisedOn.Time,
			AssigneeID: r.AssigneeID, AssigneeName: r.AssigneeName, Status: r.Status, Events: byQuery[r.ID]}
		if r.DueOn.Valid {
			d := r.DueOn.Time
			qr.DueOn = &d
		}
		if qr.Status == StatusSent {
			for i := len(qr.Events) - 1; i >= 0; i-- {
				if e := qr.Events[i]; e.Kind == "sent" {
					qr.Proof = &Proof{Note: e.Note, DocumentID: e.DocumentID, DocumentLabel: e.DocumentLabel, FileName: e.FileName,
						FileDeleted: e.FileDeleted, At: e.At, By: e.By}
					break
				}
			}
		}
		out = append(out, qr)
	}
	return out, events, nil
}

// Events returns every event of the case's queries in time order, removed queries included.
func (s *Service) Events(ctx context.Context, caseID int64) ([]Event, error) {
	rows, err := s.q.ListEvents(ctx, caseID)
	if err != nil {
		return nil, fmt.Errorf("bankqueries: events: %w", err)
	}
	out := make([]Event, 0, len(rows))
	for _, r := range rows {
		e := Event{QueryID: r.QueryID, Number: int(r.Number), Body: r.Body, Kind: r.Kind, Note: r.Note,
			DocumentLabel: r.DocumentLabel, FileName: r.FileName, FileDeleted: r.FileDeleted, At: r.At.Time, By: r.ByName}
		if r.DocumentID.Valid {
			d := r.DocumentID.Int64
			e.DocumentID = &d
		}
		out = append(out, e)
	}
	return out, nil
}

func (s *Service) get(ctx context.Context, caseID, id int64) (Query, error) {
	list, err := s.List(ctx, caseID)
	if err != nil {
		return Query{}, err
	}
	for _, q := range list {
		if q.ID == id {
			return q, nil
		}
	}
	return Query{}, ErrQueryNotFound
}

func pgInt8(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: true} }
