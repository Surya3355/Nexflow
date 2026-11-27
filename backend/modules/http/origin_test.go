package apihttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Surya3355/nexflow/backend/modules/access"
)

func originCheck(t *testing.T, allowed []string, method, host string, headers map[string]string) (int, string) {
	t.Helper()
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := RequireSameOrigin(allowed)(ok)
	r := httptest.NewRequest(method, "http://"+host+"/api/cases", nil)
	r.Host = host
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code, w.Body.String()
}

func TestOriginCheck(t *testing.T) {
	allowed := []string{"http://localhost:5173"}
	cases := []struct {
		name    string
		method  string
		host    string
		headers map[string]string
		want    int
	}{
		{"foreign origin", "POST", "localhost:8081", map[string]string{"Origin": "http://evil.example"}, http.StatusForbidden},
		{"same origin through nginx", "POST", "localhost:8081", map[string]string{"Origin": "http://localhost:8081"}, http.StatusNoContent},
		{"same origin https", "PUT", "nexflow.example.in", map[string]string{"Origin": "https://nexflow.example.in"}, http.StatusNoContent},
		{"look-alike host", "POST", "nexflow.example.in", map[string]string{"Origin": "https://nexflow.example.in.evil.example"}, http.StatusForbidden},
		{"no origin, same referer", "DELETE", "localhost:8081", map[string]string{"Referer": "http://localhost:8081/cases/4"}, http.StatusNoContent},
		{"no origin, foreign referer", "POST", "localhost:8081", map[string]string{"Referer": "http://evil.example/x"}, http.StatusForbidden},
		{"neither header", "POST", "localhost:8081", nil, http.StatusForbidden},
		{"null origin", "POST", "localhost:8081", map[string]string{"Origin": "null"}, http.StatusForbidden},
		{"GET from anywhere", "GET", "localhost:8081", map[string]string{"Origin": "http://evil.example"}, http.StatusNoContent},
		{"allowed dev origin", "POST", "localhost:8080", map[string]string{"Origin": "http://localhost:5173"}, http.StatusNoContent},
		{"forwarded host", "PATCH", "backend:8080", map[string]string{"Origin": "http://localhost:8081", "X-Forwarded-Host": "localhost:8081"}, http.StatusNoContent},
	}
	for _, c := range cases {
		code, body := originCheck(t, allowed, c.method, c.host, c.headers)
		if code != c.want {
			t.Errorf("%s: status %d, want %d", c.name, code, c.want)
		}
		if c.want == http.StatusForbidden && !strings.Contains(body, "Request refused.") {
			t.Errorf("%s: body %q, want the refusal message", c.name, body)
		}
	}
}

func TestRouterRefusesCrossSitePost(t *testing.T) {
	e := newEnv(t)
	for _, path := range []string{"/api/login", "/api/customers"} {
		req, _ := http.NewRequest("POST", e.srv.URL+path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "text/plain")
		req.Header.Set("Origin", "http://evil.example")
		resp, err := e.clients[access.RoleAdmin].Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("POST %s from another site: %d, want 403", path, resp.StatusCode)
		}
	}
}
