package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/HP-jh/TarsBifrost/pkg/config"
	"github.com/HP-jh/TarsBifrost/pkg/upstream"
)

var Version = "1.0.2"
const RepoName = "TarsBifrost"

// rateLimiter provides per-IP token-bucket-like rate limiting.
type rateLimiter struct {
	mu      sync.RWMutex
	entries map[string]*rateEntry
	limit   int
	window  time.Duration
}

type rateEntry struct {
	count int
	reset time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		entries: make(map[string]*rateEntry),
		limit:   limit,
		window:  window,
	}
}

func (rl *rateLimiter) allow(ip string) bool {
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()
	e, ok := rl.entries[ip]
	if !ok || now.After(e.reset) {
		rl.entries[ip] = &rateEntry{count: 1, reset: now.Add(rl.window)}
		return true
	}
	if e.count >= rl.limit {
		return false
	}
	e.count++
	return true
}

func (rl *rateLimiter) cleanup() {
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for ip, e := range rl.entries {
		if now.After(e.reset) {
			delete(rl.entries, ip)
		}
	}
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	if xri := r.Header.Get("X-Real-Ip"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if host != "" { return host }
	return r.RemoteAddr
}

func genRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

type contextKey string
const reqIDKey contextKey = "requestID"

func getReqID(ctx context.Context) string {
	if id, ok := ctx.Value(reqIDKey).(string); ok {
		return id
	}
	return ""
}

func main() {
	version := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *version { fmt.Printf("%s v%s\n", RepoName, Version); return }
	cfg, err := config.Load("")
	if err != nil { log.Fatalf("加载配置失败：%v", err) }
	client := upstream.NewClient(cfg.Endpoint, cfg.APIKey, cfg.Timeout)
	log.Printf("[%s] starting v%s endpoint=%s", RepoName, Version, cfg.Endpoint)

	rl := newRateLimiter(60, time.Minute)
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C { rl.cleanup() }
	}()

	mode := os.Getenv("TARS_MCP_MODE")
	if mode == "" { mode = "stdio" }
	srv := &http.Server{
		Addr:         ":8080",
		ReadTimeout:  time.Duration(cfg.Timeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Timeout) * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	http.HandleFunc("/health", withMiddleware(client, rl, healthHandler))
	http.HandleFunc("/tools", withMiddleware(client, rl, toolsHandler))
	http.HandleFunc("/invoke", withMiddleware(client, rl, invokeHandler))
	http.HandleFunc("/models", withMiddleware(client, rl, modelsHandler))

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Printf("[%s] shutting down", RepoName)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	if mode == "stdio" { runStdioServer(client); return }
	log.Printf("[%s] HTTP listening on %s", RepoName, srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func withMiddleware(client *upstream.Client, rl *rateLimiter, fn func(http.ResponseWriter, *http.Request, *upstream.Client)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqID := genRequestID()
		ctx := context.WithValue(r.Context(), reqIDKey, reqID)
		r = r.WithContext(ctx)

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
		w.Header().Set("X-Request-ID", reqID)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		if r.Method == http.MethodOptions { w.WriteHeader(http.StatusNoContent); return }

		ip := clientIP(r)
		if !rl.allow(ip) {
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error":      "请求过于频繁，请稍后再试",
				"request_id": reqID,
			})
			log.Printf("[RATE_LIMIT] ip=%s method=%s path=%s request_id=%s", ip, r.Method, r.URL.Path, reqID)
			return
		}

		// Wrap response writer to capture status code
		wr := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		fn(wr, r, client)

		log.Printf("[REQ] method=%s path=%s ip=%s status=%d duration=%s request_id=%s",
			r.Method, r.URL.Path, ip, wr.statusCode, time.Since(start).Round(time.Millisecond), reqID)
	}
}

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (rr *responseRecorder) WriteHeader(code int) {
	if !rr.written {
		rr.statusCode = code
		rr.written = true
		rr.ResponseWriter.WriteHeader(code)
	}
}

func (rr *responseRecorder) Write(b []byte) (int, error) {
	if !rr.written {
		rr.WriteHeader(http.StatusOK)
	}
	return rr.ResponseWriter.Write(b)
}

func healthHandler(w http.ResponseWriter, r *http.Request, client *upstream.Client) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	up := client.Health(ctx)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok", "version": Version, "name": RepoName, "upstream": up,
		"time": time.Now().UTC().Format(time.RFC3339),
		"request_id": getReqID(r.Context()),
	})
}

func toolsHandler(w http.ResponseWriter, r *http.Request, client *upstream.Client) {
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"tools": []map[string]interface{}{
		{
			"name":        "bifrost_list_models",
			"description": "List available upstream models",
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
				"required":   []interface{}{},
			},
		},
		{
			"name":        "bifrost_invoke",
			"description": "Invoke a model with prompt",
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{"max_tokens": map[string]interface{}{"type": "number", "description": "Parameter max_tokens"}, "model": map[string]interface{}{"type": "string", "description": "Parameter model"}, "prompt": map[string]interface{}{"type": "string", "description": "Parameter prompt"}, "temperature": map[string]interface{}{"type": "number", "description": "Parameter temperature"}},
				"required":   []interface{}{"model", "prompt"},
			},
		},
		{
			"name":        "bifrost_embeddings",
			"description": "Generate text embeddings",
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{"input": map[string]interface{}{"type": "string", "description": "Parameter input"}, "model": map[string]interface{}{"type": "string", "description": "Parameter model"}},
				"required":   []interface{}{"model", "input"},
			},
		},
		{
			"name":        "bifrost_batch_invoke",
			"description": "Batch invoke multiple prompts",
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{"model": map[string]interface{}{"type": "string", "description": "Parameter model"}, "prompts": map[string]interface{}{"type": "array", "description": "Parameter prompts"}},
				"required":   []interface{}{"model", "prompts"},
			},
		},
		{
			"name":        "bifrost_health",
			"description": "Check upstream health",
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
				"required":   []interface{}{},
			},
		},
		{
			"name":        "bifrost_status",
			"description": "Integration status",
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
				"required":   []interface{}{},
			},
		},
		},
	})
}

func modelsHandler(w http.ResponseWriter, r *http.Request, client *upstream.Client) {
	if r.Method != http.MethodGet { w.WriteHeader(http.StatusMethodNotAllowed); _ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "只接受 GET"}); return }
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	models, err := client.ListModels(ctx)
	if err != nil { w.WriteHeader(http.StatusBadGateway); _ = json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprintf("获取失败：%v", err)}); return }
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"models": models})
}

func invokeHandler(w http.ResponseWriter, r *http.Request, client *upstream.Client) {
	if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); _ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "只接受 POST"}); return }
	var req struct { Tool string `json:"tool"`; Params map[string]interface{} `json:"params"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { w.WriteHeader(http.StatusBadRequest); _ = json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprintf("解析失败：%v", err)}); return }
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	switch req.Tool {
	case "bifrost_list_models":
		result, err := client.InvokeRaw(ctx, "GET", "/v1/models", nil)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprintf("调用失败：%v", err)})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": result})
	case "bifrost_invoke":
		if req.Params["model"] == nil { w.WriteHeader(http.StatusBadRequest); _ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "缺少参数：model"}); return }
		if req.Params["prompt"] == nil { w.WriteHeader(http.StatusBadRequest); _ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "缺少参数：prompt"}); return }
		body := map[string]interface{}{"max_tokens": req.Params["max_tokens"], "model": req.Params["model"], "prompt": req.Params["prompt"], "temperature": req.Params["temperature"]}
		result, err := client.InvokeRaw(ctx, "POST", "/v1/completions", body)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprintf("调用失败：%v", err)})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": result})
	case "bifrost_embeddings":
		if req.Params["model"] == nil { w.WriteHeader(http.StatusBadRequest); _ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "缺少参数：model"}); return }
		if req.Params["input"] == nil { w.WriteHeader(http.StatusBadRequest); _ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "缺少参数：input"}); return }
		body := map[string]interface{}{"input": req.Params["input"], "model": req.Params["model"]}
		result, err := client.InvokeRaw(ctx, "POST", "/v1/embeddings", body)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprintf("调用失败：%v", err)})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": result})
	case "bifrost_batch_invoke":
		if req.Params["model"] == nil { w.WriteHeader(http.StatusBadRequest); _ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "缺少参数：model"}); return }
		if req.Params["prompts"] == nil { w.WriteHeader(http.StatusBadRequest); _ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "缺少参数：prompts"}); return }
		body := map[string]interface{}{"model": req.Params["model"], "prompts": req.Params["prompts"]}
		result, err := client.InvokeRaw(ctx, "POST", "/v1/batch", body)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprintf("调用失败：%v", err)})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": result})
	case "bifrost_health":
		result, err := client.InvokeRaw(ctx, "GET", "/v1/health", nil)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprintf("调用失败：%v", err)})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": result})
	case "bifrost_status":
		result, err := client.InvokeRaw(ctx, "GET", "/status", nil)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprintf("调用失败：%v", err)})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": result})
	default:
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprintf("不认识工具：%s", req.Tool)})
	}
}

func runStdioServer(client *upstream.Client) {
	log.Printf("[%s] stdio mode", RepoName)
	dec := json.NewDecoder(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	for {
		var msg map[string]interface{}
		if err := dec.Decode(&msg); err != nil { return }
		id := msg["id"]; method, _ := msg["method"].(string)
		var result map[string]interface{}
		switch method {
		case "tools/list":
			result = map[string]interface{}{"tools": []map[string]interface{}{{"name": "bifrost_list_models", "description": "List available upstream models"}, {"name": "bifrost_invoke", "description": "Invoke a model with prompt"}, {"name": "bifrost_embeddings", "description": "Generate text embeddings"}, {"name": "bifrost_batch_invoke", "description": "Batch invoke multiple prompts"}, {"name": "bifrost_health", "description": "Check upstream health"}, {"name": "bifrost_status", "description": "Integration status"}}}
		case "tools/call":
			params, _ := msg["params"].(map[string]interface{})
			toolName, _ := params["name"].(string)
			args, _ := params["arguments"].(map[string]interface{})
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			result = handleStdioTool(ctx, client, toolName, args)
			cancel()
		default:
			result = map[string]interface{}{"message": fmt.Sprintf("%s v%s ready", RepoName, Version)}
		}
		_ = enc.Encode(map[string]interface{}{"jsonrpc": "2.0", "id": id, "result": result})
	}
}

func handleStdioTool(ctx context.Context, client *upstream.Client, toolName string, args map[string]interface{}) map[string]interface{} {
	switch toolName {
	case "bifrost_list_models":
		result, err := client.InvokeRaw(ctx, "GET", "/v1/models", nil)
		if err != nil {
			return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": fmt.Sprintf("调用失败：%v", err)}}}
		}
		data, _ := json.Marshal(result)
		return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": string(data)}}}
	case "bifrost_invoke":
		if args["model"] == nil { return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": "缺少参数：model"}}} }
		if args["prompt"] == nil { return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": "缺少参数：prompt"}}} }
		result, err := client.InvokeRaw(ctx, "POST", "/v1/completions", map[string]interface{}{"max_tokens": args["max_tokens"], "model": args["model"], "prompt": args["prompt"], "temperature": args["temperature"]})
		if err != nil {
			return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": fmt.Sprintf("调用失败：%v", err)}}}
		}
		data, _ := json.Marshal(result)
		return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": string(data)}}}
	case "bifrost_embeddings":
		if args["model"] == nil { return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": "缺少参数：model"}}} }
		if args["input"] == nil { return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": "缺少参数：input"}}} }
		result, err := client.InvokeRaw(ctx, "POST", "/v1/embeddings", map[string]interface{}{"input": args["input"], "model": args["model"]})
		if err != nil {
			return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": fmt.Sprintf("调用失败：%v", err)}}}
		}
		data, _ := json.Marshal(result)
		return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": string(data)}}}
	case "bifrost_batch_invoke":
		if args["model"] == nil { return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": "缺少参数：model"}}} }
		if args["prompts"] == nil { return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": "缺少参数：prompts"}}} }
		result, err := client.InvokeRaw(ctx, "POST", "/v1/batch", map[string]interface{}{"model": args["model"], "prompts": args["prompts"]})
		if err != nil {
			return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": fmt.Sprintf("调用失败：%v", err)}}}
		}
		data, _ := json.Marshal(result)
		return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": string(data)}}}
	case "bifrost_health":
		result, err := client.InvokeRaw(ctx, "GET", "/v1/health", nil)
		if err != nil {
			return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": fmt.Sprintf("调用失败：%v", err)}}}
		}
		data, _ := json.Marshal(result)
		return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": string(data)}}}
	case "bifrost_status":
		result, err := client.InvokeRaw(ctx, "GET", "/status", nil)
		if err != nil {
			return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": fmt.Sprintf("调用失败：%v", err)}}}
		}
		data, _ := json.Marshal(result)
		return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": string(data)}}}
	default:
		return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": fmt.Sprintf("不认识工具：%s", toolName)}}}
	}
}
