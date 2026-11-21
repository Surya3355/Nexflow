package cases

import (
	"context"
	"testing"
)

// seedSecondCustomer adds "Meena" with a case at SBI and tracking SBI-777.
func seedSecondCustomer(t *testing.T, f fixture) (customerID, caseID int64) {
	t.Helper()
	ctx := context.Background()
	var sbi int64
	mustScan(t, f.pool.QueryRow(ctx, `INSERT INTO banks (name) VALUES ('SBI') RETURNING id`), &sbi)
	mustScan(t, f.pool.QueryRow(ctx, `INSERT INTO customers (name, mobile) VALUES ('Meena Iyer', '9123456780') RETURNING id`), &customerID)
	mustScan(t, f.pool.QueryRow(ctx, `INSERT INTO cases (customer_id, bank_id, loan_amount, current_stage_id, assignee_id, tracking_number)
		VALUES ($1, $2, 300000, $3, $4, 'SBI-777') RETURNING id`, customerID, sbi, stageID(t, f, "received"), f.actor.ID), &caseID)
	return customerID, caseID
}

func groupFor(groups []CustomerGroup, customerID int64) *CustomerGroup {
	for i := range groups {
		if groups[i].Customer.ID == customerID {
			return &groups[i]
		}
	}
	return nil
}

func TestSearchByName(t *testing.T) {
	f := newFixture(t)
	groups, err := f.svc.Search(context.Background(), SearchQuery{Text: "ravi"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if groupFor(groups, f.customer) == nil {
		t.Fatalf("partial, case-insensitive name search missed Ravi: %+v", groups)
	}
}

func TestSearchByMobileWithPlus91Prefix(t *testing.T) {
	f := newFixture(t)
	groups, err := f.svc.Search(context.Background(), SearchQuery{Text: "+91 98765 43210"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if groupFor(groups, f.customer) == nil {
		t.Fatalf("pasted +91 mobile did not find the customer: %+v", groups)
	}
}

func TestSearchByTrackingNumber(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c := newCase(t, f)
	if _, err := f.svc.ChangeStage(ctx, f.actor, c.ID, stageID(t, f, "logged_in"), tracking("FIND42"), nil, nil); err != nil {
		t.Fatalf("set tracking: %v", err)
	}
	groups, err := f.svc.Search(ctx, SearchQuery{Text: "FIND42"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	g := groupFor(groups, f.customer)
	if g == nil || len(g.Cases) == 0 {
		t.Fatalf("tracking search missed the case: %+v", groups)
	}
}

func TestSearchByBank(t *testing.T) {
	f := newFixture(t)
	customerID, _ := seedSecondCustomer(t, f)
	groups, err := f.svc.Search(context.Background(), SearchQuery{Text: "sbi"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if groupFor(groups, customerID) == nil {
		t.Fatalf("bank search missed the SBI customer: %+v", groups)
	}
	if groupFor(groups, f.customer) != nil {
		t.Fatal("bank search returned a customer with no SBI case")
	}
}

// Two cases under one customer appear in one group (spec §8).
func TestResultsGroupedByCustomer(t *testing.T) {
	f := newFixture(t)
	newCase(t, f)
	newCase(t, f)
	groups, err := f.svc.Search(context.Background(), SearchQuery{Text: "Ravi"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	g := groupFor(groups, f.customer)
	if g == nil {
		t.Fatalf("customer missing: %+v", groups)
	}
	if len(g.Cases) != 2 {
		t.Fatalf("cases in one group = %d, want 2", len(g.Cases))
	}
	if n := countGroups(groups, f.customer); n != 1 {
		t.Fatalf("customer appears in %d groups, want 1", n)
	}
}

func countGroups(groups []CustomerGroup, id int64) int {
	n := 0
	for _, g := range groups {
		if g.Customer.ID == id {
			n++
		}
	}
	return n
}
