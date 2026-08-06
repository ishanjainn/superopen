#!/usr/bin/env sh
# Superopen CLI (`so`) installer for macOS + Linux.
#
# Detects OS + architecture, downloads the matching tarball from the
# latest `cli-*.*.*` GitHub Release, and installs the binary to
# $HOME/.superopen/bin/so. Prints a PATH-add hint if that directory is
# not already on $PATH.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/superopen/so/main/scripts/install.sh | sh
#
# Environment overrides:
#   SUPEROPEN_INSTALL_DIR  Target install directory.
#                          Default: $HOME/.superopen/bin
#   SUPEROPEN_VERSION      Release tag WITHOUT the `cli-` prefix, e.g.
#                          `1.2.0`. Default: `latest`.
#   SUPEROPEN_REPO         GitHub owner/repo. Default: superopen/so
#
# Exit codes:
#   0  Installed (or already present).
#   1  Unsupported OS/arch, network failure, or missing curl/tar.

set -eu

SUPEROPEN_REPO=${SUPEROPEN_REPO:-superopen/so}
SUPEROPEN_INSTALL_DIR=${SUPEROPEN_INSTALL_DIR:-"$HOME/.superopen/bin"}
SUPEROPEN_VERSION=${SUPEROPEN_VERSION:-latest}

info()  { printf 'so: %s\n'        "$*"; }
warn()  { printf 'so: %s\n'        "$*" >&2; }
fatal() { printf 'so: error: %s\n' "$*" >&2; exit 1; }

need() {
	command -v "$1" >/dev/null 2>&1 || fatal "missing required command: $1"
}
need curl
need tar
need uname

sha256_cmd=""
if command -v sha256sum >/dev/null 2>&1; then
	sha256_cmd="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
	sha256_cmd="shasum -a 256"
fi

# --- Detect OS + arch -------------------------------------------------------

uname_os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$uname_os" in
	darwin) os=darwin ;;
	linux)  os=linux ;;
	*) fatal "unsupported OS: $uname_os (this installer supports macOS + Linux; on Windows use install.ps1)" ;;
esac

uname_arch=$(uname -m)
case "$uname_arch" in
	x86_64|amd64)        arch=amd64 ;;
	aarch64|arm64)       arch=arm64 ;;
	*) fatal "unsupported architecture: $uname_arch" ;;
esac

# --- Local source fallback (developer checkout) -----------------------------

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" 2>/dev/null && pwd || true)
if [ -n "$SCRIPT_DIR" ] && [ -f "$SCRIPT_DIR/../cmd/so/main.go" ] && command -v go >/dev/null 2>&1; then
	info "Building so from local source…"
	mkdir -p "$SUPEROPEN_INSTALL_DIR"
	(cd "$SCRIPT_DIR/.." && go build -o "$SUPEROPEN_INSTALL_DIR/so" ./cmd/so)
	chmod +x "$SUPEROPEN_INSTALL_DIR/so"
	info "Installed: $SUPEROPEN_INSTALL_DIR/so"
	export PATH="$SUPEROPEN_INSTALL_DIR:$PATH"
	if command -v so >/dev/null 2>&1; then
		so install || true
	fi
	info "Done. Try /so in your coding agent, then /so init"
	exit 0
fi

# --- Resolve the asset URL --------------------------------------------------

asset="so-${os}-${arch}.tar.gz"
if [ "$SUPEROPEN_VERSION" = "latest" ]; then
	url="https://github.com/${SUPEROPEN_REPO}/releases/latest/download/${asset}"
else
	url="https://github.com/${SUPEROPEN_REPO}/releases/download/cli-${SUPEROPEN_VERSION}/${asset}"
fi

info "Downloading ${asset}"

tmpdir=$(mktemp -d 2>/dev/null || mktemp -d -t so-install)
trap 'rm -rf "$tmpdir"' EXIT INT TERM

if ! curl -fsSL --retry 3 --retry-delay 1 -o "$tmpdir/$asset" "$url"; then
	fatal "download failed: $url (no release yet? build from source with Go, or set SUPEROPEN_VERSION)"
fi

sha_url="${url}.sha256"
if curl -fsSL --retry 3 --retry-delay 1 -o "$tmpdir/$asset.sha256" "$sha_url" 2>/dev/null; then
	if [ -n "$sha256_cmd" ]; then
		expected=$(awk '{print $1}' "$tmpdir/$asset.sha256")
		# shellcheck disable=SC2086
		actual=$($sha256_cmd "$tmpdir/$asset" | awk '{print $1}')
		if [ -z "$expected" ] || [ "$expected" != "$actual" ]; then
			fatal "checksum mismatch for ${asset} - expected ${expected:-<empty>}, got ${actual}. Refusing to install."
		fi
		info "Verified sha256 ${actual}"
	else
		warn "no sha256/shasum command found - skipping checksum verification"
	fi
else
	warn "sha256 sidecar not available at ${sha_url}; skipping checksum verification"
fi

if ! tar -xzf "$tmpdir/$asset" -C "$tmpdir"; then
	fatal "extract failed; archive may be corrupt"
fi

extracted=$(find "$tmpdir" -maxdepth 2 -type f -name 'so*' ! -name '*.tar.gz' ! -name '*.sha256' -print -quit)
if [ -z "$extracted" ]; then
	fatal "no so binary found inside ${asset}"
fi

mkdir -p "$SUPEROPEN_INSTALL_DIR"
target="$SUPEROPEN_INSTALL_DIR/so"
mv "$extracted" "$target"
chmod +x "$target"

info "Installed: $target"

case ":$PATH:" in
	*":$SUPEROPEN_INSTALL_DIR:"*) ;;
	*)
		warn ""
		warn "Add the install directory to your PATH (one of):"
		warn "  echo 'export PATH=\"$SUPEROPEN_INSTALL_DIR:\$PATH\"' >> ~/.zshrc   # zsh"
		warn "  echo 'export PATH=\"$SUPEROPEN_INSTALL_DIR:\$PATH\"' >> ~/.bashrc  # bash"
		warn "Then reload your shell or 'source' the file."
		;;
esac

info ""
info "Next (use absolute path if PATH is not updated yet):"
info "  $target install"
info "  $target init     # in a repo"
info ""
info "Or: brew install superopen/so/so   # when the Homebrew tap is published"
