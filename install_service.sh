#!/usr/bin/env bash
set -euo pipefail

# searxngmcp — systemd service installer
#
# Usage:
#   sudo ./install_service.sh              # build + install + enable + start
#   sudo ./install_service.sh --no-start    # install but do not enable/start
#   sudo ./install_service.sh --force      # overwrite existing config.json
#
# Run without arguments: builds binary, installs to /usr/local/bin,
# creates /etc/searxngmcp/config.json (if missing), installs systemd unit,
# and enables/starts the service.

NO_START=false
FORCE=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-start) NO_START=true; shift ;;
    --force)    FORCE=true; shift ;;
    --help|-h)
      echo "Usage: sudo $0 [--no-start] [--force]"
      echo ""
      echo "  --no-start    Install files but do not enable/start the service"
      echo "  --force       Overwrite existing /etc/searxngmcp/config.json"
      exit 0
      ;;
    *) echo "Unknown: $1"; exit 1 ;;
  esac
done

if [ "$(id -u)" -ne 0 ]; then
  echo "Error: this script must be run as root (sudo)." >&2
  exit 1
fi

DIR="$(cd "$(dirname "$0")" && pwd)"
BINARY="${DIR}/searxngmcp"

# ---------- build ----------
echo "==> Building searxngmcp..."
if [ ! -f "${DIR}/go.mod" ]; then
  echo "no go.mod found — running go mod init" >&2
  (cd "$DIR" && go mod init searxngmcp)
fi
(cd "$DIR" && CGO_ENABLED=0 go build -ldflags="-s -w" -o searxngmcp .)
echo "     Binary: $BINARY ($(du -h "$BINARY" | cut -f1))"

# ---------- install binary ----------
echo "==> Installing binary to /usr/local/bin/searxngmcp..."
install -m 755 "$BINARY" /usr/local/bin/searxngmcp

# ---------- config ----------
echo "==> Setting up /etc/searxngmcp/config.json..."
mkdir -p /etc/searxngmcp
if [ -f /etc/searxngmcp/config.json ] && [ "$FORCE" = false ]; then
  echo "     /etc/searxngmcp/config.json already exists (use --force to overwrite)"
else
  install -m 644 "${DIR}/config.example.json" /etc/searxngmcp/config.json
  echo "     Written /etc/searxngmcp/config.json (edit this file to configure)"
fi

# ---------- systemd unit ----------
echo "==> Installing systemd unit..."
install -m 644 "${DIR}/searxngmcp.service" /etc/systemd/system/searxngmcp.service
systemctl daemon-reload

# ---------- enable / start ----------
if [ "$NO_START" = false ]; then
  echo "==> Enabling and starting searxngmcp.service..."
  systemctl enable searxngmcp.service
  systemctl restart searxngmcp.service
  sleep 1
  systemctl status searxngmcp.service --no-pager
  echo ""
  echo "searxngmcp installed and running."
  echo "Check logs:  journalctl -u searxngmcp -f"
  echo "Configure:   nano /etc/searxngmcp/config.json  (then systemctl restart searxngmcp)"
else
  echo "==> Service installed but not started (--no-start)."
  echo "Start it:    systemctl enable --now searxngmcp"
fi
