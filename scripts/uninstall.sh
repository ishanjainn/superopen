#!/usr/bin/env sh
# Superopen CLI uninstall for macOS + Linux.
#
# Runs the same `so uninstall` production users run. Locates the binary
# at the release-installer prefix first (~/.superopen/bin/so), then PATH.
#
# Usage:
#   sh scripts/uninstall.sh
#   sh scripts/uninstall.sh --keep-data
#
# Homebrew users can also run: so uninstall && brew uninstall so

set -eu

SUPEROPEN_INSTALL_DIR=${SUPEROPEN_INSTALL_DIR:-"$HOME/.superopen/bin"}

info()  { printf 'so: %s\n'        "$*"; }
fatal() { printf 'so: error: %s\n' "$*" >&2; exit 1; }

bin=""
if [ -x "$SUPEROPEN_INSTALL_DIR/so" ]; then
	bin="$SUPEROPEN_INSTALL_DIR/so"
elif command -v so >/dev/null 2>&1; then
	bin=$(command -v so)
else
	fatal "so not found in $SUPEROPEN_INSTALL_DIR or PATH"
fi

info "Running: $bin uninstall $*"
"$bin" uninstall "$@"
info "Restart your coding agent so it drops in-memory hooks."
