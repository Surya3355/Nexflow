package customers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/customers/customersdb"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotAllowed is returned when the actor's role may not add customers.
var ErrNotAllowed = access.ErrForbidden

// Customer is a person who may hold one or more loan cases.
type Customer struct {
	ID     int64
	Name   string
	Mobile string
	Email  string
	Status string
}

// NewCustomer is the input for creating a customer.
type NewCustomer struct {
	Name   string
	Mobile string
	Email  string
}

// CreateResult is the outcome of Create. When the mobile already belongs to a
// customer, Existing is set, Customer is nil, and no second customer is made.
type CreateResult struct {
	Customer *Customer
	Existing *Customer
}

// Service holds the database handle and audit recorder for customer actions.
type Service struct {
	pool *pgxpool.Pool
	q    *customersdb.Queries
	rec  history.Recorder
}

// NewService returns a customers service on pool.
func NewService(pool *pgxpool.Pool, rec history.Recorder) *Service {
	return &Service{pool: pool, q: customersdb.New(pool), rec: rec}
}

// Create adds a customer for a new mobile number. If the mobile is already
// used, it returns the existing customer as a warning (spec §4.1, round 2).
// The unique index on mobile is the final guard against two staff creating
// the same customer at the same moment.
func (s *Service) Create(ctx context.Context, actor access.User, in NewCustomer) (CreateResult, error) {
	if !access.Can(actor.Role, access.ActionAddRecord) {
		return CreateResult{}, ErrNotAllowed
	}
	mobile, err := NormalizeMobile(in.Mobile)
	if err != nil {
		return CreateResult{}, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return CreateResult{}, errors.New("customers: name is required")
	}

	row, err := s.q.InsertCustomer(ctx, customersdb.InsertCustomerParams{
		Name:      name,
		Mobile:    mobile,
		Email:     textOrNull(in.Email),
		CreatedBy: pgtype.Int8{Int64: actor.ID, Valid: true},
	})
	if err == nil {
		c := toCustomer(row.ID, row.Name, row.Mobile, row.Email, row.Status)
		if err := s.audit(ctx, actor, "customer.created", c.ID); err != nil {
			return CreateResult{}, err
		}
		return CreateResult{Customer: &c}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return CreateResult{}, fmt.Errorf("customers: insert: %w", err)
	}

	// The mobile already exists: a conflict returns no row. Read the owner.
	existing, err := s.q.GetCustomerByMobile(ctx, mobile)
	if err != nil {
		return CreateResult{}, fmt.Errorf("customers: read existing: %w", err)
	}
	c := toCustomer(existing.ID, existing.Name, existing.Mobile, existing.Email, existing.Status)
	return CreateResult{Existing: &c}, nil
}

func (s *Service) audit(ctx context.Context, actor access.User, action string, id int64) error {
	actorID := actor.ID
	entity := id
	if err := s.rec.Record(ctx, history.Entry{ActorUserID: &actorID, Action: action, EntityType: "customer", EntityID: &entity}); err != nil {
		return fmt.Errorf("customers: audit: %w", err)
	}
	return nil
}

func toCustomer(id int64, name, mobile string, email pgtype.Text, status string) Customer {
	return Customer{ID: id, Name: name, Mobile: mobile, Email: email.String, Status: status}
}

func textOrNull(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	return pgtype.Text{String: s, Valid: s != ""}
}
