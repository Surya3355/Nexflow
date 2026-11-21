package customers

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Get returns one customer by ID.
func (s *Service) Get(ctx context.Context, customerID int64) (Customer, error) {
	row, err := s.q.GetCustomer(ctx, customerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Customer{}, ErrCustomerNotFound
	}
	if err != nil {
		return Customer{}, fmt.Errorf("customers: get: %w", err)
	}
	return toCustomer(row.ID, row.Name, row.Mobile, row.Email, row.Status), nil
}
