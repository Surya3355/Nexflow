package access

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Surya3355/nexflow/backend/internal/platform/testdb"
	"github.com/Surya3355/nexflow/backend/migrations"
	"github.com/Surya3355/nexflow/backend/modules/history"
	"github.com/jackc/pgx/v5/pgxpool"
)

// newTestServer serves the access routes on a fresh database.
func newTestServer(t *testing.T) (*httptest.Server, *pgxpool.Pool) {
	t.Helper()
	srv, pool, _ := newTestServerWith(t, history.Nop{})
	return srv, pool
}

// newTestServerWith serves the access routes and writes audit entries to rec.
func newTestServerWith(t *testing.T, rec history.Recorder) (*httptest.Server, *pgxpool.Pool, *Service) {
	t.Helper()
	pool := testdb.New(t)
	if err := migrations.Up(pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := NewService(pool, rec)
	h := NewHandler(svc, NewSessionManager(pool, false))
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)
	return srv, pool, svc
}

// newClient returns a client that keeps cookies, like a browser.
func newClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	return &http.Client{Jar: jar}
}

func postLogin(t *testing.T, c *http.Client, base, username, password string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	resp, err := c.Post(base+"/api/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/login: %v", err)
	}
	return resp
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return strings.TrimSpace(string(b))
}

func TestLoginWrongPasswordReturns401(t *testing.T) {
	srv, pool := newTestServer(t)
	seedActor(t, pool, "surya", RoleEmployee)

	resp := postLogin(t, newClient(t), srv.URL, "surya", "not-the-password")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestLoginCorrectPasswordSetsSessionCookie(t *testing.T) {
	srv, pool := newTestServer(t)
	seedActor(t, pool, "surya", RoleEmployee)

	resp := postLogin(t, newClient(t), srv.URL, "surya", goodPassword)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var found bool
	for _, c := range resp.Cookies() {
		if c.Name == "nexflow_session" && c.Value != "" && c.HttpOnly {
			found = true
		}
	}
	if !found {
		t.Fatal("no HttpOnly nexflow_session cookie in login response")
	}
	resp.Body.Close()
}

func TestInactiveUserCannotLogin(t *testing.T) {
	srv, pool := newTestServer(t)
	seedActor(t, pool, "surya", RoleEmployee)
	if _, err := pool.Exec(t.Context(), `UPDATE users SET is_active = false WHERE username = 'surya'`); err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	resp := postLogin(t, newClient(t), srv.URL, "surya", goodPassword)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("inactive login status = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestUnknownUserAndWrongPasswordGiveSameError(t *testing.T) {
	srv, pool := newTestServer(t)
	seedActor(t, pool, "surya", RoleEmployee)

	unknown := postLogin(t, newClient(t), srv.URL, "nobody", goodPassword)
	wrong := postLogin(t, newClient(t), srv.URL, "surya", "wrong-password-here")
	if unknown.StatusCode != wrong.StatusCode {
		t.Fatalf("status differs: unknown=%d wrong=%d", unknown.StatusCode, wrong.StatusCode)
	}
	if a, b := readBody(t, unknown), readBody(t, wrong); a != b {
		t.Fatalf("message differs: unknown=%q wrong=%q", a, b)
	}
}

func TestLogoutInvalidatesSession(t *testing.T) {
	srv, pool := newTestServer(t)
	seedActor(t, pool, "surya", RoleEmployee)
	c := newClient(t)

	postLogin(t, c, srv.URL, "surya", goodPassword).Body.Close()

	resp, err := c.Get(srv.URL + "/api/me")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/me after login: status %v err %v, want 200", resp, err)
	}
	resp.Body.Close()

	out, err := c.Post(srv.URL+"/api/logout", "application/json", nil)
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	out.Body.Close()

	after, err := c.Get(srv.URL + "/api/me")
	if err != nil {
		t.Fatalf("GET /api/me after logout: %v", err)
	}
	defer after.Body.Close()
	if after.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET /api/me after logout = %d, want 401", after.StatusCode)
	}
}
