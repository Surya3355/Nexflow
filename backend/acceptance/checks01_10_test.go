package acceptance

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Surya3355/nexflow/backend/modules/access"
)

// Check 1: a new customer with a new mobile is created as Active.
func TestAcceptance01(t *testing.T) {
	e := newEnv(t)
	var out struct {
		Customer struct {
			Status string `json:"status"`
		} `json:"customer"`
	}
	e.json(access.RoleEmployee, "POST", "/api/customers", map[string]string{"name": "A", "mobile": "9100000001"}, http.StatusCreated, &out)
	if out.Customer.Status != "active" {
		t.Fatalf("status = %q, want active", out.Customer.Status)
	}
}

// Check 2: an existing mobile shows a warning and links the new case to the existing customer.
func TestAcceptance02(t *testing.T) {
	e := newEnv(t)
	first := e.newCustomer(access.RoleEmployee, "9100000002")
	var warn struct {
		Existing struct {
			ID int64 `json:"id"`
		} `json:"existing"`
	}
	e.json(access.RoleEmployee, "POST", "/api/customers", map[string]string{"name": "Dup", "mobile": "+91 91000 00002"}, http.StatusOK, &warn)
	if warn.Existing.ID != first {
		t.Fatalf("warning points at %d, want %d", warn.Existing.ID, first)
	}
	c := e.newCase(access.RoleEmployee, warn.Existing.ID, "300000")
	if got := e.count(`SELECT count(*) FROM cases WHERE id=$1 AND customer_id=$2`, c, first); got != 1 {
		t.Fatal("new case is not linked to the existing customer")
	}
	if n := e.count(`SELECT count(*) FROM customers WHERE mobile='9100000002'`); n != 1 {
		t.Fatalf("customers with that mobile = %d, want 1", n)
	}
}

// Check 3: a customer can have several cases, each under the same customer.
func TestAcceptance03(t *testing.T) {
	e := newEnv(t)
	cust := e.newCustomer(access.RoleEmployee, "9100000003")
	e.newCase(access.RoleEmployee, cust, "100000")
	e.newCase(access.RoleEmployee, cust, "200000")
	var out struct {
		Cases []struct {
			CustomerID int64 `json:"customer_id"`
		} `json:"cases"`
	}
	e.json(access.RoleEmployee, "GET", fmt.Sprintf("/api/customers/%d", cust), nil, http.StatusOK, &out)
	if len(out.Cases) != 2 {
		t.Fatalf("cases under customer = %d, want 2", len(out.Cases))
	}
}

// Check 4: a case starts at Received and moves in order; each move adds a dated history event.
func TestAcceptance04(t *testing.T) {
	e := newEnv(t)
	c := e.newCase(access.RoleEmployee, e.newCustomer(access.RoleEmployee, "9100000004"), "400000")
	e.moveStage(access.RoleEmployee, c, "logged_in", "ACC4")
	e.moveStage(access.RoleEmployee, c, "under_sanction", "")
	var hist []struct {
		StageCode string    `json:"stage_code"`
		ReachedAt time.Time `json:"reached_at"`
	}
	e.json(access.RoleEmployee, "GET", fmt.Sprintf("/api/cases/%d/history", c), nil, http.StatusOK, &hist)
	want := []string{"received", "logged_in", "under_sanction"}
	if len(hist) != len(want) {
		t.Fatalf("history length = %d, want %d", len(hist), len(want))
	}
	for i := range want {
		if hist[i].StageCode != want[i] || hist[i].ReachedAt.IsZero() {
			t.Fatalf("event %d = %+v, want %s with a date", i, hist[i], want[i])
		}
	}
}

// Check 5: tracking number is required at Logged in and cannot be duplicated.
func TestAcceptance05(t *testing.T) {
	e := newEnv(t)
	cust := e.newCustomer(access.RoleEmployee, "9100000005")
	a := e.newCase(access.RoleEmployee, cust, "100000")
	b := e.newCase(access.RoleEmployee, cust, "100000")
	e.json(access.RoleEmployee, "POST", fmt.Sprintf("/api/cases/%d/stage", a), map[string]any{"stage_id": e.stage["logged_in"]}, http.StatusBadRequest, nil)
	e.moveStage(access.RoleEmployee, a, "logged_in", "DUP5")
	e.json(access.RoleEmployee, "POST", fmt.Sprintf("/api/cases/%d/stage", b),
		map[string]any{"stage_id": e.stage["logged_in"], "tracking_number": "DUP5"}, http.StatusConflict, nil)
}

// Check 6: a customer or case can be dropped with a reason; it stays visible as Dropped.
// Dropping a customer drops all its cases, each linked to the customer drop.
func TestAcceptance06(t *testing.T) {
	e := newEnv(t)
	cust := e.newCustomer(access.RoleEmployee, "9100000006")
	c1 := e.newCase(access.RoleEmployee, cust, "100000")
	c2 := e.newCase(access.RoleEmployee, cust, "200000")
	e.json(access.RoleEmployee, "POST", fmt.Sprintf("/api/cases/%d/drop", c1),
		map[string]any{"reason_id": e.reasons["Customer unreachable"]}, http.StatusOK, nil)
	e.json(access.RoleEmployee, "POST", fmt.Sprintf("/api/customers/%d/drop", cust),
		map[string]any{"reason_id": e.reasons["Not eligible"]}, http.StatusOK, nil)
	var status string
	var viaCustomer bool
	mustScan(t, e.pool.QueryRow(context.Background(), `SELECT status, dropped_via_customer FROM cases WHERE id=$1`, c2), &status, &viaCustomer)
	if status != "dropped" || !viaCustomer {
		t.Fatalf("second case status=%s via_customer=%v, want dropped via customer", status, viaCustomer)
	}
	var first string
	mustScan(t, e.pool.QueryRow(context.Background(), `SELECT status FROM cases WHERE id=$1`, c1), &first)
	if first != "dropped" {
		t.Fatalf("first case status = %s, want dropped", first)
	}
}

// Check 7: an employee cannot hard-delete; a manager or admin can; the item is archived and audited.
func TestAcceptance07(t *testing.T) {
	e := newEnv(t)
	c := e.newCase(access.RoleEmployee, e.newCustomer(access.RoleEmployee, "9100000007"), "100000")
	e.json(access.RoleEmployee, "DELETE", fmt.Sprintf("/api/cases/%d", c), nil, http.StatusForbidden, nil)
	e.json(access.RoleManager, "DELETE", fmt.Sprintf("/api/cases/%d", c), nil, http.StatusNoContent, nil)
	if n := e.count(`SELECT count(*) FROM archive WHERE entity_type='case' AND entity_id=$1`, c); n != 1 {
		t.Fatalf("archive rows = %d, want 1", n)
	}
	if n := e.count(`SELECT count(*) FROM audit_log WHERE action='case.deleted' AND entity_id=$1`, c); n != 1 {
		t.Fatalf("audit rows = %d, want 1", n)
	}
}

// Check 8: a customer that still has cases cannot be hard-deleted.
func TestAcceptance08(t *testing.T) {
	e := newEnv(t)
	cust := e.newCustomer(access.RoleEmployee, "9100000008")
	e.newCase(access.RoleEmployee, cust, "100000")
	e.json(access.RoleAdmin, "DELETE", fmt.Sprintf("/api/customers/%d", cust), nil, http.StatusConflict, nil)
}

// Check 9: admin can restore an archived item; it returns as Dropped.
func TestAcceptance09(t *testing.T) {
	e := newEnv(t)
	c := e.newCase(access.RoleEmployee, e.newCustomer(access.RoleEmployee, "9100000009"), "100000")
	e.json(access.RoleManager, "DELETE", fmt.Sprintf("/api/cases/%d", c), nil, http.StatusNoContent, nil)
	var items []struct {
		ID         int64  `json:"id"`
		EntityID   int64  `json:"entity_id"`
		EntityType string `json:"entity_type"`
	}
	e.json(access.RoleAdmin, "GET", "/api/admin/archive", nil, http.StatusOK, &items)
	var aid int64
	for _, it := range items {
		if it.EntityType == "case" && it.EntityID == c {
			aid = it.ID
		}
	}
	if aid == 0 {
		t.Fatal("archived case not listed")
	}
	e.json(access.RoleAdmin, "POST", fmt.Sprintf("/api/admin/archive/%d/restore", aid),
		map[string]any{"reason_id": e.reasons["Not eligible"]}, http.StatusOK, nil)
	var status string
	mustScan(t, e.pool.QueryRow(context.Background(), `SELECT status FROM cases WHERE id=$1`, c), &status)
	if status != "dropped" {
		t.Fatalf("restored status = %s, want dropped", status)
	}
}

// Check 10: archived items and documents are purged after 1 year; the audit entry remains.
func TestAcceptance10(t *testing.T) {
	e := newEnv(t)
	c := e.newCase(access.RoleEmployee, e.newCustomer(access.RoleEmployee, "9100000010"), "100000")
	e.json(access.RoleManager, "DELETE", fmt.Sprintf("/api/cases/%d", c), nil, http.StatusNoContent, nil)
	if _, err := e.pool.Exec(context.Background(), `UPDATE archive SET purge_after = now() - interval '1 day'`); err != nil {
		t.Fatalf("expire: %v", err)
	}
	res, err := e.arch.PurgeExpired(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("PurgeExpired: %v", err)
	}
	if res.ArchiveRows != 1 || e.count(`SELECT count(*) FROM archive`) != 0 {
		t.Fatalf("archive not purged: %+v", res)
	}
	if e.count(`SELECT count(*) FROM audit_log WHERE action='case.deleted' AND entity_id=$1`, c) != 1 {
		t.Fatal("audit entry was purged with the archive")
	}
}
