package access

import (
	"net/http"
	"testing"
)

func TestDeactivatedUserLosesExistingSession(t *testing.T) {
	srv, pool := newTestServer(t)
	seedActor(t, pool, "surya", RoleEmployee)
	c := newClient(t)

	postLogin(t, c, srv.URL, "surya", goodPassword).Body.Close()

	ok, err := c.Get(srv.URL + "/api/me")
	if err != nil || ok.StatusCode != http.StatusOK {
		t.Fatalf("before deactivation: status %v err %v, want 200", ok, err)
	}
	ok.Body.Close()

	if _, err := pool.Exec(t.Context(), `UPDATE users SET is_active = false WHERE username = 'surya'`); err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	resp, err := c.Get(srv.URL + "/api/me")
	if err != nil {
		t.Fatalf("GET /api/me: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("after deactivation: status = %d, want 401", resp.StatusCode)
	}
}

func TestRequireUserRefusesNoSession(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := newClient(t).Get(srv.URL + "/api/me")
	if err != nil {
		t.Fatalf("GET /api/me: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no session: status = %d, want 401", resp.StatusCode)
	}
}
