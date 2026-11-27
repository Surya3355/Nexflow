package apihttp

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/Surya3355/nexflow/backend/modules/access"
)

func TestLeadFlowRoutes(t *testing.T) {
	e := newEnv(t)
	c := e.clients[access.RoleEmployee]
	emp := e.ids["employee"]
	mobile := fmt.Sprintf("98%08d", seq.Add(1))

	resp := e.do(c, "POST", "/api/leads", map[string]any{"name": "Ravi Kumar", "mobile": mobile, "project_id": naID(t, e), "assignee_id": emp})
	var created struct {
		Lead     map[string]any `json:"lead"`
		Customer map[string]any `json:"customer"`
	}
	decodeJSON(t, resp, &created)
	if resp.StatusCode != http.StatusCreated || created.Lead["status"] != "open" || created.Customer["name"] != "Ravi Kumar" {
		t.Fatalf("create = %d %+v", resp.StatusCode, created)
	}
	for _, k := range []string{"customer_name", "project_name", "assignee_name", "next_call_date", "call_due", "overdue_days", "last_response", "case_id"} {
		if _, ok := created.Lead[k]; !ok {
			t.Errorf("lead JSON missing %q", k)
		}
	}
	leadID := int64(created.Lead["id"].(float64))

	again := e.do(c, "POST", "/api/leads", map[string]any{"name": "X", "mobile": mobile, "project_id": naID(t, e), "assignee_id": emp})
	var ex map[string]any
	decodeJSON(t, again, &ex)
	if again.StatusCode != http.StatusOK || ex["existing"] == nil {
		t.Fatalf("known mobile = %d %v", again.StatusCode, ex)
	}

	bad := e.do(c, "POST", fmt.Sprintf("/api/leads/%d/contacts", leadID), map[string]any{"response": "interested", "remark": " "})
	var msg map[string]string
	decodeJSON(t, bad, &msg)
	if bad.StatusCode != http.StatusBadRequest || msg["error"] != "leads: write a remark" {
		t.Fatalf("empty remark = %d %v", bad.StatusCode, msg)
	}
	ok := e.do(c, "POST", fmt.Sprintf("/api/leads/%d/contacts", leadID), map[string]any{"response": "call_back", "remark": "busy", "next_call_date": "2026-11-30"})
	ok.Body.Close()
	if ok.StatusCode != http.StatusCreated {
		t.Fatalf("contact = %d", ok.StatusCode)
	}
	got := e.do(c, "GET", fmt.Sprintf("/api/leads/%d", leadID), nil)
	var detail struct {
		Lead     map[string]any   `json:"lead"`
		Contacts []map[string]any `json:"contacts"`
	}
	decodeJSON(t, got, &detail)
	if detail.Lead["next_call_date"] != "2026-11-30" || len(detail.Contacts) != 1 || detail.Contacts[0]["remark"] != "busy" {
		t.Fatalf("detail = %+v", detail)
	}

	list := e.do(c, "GET", "/api/leads", nil)
	var rows []map[string]any
	decodeJSON(t, list, &rows)
	if len(rows) == 0 {
		t.Fatal("open leads list empty")
	}

	search := e.do(c, "GET", "/api/search?q=Ravi", nil)
	var groups []map[string]any
	decodeJSON(t, search, &groups)
	if len(groups) != 1 || len(groups[0]["leads"].([]any)) != 1 {
		t.Fatalf("search groups = %v", groups)
	}

	conv := map[string]any{"bank_id": e.ids["bank"], "loan_amount": "4500000", "assignee_id": emp,
		"unit_number": "b-14", "start_at": "logged_in", "tracking_number": fmt.Sprintf("hl%d", seq.Add(1))}
	cr := e.do(c, "POST", fmt.Sprintf("/api/leads/%d/case", leadID), conv)
	var cs map[string]any
	decodeJSON(t, cr, &cs)
	if cr.StatusCode != http.StatusCreated {
		t.Fatalf("convert = %d %v", cr.StatusCode, cs)
	}
	caseResp := e.do(c, "GET", fmt.Sprintf("/api/cases/%d", int64(cs["id"].(float64))), nil)
	var caseJSON map[string]any
	decodeJSON(t, caseResp, &caseJSON)
	if caseJSON["lead_id"] != float64(leadID) || caseJSON["unit_number"] != "B-14" {
		t.Fatalf("case = %v", caseJSON)
	}
	second := e.do(c, "POST", fmt.Sprintf("/api/leads/%d/case", leadID), conv)
	second.Body.Close()
	if second.StatusCode != http.StatusConflict {
		t.Fatalf("second convert = %d, want 409", second.StatusCode)
	}

	cust := e.do(c, "GET", fmt.Sprintf("/api/customers/%d", int64(created.Customer["id"].(float64))), nil)
	var custJSON map[string]any
	decodeJSON(t, cust, &custJSON)
	if ls, _ := custJSON["leads"].([]any); len(ls) != 1 {
		t.Fatalf("customer leads = %v", custJSON["leads"])
	}
}

func TestLeadRoutesNeedSignIn(t *testing.T) {
	e := newEnv(t)
	resp := e.do(&http.Client{}, "GET", "/api/leads", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("signed out = %d, want 401", resp.StatusCode)
	}
}

func TestNewCustomerNameRequiredIs400(t *testing.T) {
	e := newEnv(t)
	resp := e.do(e.clients[access.RoleEmployee], "POST", "/api/leads", map[string]any{
		"name": " ", "mobile": "9811111111", "project_id": naID(t, e), "assignee_id": e.ids["employee"]})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty name = %d, want 400", resp.StatusCode)
	}
}

func naID(t *testing.T, e *env) int64 {
	t.Helper()
	var id int64
	mustScan(t, e.pool.QueryRow(t.Context(), `SELECT id FROM projects WHERE is_system`), &id)
	return id
}

func TestAddCaseWithoutBankIs400(t *testing.T) {
	e := newEnv(t)
	c := e.clients[access.RoleEmployee]
	resp := e.do(c, "POST", "/api/cases", map[string]any{
		"customer_id": e.ids["customer"], "bank_id": 0, "project_id": naID(t, e), "loan_amount": "100000", "assignee_id": e.ids["employee"]})
	var msg map[string]string
	decodeJSON(t, resp, &msg)
	if resp.StatusCode != http.StatusBadRequest || msg["error"] != "cases: choose a bank" {
		t.Fatalf("no bank = %d %v, want 400 choose a bank", resp.StatusCode, msg)
	}
}
