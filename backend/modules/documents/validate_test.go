package documents

import (
	"bytes"
	"errors"
	"testing"
)

// Sample files are built in the test (no binary fixtures in git).
func samplePDF(size int) []byte {
	b := append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("0"), size)...)
	return b[:size]
}

func samplePNG(size int) []byte {
	head := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	return append(head, bytes.Repeat([]byte{0}, size-len(head))...)
}

func sampleJPEG(size int) []byte {
	head := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10, 'J', 'F', 'I', 'F'}
	return append(head, bytes.Repeat([]byte{0}, size-len(head))...)
}

func TestAcceptsRealPDFJPGPNG(t *testing.T) {
	cases := []struct {
		name, want string
		content    []byte
	}{
		{"scan.pdf", "application/pdf", samplePDF(100)},
		{"photo.jpg", "image/jpeg", sampleJPEG(100)},
		{"scan.png", "image/png", samplePNG(100)},
	}
	for _, c := range cases {
		got, err := ValidateFile(c.name, c.content)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", c.name, err)
		}
		if got != c.want {
			t.Fatalf("%s: type = %s, want %s", c.name, got, c.want)
		}
	}
}

// Review focus item 3: judged by content, not by name.
func TestRejectsHTMLNamedPDF(t *testing.T) {
	html := []byte("<html><script>alert(1)</script></html>")
	if _, err := ValidateFile("fake.pdf", html); !errors.Is(err, ErrUnsupportedType) {
		t.Fatalf("err = %v, want ErrUnsupportedType", err)
	}
}

func TestRejectsExecutableNamedJPG(t *testing.T) {
	elf := append([]byte{0x7f, 'E', 'L', 'F', 2, 1, 1, 0}, bytes.Repeat([]byte{0}, 100)...)
	if _, err := ValidateFile("photo.jpg", elf); !errors.Is(err, ErrUnsupportedType) {
		t.Fatalf("err = %v, want ErrUnsupportedType", err)
	}
}

func TestRejectsOver10MB(t *testing.T) {
	const max = 10485760
	if _, err := ValidateFile("big.pdf", samplePDF(max)); err != nil {
		t.Fatalf("exactly 10 MB refused: %v", err)
	}
	if _, err := ValidateFile("big.pdf", samplePDF(max+1)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("10 MB + 1 byte err = %v, want ErrTooLarge", err)
	}
}

func TestRejectsEmptyFile(t *testing.T) {
	if _, err := ValidateFile("empty.pdf", nil); !errors.Is(err, ErrEmpty) {
		t.Fatalf("err = %v, want ErrEmpty", err)
	}
}
