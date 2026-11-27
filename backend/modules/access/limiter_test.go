package access

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAddressLimiter(t *testing.T) {
	clk := newFakeClock()
	l := NewAddressLimiter(20, clk.now)
	for i := 1; i <= 20; i++ {
		if !l.Allow("10.0.0.1") {
			t.Fatalf("attempt %d refused, want allowed", i)
		}
	}
	if l.Allow("10.0.0.1") {
		t.Fatal("21st attempt in a minute allowed")
	}
	if !l.Allow("10.0.0.2") {
		t.Fatal("another address was limited")
	}
	clk.add(61 * time.Second)
	if !l.Allow("10.0.0.1") {
		t.Fatal("attempt after a minute refused")
	}
}

func TestHashGateBusy(t *testing.T) {
	g := NewHashGate(1, 50*time.Millisecond)
	hold := make(chan struct{})
	started := make(chan struct{})
	go func() {
		_ = g.Do(context.Background(), func() { close(started); <-hold })
	}()
	<-started
	if err := g.Do(context.Background(), func() {}); !errors.Is(err, ErrBusy) {
		t.Fatalf("second Do while full: err = %v, want ErrBusy", err)
	}
	close(hold)
}

func TestClientAddr(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	r.RemoteAddr = "172.18.0.5:51234"
	r.Header.Set("X-Real-IP", "203.0.113.9")
	if got := ClientAddr(r, false); got != "172.18.0.5" {
		t.Fatalf("untrusted proxy: %q, want the connection address", got)
	}
	if got := ClientAddr(r, true); got != "203.0.113.9" {
		t.Fatalf("trusted proxy: %q, want X-Real-IP", got)
	}
}

func TestLoginAddressLimit(t *testing.T) {
	srv, _ := newTestServer(t)
	c := newClient(t)
	var last int
	for i := 0; i < 21; i++ {
		resp := postLogin(t, c, srv.URL, "user"+string(rune('a'+i)), "wrong-password-x")
		last = resp.StatusCode
		resp.Body.Close()
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("21st attempt from one address: status %d, want 429", last)
	}
}
