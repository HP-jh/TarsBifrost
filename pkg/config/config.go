package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Cfg 全局配置
type Cfg struct {
	Endpoint string `json:"endpoint"`
	APIKey   string `json:"api_key"`
	Timeout  int    `json:"timeout"`
	LogLevel string `json:"log_level"`
}

func Default() *Cfg {
	return &Cfg{
		Endpoint: os.Getenv("TARS_BIFROST_ENDPOINT"),
		APIKey:   os.Getenv("TARS_BIFROST_API_KEY"),
		Timeout:  30,
		LogLevel: "info",
	}
}

func Load(path string) (*Cfg, error) {
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".tars", "bifrost", "config.json")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return nil, err
	}
	var c Cfg
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	// env vars override file
	if v := os.Getenv("TARS_BIFROST_ENDPOINT"); v != "" {
		c.Endpoint = v
	}
	if v := os.Getenv("TARS_BIFROST_API_KEY"); v != "" {
		c.APIKey = v
	}
	if c.Timeout <= 0 {
		c.Timeout = 30
	}
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	return &c, nil
}

func (c *Cfg) Save(path string) error {
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".tars", "bifrost", "config.json")
	}
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
