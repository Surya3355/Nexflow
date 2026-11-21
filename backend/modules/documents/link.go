package documents

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// LinkTTL is how long a download link works (owner answer E, round 13).
const LinkTTL = 10 * time.Minute

// Errors for download links. Callers should answer both with 404.
var (
	ErrLinkInvalid = errors.New("documents: link is not valid")
	ErrLinkExpired = errors.New("documents: link has expired")
)

// Linker signs and checks short-lived download links. A link is an HMAC-SHA256
// over the document ID and expiry time, keyed with NEXFLOW_LINK_SECRET.
type Linker struct {
	secret []byte
}

// NewLinker returns a linker for secret. The secret must be kept out of git.
func NewLinker(secret []byte) *Linker {
	return &Linker{secret: secret}
}

// Sign returns a link token for docID, valid for LinkTTL from now.
func (l *Linker) Sign(docID int64, now time.Time) string {
	payload := strconv.FormatInt(docID, 10) + "." + strconv.FormatInt(now.Add(LinkTTL).Unix(), 10)
	return enc([]byte(payload)) + "." + enc(l.mac([]byte(payload)))
}

// Open checks a token and returns the document ID it grants.
func (l *Linker) Open(token string, now time.Time) (int64, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return 0, ErrLinkInvalid
	}
	payload, err := enc64(parts[0])
	if err != nil {
		return 0, ErrLinkInvalid
	}
	sig, err := enc64(parts[1])
	if err != nil {
		return 0, ErrLinkInvalid
	}
	if !hmac.Equal(sig, l.mac(payload)) {
		return 0, ErrLinkInvalid
	}
	fields := strings.Split(string(payload), ".")
	if len(fields) != 2 {
		return 0, ErrLinkInvalid
	}
	id, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return 0, ErrLinkInvalid
	}
	expiry, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0, ErrLinkInvalid
	}
	if now.Unix() > expiry {
		return 0, ErrLinkExpired
	}
	return id, nil
}

func (l *Linker) mac(data []byte) []byte {
	h := hmac.New(sha256.New, l.secret)
	h.Write(data)
	return h.Sum(nil)
}

func enc(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func enc64(s string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(s) }

// NewDownloadHandler serves GET /api/documents/{token}. Only a valid, unexpired
// link returns the file. Every failure returns 404 with the same body.
func NewDownloadHandler(svc *Service, l *Linker) http.Handler {
	r := chi.NewRouter()
	r.Get("/api/documents/{token}", DownloadHandler(svc, l))
	return r
}

// DownloadHandler serves a file for a valid link. The route must have a {token} parameter.
func DownloadHandler(svc *Service, l *Linker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := l.Open(chi.URLParam(r, "token"), time.Now())
		if err != nil {
			http.NotFound(w, r)
			return
		}
		doc, content, err := svc.Download(r.Context(), id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", doc.ContentType)
		w.Header().Set("Content-Length", fmt.Sprint(len(content)))
		w.Header().Set("Content-Disposition", "inline; filename=\""+safeName(doc.FileName)+"\"")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, no-store")
		_, _ = w.Write(content)
	}
}

// safeName removes characters that could break the Content-Disposition header.
func safeName(name string) string {
	return strings.Map(func(r rune) rune {
		if r == '"' || r == '\\' || r < 0x20 {
			return '_'
		}
		return r
	}, name)
}
