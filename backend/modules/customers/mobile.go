// Package customers holds customer capture, duplicate checks and customer drops.
package customers

import (
	"errors"
	"strings"
)

// ErrInvalidMobile is returned when a mobile number is not exactly 10 digits
// after the +91 prefix and any spaces, dashes or other non-digits are removed.
var ErrInvalidMobile = errors.New("mobile number must be 10 digits")

// NormalizeMobile turns what staff typed or pasted into the stored form:
// exactly 10 digits, with a leading +91 removed (round 12). A leading 0 is
// not stripped, so the user must enter the 10 digits.
func NormalizeMobile(input string) (string, error) {
	s := strings.TrimSpace(input)
	s = strings.TrimPrefix(s, "+91")

	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	digits := b.String()
	if len(digits) != 10 {
		return "", ErrInvalidMobile
	}
	return digits, nil
}
