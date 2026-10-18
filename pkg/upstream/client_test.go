package upstream

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClient(t *testing.T) {
	c := NewClient("https://example.com", "key", 10)
	if c.endpoint != "https://example.com" { t.Fatal("endpoint mismatch") }
	if c.apiKey != "key" { t.Fatal("key mismatch") }
}

func TestHealth_Up(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{ "status": "ok" })
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := NewClient(srv.URL, "", 5)
	res := c.Health(context.Background())
	if res["healthy"] != true { t.Fatalf("expected healthy, got %v", res) }
}

func TestHealth_Down(t *testing.T) {
	c := NewClient("http://localhost:1", "", 1)
	res := c.Health(context.Background())
	if res["healthy"] != false { t.Fatalf("expected not healthy, got %v", res) }
}

func TestListModels(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{ "data": []interface{}{map[string]interface{}{ "id": "model-1" }} })
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := NewClient(srv.URL, "", 5)
	models, err := c.ListModels(context.Background())
	if err != nil { t.Fatalf("unexpected err: %v", err) }
	if len(models) != 1 { t.Fatalf("expected 1 model, got %d", len(models)) }
}

func TestInvokeRaw(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{ "ok": true })
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := NewClient(srv.URL, "key", 5)
	res, err := c.InvokeRaw(context.Background(), "GET", "/test", nil)
	if err != nil { t.Fatalf("unexpected err: %v", err) }
	if res["ok"] != true { t.Fatalf("expected ok, got %v", res) }
}
