package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Endpoint string
	APIKey   string
	Timeout  int
}

func Load(path string) (*Config, error) {
	endpoint := os.Getenv("TARS_MCP_ENDPOINT")
	if endpoint == "" { endpoint = os.Getenv("BIFROST_ENDPOINT") }
	apiKey := os.Getenv("TARS_MCP_API_KEY")
	if apiKey == "" { apiKey = os.Getenv("BIFROST_API_KEY") }
	timeout := 30
	if v := os.Getenv("TARS_MCP_TIMEOUT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 { timeout = n }
	}
	if endpoint == "" {
		return nil, fmt.Errorf("环境变量 TARS_MCP_ENDPOINT 或 BIFROST_ENDPOINT 未设置")
	}
	return &Config{
		Endpoint: strings.TrimRight(endpoint, "/"),
		APIKey:   apiKey,
		Timeout:  timeout,
	}, nil
}
