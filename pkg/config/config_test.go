package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	os.Setenv("TARS_BIFROST_ENDPOINT", "http://test")
	os.Setenv("TARS_BIFROST_API_KEY", "key123")
	defer os.Unsetenv("TARS_BIFROST_ENDPOINT")
	defer os.Unsetenv("TARS_BIFROST_API_KEY")

	c := Default()
	if c.Endpoint != "http://test" {
		t.Fatalf("expected endpoint http://test, got %s", c.Endpoint)
	}
	if c.APIKey != "key123" {
		t.Fatalf("expected api_key key123, got %s", c.APIKey)
	}
	if c.Timeout != 30 {
		t.Fatalf("expected timeout 30, got %d", c.Timeout)
	}
}

func TestLoad_DefaultOnMissing(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nonexistent", "config.json")
	c, err := Load(path)
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if c == nil {
		t.Fatal("expected config, got nil")
	}
}

func TestLoad_AndSave(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	orig := &Cfg{Endpoint: "http://e", APIKey: "k", Timeout: 10, LogLevel: "debug"}
	if err := orig.Save(path); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	c, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if c.Endpoint != "http://e" {
		t.Fatalf("expected endpoint http://e, got %s", c.Endpoint)
	}
	if c.APIKey != "k" {
		t.Fatalf("expected api_key k, got %s", c.APIKey)
	}
	if c.Timeout != 10 {
		t.Fatalf("expected timeout 10, got %d", c.Timeout)
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	orig := &Cfg{Endpoint: "http://file", APIKey: "filekey", Timeout: 10}
	_ = orig.Save(path)

	os.Setenv("TARS_BIFROST_ENDPOINT", "http://env")
	os.Setenv("TARS_BIFROST_API_KEY", "envkey")
	defer os.Unsetenv("TARS_BIFROST_ENDPOINT")
	defer os.Unsetenv("TARS_BIFROST_API_KEY")

	c, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if c.Endpoint != "http://env" {
		t.Fatalf("expected env override http://env, got %s", c.Endpoint)
	}
	if c.APIKey != "envkey" {
		t.Fatalf("expected env override envkey, got %s", c.APIKey)
	}
}

func TestLoad_BadJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "bad.json")
	_ = os.WriteFile(path, []byte("not json"), 0644)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for bad json")
	}
}

func TestLoad_DefaultTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")
	_ = os.WriteFile(path, []byte(`{"timeout":0}`), 0644)
	c, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if c.Timeout != 30 {
		t.Fatalf("expected default timeout 30, got %d", c.Timeout)
	}
}
