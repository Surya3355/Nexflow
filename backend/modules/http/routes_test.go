package apihttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Surya3355/nexflow/backend/internal/platform/testdb"
	"github.com/Surya3355/nexflow/backend/migrations"
	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/archive"
	"github.com/Surya3355/nexflow/backend/modules/cases"
	"github.com/Surya3355/nexflow/backend/modules/customers"
	"github.com/Surya3355/nexflow/backend/modules/documents"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/Surya3355/nexflow/backend/modules/leads"
	"github.com/Surya3355/nexflow/backend/modules/officers"
	"github.com/Surya3355/nexflow/backend/modules/reports"
	"github.com/Surya3355/nexflow/backend/modules/workflows"
	"github.com/jackc/pgx/v5/pgxpool"
)

const password = "a-long-enough-password"

// env is a running API with one signed-in client per role.
type env struct {
	t       *testing.T
	pool    *pgxpool.Pool
	srv     *httptest.Server
	clients map[access.Role]*http.Client
	ids     map[string]int64
}

var seq atomic.Int64

func newEnv(t *testing.T) *env {
	t.Helper()
	pool := testdb.New(t)
	if err := migrations.Up(pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	rec := history.NewPgRecorder(pool)
	accessSvc := access.NewService(pool, rec)
	sm := access.NewSessionManager(pool, false)
	wf := workflows.NewService(pool, rec)
	e := &env{t: t, pool: pool, clients: map[access.Role]*http.Client{}, ids: map[string]int64{}}

	handler := NewRouter(Deps{
		Auth:      access.NewHandler(accessSvc, sm),
		Access:    accessSvc,
		Workflows: wf,
		Customers: customers.NewService(pool, rec),
		Cases:     cases.NewService(pool, wf, rec),
		Documents: documents.NewService(pool, documents.NewMemoryStore(), rec),
		Linker:    documents.NewLinker([]byte("route-test-secret-0123456789abcd")),
		Archive:   archive.NewService(pool, rec),
		Reports:   reports.NewService(pool),
		Leads:     leads.NewService(pool, rec),
		Officers:  officers.NewService(pool, wf, rec),
	})
	e.srv = httptest.NewServer(handler)
	t.Cleanup(e.srv.Close)

	ctx := context.Background()
	for _, role := range []access.Role{access.RoleAdmin, access.RoleManager, access.RoleEmployee} {
		name := "u-" + string(role)
		hash, _ := access.HashPassword(password)
		var id int64
		mustScan(t, pool.QueryRow(ctx, `INSERT INTO users (username, full_name, password_hash, role)
			VALUES ($1, $1, $2, $3) RETURNING id`, name, hash, string(role)), &id)
		e.ids[string(role)] = id
		jar, _ := cookiejar.New(nil)
		c := &http.Client{Jar: jar}
		resp := e.do(c, "POST", "/api/login", map[string]string{"username": name, "password": password})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("login %s: %d", name, resp.StatusCode)
		}
		resp.Body.Close()
		e.clients[role] = c
	}

	var bank, customer, caseID, stage int64
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO banks (name) VALUES ('HDFC') RETURNING id`), &bank)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO customers (name, mobile) VALUES ('Base Customer', '9000000000') RETURNING id`), &customer)
	mustScan(t, pool.QueryRow(ctx, `SELECT id FROM stages WHERE code='received' AND bank_id IS NULL`), &stage)
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO cases (customer_id, bank_id, loan_amount, current_stage_id, assignee_id)
		VALUES ($1, $2, 100000, $3, $4) RETURNING id`, customer, bank, stage, e.ids["employee"]), &caseID)
	e.ids["customer"] = customer
	e.ids["case"] = caseID
	e.ids["bank"] = bank
	mustScan(t, pool.QueryRow(ctx, `SELECT id FROM stages WHERE code='sanctioned' AND bank_id IS NULL`), &stage)
	e.ids["sanctioned"] = stage
	return e
}

func mustScan(t *testing.T, row interface{ Scan(...any) error }, dest ...any) {
	t.Helper()
	if err := row.Scan(dest...); err != nil {
		t.Fatalf("scan: %v", err)
	}
}

func (e *env) do(c *http.Client, method, path string, body any) *http.Response {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.Do(req)
	if err != nil {
		e.t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

// fresh returns a unique suffix so repeated creates do not collide.
func fresh() string { return fmt.Sprint(seq.Add(1)) }

func freshCustomer(t *testing.T, e *env) int64 {
	t.Helper()
	var id int64
	mustScan(t, e.pool.QueryRow(context.Background(),
		`INSERT INTO customers (name, mobile) VALUES ('Temp', $1) RETURNING id`,
		fmt.Sprintf("9%09d", seq.Add(1))), &id)
	return id
}

func freshCase(t *testing.T, e *env) int64 {
	t.Helper()
	var id int64
	mustScan(t, e.pool.QueryRow(context.Background(), `INSERT INTO cases (customer_id, bank_id, loan_amount, current_stage_id, assignee_id)
		VALUES ($1, $2, 100000, (SELECT id FROM stages WHERE code='received' AND bank_id IS NULL), $3) RETURNING id`,
		freshCustomer(t, e), e.ids["bank"], e.ids["employee"]), &id)
	return id
}

// Spec §6 as a route table: each row is one route, the status for each role.
// Admin, manager, employee. Destructive rows use a fresh record per role.
func TestRoutePermissionsByRole(t *testing.T) {
	e := newEnv(t)
	roles := []access.Role{access.RoleAdmin, access.RoleManager, access.RoleEmployee}

	type row struct {
		name string
		want map[access.Role]int
		call func(c *http.Client, role access.Role) *http.Response
	}
	rows := []row{
		{"create bank (admin only)", map[access.Role]int{access.RoleAdmin: 201, access.RoleManager: 403, access.RoleEmployee: 403},
			func(c *http.Client, _ access.Role) *http.Response {
				return e.do(c, "POST", "/api/admin/banks", map[string]string{"name": "Bank " + fresh()})
			}},
		{"create user (admin only)", map[access.Role]int{access.RoleAdmin: 201, access.RoleManager: 403, access.RoleEmployee: 403},
			func(c *http.Client, _ access.Role) *http.Response {
				return e.do(c, "POST", "/api/admin/users", map[string]string{"username": "new" + fresh(), "full_name": "New", "role": "employee", "password": password})
			}},
		{"add dropdown value (admin only)", map[access.Role]int{access.RoleAdmin: 201, access.RoleManager: 403, access.RoleEmployee: 403},
			func(c *http.Client, _ access.Role) *http.Response {
				return e.do(c, "POST", "/api/admin/dropdowns", map[string]any{"category": "drop_reason", "value": "Reason " + fresh(), "sort_order": 50})
			}},
		{"set requires_document (admin only)", map[access.Role]int{access.RoleAdmin: 204, access.RoleManager: 403, access.RoleEmployee: 403},
			func(c *http.Client, _ access.Role) *http.Response {
				return e.do(c, "PATCH", fmt.Sprintf("/api/admin/stages/%d", e.ids["sanctioned"]), map[string]bool{"requires_document": false})
			}},
		{"list archive (admin only)", map[access.Role]int{access.RoleAdmin: 200, access.RoleManager: 403, access.RoleEmployee: 403},
			func(c *http.Client, _ access.Role) *http.Response {
				return e.do(c, "GET", "/api/admin/archive", nil)
			}},
		{"add customer (all staff)", map[access.Role]int{access.RoleAdmin: 201, access.RoleManager: 201, access.RoleEmployee: 201},
			func(c *http.Client, _ access.Role) *http.Response {
				return e.do(c, "POST", "/api/customers", map[string]string{"name": "Cust", "mobile": fmt.Sprintf("8%09d", seq.Add(1))})
			}},
		{"assign case (all staff)", map[access.Role]int{access.RoleAdmin: 200, access.RoleManager: 200, access.RoleEmployee: 200},
			func(c *http.Client, _ access.Role) *http.Response {
				return e.do(c, "POST", fmt.Sprintf("/api/cases/%d/assign", e.ids["case"]), map[string]int64{"assignee_id": e.ids["employee"]})
			}},
		{"view banks (all staff)", map[access.Role]int{access.RoleAdmin: 200, access.RoleManager: 200, access.RoleEmployee: 200},
			func(c *http.Client, _ access.Role) *http.Response {
				return e.do(c, "GET", "/api/config/banks", nil)
			}},
		{"export CSV (all staff)", map[access.Role]int{access.RoleAdmin: 200, access.RoleManager: 200, access.RoleEmployee: 200},
			func(c *http.Client, _ access.Role) *http.Response {
				return e.do(c, "GET", "/api/reports/export.csv", nil)
			}},
		{"hard-delete customer (manager and admin)", map[access.Role]int{access.RoleAdmin: 204, access.RoleManager: 204, access.RoleEmployee: 403},
			func(c *http.Client, _ access.Role) *http.Response {
				return e.do(c, "DELETE", fmt.Sprintf("/api/customers/%d", freshCustomer(t, e)), nil)
			}},
		{"hard-delete case (manager and admin)", map[access.Role]int{access.RoleAdmin: 204, access.RoleManager: 204, access.RoleEmployee: 403},
			func(c *http.Client, _ access.Role) *http.Response {
				return e.do(c, "DELETE", fmt.Sprintf("/api/cases/%d", freshCase(t, e)), nil)
			}},
		{"restore archive (admin only)", map[access.Role]int{access.RoleAdmin: 404, access.RoleManager: 403, access.RoleEmployee: 403},
			func(c *http.Client, _ access.Role) *http.Response {
				return e.do(c, "POST", "/api/admin/archive/999999/restore", map[string]any{"reason_id": 1})
			}},
	}

	for _, r := range rows {
		for _, role := range roles {
			resp := r.call(e.clients[role], role)
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != r.want[role] {
				t.Errorf("%s as %s: status %d, want %d (%s)", r.name, role, resp.StatusCode, r.want[role], strings.TrimSpace(string(body)))
			}
		}
	}
}

func TestAnonymousRequestsRefused(t *testing.T) {
	e := newEnv(t)
	anon := &http.Client{}
	for _, path := range []string{"/api/config/banks", "/api/cases", "/api/reports/export.csv", "/api/search?q=ravi"} {
		resp := e.do(anon, "GET", path, nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("anonymous GET %s = %d, want 401", path, resp.StatusCode)
		}
	}
}

// A deactivated user loses access on the next request, not only at login.
func TestDeactivatedUserRefusedOnNextRequest(t *testing.T) {
	e := newEnv(t)
	c := e.clients[access.RoleEmployee]
	if resp := e.do(c, "GET", "/api/config/banks", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("before deactivation: %d", resp.StatusCode)
	} else {
		resp.Body.Close()
	}
	if _, err := e.pool.Exec(context.Background(), `UPDATE users SET is_active=false WHERE id=$1`, e.ids["employee"]); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	resp := e.do(c, "GET", "/api/config/banks", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("after deactivation: %d, want 401", resp.StatusCode)
	}
}

// Read endpoints the case and customer screens use are signed-in only.
func TestDetailReadsReturnData(t *testing.T) {
	e := newEnv(t)
	c := e.clients[access.RoleEmployee]
	for _, path := range []string{
		fmt.Sprintf("/api/cases/%d/history", e.ids["case"]),
		fmt.Sprintf("/api/customers/%d", e.ids["customer"]),
	} {
		resp := e.do(c, "GET", path, nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, resp.StatusCode)
		}
	}
}
