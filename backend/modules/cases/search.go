package cases

import (
	"context"
	"fmt"
	"strings"

	"github.com/Surya3355/nexflow/backend/modules/cases/casesdb"
	"github.com/Surya3355/nexflow/backend/modules/customers"
	"github.com/jackc/pgx/v5/pgtype"
)

// SearchQuery is what staff typed in the search box (spec §8).
type SearchQuery struct {
	Text string
}

// CustomerSummary is the customer part of a search result.
type CustomerSummary struct {
	ID     int64
	Name   string
	Mobile string
	Email  string
	Status string
}

// CustomerGroup is one customer and all of its cases (spec §8, grouped by customer).
type CustomerGroup struct {
	Customer CustomerSummary
	Cases    []Case
}

// Search matches customers by name (partial, case-insensitive), mobile (after
// normalization, when the text has a valid mobile), tracking number (exact)
// and bank name. Results are grouped by customer.
func (s *Service) Search(ctx context.Context, q SearchQuery) ([]CustomerGroup, error) {
	text := strings.TrimSpace(q.Text)
	if text == "" {
		return nil, nil
	}
	mobile := ""
	if m, err := customers.NormalizeMobile(text); err == nil {
		mobile = m
	}
	ids, err := s.q.SearchCustomerIDs(ctx, casesdb.SearchCustomerIDsParams{
		NamePattern: "%" + escapeLike(text) + "%",
		Mobile:      mobile,
		Tracking:    pgtype.Text{String: strings.ToUpper(text), Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("cases: search: %w", err)
	}

	groups := make([]CustomerGroup, 0, len(ids))
	for _, hit := range ids {
		sum, err := s.q.CustomerSummary(ctx, hit.ID)
		if err != nil {
			return nil, fmt.Errorf("cases: customer: %w", err)
		}
		rows, err := s.q.CasesForCustomer(ctx, hit.ID)
		if err != nil {
			return nil, fmt.Errorf("cases: customer cases: %w", err)
		}
		g := CustomerGroup{Customer: CustomerSummary{
			ID: sum.ID, Name: sum.Name, Mobile: sum.Mobile, Email: sum.Email, Status: sum.Status,
		}}
		for _, r := range rows {
			g.Cases = append(g.Cases, toCase(casesdb.GetCaseRow(r)))
		}
		groups = append(groups, g)
	}
	return groups, nil
}

// escapeLike escapes LIKE wildcards so that typed % and _ match literally.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
