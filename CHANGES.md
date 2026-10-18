# Changelog

## v1.0.2 · 集成版本

### 整合的历史优点（v0.2.0 + v1.0.0）
- **真实上游 HTTP 调用**：`pkg/upstream/client.go` 实现完整的 Maxim AI / Bifrost 上游调用（v0.2.0）
- **模型列表与生成调用**：`/models`、`bifrost_invoke`、`bifrost_embeddings`、`bifrost_batch_invoke` 全工具链可用（v0.2.0）
- **健康检查**：`/health` 实时检测上游连通性（v0.2.0）
- **配置系统**：环境变量 `TARS_MCP_ENDPOINT` / `TARS_MCP_API_KEY` / `TARS_MCP_TIMEOUT` + 降级 `BIFROST_*`（v0.2.0）
- **MCP 双模式集成**：`stdio`（默认）与 `http` 模式，支持 JSON-RPC 工具列表与调用（v1.0.0）
- **CORS 与基础安全头**：跨域支持 + `X-Content-Type-Options` / `X-Frame-Options`（v1.0.0）
- **优雅关闭**：SIGINT/SIGTERM 信号处理（v1.0.0）

### v1.0.2 新增功能
- **速率限制中间件**：基于 IP 的令牌桶限速（每 IP 每 60 秒最多 60 请求），超限返回 429 + 中文提示
- **请求 ID 追踪**：每个 HTTP 请求生成唯一 `X-Request-ID`，注入响应头与日志，支持链路追踪
- **增强安全头**：新增 `Referrer-Policy`、`Content-Security-Policy`；TLS 时自动下发 `Strict-Transport-Security`
- **结构化请求日志**：每条请求记录 method / path / ip / status / duration / request_id
- **响应状态码捕获**：`responseRecorder` 包装器准确记录实际返回状态码
- **IP 提取增强**：支持 `X-Forwarded-For`、`X-Real-Ip`、`RemoteAddr` 三级回退
- **优雅关闭增强**：使用 `http.Server.Shutdown` + 10 秒超时，确保连接有序释放
- **空闲连接超时**：HTTP 服务器新增 `IdleTimeout: 120s`

## v1.0.1
- Version bump with stability monitoring

## v1.0.0
- Complete MCP integration for TarsBifrost
- 7-platform binaries + SHA256SUMS

## v0.2.0
- 真实 upstream HTTP 调用
- 模型列表、生成调用、健康检查
- 配置系统（环境变量 + 文件）
