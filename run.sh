#!/usr/bin/env bash
set -euo pipefail

# searxngmcp — standalone runner script
# Demonstrates all three config loading strategies:
#   1. Explicit --config path
#   2. Local ./config.json (auto-detected)
#   3. System /etc/searxngmcp/config.json (auto-detected)
#
# Environment variables (SEARXNGMCP_*) override any config file value.
#
# Usage:
#   ./run.sh                    # auto-detect config (local or /etc)
#   ./run.sh --config /path/to/config.json
#   ./run.sh --build            # build binary first if missing
#   SEARXNGMCP_SERVER_HOST=127.0.0.1 ./run.sh

BINARY="./searxngmcp"
CONFIG_FLAG=""
BUILD_IF_MISSING=false

# Parse --config <path> so this script passes it through
while [[ $# -gt 0 ]]; do
  case "$1" in
    --config)
      CONFIG_FLAG="--config $2"
      shift 2
      ;;
    --build)
      BUILD_IF_MISSING=true
      shift
      ;;
    --help|-h)
      echo "Usage: $0 [--config /path/to/config.json] [--build]"
      echo ""
      echo "Config search order:"
      echo "  1. --config flag (explicit, if given)"
      echo "  2. ./config.json (local working directory)"
      echo "  3. /etc/searxngmcp/config.json (system-wide)"
      echo ""
      echo "  --build    Build binary first if missing (requires Go)"
      echo ""
      echo "All values overridable via SEARXNGMCP_* env vars."
      exit 0
      ;;
    *)
      echo "Unknown argument: $1"
      echo "Usage: $0 [--config /path/to/config.json] [--build]"
      exit 1
      ;;
  esac
done

# Check binary exists
if [ ! -x "$BINARY" ]; then
  if [ "$BUILD_IF_MISSING" = true ]; then
    echo "Binary not found — building..."
    if ! command -v go &>/dev/null; then
      echo "Error: Go not installed. Install Go 1.23+ or build manually." >&2
      exit 1
    fi
    CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BINARY" .
  else
    echo "Error: $BINARY not found. Run './release.sh' or 'go build -o searxngmcp .' first." >&2
    echo "       (or use --build flag to build now)" >&2
    exit 1
  fi
fi

# Show what we are doing
if [ -n "$CONFIG_FLAG" ]; then
  echo "Starting searxngmcp with explicit config: $CONFIG_FLAG"
elif [ -f "./config.json" ]; then
  echo "Starting searxngmcp using ./config.json"
elif [ -f "/etc/searxngmcp/config.json" ]; then
  echo "Starting searxngmcp using /etc/searxngmcp/config.json"
else
  echo "Starting searxngmcp with defaults (no config file found)"
fi

exec $BINARY $CONFIG_FLAG
