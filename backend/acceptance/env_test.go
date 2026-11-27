// Package acceptance runs the spec §13 acceptance checks against the real
// router and database. Each check is one test named TestAcceptanceNN.
package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"

	"github.com/Surya3355/nexflow/backend/internal/platform/testdb"
	"github.com/Surya3355/nexflow/backend/migrations"
	"github.com/Surya3355/nexflow/backend/modules/access"
	"github.com/Surya3355/nexflow/backend/modules/alerts"
	"github.com/Surya3355/nexflow/backend/modules/archive"
	"github.com/Surya3355/nexflow/backend/modules/bankqueries"
	"github.com/Surya3355/nexflow/backend/modules/cases"
	"github.com/Surya3355/nexflow/backend/modules/customers"
	"github.com/Surya3355/nexflow/backend/modules/documents"
	"github.com/Surya3355/nexflow/backend/modules/history"
	apihttp "github.com/Surya3355/nexflow/backend/modules/http"
	"github.com/Surya3355/nexflow/backend/modules/leads"
	"github.com/Surya3355/nexflow/backend/modules/officers"
	"github.com/Surya3355/nexflow/backend/modules/reports"
	"github.com/Surya3355/nexflow/backend/modules/workflows"
	"github.com/jackc/pgx/v5/pgxpool"
)

const password = "a-long-enough-password"

// env is a fresh database with the API running and one client per role.
type env struct {
	t       *testing.T
	pool    *pgxpool.Pool
	srv     *httptest.Server
	clients map[access.Role]*http.Client
	ids     map[access.Role]int64
	queue   *alerts.Queue
	arch    *archive.Service
	store   *documents.MemoryStore
	reasons map[string]int64
	bank    int64
	project int64 // the NA project
	stage   map[string]int64
}

func newEnv(t *testing.T) *env {
	t.Helper()
	owner := testdb.New(t)
	if err := migrations.Up(owner); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// The API acts as the app role, as in production: it can't change history.
	appCfg := owner.Config().Copy()
	migrations.UseAppRole(appCfg)
	pool, err := pgxpool.NewWithConfig(context.Background(), appCfg)
	if err != nil {
		t.Fatalf("app pool: %v", err)
	}
	t.Cleanup(pool.Close)
	rec := history.NewPgRecorder(pool)
	accessSvc := access.NewService(pool, rec)
	sm := access.NewSessionManager(pool, false)
	wf := workflows.NewService(pool, rec)
	queue := alerts.NewQueue(pool)
	caseSvc := cases.NewService(pool, wf, rec)
	caseSvc.SetNotifier(queue)
	store := documents.NewMemoryStore()
	arch := archive.NewService(pool, rec)
	arch.SetFileRemover(store)

	e := &env{t: t, pool: pool, clients: map[access.Role]*http.Client{}, ids: map[access.Role]int64{},
		queue: queue, arch: arch, store: store, reasons: map[string]int64{}, stage: map[string]int64{}}
	docs := documents.NewService(pool, store, rec)
	h := apihttp.NewRouter(apihttp.Deps{
		Auth: access.NewHandler(accessSvc, sm), Access: accessSvc, Workflows: wf,
		Customers: customers.NewService(pool, rec), Cases: caseSvc,
		Documents: docs,
		Linker:    documents.NewLinker([]byte("acceptance-secret-0123456789abcdef")),
		Archive:   arch, Reports: reports.NewService(pool),
		Leads:    leads.NewService(pool, rec),
		Officers: officers.NewService(pool, wf, rec),
		Queries:  bankqueries.NewService(pool, docs, rec),
	})
	e.srv = httptest.NewServer(h)
	t.Cleanup(e.srv.Close)

	ctx := context.Background()
	for _, role := range []access.Role{access.RoleAdmin, access.RoleManager, access.RoleEmployee} {
		e.ids[role] = e.seedUser(string(role), string(role)+"@example.com", role)
		jar, _ := cookiejar.New(nil)
		c := &http.Client{Jar: jar}
		resp := e.call(c, "POST", "/api/login", map[string]string{"username": string(role), "password": password})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("login %s: %d", role, resp.StatusCode)
		}
		resp.Body.Close()
		e.clients[role] = c
	}
	mustScan(t, pool.QueryRow(ctx, `INSERT INTO banks (name) VALUES ('HDFC') RETURNING id`), &e.bank)
	mustScan(t, pool.QueryRow(ctx, `SELECT id FROM projects WHERE is_system`), &e.project)
	for _, code := range []string{"received", "logged_in", "under_sanction", "sanctioned", "documentation", "disbursed"} {
		var id int64
		mustScan(t, pool.QueryRow(ctx, `SELECT id FROM stages WHERE code=$1 AND bank_id IS NULL`, code), &id)
		e.stage[code] = id
	}
	for _, reason := range []string{"Not eligible", "Other", "Customer unreachable"} {
		var id int64
		mustScan(t, pool.QueryRow(ctx, `SELECT id FROM dropdown_values WHERE category='drop_reason' AND value=$1`, reason), &id)
		e.reasons[reason] = id
	}
	return e
}

func (e *env) seedUser(name, email string, role access.Role) int64 {
	e.t.Helper()
	hash, _ := access.HashPassword(password)
	var id int64
	mustScan(e.t, e.pool.QueryRow(context.Background(),
		`INSERT INTO users (username, full_name, password_hash, role, email) VALUES ($1, $1, $2, $3, $4) RETURNING id`,
		name, hash, string(role), email), &id)
	return id
}

func mustScan(t *testing.T, row interface{ Scan(...any) error }, dest ...any) {
	t.Helper()
	if err := row.Scan(dest...); err != nil {
		t.Fatalf("scan: %v", err)
	}
}

func (e *env) call(c *http.Client, method, path string, body any) *http.Response {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, rd)
	req.Header.Set("Origin", e.srv.URL) // as a browser on the app's own page does
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.Do(req)
	if err != nil {
		e.t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

// json calls the API as role, checks the status, and decodes the body into out (may be nil).
func (e *env) json(role access.Role, method, path string, body any, want int, out any) {
	e.t.Helper()
	resp := e.call(e.clients[role], method, path, body)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		e.t.Fatalf("%s %s as %s: status %d, want %d: %s", method, path, role, resp.StatusCode, want, raw)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			e.t.Fatalf("decode %s: %v (%s)", path, err, raw)
		}
	}
}

func (e *env) newCustomer(role access.Role, mobile string) int64 {
	e.t.Helper()
	var out struct {
		Customer struct {
			ID int64 `json:"id"`
		} `json:"customer"`
	}
	e.json(role, "POST", "/api/customers", map[string]string{"name": "Ravi " + mobile, "mobile": mobile}, http.StatusCreated, &out)
	return out.Customer.ID
}

func (e *env) newCase(role access.Role, customer int64, amount string) int64 {
	e.t.Helper()
	var out struct {
		ID int64 `json:"id"`
	}
	e.json(role, "POST", "/api/cases", map[string]any{
		"customer_id": customer, "bank_id": e.bank, "project_id": e.project, "loan_amount": amount, "assignee_id": e.ids[access.RoleEmployee],
	}, http.StatusCreated, &out)
	return out.ID
}

func (e *env) moveStage(role access.Role, caseID int64, stage string, tracking string) {
	e.t.Helper()
	body := map[string]any{"stage_id": e.stage[stage]}
	if tracking != "" {
		body["tracking_number"] = tracking
	}
	switch stage { // the sanction and disbursement stages need their amounts (spec 2026-11-25 §5.3)
	case "sanctioned":
		body["sanction"] = map[string]any{"amount": "100000", "rate_percent": "8.5"}
	case "disbursed":
		body["payment"] = map[string]any{"amount": "100000", "kind": "full"}
	}
	e.json(role, "POST", fmt.Sprintf("/api/cases/%d/stage", caseID), body, http.StatusOK, nil)
}

func (e *env) count(q string, args ...any) int {
	e.t.Helper()
	var n int
	mustScan(e.t, e.pool.QueryRow(context.Background(), q, args...), &n)
	return n
}

// upload sends a multipart document upload as role.
func (e *env) upload(role access.Role, caseID int64, label, name string, content []byte) *http.Response {
	e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("label", label)
	fw, _ := mw.CreateFormFile("file", name)
	_, _ = fw.Write(content)
	_ = mw.Close()
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/cases/%d/documents", e.srv.URL, caseID), &buf)
	req.Header.Set("Origin", e.srv.URL)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := e.clients[role].Do(req)
	if err != nil {
		e.t.Fatalf("upload: %v", err)
	}
	return resp
}

// pdf returns a small file that passes the content check.
func pdf(n int) []byte {
	b := append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("0"), n)...)
	return b
}
