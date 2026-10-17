package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client Bifrost upstream HTTP 客户端
type Client struct {
	endpoint string
	apiKey   string
	client   *http.Client
}

// NewClient 创建上游客户端
func NewClient(endpoint, apiKey string, timeoutSec int) *Client {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	return &Client{
		endpoint: endpoint,
		apiKey:   apiKey,
		client: &http.Client{
			Timeout: time.Duration(timeoutSec) * time.Second,
		},
	}
}

// Health 检查上游连通性
func (c *Client) Health(ctx context.Context) map[string]interface{} {
	if c.endpoint == "" {
		return map[string]interface{}{"healthy": false, "error": "上游地址尚未配置，请先设置 endpoint"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/health", nil)
	if err != nil {
		return map[string]interface{}{"healthy": false, "error": fmt.Sprintf("创建请求失败：%v", err)}
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return map[string]interface{}{"healthy": false, "error": fmt.Sprintf("连接失败：%v", err)}
	}
	defer resp.Body.Close()
	return map[string]interface{}{
		"healthy": resp.StatusCode == http.StatusOK,
		"status":  resp.StatusCode,
	}
}

// ListModels 获取上游模型列表（Bifrost / Maxim AI 通用适配）
func (c *Client) ListModels(ctx context.Context) ([]map[string]string, error) {
	if c.endpoint == "" {
		return nil, fmt.Errorf("上游地址尚未配置")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("上游返回 %d：%s", resp.StatusCode, string(body))
	}
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		// 兜底：如果上游不是标准格式，返回空列表不报错
		return []map[string]string{}, nil
	}
	var out []map[string]string
	for _, m := range result.Data {
		out = append(out, map[string]string{"id": m.ID, "name": m.ID})
	}
	return out, nil
}

// Invoke 调用上游生成接口
func (c *Client) Invoke(ctx context.Context, model, prompt string, params map[string]interface{}) (map[string]interface{}, error) {
	if c.endpoint == "" {
		return nil, fmt.Errorf("上游地址尚未配置，请先设置 TARS_BIFROST_ENDPOINT 或配置文件")
	}
	body := map[string]interface{}{
		"model":  model,
		"prompt": prompt,
	}
	// 合并额外参数
	for k, v := range params {
		if k != "model" && k != "prompt" {
			body[k] = v
		}
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/v1/completions", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用上游失败：%w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("上游返回 %d：%s", resp.StatusCode, string(respBody))
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return map[string]interface{}{"raw": string(respBody)}, nil
	}
	return result, nil
}
