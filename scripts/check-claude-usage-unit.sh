#!/usr/bin/env bash
# Validate the actual deployment template with systemd without installing or starting it.
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
CHECK_DIR="$(mktemp -d)"
trap 'rm -rf "$CHECK_DIR"' EXIT

sed \
  -e 's|@PROJECT_DIR@|/tmp|g' \
  -e 's|@EXECUTABLE@|/bin/true|g' \
  -e 's|@HOME_DIR@|/tmp|g' \
  -e 's|@PROXY_URL@|http://127.0.0.1:7890|g' \
  -e 's|@LISTEN_ADDR@|127.0.0.1:55667|g' \
  -e 's|@CREDENTIALS_FILE@|/tmp/.credentials.json|g' \
  "$SCRIPT_DIR/../deploy/claude-usage.service.in" > "$CHECK_DIR/claude-usage.service"

systemd-analyze --user verify "$CHECK_DIR/claude-usage.service"
