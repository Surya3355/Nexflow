package access

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
)

// Login lock rules (owner, 2026-11-27).
const (
	maxLoginFailures = 5
	lockDuration     = 15 * time.Minute
)

// ErrLocked is matched by every *LockedError.
var ErrLocked = errors.New("access: too many attempts")

// LockedError says the username is locked and until when.
type LockedError struct{ Until time.Time }

func (e *LockedError) Error() string        { return ErrLocked.Error() }
func (e *LockedError) Is(target error) bool { return target == ErrLocked }

// Message is the text shown to staff, with the minutes left rounded up.
func (e *LockedError) Message(now time.Time) string {
	mins := int(math.Ceil(e.Until.Sub(now).Minutes()))
	if mins < 1 {
		mins = 1
	}
	unit := "minutes"
	if mins == 1 {
		unit = "minute"
	}
	return fmt.Sprintf("Too many attempts. Try again in %d %s.", mins, unit)
}

// unknownLocks applies the lock rule to usernames that do not exist, in
// memory, so a locked reply does not tell an attacker which names are real.
type unknownLocks struct {
	mu      sync.Mutex
	entries map[string]*unknownEntry
}

type unknownEntry struct {
	failures int
	until    time.Time
	last     time.Time
}

// maxUnknownEntries bounds the memory a flood of random names can take.
const maxUnknownEntries = 10000

func newUnknownLocks() *unknownLocks {
	return &unknownLocks{entries: map[string]*unknownEntry{}}
}

func (u *unknownLocks) lockedUntil(name string, now time.Time) (time.Time, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if e, ok := u.entries[name]; ok && now.Before(e.until) {
		return e.until, true
	}
	return time.Time{}, false
}

// fail counts one wrong try and reports whether the name is now locked.
func (u *unknownLocks) fail(name string, now time.Time) (time.Time, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.entries) >= maxUnknownEntries {
		u.prune(now)
	}
	e, ok := u.entries[name]
	if !ok {
		e = &unknownEntry{}
		u.entries[name] = e
	}
	e.last = now
	e.failures++
	if e.failures >= maxLoginFailures {
		e.failures = 0
		e.until = now.Add(lockDuration)
		return e.until, true
	}
	return time.Time{}, false
}

// prune drops entries untouched for longer than a lock; if that frees nothing
// (a flood within 15 minutes), it starts over rather than grow without limit.
func (u *unknownLocks) prune(now time.Time) {
	for k, e := range u.entries {
		if now.Sub(e.last) > lockDuration && !now.Before(e.until) {
			delete(u.entries, k)
		}
	}
	if len(u.entries) >= maxUnknownEntries {
		u.entries = map[string]*unknownEntry{}
	}
}
