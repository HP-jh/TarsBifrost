# TarsBifrost

MCP integration for Bifrost / Maxim AI inference platform.

## Tools

- `bifrost_list_models` — List available upstream models
- `bifrost_invoke` — Invoke a model with prompt
- `bifrost_embeddings` — Generate text embeddings
- `bifrost_batch_invoke` — Batch invoke multiple prompts
- `bifrost_health` — Check upstream health
- `bifrost_status` — Integration status

## Quick Start

```bash
go build -o tarsbifrost
TARS_MCP_ENDPOINT=https://api.example.com TARS_MCP_API_KEY=xxx ./tarsbifrost
```

## Environment Variables

- `TARS_MCP_ENDPOINT` — upstream API endpoint
- `TARS_MCP_API_KEY` — upstream API key
- `TARS_MCP_MODE` — `stdio` (default) or `http`

## License

MIT

## Changelog

### v1.0.1
- Version bump with stability monitoring
