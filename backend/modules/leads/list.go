package leads

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/leads/leadsdb"
	"github.com/jackc/pgx/v5"
)

// Get returns one lead with its list details.
func (s *Service) Get(ctx context.Context, id int64, now time.Time) (Row, error) {
	r, err := s.q.GetLeadRow(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Row{}, ErrLeadNotFound
	}
	if err != nil {
		return Row{}, fmt.Errorf("leads: get: %w", err)
	}
	return toRow(r, ISTDay(now)), nil
}

// ListOpen returns open leads: calls due today or earlier first (oldest
// first), then the rest newest first.
func (s *Service) ListOpen(ctx context.Context, now time.Time) ([]Row, error) {
	today := ISTDay(now)
	rows, err := s.q.ListOpenLeadRows(ctx, dateRef(today))
	if err != nil {
		return nil, fmt.Errorf("leads: list: %w", err)
	}
	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		out = append(out, toRow(leadsdb.GetLeadRowRow(r), today))
	}
	return out, nil
}

// ForCustomer returns all of a customer's leads, newest first.
func (s *Service) ForCustomer(ctx context.Context, customerID int64, now time.Time) ([]Row, error) {
	today := ISTDay(now)
	rows, err := s.q.LeadRowsForCustomer(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("leads: for customer: %w", err)
	}
	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		out = append(out, toRow(leadsdb.GetLeadRowRow(r), today))
	}
	return out, nil
}

func toRow(r leadsdb.GetLeadRowRow, today time.Time) Row {
	row := Row{
		Lead: Lead{
			ID: r.ID, CustomerID: r.CustomerID, ProjectID: r.ProjectID, AssigneeID: r.AssigneeID,
			Status: r.Status, CreatedAt: r.CreatedAt.Time,
		},
		CustomerName: r.CustomerName, CustomerMobile: r.CustomerMobile, CustomerEmail: r.CustomerEmail,
		ProjectName: r.ProjectName, AssigneeName: r.AssigneeName, LastResponse: r.LastResponse,
	}
	if r.NextCallDate.Valid {
		d := r.NextCallDate.Time
		row.NextCallDate = &d
		if row.Status == StatusOpen && !d.After(today) {
			row.CallDue = true
			row.OverdueDays = int(today.Sub(d) / (24 * time.Hour))
		}
	}
	if r.ConvertedAt.Valid {
		t := r.ConvertedAt.Time
		row.ConvertedAt = &t
	}
	if r.LastContactAt.Valid {
		t := r.LastContactAt.Time
		row.LastContactAt = &t
	}
	if r.CaseID.Valid {
		id := r.CaseID.Int64
		row.CaseID = &id
	}
	if r.BuilderContactID.Valid {
		id := r.BuilderContactID.Int64
		row.BuilderContactID = &id
		row.BuilderContact = &BuilderContact{ID: id, Name: r.BcName, Phone: r.BcPhone, Email: r.BcEmail, Label: r.BcLabel, Removed: r.BcRemoved}
	}
	return row
}
