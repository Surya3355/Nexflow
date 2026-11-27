package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthcheckCommand(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer ok.Close()
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer down.Close()
	if err := checkHealth(ok.URL + "/healthz"); err != nil {
		t.Fatalf("healthy server: %v", err)
	}
	if err := checkHealth(down.URL + "/healthz"); err == nil {
		t.Fatal("503 counted as healthy")
	}
	if err := checkHealth("http://127.0.0.1:1/healthz"); err == nil {
		t.Fatal("no server counted as healthy")
	}
}

func TestHealthURL(t *testing.T) {
	for addr, want := range map[string]string{":8080": "http://127.0.0.1:8080/healthz", "0.0.0.0:9000": "http://127.0.0.1:9000/healthz", "": "http://127.0.0.1:8080/healthz"} {
		if got := healthURL(addr); got != want {
			t.Errorf("healthURL(%q) = %q, want %q", addr, got, want)
		}
	}
}
