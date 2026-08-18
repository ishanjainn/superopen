#!/usr/bin/env bash
# Assembles embedded marketplace under internal/agent/install/marketplace/
# from .claude-plugin/marketplace.json + plugins/<vendor>/
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
SRC_MARKETPLACE="${REPO_ROOT}/.claude-plugin"
SRC_PLUGINS="${REPO_ROOT}/plugins"
DEST="${REPO_ROOT}/internal/agent/install/marketplace"

if [ ! -f "${SRC_MARKETPLACE}/marketplace.json" ]; then
  echo "sync-plugins: missing ${SRC_MARKETPLACE}/marketplace.json" >&2
  exit 1
fi
rm -rf "${DEST}/.claude-plugin" "${DEST}/plugins"
mkdir -p "${DEST}/.claude-plugin" "${DEST}/plugins"
cp "${SRC_MARKETPLACE}/marketplace.json" "${DEST}/.claude-plugin/marketplace.json"
for vendor_dir in "${SRC_PLUGINS}"/*/; do
  vendor=$(basename "${vendor_dir}")
  rm -rf "${DEST}/plugins/${vendor}"
  cp -R "${vendor_dir}" "${DEST}/plugins/${vendor}"
done
echo "sync-plugins: wrote ${DEST}"
