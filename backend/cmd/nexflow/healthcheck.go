package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

// runHealthcheck is `nexflow healthcheck`: it asks this server's /healthz and
// exits non-zero when the server or its database is down. Docker runs it,
// since the image has no shell or curl.
func runHealthcheck() error {
	return checkHealth(healthURL(os.Getenv("NEXFLOW_HTTP_ADDR")))
}

func checkHealth(url string) error {
	c := http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return fmt.Errorf("healthcheck: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck: status %d", resp.StatusCode)
	}
	return nil
}

// healthURL is the local /healthz address for a listen address like ":8080".
func healthURL(addr string) string {
	port := "8080"
	if _, p, err := net.SplitHostPort(addr); err == nil && p != "" {
		port = p
	}
	return "http://127.0.0.1:" + port + "/healthz"
}
