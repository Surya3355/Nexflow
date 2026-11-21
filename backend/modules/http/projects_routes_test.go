package apihttp

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/Surya3355/nexflow/backend/modules/access"
)

func TestProjectRoutes(t *testing.T) {
	e := newEnv(t)
	list := e.do(e.clients[access.RoleEmployee], "GET", "/api/config/projects", nil)
	var ps []map[string]any
	decodeJSON(t, list, &ps)
	if list.StatusCode != http.StatusOK || len(ps) == 0 || ps[0]["name"] != "NA" || ps[0]["is_system"] != true {
		t.Fatalf("list = %d %v, want NA first", list.StatusCode, ps)
	}
	for _, k := range []string{"id", "open_cases", "total_cases"} {
		if _, ok := ps[0][k]; !ok {
			t.Fatalf("project missing %q: %v", k, ps[0])
		}
	}

	denied := e.do(e.clients[access.RoleEmployee], "POST", "/api/admin/projects", map[string]string{"name": "Prestige"})
	denied.Body.Close()
	if denied.StatusCode != http.StatusForbidden {
		t.Fatalf("employee create = %d, want 403", denied.StatusCode)
	}

	created := e.do(e.clients[access.RoleAdmin], "POST", "/api/admin/projects", map[string]string{"name": "Prestige"})
	var p map[string]any
	decodeJSON(t, created, &p)
	if created.StatusCode != http.StatusCreated || p["name"] != "Prestige" {
		t.Fatalf("create = %d %v", created.StatusCode, p)
	}

	dup := e.do(e.clients[access.RoleAdmin], "POST", "/api/admin/projects", map[string]string{"name": " prestige "})
	var body map[string]string
	decodeJSON(t, dup, &body)
	if dup.StatusCode != http.StatusConflict || !strings.Contains(body["error"], "a project with this name already exists") {
		t.Fatalf("duplicate = %d %v", dup.StatusCode, body)
	}

	renamed := e.do(e.clients[access.RoleAdmin], "PUT", fmt.Sprintf("/api/admin/projects/%v", p["id"]), map[string]string{"name": "Prestige Lakeside"})
	decodeJSON(t, renamed, &p)
	if renamed.StatusCode != http.StatusOK || p["name"] != "Prestige Lakeside" {
		t.Fatalf("rename = %d %v", renamed.StatusCode, p)
	}

	locked := e.do(e.clients[access.RoleAdmin], "PUT", fmt.Sprintf("/api/admin/projects/%v", ps[0]["id"]), map[string]string{"name": "None"})
	locked.Body.Close()
	if locked.StatusCode != http.StatusConflict {
		t.Fatalf("rename NA = %d, want 409", locked.StatusCode)
	}
}

func TestChangeCaseProjectRoute(t *testing.T) {
	e := newEnv(t)
	created := e.do(e.clients[access.RoleAdmin], "POST", "/api/admin/projects", map[string]string{"name": "Sobha"})
	var p map[string]any
	decodeJSON(t, created, &p)

	path := fmt.Sprintf("/api/cases/%d/project", e.ids["case"])
	resp := e.do(e.clients[access.RoleEmployee], "POST", path, map[string]any{"project_id": p["id"]})
	var c map[string]any
	decodeJSON(t, resp, &c)
	if resp.StatusCode != http.StatusOK || c["project_id"] != p["id"] {
		t.Fatalf("change = %d %v", resp.StatusCode, c)
	}
	missing := e.do(e.clients[access.RoleEmployee], "POST", path, map[string]any{"project_id": 0})
	missing.Body.Close()
	if missing.StatusCode != http.StatusBadRequest {
		t.Fatalf("no project = %d, want 400", missing.StatusCode)
	}

	rows := e.do(e.clients[access.RoleEmployee], "GET", "/api/cases", nil)
	var list []map[string]any
	decodeJSON(t, rows, &list)
	if len(list) == 0 || list[0]["project_name"] == nil {
		t.Fatalf("case rows missing project_name: %v", list)
	}
}

func TestBringBackRoutes(t *testing.T) {
	e := newEnv(t)
	cust := freshCustomer(t, e)
	if _, err := e.pool.Exec(context.Background(), `UPDATE customers SET status = 'dropped', dropped_at = now(),
		dropped_reason_id = (SELECT id FROM dropdown_values WHERE category = 'drop_reason' LIMIT 1) WHERE id = $1`, cust); err != nil {
		t.Fatal(err)
	}
	var na int64
	mustScan(t, e.pool.QueryRow(context.Background(), `SELECT id FROM projects WHERE is_system`), &na)
	body := map[string]any{"customer_id": cust, "bank_id": e.ids["bank"], "project_id": na, "loan_amount": "100000", "assignee_id": e.ids["employee"]}

	refused := e.do(e.clients[access.RoleEmployee], "POST", "/api/cases", body)
	var msg map[string]string
	decodeJSON(t, refused, &msg)
	if refused.StatusCode != http.StatusConflict || !strings.Contains(msg["error"], "Bring them back first") {
		t.Fatalf("dropped customer case = %d %v, want 409", refused.StatusCode, msg)
	}
	body["bring_back"] = true
	ok := e.do(e.clients[access.RoleEmployee], "POST", "/api/cases", body)
	ok.Body.Close()
	if ok.StatusCode != http.StatusCreated {
		t.Fatalf("bring back case = %d, want 201", ok.StatusCode)
	}

	again := e.do(e.clients[access.RoleEmployee], "POST", fmt.Sprintf("/api/customers/%d/reactivate", cust), nil)
	again.Body.Close()
	if again.StatusCode != http.StatusConflict {
		t.Fatalf("reactivate active = %d, want 409", again.StatusCode)
	}
}
