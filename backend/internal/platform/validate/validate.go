// Package validate holds the input rules shared by the modules (spec
// 2026-11-27-hardening §5): names, free text, email and money amounts.
// Each function returns the cleaned value and whether it is allowed.
package validate

import (
	"fmt"
	"math/big"
	"net/mail"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"
)

// Length limits, in characters.
const (
	MaxName     = 150
	MaxBankName = 100
	MaxText     = 2000
	maxEmail    = 254
)

// MaxAmount is ₹100 crore, the largest loan, sanctioned or paid amount.
const MaxAmount = 1_000_000_000

// Name trims s and accepts it when it is not empty and at most max characters.
func Name(s string, max int) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > max {
		return "", false
	}
	return s, true
}

// Text trims s and accepts it when it is at most max characters. It may be empty.
func Text(s string, max int) (string, bool) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > max {
		return "", false
	}
	return s, true
}

// Email trims s and accepts an empty value or one plain address with a
// dotted domain, such as name@example.in.
func Email(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", true
	}
	if len(s) > maxEmail || strings.ContainsAny(s, " ,<>") {
		return "", false
	}
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s || a.Name != "" {
		return "", false
	}
	at := strings.LastIndex(s, "@")
	local, domain := s[:at], s[at+1:]
	if local == "" || !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return "", false
	}
	return s, true
}

var amountPattern = regexp.MustCompile(`^[0-9]{1,10}(\.[0-9]{1,2})?$`)

// Amount accepts a plain rupee amount: digits with at most 2 decimals, more
// than 0 and at most MaxAmount. Anything else (signs, exponents, commas,
// more decimals) is refused, never rounded.
func Amount(s string) (pgtype.Numeric, bool) {
	s = strings.TrimSpace(s)
	if !amountPattern.MatchString(s) {
		return pgtype.Numeric{}, false
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok || r.Sign() <= 0 || r.Cmp(big.NewRat(MaxAmount, 1)) > 0 {
		return pgtype.Numeric{}, false
	}
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		return pgtype.Numeric{}, false
	}
	return n, true
}

// Error is a refused input. The API answers it with 400 and its message.
type Error struct{ Msg string }

func (e *Error) Error() string { return "validate: " + e.Msg }

// Invalid returns an *Error with the message shown to staff.
func Invalid(format string, args ...any) error { return &Error{Msg: fmt.Sprintf(format, args...)} }

// RequireName is Name with an error that says what is wrong with the field.
func RequireName(s string, max int, what string) (string, error) {
	if strings.TrimSpace(s) == "" {
		return "", Invalid("%s is required", what)
	}
	v, ok := Name(s, max)
	if !ok {
		return "", Invalid("%s must be %d characters or fewer", what, max)
	}
	return v, nil
}

// OptionalText is Text with an error that names the field.
func OptionalText(s string, max int, what string) (string, error) {
	v, ok := Text(s, max)
	if !ok {
		return "", Invalid("%s must be %d characters or fewer", what, max)
	}
	return v, nil
}

// EmailField is Email with the error staff see.
func EmailField(s string) (string, error) {
	v, ok := Email(s)
	if !ok {
		return "", Invalid("enter a valid email, like name@example.com")
	}
	return v, nil
}
