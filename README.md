# TarsBifrost

TSG (TarsSecureGuard) MCP adapter for **Bifrost (Maxim AI)** — Unified LLM routing & failover in Go.

This is one of the **15 MCP integration projects** under github.com/HP-jh/, designed to be:

- **One-click downloadable** from TSG's MCP Marketplace
- **One-click enableable** in the TSG gateway
- **Independently versioned** (this repo) with its own release cycle
- **Tightly integrated** with TSG via the standard MCP protocol

## Quick Start

### Standalone

```bash
# Download from GitHub Releases
curl -L -o tars-bifrost https://github.com/HP-jh/TarsBifrost/releases/latest/download/tars-bifrost-$(uname -s | tr A-Z a-z)-amd64
chmod +x tars-bifrost
./tars-bifrost --upstream-endpoint <your-Bifrost (Maxim AI)-endpoint>
```

### Via TSG MCP Marketplace

1. Open TSG admin → **MCP Marketplace**
2. Find **TarsBifrost** in the list
3. Click **Download** → wait for completion
4. Click **Enable** → enter your Bifrost (Maxim AI) endpoint
5. The MCP server is now available to all TSG clients

## MCP Tools Exposed

| Tool | Description |
|------|-------------|
| `bifrost_list_models` | List available Bifrost (Maxim AI) models |
| `bifrost_invoke` | Invoke Bifrost (Maxim AI) with prompt + params |
| `bifrost_health` | Health check of upstream connection |
| `bifrost_status` | Current TSG integration status |

## Configuration

Configurable via env vars or `~/.tars/bifrost/config.json`:

- `TARS_BIFROST_ENDPOINT` — upstream endpoint URL
- `TARS_BIFROST_API_KEY` — upstream API key (optional, for hosted services)
- `TARS_BIFROST_TIMEOUT` — request timeout (default 30s)
- `TARS_BIFROST_LOG_LEVEL` — log verbosity (default info)

## Version

- **v0.1.0** — Initial MVP (basic MCP adapter)
- See [Releases](https://github.com/HP-jh/TarsBifrost/releases) for full changelog

## Cross-Repo Coordination

This repo is part of the **15 MCP integration projects** in github.com/HP-jh/. See [HP-jh/TarsSecureGuard](https://github.com/HP-jh/TarsSecureGuard) for the master gateway that orchestrates all 15.

## License

MIT — see [LICENSE](LICENSE).
