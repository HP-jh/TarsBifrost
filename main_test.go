package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HP-jh/TarsBifrost/pkg/upstream"
)

func TestHealthHandler(t *testing.T) {
	client := upstream.NewClient("", "", 5)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	healthHandler(w, req, client)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", resp["status"])
	}
	if resp["version"] != Version {
		t.Fatalf("expected version %s, got %v", Version, resp["version"])
	}
	up, ok := resp["upstream"].(map[string]any)
	if !ok {
		t.Fatal("expected upstream field")
	}
	if up["healthy"] != false {
		t.Fatalf("expected upstream not healthy (no endpoint), got %v", up["healthy"])
	}
}

func TestToolsHandler(t *testing.T) {
	client := upstream.NewClient("", "", 5)
	req := httptest.NewRequest(http.MethodGet, "/tools", nil)
	w := httptest.NewRecorder()

	toolsHandler(w, req, client)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	tools, ok := resp["tools"].([]any)
	if !ok || len(tools) != 4 {
		t.Fatalf("expected 4 tools, got %v", tools)
	}
}

func TestInvokeHandler_MethodNotAllowed(t *testing.T) {
	client := upstream.NewClient("", "", 5)
	req := httptest.NewRequest(http.MethodGet, "/invoke", nil)
	w := httptest.NewRecorder()

	invokeHandler(w, req, client)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp["error"].(string), "POST") {
		t.Fatalf("expected POST error message, got %v", resp["error"])
	}
}

func TestInvokeHandler_BadBody(t *testing.T) {
	client := upstream.NewClient("", "", 5)
	req := httptest.NewRequest(http.MethodPost, "/invoke", strings.NewReader("not-json"))
	w := httptest.NewRecorder()

	invokeHandler(w, req, client)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestInvokeHandler_UnknownTool(t *testing.T) {
	client := upstream.NewClient("", "", 5)
	body, _ := json.Marshal(map[string]any{"tool": "unknown_tool"})
	req := httptest.NewRequest(http.MethodPost, "/invoke", bytes.NewReader(body))
	w := httptest.NewRecorder()

	invokeHandler(w, req, client)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp["error"].(string), "不认识") {
		t.Fatalf("expected unknown tool error, got %v", resp["error"])
	}
}

func TestInvokeHandler_Status(t *testing.T) {
	client := upstream.NewClient("", "", 5)
	body, _ := json.Marshal(map[string]any{"tool": "bifrost_status"})
	req := httptest.NewRequest(http.MethodPost, "/invoke", bytes.NewReader(body))
	w := httptest.NewRecorder()

	invokeHandler(w, req, client)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatal("expected result")
	}
	if result["name"] != RepoName {
		t.Fatalf("expected name %s, got %v", RepoName, result["name"])
	}
}

func TestModelsHandler_MethodNotAllowed(t *testing.T) {
	client := upstream.NewClient("", "", 5)
	req := httptest.NewRequest(http.MethodPost, "/models", nil)
	w := httptest.NewRecorder()

	modelsHandler(w, req, client)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestFriendlyError(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"connection refused", "连接被拒绝"},
		{"context deadline exceeded", "请求超时"},
		{"no such host", "域名解析失败"},
		{"401 Unauthorized", "认证失败"},
		{"some random error", "some random error"},
	}
	for _, tt := range tests {
		got := friendlyError(&testError{msg: tt.input})
		if !strings.Contains(got, tt.expected) {
			t.Fatalf("friendlyError(%q) = %q, want containing %q", tt.input, got, tt.expected)
		}
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string { return e.msg }

func TestHandleStdioTool_Status(t *testing.T) {
	client := upstream.NewClient("", "", 5)
	res := handleStdioTool(nil, client, "bifrost_status", nil)
	content, ok := res["content"].([]map[string]string)
	if !ok || len(content) == 0 {
		t.Fatal("expected content")
	}
	if !strings.Contains(content[0]["text"], RepoName) {
		t.Fatalf("expected repo name in status, got %v", content[0]["text"])
	}
}

func TestHandleStdioTool_Unknown(t *testing.T) {
	client := upstream.NewClient("", "", 5)
	res := handleStdioTool(nil, client, "xxx", nil)
	content, ok := res["content"].([]map[string]string)
	if !ok || len(content) == 0 {
		t.Fatal("expected content")
	}
	if !strings.Contains(content[0]["text"], "不认识") {
		t.Fatalf("expected unknown tool message, got %v", content[0]["text"])
	}
}

func TestVersion(t *testing.T) {
	if Version != "0.2.0" {
		t.Fatalf("expected version 0.2.0, got %s", Version)
	}
}
