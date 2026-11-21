package access

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Surya3355/nexflow/backend/modules/history"
)

// recordingRecorder keeps audit entries in memory so tests can read them.
type recordingRecorder struct{ entries []history.Entry }

func (r *recordingRecorder) Record(_ context.Context, e history.Entry) error {
	r.entries = append(r.entries, e)
	return nil
}

func postChangePassword(t *testing.T, c *http.Client, base, current, next string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"current": current, "new": next})
	resp, err := c.Post(base+"/api/me/password", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/me/password: %v", err)
	}
	return resp
}

const newPassword = "a-brand-new-password"

func TestChangePasswordNeedsCurrentPassword(t *testing.T) {
	srv, pool := newTestServer(t)
	seedActor(t, pool, "surya", RoleEmployee)
	c := newClient(t)
	postLogin(t, c, srv.URL, "surya", goodPassword).Body.Close()

	resp := postChangePassword(t, c, srv.URL, "wrong-current-password", newPassword)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong current status = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()

	// Nothing changed: the old password still works.
	check := postLogin(t, newClient(t), srv.URL, "surya", goodPassword)
	if check.StatusCode != http.StatusOK {
		t.Fatalf("old password stopped working after refused change: %d", check.StatusCode)
	}
	check.Body.Close()
}

func TestChangePasswordOldPasswordStopsWorking(t *testing.T) {
	srv, pool := newTestServer(t)
	seedActor(t, pool, "surya", RoleEmployee)
	c := newClient(t)
	postLogin(t, c, srv.URL, "surya", goodPassword).Body.Close()

	resp := postChangePassword(t, c, srv.URL, goodPassword, newPassword)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("change status = %d, want 204", resp.StatusCode)
	}
	resp.Body.Close()

	old := postLogin(t, newClient(t), srv.URL, "surya", goodPassword)
	if old.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old password status = %d, want 401", old.StatusCode)
	}
	old.Body.Close()

	fresh := postLogin(t, newClient(t), srv.URL, "surya", newPassword)
	if fresh.StatusCode != http.StatusOK {
		t.Fatalf("new password status = %d, want 200", fresh.StatusCode)
	}
	fresh.Body.Close()
}

func TestChangePasswordSetsPasswordChangedAt(t *testing.T) {
	srv, pool := newTestServer(t)
	seedActor(t, pool, "surya", RoleEmployee)
	c := newClient(t)
	postLogin(t, c, srv.URL, "surya", goodPassword).Body.Close()

	var before bool
	if err := pool.QueryRow(context.Background(),
		`SELECT password_changed_at IS NULL FROM users WHERE username='surya'`).Scan(&before); err != nil || !before {
		t.Fatalf("password_changed_at should start NULL: before=%v err=%v", before, err)
	}

	postChangePassword(t, c, srv.URL, goodPassword, newPassword).Body.Close()

	var set bool
	if err := pool.QueryRow(context.Background(),
		`SELECT password_changed_at IS NOT NULL FROM users WHERE username='surya'`).Scan(&set); err != nil || !set {
		t.Fatalf("password_changed_at not set after change: set=%v err=%v", set, err)
	}
}

func TestChangePasswordWritesAuditEntry(t *testing.T) {
	rec := &recordingRecorder{}
	srv, pool, _ := newTestServerWith(t, rec)
	seedActor(t, pool, "surya", RoleEmployee)
	c := newClient(t)
	postLogin(t, c, srv.URL, "surya", goodPassword).Body.Close()

	postChangePassword(t, c, srv.URL, goodPassword, newPassword).Body.Close()

	for _, e := range rec.entries {
		if e.Action == "user.password_changed" {
			return
		}
	}
	t.Fatalf("no user.password_changed audit entry; got %+v", rec.entries)
}

func TestChangePasswordRefusesShortNewPassword(t *testing.T) {
	srv, pool := newTestServer(t)
	seedActor(t, pool, "surya", RoleEmployee)
	c := newClient(t)
	postLogin(t, c, srv.URL, "surya", goodPassword).Body.Close()

	resp := postChangePassword(t, c, srv.URL, goodPassword, "short")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("short new password status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()
}
