package cases

import (
	"errors"
	"regexp"
	"strings"
)

// ErrInvalidTracking is returned for a tracking number with anything other
// than letters A to Z and digits 0 to 9 (spec 2026-11-21 §4.7).
var ErrInvalidTracking = errors.New("cases: tracking number can contain only letters and numbers")

var trackingPattern = regexp.MustCompile(`^[A-Z0-9]+$`)

// NormalizeTracking trims and upper-cases a typed tracking number and checks
// it is letters and digits only. Blank input returns "" and no error; the
// caller decides whether a tracking number is required.
func NormalizeTracking(s string) (string, error) {
	v := strings.ToUpper(strings.TrimSpace(s))
	if v == "" {
		return "", nil
	}
	if !trackingPattern.MatchString(v) {
		return "", ErrInvalidTracking
	}
	return v, nil
}
