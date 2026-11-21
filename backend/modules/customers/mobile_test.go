package customers

import (
	"errors"
	"testing"
)

func TestNormalizeMobile(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{"plain ten digits", "9876543210", "9876543210", nil},
		{"plus91 with spaces", "+91 98765 43210", "9876543210", nil},
		{"plus91 with dashes", "+91-98765-43210", "9876543210", nil},
		{"plus91 joined", "+919876543210", "9876543210", nil},
		{"surrounding spaces", " 98765 43210 ", "9876543210", nil},
		{"leading zero is not stripped", "09876543210", "", ErrInvalidMobile},
		{"too short", "12345", "", ErrInvalidMobile},
		{"empty", "", "", ErrInvalidMobile},
		{"letters inside are not digits", "98765abc10", "", ErrInvalidMobile},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeMobile(tc.input)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("NormalizeMobile(%q) err = %v, want %v", tc.input, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeMobile(%q) unexpected err: %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("NormalizeMobile(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
