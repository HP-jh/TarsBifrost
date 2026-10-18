package config

import (
	"os"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	os.Setenv("TARS_MCP_ENDPOINT", "https://example.com")
	os.Setenv("TARS_MCP_API_KEY", "test-key")
	defer os.Unsetenv("TARS_MCP_ENDPOINT")
	defer os.Unsetenv("TARS_MCP_API_KEY")
	cfg, err := Load("")
	if err != nil { t.Fatalf("unexpected err: %v", err) }
	if cfg.Endpoint != "https://example.com" { t.Fatalf("expected endpoint, got %s", cfg.Endpoint) }
	if cfg.APIKey != "test-key" { t.Fatalf("expected key, got %s", cfg.APIKey) }
	if cfg.Timeout != 30 { t.Fatalf("expected timeout 30, got %d", cfg.Timeout) }
}

func TestLoad_MissingEndpoint(t *testing.T) {
	os.Unsetenv("TARS_MCP_ENDPOINT")
	os.Unsetenv("BIFROST_ENDPOINT")
	_, err := Load("")
	if err == nil { t.Fatal("expected error for missing endpoint") }
}
