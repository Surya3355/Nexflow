package access

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestResetPasswordAdminOnly(t *testing.T) {
	svc, pool := setup(t)
	manager := seedActor(t, pool, "lead", RoleManager)
	staff := seedActor(t, pool, "staff", RoleEmployee)
	if err := svc.ResetPassword(context.Background(), manager, staff.ID, newPassword); !errors.Is(err, ErrForbidden) {
		t.Fatalf("manager err = %v, want ErrForbidden", err)
	}
}

func TestResetPasswordWeak(t *testing.T) {
	svc, pool := setup(t)
	admin := seedActor(t, pool, "root", RoleAdmin)
	staff := seedActor(t, pool, "staff", RoleEmployee)
	if err := svc.ResetPassword(context.Background(), admin, staff.ID, "1234567"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("err = %v, want ErrWeakPassword", err)
	}
}

func TestResetPasswordUnknownUser(t *testing.T) {
	svc, pool := setup(t)
	admin := seedActor(t, pool, "root", RoleAdmin)
	if err := svc.ResetPassword(context.Background(), admin, 999999, newPassword); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("err = %v, want ErrUserNotFound", err)
	}
}

func TestResetPasswordAudited(t *testing.T) {
	rec := &recordingRecorder{}
	_, pool, svc := newTestServerWith(t, rec)
	admin := seedActor(t, pool, "root", RoleAdmin)
	staff := seedActor(t, pool, "staff", RoleEmployee)
	if err := svc.ResetPassword(context.Background(), admin, staff.ID, newPassword); err != nil {
		t.Fatalf("reset: %v", err)
	}
	for _, e := range rec.entries {
		if e.Action == "user.password_reset" && e.EntityType == "user" && e.EntityID != nil && *e.EntityID == staff.ID &&
			e.ActorUserID != nil && *e.ActorUserID == admin.ID {
			return
		}
	}
	t.Fatalf("no user.password_reset audit entry in %+v", rec.entries)
}

func TestResetSignsOutOtherSessions(t *testing.T) {
	srv, pool, svc := newTestServerWith(t, &recordingRecorder{})
	admin := seedActor(t, pool, "root", RoleAdmin)
	staff := seedActor(t, pool, "staff", RoleEmployee)
	c := newClient(t)
	postLogin(t, c, srv.URL, "staff", goodPassword).Body.Close()
	if err := svc.ResetPassword(context.Background(), admin, staff.ID, newPassword); err != nil {
		t.Fatalf("reset: %v", err)
	}
	resp, err := c.Get(srv.URL + "/api/me")
	if err != nil {
		t.Fatalf("GET /api/me: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("after reset: %d, want 401", resp.StatusCode)
	}
	again := newClient(t)
	postLogin(t, again, srv.URL, "staff", newPassword).Body.Close()
	me, err := again.Get(srv.URL + "/api/me")
	if err != nil {
		t.Fatalf("GET /api/me: %v", err)
	}
	me.Body.Close()
	if me.StatusCode != http.StatusOK {
		t.Fatalf("new password session: %d, want 200", me.StatusCode)
	}
}

func TestOwnPasswordChangeKeepsSession(t *testing.T) {
	srv, pool := newTestServer(t)
	seedActor(t, pool, "surya", RoleEmployee)
	c := newClient(t)
	postLogin(t, c, srv.URL, "surya", goodPassword).Body.Close()
	resp := postChangePassword(t, c, srv.URL, goodPassword, newPassword)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("change: %d", resp.StatusCode)
	}
	me, err := c.Get(srv.URL + "/api/me")
	if err != nil {
		t.Fatalf("GET /api/me: %v", err)
	}
	me.Body.Close()
	if me.StatusCode != http.StatusOK {
		t.Fatalf("own session after own change: %d, want 200", me.StatusCode)
	}
}
