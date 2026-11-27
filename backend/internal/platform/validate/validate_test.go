package validate

import (
	"strings"
	"testing"
)

func TestName(t *testing.T) {
	cases := []struct {
		in   string
		max  int
		want string
		ok   bool
	}{
		{"  Ravi Kumar ", MaxName, "Ravi Kumar", true},
		{"   ", MaxName, "", false},
		{"", MaxName, "", false},
		{strings.Repeat("a", 150), MaxName, strings.Repeat("a", 150), true},
		{strings.Repeat("a", 151), MaxName, "", false},
		{strings.Repeat("र", 150), MaxName, strings.Repeat("र", 150), true}, // counted as characters, not bytes
		{strings.Repeat("b", 101), MaxBankName, "", false},
	}
	for _, c := range cases {
		got, ok := Name(c.in, c.max)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("Name(%q…, %d) = %q, %v; want %q, %v", firstN(c.in), c.max, firstN(got), ok, firstN(c.want), c.ok)
		}
	}
}

func TestText(t *testing.T) {
	if got, ok := Text("  note  ", MaxText); !ok || got != "note" {
		t.Errorf("Text trims: %q %v", got, ok)
	}
	if got, ok := Text("   ", MaxText); !ok || got != "" {
		t.Errorf("Text may be empty: %q %v", got, ok)
	}
	if _, ok := Text(strings.Repeat("x", 2001), MaxText); ok {
		t.Error("Text accepted 2,001 characters")
	}
	if _, ok := Text(strings.Repeat("x", 2000), MaxText); !ok {
		t.Error("Text refused 2,000 characters")
	}
}

func TestEmail(t *testing.T) {
	good := map[string]string{"": "", "a@b.co": "a@b.co", " A@B.CO ": "A@B.CO", "first.last+tag@mail.example.in": "first.last+tag@mail.example.in"}
	for in, want := range good {
		if got, ok := Email(in); !ok || got != want {
			t.Errorf("Email(%q) = %q, %v; want %q, true", in, got, ok, want)
		}
	}
	for _, in := range []string{"nope", "a@b", "a@", "@b.co", "Ravi <a@b.co>", "a b@c.co", "a@b.co,c@d.co", "a@.co", "a@b.", strings.Repeat("a", 250) + "@b.co"} {
		if _, ok := Email(in); ok {
			t.Errorf("Email(%q) accepted", in)
		}
	}
}

func TestAmount(t *testing.T) {
	for _, in := range []string{"1", "100.5", "2500000.00", "1000000000", " 45000 ", "0.01"} {
		if _, ok := Amount(in); !ok {
			t.Errorf("Amount(%q) refused", in)
		}
	}
	for _, in := range []string{"", "0", "0.00", "-1", "100.555", "1e5", "999999999999999999", "1000000000.01", "abc", "NaN", "Inf", "12,000", "+5", ".5", "5."} {
		if _, ok := Amount(in); ok {
			t.Errorf("Amount(%q) accepted", in)
		}
	}
	n, _ := Amount("2500000.50")
	if v, _ := n.Value(); v != "2500000.50" {
		t.Errorf("Amount value = %v, want 2500000.50", v)
	}
}

func firstN(s string) string {
	if len(s) > 20 {
		return s[:20]
	}
	return s
}
