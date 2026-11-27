// Package httpserver builds the HTTP router. Feature modules mount their
// routes under /api; see internal/platform/httpserver.NewRouter.
package httpserver

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// NewRouter returns the root router with the health endpoint.
func NewRouter() http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return r
}
