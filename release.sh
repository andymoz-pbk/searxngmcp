#!/usr/bin/env bash
set -euo pipefail

# searxngmcp — release build script
#
# Ensures all requirements are installed, dependencies vendored, tests pass,
# and creates a distributable tarball with binary + runtime files.
#
# Usage:
#   ./release.sh                    # build + test + dist tarball
#   ./release.sh --no-test          # skip tests (faster, for dev builds)
#   ./release.sh --version v1.2.3   # override version string
#   ./release.sh --docker           # also build Docker image
#
# Requirements (auto-checked):
#   - Go 1.23+
#   - git (optional, for version string)
#   - docker (only if --docker flag)

NO_TEST=false
DOCKER=false
VERSION_OVERRIDE=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-test)  NO_TEST=true; shift ;;
    --docker)   DOCKER=true; shift ;;
    --version)  VERSION_OVERRIDE="$2"; shift 2 ;;
    --help|-h)
      echo "Usage: $0 [--no-test] [--docker] [--version <tag>]"
      echo ""
      echo "  --no-test       Skip running tests"
      echo "  --docker        Also build Docker image"
      echo "  --version <tag> Override version string (default: git tag or 'dev')"
      exit 0
      ;;
    *) echo "Unknown: $1"; exit 1 ;;
  esac
done

DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

# ─── 1. Check Go installation ───────────────────────────────────────────────

echo "==> Checking Go installation..."
if ! command -v go &>/dev/null; then
  echo "   Go not found. Installing Go 1.23..."
  ARCH=$(uname -m)
  OS=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$ARCH" in
    x86_64|amd64) GOARCH="amd64" ;;
    aarch64|arm64) GOARCH="arm64" ;;
    *) echo "   Unsupported arch: $ARCH"; exit 1 ;;
  esac
  GO_VERSION="1.23.0"
  TARBALL="go${GO_VERSION}.${OS}-${GOARCH}.tar.gz"
  URL="https://go.dev/dl/${TARBALL}"
  echo "   Downloading $URL..."
  curl -fsSL "$URL" | sudo tar -C /usr/local -xz
  export PATH="$PATH:/usr/local/go/bin"
  echo "   Go installed: $(go version)"
else
  echo "   Go found: $(go version)"
  # Check version >= 1.23
  GO_VER=$(go version | grep -oP 'go\K\d+\.\d+' | head -1)
  GO_MINOR=$(echo "$GO_VER" | cut -d. -f2)
  if [ "$GO_MINOR" -lt 23 ]; then
    echo "   ERROR: Go 1.23+ required (found $(go version))" >&2
    exit 1
  fi
fi

# ─── 2. Tidy modules ────────────────────────────────────────────────────────

echo "==> Tidying Go modules..."
go mod tidy
echo "   Dependencies:"
sed -n '/^require/,/^)/p' go.mod | grep -v 'require\|^)' | sed 's/^/     /'

# ─── 3. Vendor dependencies (self-contained offline build) ─────────────────

echo "==> Vendoring dependencies..."
go mod vendor
echo "   vendor/ ($(du -sh vendor/ | cut -f1))"

# ─── 4. Run tests ────────────────────────────────────────────────────────────

if [ "$NO_TEST" = false ]; then
  echo "==> Running unit tests..."
  GOFLAGS=-mod=vendor go test -count=1 ./... 2>&1 | tail -3
  echo "   Tests passed."
else
  echo "==> Skipping tests (--no-test)"
fi

# ─── 5. Build binary ──────────────────────────────────────────────────────────

echo "==> Building binary..."
VERSION="${VERSION_OVERRIDE:-$(git describe --tags --dirty 2>/dev/null || echo 'dev')}"
LDFLAGS="-s -w -X main.version=${VERSION}"
GOFLAGS=-mod=vendor CGO_ENABLED=0 go build -ldflags="$LDFLAGS" -o searxngmcp .
echo "   Binary: searxngmcp ($(du -h searxngmcp | cut -f1)) (version: ${VERSION})"

# ─── 6. Cross-compile (optional) ─────────────────────────────────────────────

echo "==> Cross-compiling..."
for TARGET in "linux amd64" "linux arm64" "darwin amd64" "darwin arm64" "windows amd64" "windows arm64"; do
  set -- $TARGET
  GOOS="$1"; GOARCH="$2"
  if [ "$GOOS" = "windows" ]; then
    EXT=".exe"
  else
    EXT=""
  fi
  OUT="searxngmcp-${VERSION}-${GOOS}-${GOARCH}${EXT}"
  GOFLAGS=-mod=vendor CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
    go build -ldflags="$LDFLAGS" -o "$OUT" . 2>/dev/null && \
    echo "   $OUT ($(du -h "$OUT" | cut -f1))" || \
    echo "   $OUT: skipped"
done

# ─── 7. Create dist tarball ───────────────────────────────────────────────

echo "==> Creating distribution tarball..."
DIST_DIR="deploy"
DIST_VERSION="${VERSION}"
DIST_NAME="searxngmcp-${DIST_VERSION}"
mkdir -p "$DIST_DIR"

# Copy runtime files (not source code)
cp searxngmcp "$DIST_DIR/"
cp config.example.json "$DIST_DIR/"
cp searxngmcp.service "$DIST_DIR/"
cp install_service.sh "$DIST_DIR/"
cp install_service.bat "$DIST_DIR/"
cp run.sh "$DIST_DIR/"
cp run.bat "$DIST_DIR/"
cp docker-compose.yml "$DIST_DIR/"
cp searxng-settings.yml "$DIST_DIR/"
cp Dockerfile "$DIST_DIR/"
cp README.md "$DIST_DIR/"
cp go.mod "$DIST_DIR/"
cp go.sum "$DIST_DIR/"
cp -r vendor/ "$DIST_DIR/vendor/"
chmod +x "$DIST_DIR/searxngmcp" "$DIST_DIR/"*.sh

TARBALL="${DIST_NAME}.tar.gz"
tar czf "$TARBALL" -C "$(dirname "$DIST_DIR")" --transform "s/^$(basename "$DIST_DIR")/${DIST_NAME}/" "$(basename "$DIST_DIR")"
echo "   Created $TARBALL ($(du -sh "$TARBALL" | cut -f1)) — self-contained (vendor/ included)"

# ─── 8. Docker build (optional) ──────────────────────────────────────────────

if [ "$DOCKER" = true ]; then
  if command -v docker &>/dev/null; then
    echo "==> Building Docker image..."
    docker build -t "searxngmcp:${VERSION}" .
    echo "   Docker image: searxngmcp:${VERSION} ($(docker image inspect searxngmcp:${VERSION} --format '{{.Size}}' | awk '{printf "%.1f MB\n", $1/1048576}'))"
  else
    echo "   Docker not found — skipping image build"
  fi
fi

# ─── Summary ──────────────────────────────────────────────────────────────────

echo ""
echo "═══════════════════════════════════════════════════════════════"
echo "  Release build complete: searxngmcp ${VERSION}"
echo "═══════════════════════════════════════════════════════════════"
echo ""
echo "  Binary:       ./searxngmcp ($(du -h searxngmcp | cut -f1))"
echo "  Tarball:      ./${TARBALL} ($(du -sh "$TARBALL" | cut -f1))"
echo "  Dependencies: vendor/ ($(du -sh vendor/ | cut -f1))"
echo ""
echo "  To install as systemd service:"
echo "    sudo ./install_service.sh"
echo ""
echo "  To run directly:"
echo "    ./run.sh"
echo ""
echo "  To run with Docker:"
echo "    docker compose up -d                        # existing SearXNG"
echo "    docker compose --profile searxng up -d   # bundled SearXNG"
echo ""