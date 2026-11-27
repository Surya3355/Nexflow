package access

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Login limits beyond the per-username lock (spec 2026-11-27-hardening §4.1).
const (
	loginsPerAddressPerMinute = 20
	maxConcurrentHashes       = 4
	hashWait                  = 5 * time.Second
)

// ErrBusy means too many password checks are running; the caller should retry.
var ErrBusy = errors.New("access: busy")

// AddressLimiter allows a fixed number of attempts per address per minute.
type AddressLimiter struct {
	mu        sync.Mutex
	perMinute int
	clock     func() time.Time
	windows   map[string]*window
}

type window struct {
	start time.Time
	count int
}

// NewAddressLimiter allows perMinute attempts per address in each one-minute window.
func NewAddressLimiter(perMinute int, clock func() time.Time) *AddressLimiter {
	return &AddressLimiter{perMinute: perMinute, clock: clock, windows: map[string]*window{}}
}

// Allow counts one attempt from addr and reports whether it is within the limit.
func (l *AddressLimiter) Allow(addr string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock()
	for k, w := range l.windows {
		if now.Sub(w.start) >= time.Minute {
			delete(l.windows, k)
		}
	}
	w, ok := l.windows[addr]
	if !ok {
		w = &window{start: now}
		l.windows[addr] = w
	}
	w.count++
	return w.count <= l.perMinute
}

// HashGate caps how many password checks run at once. Each check takes about
// 19 MiB of memory, so a flood of logins cannot use up the server.
type HashGate struct {
	slots chan struct{}
	wait  time.Duration
}

// NewHashGate allows n checks at once; others wait up to wait, then get ErrBusy.
func NewHashGate(n int, wait time.Duration) *HashGate {
	return &HashGate{slots: make(chan struct{}, n), wait: wait}
}

// Do runs fn when a slot is free.
func (g *HashGate) Do(ctx context.Context, fn func()) error {
	t := time.NewTimer(g.wait)
	defer t.Stop()
	select {
	case g.slots <- struct{}{}:
	case <-t.C:
		return ErrBusy
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-g.slots }()
	fn()
	return nil
}

// ClientAddr is the address a request came from. Behind nginx every request
// comes from nginx, so with trustProxy the X-Real-IP header nginx sets is used.
// Never trust it when the API is reachable without the proxy.
func ClientAddr(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
