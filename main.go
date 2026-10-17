package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/HP-jh/TarsBifrost/pkg/config"
	"github.com/HP-jh/TarsBifrost/pkg/upstream"
)

// Version is set via -ldflags at build time
var Version = "0.2.0"

// RepoName is the GitHub repo name
const RepoName = "TarsBifrost"

func main() {
	version := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *version {
		fmt.Printf("%s v%s\n", RepoName, Version)
		return
	}

	cfg, err := config.Load("")
	if err != nil {
		log.Fatalf("加载配置失败：%v", err)
	}

	client := upstream.NewClient(cfg.Endpoint, cfg.APIKey, cfg.Timeout)

	log.Printf("[%s] starting v%s endpoint=%s", RepoName, Version, cfg.Endpoint)

	mode := os.Getenv("TARS_MCP_MODE")
	if mode == "" {
		mode = "stdio"
	}

	srv := &http.Server{
		Addr:         ":8080",
		ReadTimeout:  time.Duration(cfg.Timeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Timeout) * time.Second,
	}

	// 注册 handler，注入 client
	http.HandleFunc("/health", withClient(client, healthHandler))
	http.HandleFunc("/tools", withClient(client, toolsHandler))
	http.HandleFunc("/invoke", withClient(client, invokeHandler))
	http.HandleFunc("/models", withClient(client, modelsHandler))

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Printf("[%s] shutting down", RepoName)
		_ = srv.Close()
	}()

	if mode == "stdio" {
		runStdioServer(client)
		return
	}

	log.Printf("[%s] HTTP listening on %s", RepoName, srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

// withClient 将 upstream client 注入 handler
func withClient(client *upstream.Client, fn func(http.ResponseWriter, *http.Request, *upstream.Client)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// CORS
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// 安全头
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		fn(w, r, client)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request, client *upstream.Client) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	up := client.Health(ctx)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":    "ok",
		"version":   Version,
		"name":      RepoName,
		"upstream":  up,
		"time":      time.Now().UTC().Format(time.RFC3339),
	})
}

func toolsHandler(w http.ResponseWriter, r *http.Request, client *upstream.Client) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"tools": []map[string]any{
			{
				"name":        "bifrost_list_models",
				"description": "列出 Bifrost (Maxim AI) 上游可用的模型列表",
				"inputSchema": map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
			{
				"name":        "bifrost_invoke",
				"description": "调用 Bifrost (Maxim AI) 生成内容，传入模型名、提示词和可选参数",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"model":  map[string]any{"type": "string", "description": "模型 ID"},
						"prompt": map[string]any{"type": "string", "description": "提示词"},
						"params": map[string]any{"type": "object", "description": "额外参数（温度、最大 token 等）"},
					},
					"required": []string{"model", "prompt"},
				},
			},
			{
				"name":        "bifrost_health",
				"description": "检查上游 Bifrost (Maxim AI) 连接健康状态",
				"inputSchema": map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
			{
				"name":        "bifrost_status",
				"description": "查看当前 TSG 集成状态与版本信息",
				"inputSchema": map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		},
	})
}

func modelsHandler(w http.ResponseWriter, r *http.Request, client *upstream.Client) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "这个接口只接受 GET 请求"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	models, err := client.ListModels(ctx)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": fmt.Sprintf("获取模型列表失败：%v", err)})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"models": models})
}

func invokeHandler(w http.ResponseWriter, r *http.Request, client *upstream.Client) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "这个接口只接受 POST 请求，请确认请求方式是否正确"})
		return
	}

	var req struct {
		Tool   string                 `json:"tool"`
		Params map[string]interface{} `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": fmt.Sprintf("请求体解析失败：%v", err)})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(60)*time.Second)
	defer cancel()

	switch req.Tool {
	case "bifrost_list_models":
		models, err := client.ListModels(ctx)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": fmt.Sprintf("获取模型列表失败：%v", err)})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": models})

	case "bifrost_invoke":
		model, _ := req.Params["model"].(string)
		prompt, _ := req.Params["prompt"].(string)
		if model == "" || prompt == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "缺少必要参数：model 和 prompt 都不能为空"})
			return
		}
		// 提取额外参数
		extra := make(map[string]interface{})
		for k, v := range req.Params {
			if k != "model" && k != "prompt" {
				extra[k] = v
			}
		}
		result, err := client.Invoke(ctx, model, prompt, extra)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": fmt.Sprintf("调用上游失败：%v", err)})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": result})

	case "bifrost_health":
		up := client.Health(ctx)
		_ = json.NewEncoder(w).Encode(map[string]any{"result": up})

	case "bifrost_status":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]any{
				"name":    RepoName,
				"version": Version,
				"mode":    os.Getenv("TARS_MCP_MODE"),
				"endpoint": func() string {
					if client == nil {
						return ""
					}
					return "configured"
				}(),
			},
		})

	default:
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": fmt.Sprintf("不认识的工具：%s，可用工具请访问 /tools", req.Tool)})
	}
}

func runStdioServer(client *upstream.Client) {
	log.Printf("[%s] running in stdio mode (MCP protocol over stdin/stdout)", RepoName)
	dec := json.NewDecoder(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	for {
		var msg map[string]any
		if err := dec.Decode(&msg); err != nil {
			return
		}
		id := msg["id"]
		method, _ := msg["method"].(string)

		var result map[string]any
		switch method {
		case "tools/list":
			result = map[string]any{
				"tools": []map[string]any{
					{"name": "bifrost_list_models", "description": "列出可用模型"},
					{"name": "bifrost_invoke", "description": "调用生成接口"},
					{"name": "bifrost_health", "description": "健康检查"},
					{"name": "bifrost_status", "description": "集成状态"},
				},
			}
		case "tools/call":
			params, _ := msg["params"].(map[string]any)
			toolName, _ := params["name"].(string)
			args, _ := params["arguments"].(map[string]any)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			r := handleStdioTool(ctx, client, toolName, args)
			cancel()
			result = r
		default:
			result = map[string]any{"message": fmt.Sprintf("%s v%s MCP stdio ready", RepoName, Version)}
		}

		_ = enc.Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"result":  result,
		})
	}
}

func handleStdioTool(ctx context.Context, client *upstream.Client, toolName string, args map[string]any) map[string]any {
	switch toolName {
	case "bifrost_list_models":
		models, err := client.ListModels(ctx)
		if err != nil {
			return map[string]any{"content": []map[string]string{{"type": "text", "text": fmt.Sprintf("获取模型列表失败：%v", err)}}}
		}
		data, _ := json.Marshal(models)
		return map[string]any{"content": []map[string]string{{"type": "text", "text": string(data)}}}
	case "bifrost_invoke":
		model, _ := args["model"].(string)
		prompt, _ := args["prompt"].(string)
		if model == "" || prompt == "" {
			return map[string]any{"content": []map[string]string{{"type": "text", "text": "缺少必要参数：model 和 prompt"}}}
		}
		extra := make(map[string]interface{})
		for k, v := range args {
			if k != "model" && k != "prompt" {
				extra[k] = v
			}
		}
		res, err := client.Invoke(ctx, model, prompt, extra)
		if err != nil {
			return map[string]any{"content": []map[string]string{{"type": "text", "text": fmt.Sprintf("调用失败：%v", err)}}}
		}
		data, _ := json.Marshal(res)
		return map[string]any{"content": []map[string]string{{"type": "text", "text": string(data)}}}
	case "bifrost_health":
		up := client.Health(ctx)
		data, _ := json.Marshal(up)
		return map[string]any{"content": []map[string]string{{"type": "text", "text": string(data)}}}
	case "bifrost_status":
		return map[string]any{"content": []map[string]string{{"type": "text", "text": fmt.Sprintf("%s v%s 运行中", RepoName, Version)}}}
	default:
		return map[string]any{"content": []map[string]string{{"type": "text", "text": fmt.Sprintf("不认识的工具：%s", toolName)}}}
	}
}

// friendlyError 将技术错误转换为用户友好的中文提示
func friendlyError(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "connection refused"):
		return "连接被拒绝，请检查上游服务是否启动，或 endpoint 地址是否正确"
	case strings.Contains(s, "timeout") || strings.Contains(s, "deadline exceeded"):
		return "请求超时，上游响应太慢或网络不稳定，请稍后重试"
	case strings.Contains(s, "no such host"):
		return "域名解析失败，请检查 endpoint 地址是否拼写正确"
	case strings.Contains(s, "Unauthorized") || strings.Contains(s, "401"):
		return "认证失败，请检查 API Key 是否正确或是否已过期"
	default:
		return s
	}
}
