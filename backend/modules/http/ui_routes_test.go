package apihttp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/Surya3355/nexflow/backend/modules/access"
)

// decodeJSON reads a JSON response body into v and closes it.
func decodeJSON(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func TestCaseListRowsHaveNamesAndProgress(t *testing.T) {
	e := newEnv(t)
	resp := e.do(e.clients[access.RoleEmployee], "GET", "/api/cases", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/cases = %d", resp.StatusCode)
	}
	var rows []map[string]any
	decodeJSON(t, resp, &rows)
	if len(rows) == 0 {
		t.Fatal("no rows")
	}
	r := rows[0]
	if r["customer_name"] == "" || r["customer_name"] == nil || r["bank_name"] == nil || r["assignee_name"] == nil {
		t.Fatalf("missing names: %v", r)
	}
	if r["stage_count"] != float64(6) || r["stage_name"] == nil || r["stage_since"] == nil {
		t.Fatalf("missing progress: %v", r)
	}
	if _, ok := r["needs_document"].(bool); !ok {
		t.Fatalf("needs_document missing: %v", r)
	}
}

func TestCaseAttentionRoute(t *testing.T) {
	e := newEnv(t)
	resp := e.do(e.clients[access.RoleEmployee], "GET", "/api/cases/attention", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/cases/attention = %d", resp.StatusCode)
	}
	var rows []map[string]any
	decodeJSON(t, resp, &rows)
	if rows == nil {
		t.Fatal("want a JSON array")
	}
}

func TestDashboardRouteHasPeriodKeys(t *testing.T) {
	e := newEnv(t)
	resp := e.do(e.clients[access.RoleEmployee], "GET", "/api/reports/dashboard?bank_id="+fmt.Sprint(e.ids["bank"]), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET dashboard = %d", resp.StatusCode)
	}
	var d map[string]any
	decodeJSON(t, resp, &d)
	for _, k := range []string{"new_customers", "new_cases", "stage_moves_today", "new_customers_today"} {
		if _, ok := d[k]; !ok {
			t.Errorf("missing %q in %v", k, d)
		}
	}
	bad := e.do(e.clients[access.RoleEmployee], "GET", "/api/reports/dashboard?bank_id=x", nil)
	bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad bank_id = %d, want 400", bad.StatusCode)
	}
}

func TestAdminUsersRoute(t *testing.T) {
	e := newEnv(t)
	for role, want := range map[access.Role]int{access.RoleAdmin: 200, access.RoleManager: 403, access.RoleEmployee: 403} {
		resp := e.do(e.clients[role], "GET", "/api/admin/users", nil)
		if resp.StatusCode != want {
			resp.Body.Close()
			t.Fatalf("%s GET /api/admin/users = %d, want %d", role, resp.StatusCode, want)
		}
		if role == access.RoleAdmin {
			var team []map[string]any
			decodeJSON(t, resp, &team)
			if len(team) < 3 || team[0]["open_cases"] == nil || team[0]["role"] == nil {
				t.Fatalf("team = %v", team)
			}
		} else {
			resp.Body.Close()
		}
	}
}

func TestAdminResetPasswordRoute(t *testing.T) {
	e := newEnv(t)
	path := fmt.Sprintf("/api/admin/users/%d/password", e.ids["employee"])
	for _, role := range []access.Role{access.RoleManager, access.RoleEmployee} {
		resp := e.do(e.clients[role], "POST", path, map[string]string{"password": "another-long-password"})
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("%s reset = %d, want 403", role, resp.StatusCode)
		}
	}
	weak := e.do(e.clients[access.RoleAdmin], "POST", path, map[string]string{"password": "short"})
	weak.Body.Close()
	if weak.StatusCode != http.StatusBadRequest {
		t.Fatalf("weak reset = %d, want 400", weak.StatusCode)
	}
	ok := e.do(e.clients[access.RoleAdmin], "POST", path, map[string]string{"password": "another-long-password"})
	ok.Body.Close()
	if ok.StatusCode != http.StatusNoContent {
		t.Fatalf("admin reset = %d, want 204", ok.StatusCode)
	}
}

func TestRenameBankRoute(t *testing.T) {
	e := newEnv(t)
	path := fmt.Sprintf("/api/admin/banks/%d", e.ids["bank"])
	denied := e.do(e.clients[access.RoleManager], "PUT", path, map[string]string{"name": "Renamed"})
	denied.Body.Close()
	if denied.StatusCode != http.StatusForbidden {
		t.Fatalf("manager rename = %d, want 403", denied.StatusCode)
	}
	empty := e.do(e.clients[access.RoleAdmin], "PUT", path, map[string]string{"name": " "})
	empty.Body.Close()
	if empty.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty rename = %d, want 400", empty.StatusCode)
	}
	ok := e.do(e.clients[access.RoleAdmin], "PUT", path, map[string]string{"name": "Renamed Bank"})
	var b map[string]any
	decodeJSON(t, ok, &b)
	if ok.StatusCode != http.StatusOK || b["name"] != "Renamed Bank" {
		t.Fatalf("rename = %d %v", ok.StatusCode, b)
	}
	list := e.do(e.clients[access.RoleEmployee], "GET", "/api/config/banks", nil)
	var banks []map[string]any
	decodeJSON(t, list, &banks)
	if len(banks) == 0 {
		t.Fatal("no banks")
	}
	if _, has := banks[0]["has_own_stages"]; !has {
		t.Fatalf("has_own_stages missing: %v", banks[0])
	}
}

func TestCustomerCasesHaveStageNames(t *testing.T) {
	e := newEnv(t)
	resp := e.do(e.clients[access.RoleEmployee], "GET", fmt.Sprintf("/api/customers/%d", e.ids["customer"]), nil)
	var c map[string]any
	decodeJSON(t, resp, &c)
	list, _ := c["cases"].([]any)
	if len(list) == 0 {
		t.Fatalf("no cases: %v", c)
	}
	first, _ := list[0].(map[string]any)
	if first["stage_name"] == nil || first["bank_name"] == nil {
		t.Fatalf("case lacks names: %v", first)
	}
}

// A customer created at 00:30 IST on 13 Nov (19:00 UTC on 12 Nov) belongs to 13 Nov.
func TestDashboardDatesAreIndiaDays(t *testing.T) {
	e := newEnv(t)
	id := freshCustomer(t, e)
	if _, err := e.pool.Exec(context.Background(), `UPDATE customers SET created_at = '2026-11-12 19:00:00+00' WHERE id = $1`, id); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	get := func(day string) float64 {
		resp := e.do(e.clients[access.RoleEmployee], "GET", "/api/reports/dashboard?from="+day+"&to="+day, nil)
		var d map[string]any
		decodeJSON(t, resp, &d)
		n, _ := d["new_customers"].(float64)
		return n
	}
	if n := get("2026-11-13"); n != 1 {
		t.Fatalf("13 Nov IST new customers = %v, want 1", n)
	}
	if n := get("2026-11-12"); n != 0 {
		t.Fatalf("12 Nov IST new customers = %v, want 0", n)
	}
}
