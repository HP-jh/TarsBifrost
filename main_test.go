package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HP-jh/TarsBifrost/pkg/upstream"
)

func TestHealthHandler(t *testing.T) {
	client := upstream.NewClient("", "", 5)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	healthHandler(w, req, client)
	if w.Code != http.StatusOK { t.Fatalf("expected 200, got %d", w.Code) }
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil { t.Fatal(err) }
	if resp["status"] != "ok" { t.Fatalf("expected ok, got %v", resp["status"]) }
}

func TestToolsHandler(t *testing.T) {
	client := upstream.NewClient("", "", 5)
	req := httptest.NewRequest(http.MethodGet, "/tools", nil)
	w := httptest.NewRecorder()
	toolsHandler(w, req, client)
	if w.Code != http.StatusOK { t.Fatalf("expected 200, got %d", w.Code) }
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil { t.Fatal(err) }
	tools, ok := resp["tools"].([]interface{})
	if !ok || len(tools) == 0 { t.Fatal("expected tools") }
}

func TestInvokeHandler_MethodNotAllowed(t *testing.T) {
	client := upstream.NewClient("", "", 5)
	req := httptest.NewRequest(http.MethodGet, "/invoke", nil)
	w := httptest.NewRecorder()
	invokeHandler(w, req, client)
	if w.Code != http.StatusMethodNotAllowed { t.Fatalf("expected 405, got %d", w.Code) }
}

func TestInvokeHandler_UnknownTool(t *testing.T) {
	client := upstream.NewClient("", "", 5)
	body, _ := json.Marshal(map[string]interface{}{ "tool": "xxx", "params": map[string]interface{}{} })
	req := httptest.NewRequest(http.MethodPost, "/invoke", bytes.NewReader(body))
	w := httptest.NewRecorder()
	invokeHandler(w, req, client)
	if w.Code != http.StatusBadRequest { t.Fatalf("expected 400, got %d", w.Code) }
}

func TestVersionBumped(t *testing.T) {
	if Version != "1.0.2" {
		t.Fatalf("Version expected 1.0.2, got %s", Version)
	}
}

func TestRateLimiter(t *testing.T) {
	rl := newRateLimiter(3, time.Second)
	ip := "192.0.2.1"
	if !rl.allow(ip) { t.Fatal("1st should pass") }
	if !rl.allow(ip) { t.Fatal("2nd should pass") }
	if !rl.allow(ip) { t.Fatal("3rd should pass") }
	if rl.allow(ip) { t.Fatal("4th should be blocked") }
}

func TestRateLimiter_Cleanup(t *testing.T) {
	rl := newRateLimiter(1, 50*time.Millisecond)
	ip := "192.0.2.2"
	if !rl.allow(ip) { t.Fatal("1st should pass") }
	if rl.allow(ip) { t.Fatal("2nd should be blocked") }
	time.Sleep(60 * time.Millisecond)
	rl.cleanup()
	if !rl.allow(ip) { t.Fatal("after cleanup should pass") }
}

func TestRequestID(t *testing.T) {
	id1 := genRequestID()
	id2 := genRequestID()
	if id1 == "" { t.Fatal("request id should not be empty") }
	if id1 == id2 { t.Fatal("request ids should be unique") }
}

func TestClientIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.10:1234"
	if clientIP(req) != "192.0.2.10" { t.Fatalf("expected 192.0.2.10, got %s", clientIP(req)) }

	req.Header.Set("X-Forwarded-For", "203.0.113.1, 70.41.3.18")
	if clientIP(req) != "203.0.113.1" { t.Fatalf("expected 203.0.113.1, got %s", clientIP(req)) }
}

func TestMiddleware_SecurityHeaders(t *testing.T) {
	client := upstream.NewClient("", "", 5)
	rl := newRateLimiter(100, time.Minute)
	handler := withMiddleware(client, rl, func(w http.ResponseWriter, r *http.Request, c *upstream.Client) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
	})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusOK { t.Fatalf("expected 200, got %d", w.Code) }
	headers := w.Header()
	if headers.Get("X-Content-Type-Options") != "nosniff" { t.Fatal("missing nosniff") }
	if headers.Get("X-Frame-Options") != "DENY" { t.Fatal("missing DENY") }
	if headers.Get("Referrer-Policy") != "strict-origin-when-cross-origin" { t.Fatal("missing referrer-policy") }
	if headers.Get("Content-Security-Policy") == "" { t.Fatal("missing CSP") }
	if headers.Get("X-Request-ID") == "" { t.Fatal("missing request id") }
}

func TestMiddleware_RateLimit(t *testing.T) {
	client := upstream.NewClient("", "", 5)
	rl := newRateLimiter(2, time.Minute)
	handler := withMiddleware(client, rl, func(w http.ResponseWriter, r *http.Request, c *upstream.Client) {
		w.WriteHeader(http.StatusOK)
	})
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.RemoteAddr = "192.0.2.99:1234"
		w := httptest.NewRecorder()
		handler(w, req)
		if w.Code != http.StatusOK { t.Fatalf("request %d expected 200, got %d", i+1, w.Code) }
	}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.RemoteAddr = "192.0.2.99:1234"
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusTooManyRequests { t.Fatalf("expected 429, got %d", w.Code) }
	body := w.Body.String()
	if !strings.Contains(body, "频繁") { t.Fatal("expected rate limit message in Chinese") }
}
