#!/usr/bin/env sh
set -eu

if [ "$#" -lt 3 ]; then
  echo "usage: scripts/release/smoke-install.sh <install-script-url> <version> <install-dir> [binary-name]" >&2
  exit 1
fi

INSTALL_SCRIPT_URL="$1"
INSTALL_VERSION="$2"
INSTALL_DIR="$3"
BINARY_NAME="${4:-agent-factory}"
TEMP_HOME="$(mktemp -d)"
trap 'rm -rf "$TEMP_HOME"' EXIT

mkdir -p "$INSTALL_DIR"

curl -fsSL "$INSTALL_SCRIPT_URL" | env \
  HOME="$TEMP_HOME" \
  AGENT_FACTORY_VERSION="$INSTALL_VERSION" \
  AGENT_FACTORY_INSTALL_DIR="$INSTALL_DIR" \
  AGENT_FACTORY_INSTALL_BASE_URL="${AGENT_FACTORY_INSTALL_BASE_URL:-}" \
  AGENT_FACTORY_INSTALL_OS="${AGENT_FACTORY_INSTALL_OS:-}" \
  AGENT_FACTORY_INSTALL_ARCH="${AGENT_FACTORY_INSTALL_ARCH:-}" \
  sh

BINARY_PATH="$INSTALL_DIR/$BINARY_NAME"
if [ ! -x "$BINARY_PATH" ]; then
  echo "installed binary missing or not executable: $BINARY_PATH" >&2
  exit 1
fi

"$BINARY_PATH" --help >/dev/null

printf 'hosted install smoke passed for %s via %s\n' "$BINARY_PATH" "$INSTALL_SCRIPT_URL"
