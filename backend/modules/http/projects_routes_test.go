package apihttp

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/Surya3355/nexflow/backend/modules/access"
)

// projectBody is a complete project form with one contact.
func projectBody(name, phone string) map[string]any {
	return map[string]any{"name": name, "type": "villas", "address": "Road 1",
		"contacts": []map[string]any{{"name": "Ravi", "phone": phone, "label": "Sales"}}}
}

func TestProjectRoutes(t *testing.T) {
	e := newEnv(t)
	list := e.do(e.clients[access.RoleEmployee], "GET", "/api/config/projects", nil)
	var ps []map[string]any
	decodeJSON(t, list, &ps)
	if list.StatusCode != http.StatusOK || len(ps) == 0 || ps[0]["name"] != "NA" || ps[0]["is_system"] != true {
		t.Fatalf("list = %d %v, want NA first", list.StatusCode, ps)
	}
	for _, k := range []string{"id", "open_cases", "total_cases", "type", "address", "active_contacts"} {
		if _, ok := ps[0][k]; !ok {
			t.Fatalf("project missing %q: %v", k, ps[0])
		}
	}

	for _, role := range []access.Role{access.RoleEmployee, access.RoleManager} {
		denied := e.do(e.clients[role], "POST", "/api/admin/projects", projectBody("Prestige", "9876543210"))
		denied.Body.Close()
		if denied.StatusCode != http.StatusForbidden {
			t.Fatalf("%s create = %d, want 403", role, denied.StatusCode)
		}
	}

	created := e.do(e.clients[access.RoleAdmin], "POST", "/api/admin/projects", map[string]any{"name": "Prestige", "type": "villas", "address": "Road 1",
		"contacts": []map[string]any{{"name": "Ravi", "phone": "+91 98765-43210", "label": "Sales"}}})
	var p map[string]any
	decodeJSON(t, created, &p)
	contacts, _ := p["contacts"].([]any)
	if created.StatusCode != http.StatusCreated || p["name"] != "Prestige" || p["type"] != "villas" || len(contacts) != 1 {
		t.Fatalf("create = %d %v", created.StatusCode, p)
	}
	first, _ := contacts[0].(map[string]any)
	if first["phone"] != "9876543210" || first["label"] != "Sales" {
		t.Fatalf("contact = %v", first)
	}

	dup := e.do(e.clients[access.RoleAdmin], "POST", "/api/admin/projects", projectBody(" prestige ", "9876543210"))
	var body map[string]string
	decodeJSON(t, dup, &body)
	if dup.StatusCode != http.StatusConflict || !strings.Contains(body["error"], "a project with this name already exists") {
		t.Fatalf("duplicate = %d %v", dup.StatusCode, body)
	}

	path := fmt.Sprintf("/api/admin/projects/%v", p["id"])
	for _, tc := range []struct {
		body map[string]any
		code int
		msg  string
	}{
		{map[string]any{"name": "P", "type": "villas", "address": "Road 1", "contacts": []any{}}, http.StatusBadRequest, "add at least one contact"},
		{map[string]any{"name": "P", "type": "flats", "address": "Road 1"}, http.StatusBadRequest, "choose a project type"},
		{map[string]any{"name": "P", "type": "villas", "address": " "}, http.StatusBadRequest, "enter the project address"},
		{map[string]any{"name": "P", "type": "villas", "address": "R", "contacts": []map[string]any{{"name": " ", "phone": "9000000001"}}}, http.StatusBadRequest, "contact name is required"},
		{map[string]any{"name": "P", "type": "villas", "address": "R", "contacts": []map[string]any{{"name": "A", "phone": "9000000001"}, {"name": "B", "phone": "9000000001"}}}, http.StatusConflict, "already a contact on this project"},
		{map[string]any{"name": "P", "type": "villas", "address": "R", "contacts": []map[string]any{{"id": 999999, "name": "A", "phone": "9000000001"}}}, http.StatusBadRequest, "contact not found on this project"},
	} {
		resp := e.do(e.clients[access.RoleAdmin], "PUT", path, tc.body)
		var m map[string]string
		decodeJSON(t, resp, &m)
		if resp.StatusCode != tc.code || !strings.Contains(m["error"], tc.msg) {
			t.Errorf("save %v = %d %v, want %d %q", tc.body, resp.StatusCode, m, tc.code, tc.msg)
		}
	}

	save := map[string]any{"name": "Prestige Lakeside", "type": "plots", "address": "Road 2",
		"contacts": []map[string]any{{"id": first["id"], "name": "Ravi", "phone": "9876543210"}, {"name": "Site", "phone": "9000000002"}}}
	renamed := e.do(e.clients[access.RoleAdmin], "PUT", path, save)
	decodeJSON(t, renamed, &p)
	if renamed.StatusCode != http.StatusOK || p["name"] != "Prestige Lakeside" || p["type"] != "plots" || len(p["contacts"].([]any)) != 2 {
		t.Fatalf("save = %d %v", renamed.StatusCode, p)
	}

	one := e.do(e.clients[access.RoleEmployee], "GET", fmt.Sprintf("/api/config/projects/%v", p["id"]), nil)
	var g map[string]any
	decodeJSON(t, one, &g)
	if one.StatusCode != http.StatusOK || g["address"] != "Road 2" || len(g["contacts"].([]any)) != 2 {
		t.Fatalf("get = %d %v", one.StatusCode, g)
	}
	missing := e.do(e.clients[access.RoleEmployee], "GET", "/api/config/projects/999999", nil)
	missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("get missing = %d, want 404", missing.StatusCode)
	}

	locked := e.do(e.clients[access.RoleAdmin], "PUT", fmt.Sprintf("/api/admin/projects/%v", ps[0]["id"]), projectBody("None", "9000000003"))
	locked.Body.Close()
	if locked.StatusCode != http.StatusConflict {
		t.Fatalf("save NA = %d, want 409", locked.StatusCode)
	}
}

// newProject creates a complete project as admin and returns its id and first contact id.
func newProject(t *testing.T, e *env, name, phone string) (int64, int64) {
	t.Helper()
	resp := e.do(e.clients[access.RoleAdmin], "POST", "/api/admin/projects", projectBody(name, phone))
	var p map[string]any
	decodeJSON(t, resp, &p)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create project = %d %v", resp.StatusCode, p)
	}
	c := p["contacts"].([]any)[0].(map[string]any)
	return int64(p["id"].(float64)), int64(c["id"].(float64))
}

func TestChangeCaseProjectRoute(t *testing.T) {
	e := newEnv(t)
	pid, contact := newProject(t, e, "Sobha", "9000000011")

	path := fmt.Sprintf("/api/cases/%d/project", e.ids["case"])
	resp := e.do(e.clients[access.RoleEmployee], "POST", path, map[string]any{"project_id": pid})
	var c map[string]any
	decodeJSON(t, resp, &c)
	if resp.StatusCode != http.StatusOK || c["project_id"] != float64(pid) || c["builder_contact_id"] != float64(contact) {
		t.Fatalf("change = %d %v", resp.StatusCode, c)
	}
	bc, _ := c["builder_contact"].(map[string]any)
	if bc["phone"] != "9000000011" || bc["removed"] != false {
		t.Fatalf("builder_contact = %v", c["builder_contact"])
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
	if _, ok := list[0]["builder_contact_id"]; !ok {
		t.Fatalf("case rows missing builder_contact_id: %v", list[0])
	}
}

func TestCaseBuilderContactRoute(t *testing.T) {
	e := newEnv(t)
	pid, _ := newProject(t, e, "Sobha", "9000000011")
	_, foreign := newProject(t, e, "Other", "9000000012")
	if r := e.do(e.clients[access.RoleEmployee], "POST", fmt.Sprintf("/api/cases/%d/project", e.ids["case"]), map[string]any{"project_id": pid}); r.StatusCode != http.StatusOK {
		t.Fatalf("move case = %d", r.StatusCode)
	}
	path := fmt.Sprintf("/api/cases/%d/builder-contact", e.ids["case"])
	resp := e.do(e.clients[access.RoleEmployee], "POST", path, map[string]any{"contact_id": foreign})
	var m map[string]string
	decodeJSON(t, resp, &m)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(m["error"], "pick a current contact of this project") {
		t.Fatalf("foreign contact = %d %v", resp.StatusCode, m)
	}

	var na int64
	mustScan(t, e.pool.QueryRow(context.Background(), `SELECT id FROM projects WHERE is_system`), &na)
	cust := freshCustomer(t, e)
	body := map[string]any{"customer_id": cust, "bank_id": e.ids["bank"], "project_id": pid, "loan_amount": "100000", "assignee_id": e.ids["employee"]}
	need := e.do(e.clients[access.RoleEmployee], "POST", "/api/cases", body)
	decodeJSON(t, need, &m)
	if need.StatusCode != http.StatusBadRequest || !strings.Contains(m["error"], "choose a builder contact") {
		t.Fatalf("case without contact = %d %v", need.StatusCode, m)
	}
}

func TestLeadBuilderContactRoute(t *testing.T) {
	e := newEnv(t)
	pid, contact := newProject(t, e, "Sobha", "9000000011")
	created := e.do(e.clients[access.RoleEmployee], "POST", "/api/leads", map[string]any{"name": "Asha", "mobile": "9811111111", "project_id": pid, "assignee_id": e.ids["employee"]})
	var res map[string]any
	decodeJSON(t, created, &res)
	lead, _ := res["lead"].(map[string]any)
	if created.StatusCode != http.StatusCreated || lead == nil {
		t.Fatalf("create lead = %d %v", created.StatusCode, res)
	}
	id := lead["id"]
	if _, err := e.pool.Exec(context.Background(), `UPDATE project_contacts SET removed_at = now() WHERE id = $1`, contact); err != nil {
		t.Fatal(err)
	}
	got := e.do(e.clients[access.RoleEmployee], "GET", fmt.Sprintf("/api/leads/%v", id), nil)
	var wrap map[string]any
	decodeJSON(t, got, &wrap)
	row, _ := wrap["lead"].(map[string]any)
	bc, _ := row["builder_contact"].(map[string]any)
	if row["builder_contact_id"] != float64(contact) || bc["removed"] != true || bc["name"] != "Ravi" {
		t.Fatalf("lead row = %v", row)
	}
	resp := e.do(e.clients[access.RoleEmployee], "POST", fmt.Sprintf("/api/leads/%v/builder-contact", id), map[string]any{"contact_id": contact})
	var m map[string]string
	decodeJSON(t, resp, &m)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(m["error"], "pick a current contact of this project") {
		t.Fatalf("removed contact = %d %v", resp.StatusCode, m)
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
