package upstream

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient_DefaultTimeout(t *testing.T) {
	c := NewClient("http://test", "key", 0)
	if c == nil {
		t.Fatal("expected client")
	}
}

func TestHealth_NoEndpoint(t *testing.T) {
	c := NewClient("", "", 5)
	res := c.Health(context.Background())
	if res["healthy"] != false {
		t.Fatalf("expected not healthy, got %v", res["healthy"])
	}
}

func TestHealth_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		t.Fatalf("unexpected path %s", r.URL.Path)
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "key", 5)
	res := c.Health(context.Background())
	if res["healthy"] != true {
		t.Fatalf("expected healthy, got %v", res)
	}
	if res["status"] != http.StatusOK {
		t.Fatalf("expected status 200, got %v", res["status"])
	}
}

func TestHealth_WithAuth(t *testing.T) {
	var authHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "mytoken", 5)
	_ = c.Health(context.Background())
	if authHeader != "Bearer mytoken" {
		t.Fatalf("expected Bearer mytoken, got %s", authHeader)
	}
}

func TestListModels_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{
					{"id": "model-a"},
					{"id": "model-b"},
				},
			})
			return
		}
		t.Fatalf("unexpected path %s", r.URL.Path)
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "", 5)
	models, err := c.ListModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0]["id"] != "model-a" {
		t.Fatalf("expected model-a, got %s", models[0]["id"])
	}
}

func TestListModels_NoEndpoint(t *testing.T) {
	c := NewClient("", "", 5)
	_, err := c.ListModels(context.Background())
	if err == nil {
		t.Fatal("expected error for no endpoint")
	}
}

func TestListModels_NonOK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "boom"})
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "", 5)
	_, err := c.ListModels(context.Background())
	if err == nil {
		t.Fatal("expected error for non-OK status")
	}
}

func TestInvoke_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/completions" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]string{{"text": "hello"}},
			})
			return
		}
		t.Fatalf("unexpected path %s", r.URL.Path)
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "", 5)
	res, err := c.Invoke(context.Background(), "m1", "hi", map[string]interface{}{"temperature": 0.7})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	choices, ok := res["choices"].([]any)
	if !ok || len(choices) == 0 {
		t.Fatalf("expected choices, got %v", res)
	}
}

func TestInvoke_NoEndpoint(t *testing.T) {
	c := NewClient("", "", 5)
	_, err := c.Invoke(context.Background(), "m", "p", nil)
	if err == nil {
		t.Fatal("expected error for no endpoint")
	}
}

func TestInvoke_MergeParams(t *testing.T) {
	var received map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "", 5)
	_, _ = c.Invoke(context.Background(), "m1", "hi", map[string]interface{}{"temperature": 0.7, "model": "should-be-ignored"})
	if received["model"] != "m1" {
		t.Fatalf("expected model m1, got %v", received["model"])
	}
	if received["temperature"] != 0.7 {
		t.Fatalf("expected temperature 0.7, got %v", received["temperature"])
	}
}

func TestInvoke_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "", 1) // 1s timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := c.Invoke(ctx, "m", "p", nil)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestInvoke_NonJSONResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("plain text response"))
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "", 5)
	res, err := c.Invoke(context.Background(), "m", "p", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res["raw"] != "plain text response" {
		t.Fatalf("expected raw response, got %v", res)
	}
}
