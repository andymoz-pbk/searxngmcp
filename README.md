# searxngmcp

Go MCP server wrapping SearXNG metasearch. **Single static executable** — no
runtime dependencies, no libraries to install. Pre-built binaries for every
major platform:

| Platform | Architecture | Binary |
|----------|-------------|--------|
| Linux | amd64 | `searxngmcp` |
| Linux | arm64 | `searxngmcp` |
| macOS | amd64 (Intel) | `searxngmcp` |
| macOS | arm64 (Apple Silicon) | `searxngmcp` |
| Windows | amd64 | `searxngmcp.exe` |
| Windows | arm64 | `searxngmcp.exe` |

All 6 binaries available from the [releases page](https://github.com/andymoz-pbk/searxngmcp/releases) plus a public Docker image (~8 MB).

**11 tools** — search, news, web fetch (with Mozilla Readability), DNS lookup,
datetime, UUID, base64, hashing, random strings.

---

## Quick Start

> **Recommended: Docker** — fastest way to get running. Pre-built public image
> at `ghcr.io/andymoz-pbk/searxngmcp:latest`.

```bash
docker run -d --name searxngmcp -p 8000:8000 \
  -e SEARXNGMCP_SEARXNG_BASE_URL=http://host.docker.internal:8080 \
  ghcr.io/andymoz-pbk/searxngmcp:latest
```

That's it. Server is live on `http://localhost:8000`.

> **No config file needed.** The binary runs with sensible defaults out of the
> box. A config file is optional — every setting is overridable via
> `SEARXNGMCP_*` environment variables.

### Verify it works

```bash
curl http://localhost:8000/mcp \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

Returns all 11 tools. Try a search:

```bash
curl http://localhost:8000/mcp \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"searxng_search","arguments":{"query":"hello world","max_results":3}}}'
```

### Prerequisites

- A running SearXNG instance with `format: json` enabled (see [SearXNG Setup](#searxng-setup))
- Go 1.23+ (to build from source) or Docker

### Docker Compose

**With an existing SearXNG instance** (default — MCP server only):

```bash
docker compose up -d
```

Uses the pre-built image `ghcr.io/andymoz-pbk/searxngmcp:latest`. Connects to
your existing SearXNG at `http://host.docker.internal:8080`. Override:

```bash
SEARXNGMCP_SEARXNG_BASE_URL=http://my-searxng:8080 docker compose up -d
```

**With a bundled SearXNG** (starts both MCP server and SearXNG):

```bash
docker compose --profile searxng up -d
```

This starts both containers: the MCP server and a pre-configured SearXNG
instance (JSON format enabled, exposed on port `8888`). The release tarballs
include `docker-compose.yml` and `searxng-settings.yml` so you can run this
anywhere without needing SearXNG installed separately.

**Build from source** (optional):

```bash
docker build -t searxngmcp .
```

Server listens on `0.0.0.0:8000` in all cases.

### Docker networking

The `host.docker.internal` hostname resolves to your host machine's IP from
inside a container (Docker Desktop and Docker Engine 20.10+ with
`extra_hosts`). If it doesn't work, use one of these alternatives:

**Remote SearXNG** (most common):
```bash
docker run -d --name searxngmcp -p 8000:8000 \
  -e SEARXNGMCP_SEARXNG_BASE_URL=http://your-server-ip:8080 \
  ghcr.io/andymoz-pbk/searxngmcp:latest
```

**SearXNG on the Docker host** (Linux without `host.docker.internal`):
```bash
docker run -d --name searxngmcp -p 8000:8000 \
  --network host \
  -e SEARXNGMCP_SEARXNG_BASE_URL=http://127.0.0.1:8080 \
  ghcr.io/andymoz-pbk/searxngmcp:latest
```

**SearXNG on the Docker host** (using `--add-host`):
```bash
docker run -d --name searxngmcp -p 8000:8000 \
  --add-host=host.docker.internal:host-gateway \
  -e SEARXNGMCP_SEARXNG_BASE_URL=http://host.docker.internal:8080 \
  ghcr.io/andymoz-pbk/searxngmcp:latest
```

---

## Other Deployment Options

### Standalone binary

Download the pre-built binary for your platform from the [releases page](https://github.com/andymoz-pbk/searxngmcp/releases), or grab the tarball (includes `docker-compose.yml`, `run.sh`, `install_service.sh`, config example, and all scripts):

```bash
curl -LO https://github.com/andymoz-pbk/searxngmcp/releases/download/v0.1.0/searxngmcp-dev.tar.gz
tar xzf searxngmcp-dev.tar.gz
chmod +x searxngmcp-dev-linux-amd64
./searxngmcp-dev-linux-amd64
```

> **No config file needed.** The binary runs with sensible defaults. Set
> `SEARXNGMCP_SEARXNG_BASE_URL` to point to your SearXNG instance:
>
> ```bash
> SEARXNGMCP_SEARXNG_BASE_URL=http://your-searxng:8080 ./searxngmcp-dev-linux-amd64
> ```

The `run.sh` wrapper auto-detects config from the standard search order
(`--config` flag → `./config.json` → `/etc/searxngmcp/config.json`) and passes
through any `SEARXNGMCP_*` env vars. Run `./run.sh --help` for details.

You can also run the binary directly:

```bash
./searxngmcp                                 # uses config.json or defaults
./searxngmcp --config /path/to/config.json   # explicit config path
```

### systemd service

```bash
sudo ./install_service.sh              # build + install + enable + start
sudo ./install_service.sh --no-start   # install but don't start
sudo ./install_service.sh --force      # overwrite existing config.json
```

Installs the binary to `/usr/local/bin/searxngmcp`, config to
`/etc/searxngmcp/config.json`, and the systemd unit to
`/etc/systemd/system/searxngmcp.service`.

```bash
systemctl enable --now searxngmcp       # enable and start
systemctl status searxngmcp             # check status
journalctl -u searxngmcp -f             # view logs
```

The service runs as `nobody` with strict systemd hardening
(`ProtectSystem=strict`, `PrivateTmp`, `NoNewPrivileges`, etc.).

### Windows

**Manual run:**

```batch
REM Cross-compile on Linux/macOS (or build on Windows with Go installed):
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o searxngmcp.exe .

REM Edit run.bat — uncomment and set env vars as needed
run.bat
```

Config file is optional — env vars in `run.bat` override all defaults. Config search order:
1. `--config` flag
2. `.\config.json` (current directory)
3. `%ProgramData%\searxngmcp\config.json` (system-wide)

**Install as Windows Service:**

```batch
REM Run as administrator:
install_service.bat
```

Installs binary + `run.bat` to `%ProgramFiles%\searxngmcp\`, creates config at `%ProgramData%\searxngmcp\config.json`, registers service via NSSM, and starts it. Service logs to `%ProgramData%\searxngmcp\searxngmcp.log`.

Edit env vars in `%ProgramFiles%\searxngmcp\run.bat` and restart the service:

```batch
nssm restart searxngmcp
```

Remove the service:

```batch
install_service.bat --remove
```

**Windows-specific notes:**
- DNS lookup: uses `1.1.1.1` fallback on Windows (no `/etc/resolv.conf`). Override via `server` parameter in tool call or config file.
- Cross-compile targets: `windows/amd64`, `windows/arm64`

## SearXNG Setup

SearXNG requires `format: json` in its `search.formats` list.
Without it the search API returns HTTP 403.

**To enable:** create `/etc/searxng/settings.yml` (or your config path):

```yaml
use_default_settings: true
search:
  formats:
    - html
    - json
```

This merges with all default settings, only overriding the formats list.
The same file is included in this repo as `searxng-settings.yml` and mounted
automatically when using the bundled SearXNG via `docker compose --profile searxng`.

---

## Configuration

### Config file (JSON)

Loaded in order (later files override earlier):

1. `--config <path>` CLI flag
2. `./config.json`
3. `/etc/searxngmcp/config.json`

Missing files are silently skipped.

```json
{
  "server": {
    "host": "0.0.0.0",
    "port": 8000
  },
  "searxng": {
    "base_url": "http://localhost:8080",
    "timeout": 30
  },
  "search": {
    "default_max_results": 10,
    "max_max_results": 50,
    "default_categories": "general",
    "default_language": "",
    "default_safesearch": 0
  },
  "fetch": {
    "max_content_length": 1048576,
    "timeout": 30,
    "user_agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
    "max_concurrent": 5
  },
  "logging": {
    "level": "info"
  }
}
```

### Environment variables

Every config field can be overridden with `SEARXNGMCP_*` env vars.
These take precedence over all config files.

| Env var | Config key | Default |
|---------|-----------|---------|
| `SEARXNGMCP_SEARXNG_BASE_URL` | `searxng.base_url` | `http://localhost:8080` |
| `SEARXNGMCP_SEARXNG_TIMEOUT` | `searxng.timeout` | `30` |
| `SEARXNGMCP_SERVER_HOST` | `server.host` | `0.0.0.0` |
| `SEARXNGMCP_SERVER_PORT` | `server.port` | `8000` |
| `SEARXNGMCP_SEARCH_DEFAULT_MAX_RESULTS` | `search.default_max_results` | `10` |
| `SEARXNGMCP_SEARCH_MAX_MAX_RESULTS` | `search.max_max_results` | `50` |
| `SEARXNGMCP_SEARCH_DEFAULT_CATEGORIES` | `search.default_categories` | `general` |
| `SEARXNGMCP_SEARCH_DEFAULT_LANGUAGE` | `search.default_language` | `""` |
| `SEARXNGMCP_SEARCH_DEFAULT_SAFESEARCH` | `search.default_safesearch` | `0` |
| `SEARXNGMCP_FETCH_MAX_CONTENT_LENGTH` | `fetch.max_content_length` | `1048576` |
| `SEARXNGMCP_FETCH_TIMEOUT` | `fetch.timeout` | `30` |
| `SEARXNGMCP_FETCH_USER_AGENT` | `fetch.user_agent` | Chrome 125 browser UA |
| `SEARXNGMCP_FETCH_MAX_CONCURRENT` | `fetch.max_concurrent` | `5` |
| `SEARXNGMCP_LOGGING_LEVEL` | `logging.level` | `info` |

### CLI flags

```
--config   string   path to config file
```

Only one flag. All other configuration comes from config files or env vars.

---

## Tools

### searxng_search

Search the web via SearXNG metasearch.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `query` | string | yes | — | Search query (supports `site:example.com` syntax) |
| `categories` | string | no | `general` | `general`, `images`, `news`, `files`, `map`, `music`, `it`, `science`, `social media` |
| `language` | string | no | `""` | Language code (`en`, `de`, `fr`, …). Empty = auto-detect |
| `pageno` | number | no | `1` | Page number (1-based) |
| `time_range` | string | no | `""` | `day`, `week`, `month`, `year`. Empty = no filter |
| `safesearch` | number | no | `0` | `0` off, `1` moderate, `2` strict |
| `max_results` | number | no | `10` | Maximum results to return (capped by `search.max_max_results`) |
| `engines` | string | no | `""` | Comma-separated engine list (`google,bing,reddit`) |

### searxng_search_news

News search. Shortcut for `categories=news` + `time_range=week`.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `query` | string | yes | — | Search query |
| `language` | string | no | `""` | Language code |
| `time_range` | string | no | `week` | Time range filter |
| `safesearch` | number | no | `0` | SafeSearch level |
| `max_results` | number | no | `10` | Maximum results |
| `engines` | string | no | `""` | Comma-separated engines |

### searxng_fetch

Fetch a single web page. Returns content with metadata including connection info.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `url` | string | yes | — | Full URL to fetch |
| `max_length` | number | no | `1048576` | Max content bytes to return (`0` = unlimited). Applied to raw response before text/markdown processing. |
| `start_index` | number | no | `0` | Character offset into the returned body to start reading from (for paginated/chunked reading of processed output) |
| `format` | string | no | `text` | `text` (cleaned HTML→plain), `markdown` (HTML→Markdown), `raw` (unprocessed body), `html` (raw HTML), `full` (raw + unlimited + all metadata) |
| `mode` | string | no | `"full"` | Content extraction mode: `"full"` (default, strip only scripts/styles, keep all content) or `"smart"` (Mozilla Readability: extract main article, strip navigation/sidebars/comments) |
| `check_llms_txt` | boolean | no | `false` | Also fetch `{origin}/llms.txt` and include in response |
| `headers` | object | no | `{}` | Custom HTTP request headers (key-value pairs) |
| `cookies` | string | no | `""` | Cookie header value from browser devtools (bypasses some bot protection) |
| `proxy` | string | no | `""` | Reader-mode proxy URL template (e.g. `https://r.jina.ai/`). Target URL appended. |

> **Note on bot-protected sites**: Some sites (Reuters, The Mirror, etc.) use DataDome, CloudFront, or Cloudflare bot detection that blocks non-browser HTTP clients regardless of headers. To fetch content from these sites, pass cookies from your browser session via the `cookies` parameter. Copy the `Cookie` header value from the network tab in DevTools. Most other major news sites (BBC, Guardian, CNBC, Ars Technica) work without cookies.

**Response includes:**
- `url`, `status_code`, `content_type`, `content_length`
- `truncated` — boolean, `true` if response exceeded `max_length`
- `headers` — selected response headers
- `body` — page content (format depends on `format` parameter)
- `external_resources` — inventory of images, scripts, stylesheets, iframes, videos, objects extracted from HTML (always-on, no extra requests)
- `llms_txt` — content of `{origin}/llms.txt` if `check_llms_txt=true`, or `null` if not found
- `connection` — connection metadata (captured via Go `httptrace`):
  - `remote_addr` — remote IP:port
  - `local_addr` — local IP:port
  - `dns_results` — resolved IP addresses
  - `tls_version` — TLS version (e.g. `TLS 1.3`)
  - `tls_cipher` — cipher suite name
  - `tls_server_name` — TLS SNI
- `redirect_chain` — list of URLs followed through redirects

### searxng_fetch_many

Fetch multiple pages in parallel. Concurrency limited by `fetch.max_concurrent` (default 5).

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `urls` | array of strings | yes | — | URLs to fetch |
| `max_length` | number | no | `1048576` | Max bytes per page (`0` = unlimited) |
| `start_index` | number | no | `0` | Character offset for paginated reading |
| `format` | string | no | `text` | Output format (same as `searxng_fetch`) |
| `mode` | string | no | `"full"` | Content extraction mode: `"full"` (default) or `"smart"` (Mozilla Readability) |
| `check_llms_txt` | boolean | no | `false` | Also fetch `llms.txt` per URL |
| `cookies` | string | no | `""` | Cookie header value (applies to all URLs) |
| `proxy` | string | no | `""` | Reader-mode proxy URL template (applied to all URLs) |

> Bot-protected sites note: same as `searxng_fetch` above.

Returns an array of per-page results, each with the same structure as `searxng_fetch`.

### get_datetime

Current date, time, timezone, and timestamp.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `timezone` | string | no | `UTC` | IANA timezone name (`America/New_York`, `Europe/London`, …) |

### generate_uuid

Generate a random UUID v4. No parameters. Uses `crypto/rand`.

### base64_encode

Encode a string to Base64 (standard encoding).

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `text` | string | yes | — | Text to encode |

### base64_decode

Decode Base64 to text. Tries standard encoding first, falls back to raw (unpadded) standard encoding.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `encoded` | string | yes | — | Base64 string to decode |

### hash_string

Generate a cryptographic hash of a string.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `text` | string | yes | — | String to hash |
| `algorithm` | string | no | `sha256` | `sha256`, `sha512`, or `md5` |

### generate_random_string

Cryptographically secure random string.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `length` | number | no | `16` | Length (max `4096`) |
| `charset` | string | no | `alphanumeric` | `alphanumeric`, `alphabetic`, `numeric`, `hex`, `ascii` |

### dns_lookup

DNS record lookup. Every call performs a fresh DNS query — no caching, no search domain interference. Uses raw DNS wire protocol over UDP (not Go's `net.Resolver`), bypassing system resolver behavior.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | yes | — | Domain name or IP (for PTR) |
| `type` | string | no | `A` | Record type (see below) |
| `server` | string | no | `""` | Custom DNS server IP. Empty = system nameservers from `/etc/resolv.conf` |
| `port` | number | no | `53` | DNS server port (1–65535) |

**Supported record types:**

| Type | Description |
|------|-------------|
| `A` | IPv4 address |
| `AAAA` | IPv6 address |
| `MX` | Mail exchange with preference |
| `NS` | Nameserver |
| `CNAME` | Canonical name |
| `TXT` | Text records (SPF, DKIM, verification tokens, …) |
| `SRV` | Service records |
| `PTR` | Reverse DNS. Pass an IP as `name` for auto-conversion to in-addr.arpa / ip6.arpa |
| `SOA` | Start of authority (primary NS, hostmaster, serial, refresh, retry, expire, minimum TTL) |
| `ALL` | Queries A + AAAA + MX + NS + CNAME + TXT + SOA sequentially, returns deduplicated results |

**Custom server example:**

```bash
curl http://localhost:8000/mcp \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"dns_lookup","arguments":{"name":"google.com","type":"ALL","server":"8.8.8.8"}}}'
```

**PTR reverse lookup:**

```bash
curl http://localhost:8000/mcp \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"dns_lookup","arguments":{"name":"8.8.8.8","type":"PTR"}}}'
```

**Response format:**

```json
[
  { "type": "A", "name": "example.com", "value": "93.184.216.34", "ttl": 300 },
  { "type": "AAAA", "name": "example.com", "value": "2606:2800:220:1:248:1893:25c8:1946", "ttl": 300 }
]
```

---

## Transport

The server supports two MCP transports on the same port:

| Endpoint | Protocol |
|----------|----------|
| `GET /sse` | Legacy HTTP + SSE. Opens an SSE stream, sends `event: endpoint`, receives JSON-RPC messages via POST to `/messages/<sessionID>` |
| `POST /mcp` | Streamable HTTP. Each request/response pair is a complete JSON-RPC exchange |

Both transports support the same MCP methods: `initialize`, `tools/list`, `tools/call`.

---

## Architecture

```
main.go        → flag parsing, config load, http.Server start (_ "time/tzdata" for scratch image)
config.go      → Config struct, DefaultConfig(), LoadConfig(), env overrides
server.go      → MCPServer (HTTP mux, SSE streams, JSON-RPC dispatch, CORS, panic recovery)
streamable.go  → Streamable HTTP transport (/mcp endpoint)
tools.go       → Tool definitions + handlers (search, fetch, datetime, uuid, base64, hash, random, DNS)
extract.go     → Mozilla Readability wrapper (go-readability v2) for smart mode content extraction
searxng.go     → SearXNG HTTP client, SearchParams, SearXNGResponse types
dnslookup.go   → Raw DNS wire-protocol client (UDP, no external deps)
```

- **Smart mode** — uses [go-readability v2](https://codeberg.org/readeck/go-readability) (Mozilla Readability algorithm) for main-content extraction. Falls through to regex-based full mode when readability returns <500 chars (non-article pages, SPAs).
- **HTTPS tracing** — `searxng_fetch` uses `net/http/httptrace` to capture connection metadata
- **CORS** — echoes specific `Origin` from request (or `*` when no Origin header), `Access-Control-Allow-Credentials: true`, `Vary: Origin`
- **Panic recovery** — middleware catches panics, returns JSON-RPC error responses
- **HTTP timeouts** — `ReadTimeout=30s`, `WriteTimeout=0` (SSE needs indefinite), `IdleTimeout=120s`

---

## Testing

### Unit tests (no external dependencies)

```bash
go test -v -count=1 ./...
```

143 tests covering mock DNS servers, HTML→text conversion, MCP protocol flow,
CORS, edge cases (unicode, XSS, SSRF, redirect exhaustion, concurrent sessions).

### Integration tests (require SearXNG on :8080)

```bash
go test -tags=integration -v -count=1 ./...
```

Tests real SearXNG search, real URL fetching, and real DNS resolution for
ibm.com, google.com, google.com MX/NS/SOA, custom DNS server (8.8.8.8), and more.

### Test files

| File | What it tests |
|------|---------------|
| `searxng_test.go` | SearXNG HTTP client (mock server, 403, bad JSON, connection refused) |
| `tools_test.go` | Tool handler logic, HTML→text conversion (8 cases), fetch formats |
| `server_test.go` | Full MCP flow (SSE, tools/list, tools/call, initialize, CORS, 404) |
| `edge_test.go` | Extreme/security: Unicode, XSS, SQL injection, SSRF, binary payloads, redirect exhaustion, concurrent sessions |
| `dnslookup_test.go` | All DNS record types via mock UDP server, dedup, timeouts, very long names |
| `dns_integration_test.go` | Real DNS resolution for production domains |
| `integration_test.go` | Real SearXNG search, real URL fetch, fake URL error handling |

---

## Cross-compilation

```bash
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o searxngmcp-linux-arm64 .
GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o searxngmcp-darwin-amd64 .
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o searxngmcp-windows-amd64.exe .
GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o searxngmcp-windows-arm64.exe .
```

Static binaries — no runtime dependencies. Includes `time/tzdata` for IANA
timezone database (needed for scratch Docker images that have no `/usr/share/zoneinfo`).

---

## Project files

| File | Purpose |
|------|---------|
| `main.go` | Entry point |
| `config.go` | Config struct, loading, env overrides |
| `server.go` | HTTP/SSE MCP server, CORS, panic recovery |
| `streamable.go` | Streamable HTTP transport |
| `tools.go` | 11 tool definitions + handlers |
| `extract.go` | Mozilla Readability wrapper (go-readability) |
| `searxng.go` | SearXNG HTTP client |
| `dnslookup.go` | DNS lookup (raw UDP wire protocol) |
| `Dockerfile` | Multi-stage scratch build (~8 MB image) |
| `docker-compose.yml` | Docker Compose (two modes: existing or bundled SearXNG) |
| `searxng-settings.yml` | SearXNG config override (enables JSON format) |
| `config.example.json` | Documented default config |
| `run.sh` | Standalone runner script — Linux/macOS (auto-detects config, optional --build) |
| `run.bat` | Standalone runner script — Windows (env vars as comments, manual or service) |
| `install_service.sh` | systemd service installer (Linux) |
| `install_service.bat` | Windows Service installer using NSSM |
| `searxngmcp.service` | systemd unit file |
| `release.sh` | Full release build script (deps check, vendor, test, build, cross-compile, dist) |
| `Makefile` | Build, install, dist, vendor, test, release targets |
| `vendor/` | Vendored Go dependencies (for offline/self-contained builds) |
| `AGENTS.md` | Developer reference |