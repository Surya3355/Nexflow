package access

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

// fakeClock is a settable clock for the lock rules.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time      { return c.t }
func (c *fakeClock) add(d time.Duration) { c.t = c.t.Add(d) }
func newFakeClock() *fakeClock           { return &fakeClock{t: time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)} }
func wrongTimes(svc *Service, name string, n int) {
	for i := 0; i < n; i++ {
		_, _ = svc.Authenticate(context.Background(), name, "wrong-password-x")
	}
}

func TestFiveWrongLocks(t *testing.T) {
	svc, pool := setup(t)
	clk := newFakeClock()
	svc.SetClock(clk.now)
	seedActor(t, pool, "surya", RoleEmployee)

	for i := 1; i <= 4; i++ {
		if _, err := svc.Authenticate(context.Background(), "surya", "wrong-password-x"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("wrong try %d: err = %v, want ErrInvalidCredentials", i, err)
		}
	}
	fifth := clk.t
	if _, err := svc.Authenticate(context.Background(), "surya", "wrong-password-x"); !errors.Is(err, ErrLocked) {
		t.Fatalf("5th wrong: err = %v, want ErrLocked", err)
	}
	clk.add(time.Minute)
	_, err := svc.Authenticate(context.Background(), "surya", goodPassword)
	var le *LockedError
	if !errors.As(err, &le) || !errors.Is(err, ErrLocked) {
		t.Fatalf("right password while locked: err = %v, want *LockedError", err)
	}
	if want := fifth.Add(15 * time.Minute); !le.Until.Equal(want) {
		t.Fatalf("locked until %v, want %v", le.Until, want)
	}
}

func TestRightPasswordResetsCount(t *testing.T) {
	svc, pool := setup(t)
	svc.SetClock(newFakeClock().now)
	seedActor(t, pool, "surya", RoleEmployee)

	wrongTimes(svc, "surya", 4)
	if _, err := svc.Authenticate(context.Background(), "surya", goodPassword); err != nil {
		t.Fatalf("right password after 4 wrong: %v", err)
	}
	wrongTimes(svc, "surya", 4)
	if _, err := svc.Authenticate(context.Background(), "surya", goodPassword); err != nil {
		t.Fatalf("right password after 4+4 wrong: %v", err)
	}
}

func TestLockExpires(t *testing.T) {
	svc, pool := setup(t)
	clk := newFakeClock()
	svc.SetClock(clk.now)
	seedActor(t, pool, "surya", RoleEmployee)

	wrongTimes(svc, "surya", 5)
	clk.add(15*time.Minute + time.Second)
	if _, err := svc.Authenticate(context.Background(), "surya", goodPassword); err != nil {
		t.Fatalf("right password after lock expired: %v", err)
	}
	// The count started again: one wrong try does not lock.
	wrongTimes(svc, "surya", 1)
	if _, err := svc.Authenticate(context.Background(), "surya", goodPassword); err != nil {
		t.Fatalf("one wrong after expiry locked the user: %v", err)
	}
}

func TestResetPasswordUnlocks(t *testing.T) {
	svc, pool := setup(t)
	svc.SetClock(newFakeClock().now)
	admin := seedActor(t, pool, "root", RoleAdmin)
	staff := seedActor(t, pool, "staff", RoleEmployee)

	wrongTimes(svc, "staff", 5)
	if err := svc.ResetPassword(context.Background(), admin, staff.ID, newPassword); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if _, err := svc.Authenticate(context.Background(), "staff", newPassword); err != nil {
		t.Fatalf("login after admin reset: %v", err)
	}
}

func TestUnknownUserLocksToo(t *testing.T) {
	svc, _ := setup(t)
	svc.SetClock(newFakeClock().now)

	wrongTimes(svc, "NoSuchUser", 5)
	if _, err := svc.Authenticate(context.Background(), "nosuchuser", "anything-at-all"); !errors.Is(err, ErrLocked) {
		t.Fatalf("6th try on unknown user: err = %v, want ErrLocked", err)
	}
}

func TestLoginLockedResponse(t *testing.T) {
	srv, pool := newTestServer(t)
	seedActor(t, pool, "surya", RoleEmployee)
	c := newClient(t)
	for i := 0; i < 5; i++ {
		postLogin(t, c, srv.URL, "surya", "wrong-password-x").Body.Close()
	}
	resp := postLogin(t, c, srv.URL, "surya", goodPassword)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 (body %s)", resp.StatusCode, body)
	}
	if !strings.Contains(body, "Too many attempts. Try again in 15 minutes.") {
		t.Fatalf("body = %s, want the lock message", body)
	}
}

func TestLockedMessageMinutes(t *testing.T) {
	now := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	cases := map[time.Duration]string{
		15 * time.Minute:             "Too many attempts. Try again in 15 minutes.",
		14*time.Minute + time.Second: "Too many attempts. Try again in 15 minutes.",
		30 * time.Second:             "Too many attempts. Try again in 1 minute.",
	}
	for left, want := range cases {
		if got := (&LockedError{Until: now.Add(left)}).Message(now); got != want {
			t.Errorf("%v left: %q, want %q", left, got, want)
		}
	}
}
