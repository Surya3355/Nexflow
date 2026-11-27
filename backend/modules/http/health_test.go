package apihttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthzChecksDB(t *testing.T) {
	for name, c := range map[string]struct {
		check func(context.Context) error
		want  int
	}{
		"database up":   {func(context.Context) error { return nil }, http.StatusOK},
		"database down": {func(context.Context) error { return errors.New("connection refused") }, http.StatusServiceUnavailable},
	} {
		w := httptest.NewRecorder()
		healthHandler(c.check)(w, httptest.NewRequest("GET", "/healthz", nil))
		if w.Code != c.want {
			t.Errorf("%s: %d, want %d", name, w.Code, c.want)
		}
	}
}
