package cases

import (
	"context"
	"testing"
)

func TestListFilterByProject(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	prestige := addProject(t, f, "Prestige")
	newCase(t, f) // NA
	p, err := f.svc.Create(ctx, f.actor, NewCase{ProjectID: prestige, CustomerID: f.customer, BankID: f.bankID, LoanAmount: "100000", AssigneeID: f.actor.ID})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	list, err := f.svc.List(ctx, ListFilter{ProjectID: &prestige})
	if err != nil || len(list) != 1 || list[0].ID != p.ID {
		t.Fatalf("List = %+v %v, want only the Prestige case", list, err)
	}
	rows, err := f.svc.ListRows(ctx, ListFilter{ProjectID: &prestige})
	if err != nil || len(rows) != 1 || rows[0].ID != p.ID {
		t.Fatalf("ListRows = %+v %v, want only the Prestige case", rows, err)
	}
}

func TestSearchByProjectName(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	newCase(t, f) // Ravi, NA project
	nandanam := addProject(t, f, "Nandanam Heights")
	var asha, nandini int64
	mustScan(t, f.pool.QueryRow(ctx, `INSERT INTO customers (name, mobile) VALUES ('Asha', '9123400001') RETURNING id`), &asha)
	mustScan(t, f.pool.QueryRow(ctx, `INSERT INTO customers (name, mobile) VALUES ('Nandini', '9123400002') RETURNING id`), &nandini)
	if _, err := f.svc.Create(ctx, f.actor, NewCase{ProjectID: nandanam, CustomerID: asha, BankID: f.bankID, LoanAmount: "100000", AssigneeID: f.actor.ID}); err != nil {
		t.Fatalf("create: %v", err)
	}

	groups, err := f.svc.Search(ctx, SearchQuery{Text: "nandanam"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if groupFor(groups, asha) == nil {
		t.Fatalf("project search missed Asha: %+v", groups)
	}

	groups, _ = f.svc.Search(ctx, SearchQuery{Text: "na"})
	if groupFor(groups, f.customer) != nil {
		t.Fatalf("search 'na' matched Ravi through the NA project: %+v", groups)
	}
	if groupFor(groups, nandini) == nil || groupFor(groups, asha) == nil {
		t.Fatalf("search 'na' should still match Nandini by name and Asha by project: %+v", groups)
	}
}
