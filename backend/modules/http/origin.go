package apihttp

import (
	"net/http"
	"net/url"
	"strings"
)

// RequireSameOrigin refuses any changing request (not GET, HEAD or OPTIONS)
// that a browser sent from another site (spec 2026-11-27-hardening §4.5).
// The Origin header, or the Referer when Origin is missing, must name this
// site's own host or one of allowed (full origins such as
// "http://localhost:5173"). Requests with neither header are refused too:
// browsers always send one, and scripts can set it.
func RequireSameOrigin(allowed []string) func(http.Handler) http.Handler {
	extra := map[string]bool{}
	for _, o := range allowed {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" {
			extra[strings.ToLower(o)] = true
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}
			if sameOrigin(r, extra) {
				next.ServeHTTP(w, r)
				return
			}
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Request refused."})
		})
	}
}

func sameOrigin(r *http.Request, extra map[string]bool) bool {
	src := r.Header.Get("Origin")
	if src == "" {
		src = r.Header.Get("Referer")
	}
	if src == "" || src == "null" {
		return false
	}
	u, err := url.Parse(src)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	if extra[strings.ToLower(u.Scheme+"://"+u.Host)] {
		return true
	}
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	return strings.EqualFold(u.Host, host)
}
