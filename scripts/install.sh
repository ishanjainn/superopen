#!/usr/bin/env sh
# Superopen CLI (`so`) installer for macOS + Linux.
#
# Release (no checkout): downloads the latest GitHub Release tarball into
# $HOME/.superopen/bin/so. Then run: so install
#
# Local checkout: builds from source into the same prefix and runs
# `so install` (same layout as production curl users).
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/ishanjainn/superopen/main/scripts/install.sh | sh
#   sh scripts/install.sh    # from a git checkout
#
# Environment overrides:
#   SUPEROPEN_INSTALL_DIR  Target install directory.
#                          Default: $HOME/.superopen/bin
#   SUPEROPEN_VERSION      Release tag WITHOUT the `cli-` prefix, e.g.
#                          `1.2.0`. Default: `latest`.
#   SUPEROPEN_REPO         GitHub owner/repo. Default: ishanjainn/superopen
#
# Exit codes:
#   0  Installed (or already present).
#   1  Unsupported OS/arch, network failure, or missing curl/tar.

set -eu

SUPEROPEN_REPO=${SUPEROPEN_REPO:-ishanjainn/superopen}
SUPEROPEN_INSTALL_DIR=${SUPEROPEN_INSTALL_DIR:-"$HOME/.superopen/bin"}
SUPEROPEN_VERSION=${SUPEROPEN_VERSION:-latest}

info()  { printf 'so: %s\n'        "$*"; }
warn()  { printf 'so: %s\n'        "$*" >&2; }
fatal() { printf 'so: error: %s\n' "$*" >&2; exit 1; }

need() {
	command -v "$1" >/dev/null 2>&1 || fatal "missing required command: $1"
}

path_hint() {
	persist_path
}

# Persist ~/.superopen/bin on PATH for new terminals (curl + local). The
# running `sh scripts/install.sh` process cannot change the parent shell.
persist_path() {
	dir_expr='$HOME/.superopen/bin'
	marker='.superopen/bin'
	if [ "$SUPEROPEN_INSTALL_DIR" != "$HOME/.superopen/bin" ]; then
		dir_expr=$SUPEROPEN_INSTALL_DIR
		marker=$SUPEROPEN_INSTALL_DIR
	fi
	for name in .zprofile .zshrc .bash_profile .bashrc; do
		file="$HOME/$name"
		if [ -f "$file" ] && grep -F "$marker" "$file" >/dev/null 2>&1; then
			continue
		fi
		if [ ! -f "$file" ]; then
			umask 022
			: > "$file"
		fi
		printf '\n# Superopen CLI\nexport PATH="%s:$PATH"\n' "$dir_expr" >> "$file"
		info "Added $dir_expr to $file"
	done
	info "This terminal will not see so until PATH is reloaded. Run:"
	info "  export PATH=\"$SUPEROPEN_INSTALL_DIR:\$PATH\""
	info "or open a new terminal, then: so --help"
}

# Install prefix is the parent of bin/: ~/.superopen or Homebrew's Cellar prefix.
# so dev looks only here — never in the repo you ran it from.
web_dst() {
	printf '%s\n' "$(dirname "$SUPEROPEN_INSTALL_DIR")/share/superopen/web"
}

# Copy UI sources into the prefix, then npm install + production build.
# --ignore-scripts: do not run package lifecycle scripts.
stage_web_ui() {
	web_src=$1
	if [ ! -f "$web_src/package.json" ]; then
		fatal "web UI sources missing at $web_src"
	fi
	dst=$(web_dst)
	info "Installing web UI into $dst"
	rm -rf "$dst"
	mkdir -p "$(dirname "$dst")"
	if command -v rsync >/dev/null 2>&1; then
		mkdir -p "$dst"
		rsync -a --delete --exclude node_modules --exclude .next "$web_src/" "$dst/"
	else
		cp -R "$web_src" "$dst"
		rm -rf "$dst/node_modules" "$dst/.next"
	fi
	need npm
	info "npm install --ignore-scripts (web UI)"
	(cd "$dst" && npm install --ignore-scripts)
	info "npm run build (web UI)"
	(cd "$dst" && npm run build)
}

# Curl/brew users have no Superopen clone. Pull web/ from the same GitHub repo/tag.
fetch_web_sources() {
	tmpdir=$1
	if [ "$SUPEROPEN_VERSION" = "latest" ]; then
		src_url="https://github.com/${SUPEROPEN_REPO}/archive/refs/heads/main.tar.gz"
	else
		src_url="https://github.com/${SUPEROPEN_REPO}/archive/refs/tags/cli-${SUPEROPEN_VERSION}.tar.gz"
	fi
	info "Downloading web UI sources"
	if ! curl -fsSL --retry 3 --retry-delay 1 -o "$tmpdir/src.tar.gz" "$src_url"; then
		fatal "download failed: $src_url"
	fi
	mkdir -p "$tmpdir/src"
	tar -xzf "$tmpdir/src.tar.gz" -C "$tmpdir/src"
	found=$(find "$tmpdir/src" -maxdepth 2 -type d -name web -print -quit)
	if [ -z "$found" ] || [ ! -f "$found/package.json" ]; then
		fatal "no web/ in $src_url"
	fi
	printf '%s\n' "$found"
}

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" 2>/dev/null && pwd || true)
if [ -n "$SCRIPT_DIR" ] && [ -f "$SCRIPT_DIR/../cmd/so/main.go" ] && command -v go >/dev/null 2>&1; then
	info "Building so from local source into $SUPEROPEN_INSTALL_DIR (same layout as the curl installer)…"
	mkdir -p "$SUPEROPEN_INSTALL_DIR"
	(cd "$SCRIPT_DIR/.." && go build -o "$SUPEROPEN_INSTALL_DIR/so" ./cmd/so)
	chmod +x "$SUPEROPEN_INSTALL_DIR/so"
	info "Installed: $SUPEROPEN_INSTALL_DIR/so"
	stage_web_ui "$SCRIPT_DIR/../web"
	export PATH="$SUPEROPEN_INSTALL_DIR:$PATH"
	"$SUPEROPEN_INSTALL_DIR/so" install
	path_hint
	info "Done. In a test repo: so init && so dev"
	info "Wipe with: sh scripts/uninstall.sh"
	exit 0
fi

need curl
need tar
need uname

sha256_cmd=""
if command -v sha256sum >/dev/null 2>&1; then
	sha256_cmd="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
	sha256_cmd="shasum -a 256"
fi

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
web_src=$(fetch_web_sources "$tmpdir")
stage_web_ui "$web_src"
export PATH="$SUPEROPEN_INSTALL_DIR:$PATH"
"$target" install
path_hint
info "Done. In a test repo: so init && so dev"
info "Wipe with: sh scripts/uninstall.sh"
