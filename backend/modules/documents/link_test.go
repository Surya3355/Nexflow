package documents

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

var testSecret = []byte("0123456789abcdef0123456789abcdef")

func TestValidLinkOpensDocument(t *testing.T) {
	l := NewLinker(testSecret)
	now := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	tok := l.Sign(42, now)
	id, err := l.Open(tok, now.Add(time.Minute))
	if err != nil || id != 42 {
		t.Fatalf("Open = %d, %v; want 42, nil", id, err)
	}
}

// Review focus item 4: a link past its expiry is refused.
func TestExpiredLinkRefused(t *testing.T) {
	l := NewLinker(testSecret)
	now := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	tok := l.Sign(42, now)
	if _, err := l.Open(tok, now.Add(11*time.Minute)); !errors.Is(err, ErrLinkExpired) {
		t.Fatalf("err = %v, want ErrLinkExpired", err)
	}
}

// Review focus item 4: a changed document ID is refused.
func TestTamperedLinkRefused(t *testing.T) {
	l := NewLinker(testSecret)
	now := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	tok := l.Sign(42, now)
	parts := strings.SplitN(tok, ".", 2)
	forged := NewLinker([]byte("another-secret-entirely-different")).Sign(99, now)
	forgedParts := strings.SplitN(forged, ".", 2)
	for _, bad := range []string{
		forgedParts[0] + "." + parts[1], // different payload, old signature
		parts[0] + "x." + parts[1],      // edited payload
		"garbage",
		"",
	} {
		if _, err := l.Open(bad, now); !errors.Is(err, ErrLinkInvalid) {
			t.Fatalf("token %q: err = %v, want ErrLinkInvalid", bad, err)
		}
	}
}

// Review focus item 4 and plan test: no link means no file.
func TestDocumentNotReachableWithoutLink(t *testing.T) {
	f := newFixture(t)
	doc, err := f.svc.Upload(t.Context(), f.actor, f.caseID, "PAN", UploadedFile{Name: "pan.pdf", Content: samplePDF(200)})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	srv := httptest.NewServer(NewDownloadHandler(f.svc, NewLinker(testSecret)))
	defer srv.Close()

	for _, path := range []string{"/api/documents/1", "/api/documents/" + strconv.FormatInt(doc.ID, 10)} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound || bytes.Contains(body, []byte("%PDF")) {
			t.Fatalf("GET %s without a valid link: status %d, want 404 with no file", path, resp.StatusCode)
		}
	}
}

func TestValidLinkDownloadsFile(t *testing.T) {
	f := newFixture(t)
	content := samplePDF(300)
	doc, err := f.svc.Upload(t.Context(), f.actor, f.caseID, "PAN", UploadedFile{Name: "pan.pdf", Content: content})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	l := NewLinker(testSecret)
	srv := httptest.NewServer(NewDownloadHandler(f.svc, l))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/documents/" + l.Sign(doc.ID, time.Now()))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !bytes.Equal(body, content) {
		t.Fatalf("status %d, body %d bytes; want 200 with the file", resp.StatusCode, len(body))
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff header")
	}
}
