package apihttp

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Surya3355/nexflow/backend/modules/access"
)

// hdfcEnv switches the env's bank to the HDFC stage list and returns the stage ids by code.
func hdfcEnv(t *testing.T, e *env) map[string]float64 {
	t.Helper()
	resp := e.do(e.clients[access.RoleAdmin], "PATCH", fmt.Sprintf("/api/admin/banks/%d", e.ids["bank"]), map[string]any{"stage_list": "hdfc"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("switch bank = %d, want 204", resp.StatusCode)
	}
	var stages []map[string]any
	decodeJSON(t, e.do(e.clients[access.RoleEmployee], "GET", fmt.Sprintf("/api/config/banks/%d/stages", e.ids["bank"]), nil), &stages)
	ids := map[string]float64{}
	for _, s := range stages {
		ids[s["code"].(string)] = s["id"].(float64)
	}
	return ids
}

func TestBankSettingsRoutes(t *testing.T) {
	e := newEnv(t)
	path := fmt.Sprintf("/api/admin/banks/%d", e.ids["bank"])
	for _, role := range []access.Role{access.RoleManager, access.RoleEmployee} {
		r := e.do(e.clients[role], "PATCH", path, map[string]any{"part_payments": true})
		r.Body.Close()
		if r.StatusCode != http.StatusForbidden {
			t.Fatalf("%s patch bank = %d, want 403", role, r.StatusCode)
		}
	}
	bad := e.do(e.clients[access.RoleAdmin], "PATCH", path, map[string]any{"stage_list": "standard"})
	bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("stage_list standard = %d, want 400", bad.StatusCode)
	}
	ids := hdfcEnv(t, e)
	again := e.do(e.clients[access.RoleAdmin], "PATCH", path, map[string]any{"stage_list": "hdfc"})
	again.Body.Close()
	if again.StatusCode != http.StatusConflict {
		t.Fatalf("second switch = %d, want 409", again.StatusCode)
	}
	var banks []map[string]any
	decodeJSON(t, e.do(e.clients[access.RoleEmployee], "GET", "/api/config/banks", nil), &banks)
	if banks[0]["stage_list"] != "hdfc" || banks[0]["part_payments"] != true {
		t.Fatalf("bank = %v", banks[0])
	}
	var stages []map[string]any
	decodeJSON(t, e.do(e.clients[access.RoleEmployee], "GET", fmt.Sprintf("/api/config/banks/%d/stages", e.ids["bank"]), nil), &stages)
	if stages[1]["role"] != "start" || stages[0]["role"] != "" || len(ids) != 11 {
		t.Fatalf("stages = %v", stages)
	}
	off := e.do(e.clients[access.RoleAdmin], "PATCH", path, map[string]any{"part_payments": false})
	off.Body.Close()
	decodeJSON(t, e.do(e.clients[access.RoleEmployee], "GET", "/api/config/banks", nil), &banks)
	if off.StatusCode != http.StatusNoContent || banks[0]["part_payments"] != false {
		t.Fatalf("part payments off = %d %v", off.StatusCode, banks[0])
	}

	var lists []map[string]any
	r := e.do(e.clients[access.RoleAdmin], "GET", "/api/admin/stage-lists", nil)
	decodeJSON(t, r, &lists)
	if r.StatusCode != http.StatusOK || len(lists) != 2 || lists[0]["bank_id"] != nil || lists[1]["name"] != "HDFC" {
		t.Fatalf("stage lists = %d %v", r.StatusCode, lists)
	}
	denied := e.do(e.clients[access.RoleManager], "GET", "/api/admin/stage-lists", nil)
	denied.Body.Close()
	if denied.StatusCode != http.StatusForbidden {
		t.Fatalf("manager stage lists = %d, want 403", denied.StatusCode)
	}
}

func TestHDFCCaseFlowRoutes(t *testing.T) {
	e := newEnv(t)
	ids := hdfcEnv(t, e)
	emp, boss := e.clients[access.RoleEmployee], e.clients[access.RoleManager]
	c := e.ids["case"] // moved to Link sent by the switch
	stage := fmt.Sprintf("/api/cases/%d/stage", c)

	var out map[string]any
	r := e.do(emp, "POST", stage, map[string]any{"stage_id": ids["sanctioned"]})
	decodeJSON(t, r, &out)
	if r.StatusCode != http.StatusBadRequest || !strings.Contains(out["error"].(string), "sanctioned amount") {
		t.Fatalf("sanction without amount = %d %v", r.StatusCode, out)
	}
	r = e.do(emp, "POST", stage, map[string]any{"stage_id": ids["sanctioned"], "remark": "bank cut the amount",
		"sanction": map[string]any{"amount": "80000", "rate_percent": "8.5"}})
	decodeJSON(t, r, &out)
	if r.StatusCode != http.StatusOK || out["loan_amount"] != "80000.00" {
		t.Fatalf("sanction = %d %v", r.StatusCode, out)
	}
	r = e.do(emp, "POST", stage, map[string]any{"stage_id": ids["disbursement"], "payment": map[string]any{"amount": "30000", "date": "2026-11-05", "kind": "part"}})
	decodeJSON(t, r, &out)
	if r.StatusCode != http.StatusOK || out["status"] != "active" {
		t.Fatalf("first part = %d %v", r.StatusCode, out)
	}

	pay := fmt.Sprintf("/api/cases/%d/payments", c)
	r = e.do(emp, "POST", pay, map[string]any{"amount": "20000", "date": "2026-11-18", "kind": "part"})
	decodeJSON(t, r, &out)
	if r.StatusCode != http.StatusOK || out["total_paid"] != "50000.00" || out["remaining"] != "30000.00" || out["part_payments"] != true {
		t.Fatalf("add part = %d %v", r.StatusCode, out)
	}
	payments, _ := out["payments"].([]any)
	if len(payments) != 2 {
		t.Fatalf("payments = %v", out["payments"])
	}
	first := payments[0].(map[string]any)
	if first["paid_on"] != "2026-11-05" || first["kind"] != "part" || first["amount"] != "30000.00" {
		t.Fatalf("first payment = %v", first)
	}
	one := fmt.Sprintf("%s/%v", pay, first["id"])
	for _, m := range []string{"PATCH", "DELETE"} {
		r = e.do(emp, m, one, map[string]any{"amount": "1", "kind": "part"})
		r.Body.Close()
		if r.StatusCode != http.StatusForbidden {
			t.Fatalf("employee %s payment = %d, want 403", m, r.StatusCode)
		}
	}
	r = e.do(boss, "PATCH", one, map[string]any{"amount": "35000", "date": "2026-11-05", "kind": "part"})
	decodeJSON(t, r, &out)
	if r.StatusCode != http.StatusOK || out["total_paid"] != "55000.00" {
		t.Fatalf("manager patch = %d %v", r.StatusCode, out)
	}
	r = e.do(emp, "POST", pay, map[string]any{"amount": "25000", "date": "2026-11-20", "kind": "full"})
	decodeJSON(t, r, &out)
	if r.StatusCode != http.StatusOK || out["status"] != "completed" || out["fully_paid"] != true || out["remaining"] != "0.00" {
		t.Fatalf("full = %d %v", r.StatusCode, out)
	}
	r = e.do(emp, "POST", pay, map[string]any{"amount": "1", "kind": "full"})
	r.Body.Close()
	if r.StatusCode != http.StatusConflict && r.StatusCode != http.StatusBadRequest {
		t.Fatalf("payment after full = %d, want 409", r.StatusCode)
	}

	r = e.do(emp, "GET", fmt.Sprintf("/api/cases/%d", c), nil)
	decodeJSON(t, r, &out)
	if out["over_paid"] != false || len(out["payments"].([]any)) != 3 {
		t.Fatalf("get case = %v", out)
	}
	r = e.do(boss, "DELETE", one, nil)
	decodeJSON(t, r, &out)
	if r.StatusCode != http.StatusOK || out["status"] != "completed" {
		t.Fatalf("delete part = %d %v", r.StatusCode, out)
	}

	var hist []map[string]any
	decodeJSON(t, e.do(emp, "GET", fmt.Sprintf("/api/cases/%d/history", c), nil), &hist)
	found := false
	for _, h := range hist {
		if h["remark"] == "bank cut the amount" {
			found = true
		}
	}
	if !found {
		t.Fatalf("history has no remark: %v", hist)
	}

	var dash map[string]any
	decodeJSON(t, e.do(emp, "GET", "/api/reports/dashboard?from=2026-11-01&to=2026-11-30", nil), &dash)
	by, _ := dash["cases_by_stage"].([]any)
	if len(by) == 0 {
		t.Fatalf("cases_by_stage = %v", dash["cases_by_stage"])
	}
	row := by[0].(map[string]any)
	for _, k := range []string{"bank_id", "bank_name", "stage_id", "stage_name", "sort_order", "total"} {
		if _, ok := row[k]; !ok {
			t.Fatalf("cases_by_stage row missing %s: %v", k, row)
		}
	}
	disb := dash["disbursements"].(map[string]any)
	if disb["count"] != float64(1) || disb["amount"] != "45000.00" {
		t.Fatalf("disbursements tile = %v", disb)
	}

	var rep struct {
		Columns []string   `json:"columns"`
		Rows    [][]string `json:"rows"`
	}
	r = e.do(emp, "GET", "/api/reports/disbursements?from=2026-11-01&to=2026-11-30", nil)
	decodeJSON(t, r, &rep)
	if r.StatusCode != http.StatusOK || len(rep.Rows) != 1 || rep.Columns[10] != "Type" || rep.Rows[0][10] != "Full" || rep.Rows[0][7] != "45000.00" {
		t.Fatalf("disbursement report = %d %+v", r.StatusCode, rep)
	}
	csv := e.do(emp, "GET", "/api/reports/disbursements.csv?from=2026-11-01&to=2026-11-30", nil)
	body, _ := io.ReadAll(csv.Body)
	csv.Body.Close()
	if csv.StatusCode != http.StatusOK || !strings.Contains(csv.Header.Get("Content-Type"), "text/csv") || !strings.Contains(string(body), "Paid in period") {
		t.Fatalf("csv = %d %q", csv.StatusCode, body)
	}
}
