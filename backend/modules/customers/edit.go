package customers

import (
	"context"
	"errors"
	"fmt"
	"github.com/Surya3355/nexflow/backend/internal/platform/validate"
	"strings"

	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/customers/customersdb"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrMobileInUse is returned when an edit would give a customer a mobile
// number that another customer already has.
var ErrMobileInUse = errors.New("customers: that mobile number belongs to another customer")

// ErrCustomerNotFound is returned when the customer does not exist.
var ErrCustomerNotFound = errors.New("customers: customer not found")

// ErrNameRequired refuses a customer with an empty name.
var ErrNameRequired = errors.New("customers: name is required")

// CleanNameEmail trims and checks a customer's name (required, at most
// validate.MaxName characters) and email (empty or a valid address).
// Leads use it too, since a new lead creates the customer.
func CleanNameEmail(name, email string) (string, string, error) {
	if strings.TrimSpace(name) == "" {
		return "", "", ErrNameRequired
	}
	n, err := validate.RequireName(name, validate.MaxName, "name")
	if err != nil {
		return "", "", err
	}
	e, err := validate.EmailField(email)
	if err != nil {
		return "", "", err
	}
	return n, e, nil
}

// CustomerFields are the editable customer details.
type CustomerFields struct {
	Name   string
	Mobile string
	Email  string
}

// Update edits a customer's details. All staff may edit (plan Task 15).
// The mobile is normalized and must stay unique.
func (s *Service) Update(ctx context.Context, actor access.User, customerID int64, in CustomerFields) (Customer, error) {
	if !access.Can(actor.Role, access.ActionEditRecord) {
		return Customer{}, access.ErrForbidden
	}
	mobile, err := NormalizeMobile(in.Mobile)
	if err != nil {
		return Customer{}, err
	}
	name, email, err := CleanNameEmail(in.Name, in.Email)
	if err != nil {
		return Customer{}, err
	}
	if _, err := s.q.GetCustomer(ctx, customerID); errors.Is(err, pgx.ErrNoRows) {
		return Customer{}, ErrCustomerNotFound
	} else if err != nil {
		return Customer{}, fmt.Errorf("customers: load: %w", err)
	}

	err = s.q.UpdateCustomerFields(ctx, updateParams(customerID, name, mobile, email, actor.ID))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Customer{}, ErrMobileInUse
		}
		return Customer{}, fmt.Errorf("customers: update: %w", err)
	}

	actorID, id := actor.ID, customerID
	if err := s.rec.Record(ctx, history.Entry{
		ActorUserID: &actorID, Action: "customer.updated", EntityType: "customer", EntityID: &id,
	}); err != nil {
		return Customer{}, fmt.Errorf("customers: audit: %w", err)
	}
	row, err := s.q.GetCustomer(ctx, customerID)
	if err != nil {
		return Customer{}, fmt.Errorf("customers: reload: %w", err)
	}
	return toCustomer(row.ID, row.Name, row.Mobile, row.Email, row.Status), nil
}

func updateParams(id int64, name, mobile, email string, actorID int64) customersdb.UpdateCustomerFieldsParams {
	return customersdb.UpdateCustomerFieldsParams{
		ID: id, Name: name, Mobile: mobile, Email: textOrNull(email),
		UpdatedBy: pgtype.Int8{Int64: actorID, Valid: true},
	}
}
