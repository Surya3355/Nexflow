package apihttp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Surya3355/nexflow/backend/modules/access"
)

func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func TestOpenWorkRoute(t *testing.T) {
	e := newEnv(t)
	resp := e.do(e.clients[access.RoleAdmin], "GET", fmt.Sprintf("/api/admin/users/%d/open-work", e.ids["employee"]), nil)
	body := readAll(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	var w struct{ Leads, Cases, Queries int }
	if err := json.Unmarshal([]byte(body), &w); err != nil || w.Cases != 1 {
		t.Fatalf("open work = %s, want 1 case", body)
	}
}

func TestDeactivateRoutes(t *testing.T) {
	e := newEnv(t)
	admin := e.clients[access.RoleAdmin]

	// The employee has an open case: a takeover person is needed.
	resp := e.do(admin, "POST", fmt.Sprintf("/api/admin/users/%d/deactivate", e.ids["employee"]), map[string]any{})
	if body := readAll(t, resp); resp.StatusCode != http.StatusConflict || !strings.Contains(body, "takes over") {
		t.Fatalf("no takeover: %d %s, want 409 asking who takes over", resp.StatusCode, body)
	}

	// The manager has nothing open: switched off at once, and their session ends.
	resp = e.do(admin, "POST", fmt.Sprintf("/api/admin/users/%d/deactivate", e.ids["manager"]), map[string]any{})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("switch off manager: %d %s", resp.StatusCode, readAll(t, resp))
	}
	resp.Body.Close()
	if resp = e.do(e.clients[access.RoleManager], "GET", "/api/me", nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("switched-off manager /api/me: %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()

	resp = e.do(admin, "POST", fmt.Sprintf("/api/admin/users/%d/reactivate", e.ids["manager"]), nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("switch on: %d %s", resp.StatusCode, readAll(t, resp))
	}
	resp.Body.Close()

	// Admin can't switch themselves off.
	resp = e.do(admin, "POST", fmt.Sprintf("/api/admin/users/%d/deactivate", e.ids["admin"]), map[string]any{})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("self switch off: %d, want 409", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestChangeRoleRoute(t *testing.T) {
	e := newEnv(t)
	admin := e.clients[access.RoleAdmin]
	resp := e.do(admin, "PUT", fmt.Sprintf("/api/admin/users/%d/role", e.ids["employee"]), map[string]string{"role": "manager"})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("change role: %d %s", resp.StatusCode, readAll(t, resp))
	}
	resp.Body.Close()
	resp = e.do(admin, "PUT", fmt.Sprintf("/api/admin/users/%d/role", e.ids["admin"]), map[string]string{"role": "employee"})
	if body := readAll(t, resp); resp.StatusCode != http.StatusConflict || !strings.Contains(body, "last active admin") {
		t.Fatalf("demote last admin: %d %s, want 409", resp.StatusCode, body)
	}
	resp = e.do(admin, "PUT", fmt.Sprintf("/api/admin/users/%d/role", e.ids["employee"]), map[string]string{"role": "boss"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad role: %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestUserRoutesAdminOnly(t *testing.T) {
	e := newEnv(t)
	id := e.ids["employee"]
	for _, role := range []access.Role{access.RoleManager, access.RoleEmployee} {
		c := e.clients[role]
		for _, r := range []struct{ method, path string }{
			{"GET", fmt.Sprintf("/api/admin/users/%d/open-work", id)},
			{"POST", fmt.Sprintf("/api/admin/users/%d/deactivate", id)},
			{"POST", fmt.Sprintf("/api/admin/users/%d/reactivate", id)},
			{"PUT", fmt.Sprintf("/api/admin/users/%d/role", id)},
		} {
			resp := e.do(c, r.method, r.path, map[string]string{"role": "admin"})
			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("%s %s %s: %d, want 403", role, r.method, r.path, resp.StatusCode)
			}
			resp.Body.Close()
		}
	}
}

func TestDeactivateWithTakeoverMovesCase(t *testing.T) {
	e := newEnv(t)
	admin := e.clients[access.RoleAdmin]
	resp := e.do(admin, "POST", fmt.Sprintf("/api/admin/users/%d/deactivate", e.ids["employee"]),
		map[string]any{"takeover_id": e.ids["manager"]})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("switch off with takeover: %d %s", resp.StatusCode, readAll(t, resp))
	}
	resp.Body.Close()
	var assignee int64
	mustScan(t, e.pool.QueryRow(t.Context(), `SELECT assignee_id FROM cases WHERE id = $1`, e.ids["case"]), &assignee)
	if assignee != e.ids["manager"] {
		t.Fatalf("case assignee = %d, want the manager %d", assignee, e.ids["manager"])
	}
}
