# AGENTS.md — searxngmcp

Go-based MCP server wrapping SearXNG. 20 tools over HTTP SSE + Streamable HTTP transport. Uses go-readability (Mozilla Readability) for smart content extraction. All dependencies vendored into `vendor/` for offline builds.

## Build

```bash
go mod init searxngmcp   # if not yet initialised
go mod tidy
go build -o searxngmcp .
```

Produces a static binary. Cross-compile: `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build ...`

## Run

```bash
./searxngmcp                                 # uses config.json or defaults
./searxngmcp --config /path/to/config.json   # explicit path
```

Config loaded from `--config` flag, then `./config.json`, then `/etc/searxngmcp/config.json`.
All settings also overridable via `SEARXNGMCP_*` env vars (see `config.go:applyEnvOverrides`).

## Architecture

```
main.go        → flag parsing, config load, http.Server start (_ "time/tzdata")
config.go      → Config struct, DefaultConfig(), LoadConfig(), env overrides
server.go      → MCPServer (HTTP mux, SSE streams, JSON-RPC dispatch, CORS, panic recovery)
streamable.go  → Streamable HTTP transport (POST /mcp)
tools.go       → Tool definitions + handlers (search, fetch with HTML→text/markdown, datetime, uuid, base64, hash, random)
extract.go     → Mozilla Readability wrapper (go-readability v2) for smart mode content extraction
searxng.go     → SearXNG HTTP client, SearchParams, SearXNGResponse types
dnslookup.go   → Raw DNS wire protocol (A/AAAA/MX/NS/CNAME/TXT/SRV/PTR/SOA/ALL)
pentest.go     → Pentesting tools (url_encode/decode, hex_encode/decode, jwt_decode, hash_identify, xor_cipher, whois_lookup, ssl_cert_info)
```

- **SSE**: `GET /sse` opens stream, sends `event: endpoint`, receives JSON-RPC
- **Messages**: `POST /messages/<sessionID>` enqueues JSON-RPC calls
- **Streamable HTTP**: `POST /mcp` (JSON-RPC over HTTP, no SSE)
- **CORS**: echoes specific Origin, `Access-Control-Allow-Credentials: true`, `Vary: Origin`, `Access-Control-Expose-Headers: MCP-Session-Id`
- **Timeout**: http.Server `ReadTimeout=30s`, `WriteTimeout=0` (SSE needs indefinite), `IdleTimeout=120s`
- **SearXNG client**: calls go through a 30s `http.Client.Timeout`; response headers (status, rate-limit, retry) passed through

## Test

```bash
go test -v -count=1 ./...          # unit tests (mock servers, no external deps)
go test -tags=integration ./...    # integration tests (requires SearXNG on :8080)
```

`searxng_test.go` — mock SearXNG server via httptest, covers success, 403, bad JSON, connection refused, param passthrough, result limiting.
`tools_test.go` — tool handler logic, HTML→text conversion (8 cases), URL error handling, fetch enhancements (external resources, llms_txt, truncated, mode, start_index, custom headers, unlimited, full format).
`server_test.go` — full MCP flow (SSE, tools/list, tools/call, initialize, invalid JSON, CORS preflight, 404, method validation).
`edge_test.go` — extreme/security tests: unicode/emoji/control-char queries, XSS/SQL/command injection payloads, very long queries (10k chars), SSRF attempts (internal IPs, metadata, redirect), binary/non-UTF8 content, redirect exhaustion, CRLF injection, malformed JSON-RPC, concurrent sessions, nil/wrong-type args.
`integration_test.go` — build-tagged (`//go:build integration`), hits real SearXNG and example.com.
`pentest_test.go` — pentesting tool tests: url/hex encode/decode round-trips, JWT decode, hash identification, XOR cipher symmetry, WHOIS/SSL error handling, nil args.

## Deploy

- **systemd**: `make install` copies binary + config + unit, then `systemctl enable --now searxngmcp`
- **Docker**: `docker build -t searxngmcp .` multi-stage → scratch image (~8 MB)
- **Windows**: `run.bat` for manual, `install_service.bat` for NSSM-based Windows Service

## Key constraints

- Dependencies vendored into `vendor/` — run `go mod vendor` to refresh
- SearXNG JSON API endpoint must enable `format: json` in its `settings.yml`
- Bot-protected sites (Reuters, The Mirror — DataDome/CloudFront) block non-browser HTTP clients; workaround: pass `cookies` from browser devtools
- Most other news sites (BBC, Guardian, CNBC, Ars Technica) work without cookies
- **Windows**: cross-compiles via `GOOS=windows`. Config path: `%ProgramData%\searxngmcp\config.json`. DNS falls back to `1.1.1.1` (no `/etc/resolv.conf`). Windows Service via NSSM (`install_service.bat`). `run.bat` works for both manual and service.
