#!/usr/bin/env sh
# Superopen CLI (`so`) installer for macOS + Linux.
#
# Release (no checkout): downloads the latest GitHub Release CLI tarball and
# the prebuilt UI bundle (so-web.tar.gz) into $HOME/.superopen. Then: so install
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

if [ -t 1 ]; then
	BOLD=$(printf '\033[1m')
	GREEN=$(printf '\033[32m')
	DIM=$(printf '\033[2m')
	RESET=$(printf '\033[0m')
else
	BOLD=''
	GREEN=''
	DIM=''
	RESET=''
fi

SPIN_PID=
_spin_unicode=0
case "${LC_ALL:-${LC_CTYPE:-${LANG:-}}}" in
	*UTF-8*|*utf8*|*UTF8*) _spin_unicode=1 ;;
esac
if sleep 0.1 2>/dev/null; then
	_spin_delay=0.1
else
	_spin_delay=1
fi

_spin_stop() {
	if [ -z "${SPIN_PID:-}" ]; then
		return 0
	fi
	kill "$SPIN_PID" 2>/dev/null || true
	wait "$SPIN_PID" 2>/dev/null || true
	SPIN_PID=
	if [ -t 1 ]; then
		printf '\r\033[2K'
	fi
}

_spin_start() {
	_spin_stop
	if [ ! -t 1 ]; then
		printf '  …  %s\n' "$1"
		return 0
	fi
	_spin_msg=$1
	(
		i=0
		while :; do
			i=$(( (i + 1) % 10 ))
			if [ "$_spin_unicode" -eq 1 ]; then
				case $i in
					0) c='⠋' ;;
					1) c='⠙' ;;
					2) c='⠹' ;;
					3) c='⠸' ;;
					4) c='⠼' ;;
					5) c='⠴' ;;
					6) c='⠦' ;;
					7) c='⠧' ;;
					8) c='⠇' ;;
					9) c='⠏' ;;
				esac
			else
				case $i in
					0|4|8) c='|' ;;
					1|5|9) c='/' ;;
					2|6) c='-' ;;
					*) c='\' ;;
				esac
			fi
			printf '\r  %s  %s' "$c" "$_spin_msg"
			sleep "$_spin_delay"
		done
	) &
	SPIN_PID=$!
}

info()  { printf 'so: %s\n'        "$*"; }
warn()  { printf 'so: %s\n'        "$*" >&2; }
fatal() { _spin_stop; printf 'so: error: %s\n' "$*" >&2; exit 1; }
banner() {
	printf '\n%s\n\n' "$BOLD"
	printf '  ____  _   _ ____  _____ ____   ___  ____  _____ _   _ \n'
	printf ' / ___|| | | |  _ \\| ____|  _ \\ / _ \\|  _ \\| ____| \\ | |\n'
	printf ' \\___ \\| | | | |_) |  _| | |_) | | | | |_) |  _| |  \\| |\n'
	printf '  ___) | |_| |  __/| |___|  _ <| |_| |  __/| |___| |\\  |\n'
	printf ' |____/ \\___/|_|   |_____|_| \\_\\\\___/|_|   |_____|_| \\_|\n'
	printf '%s\n' "$RESET"
}
step() { _spin_start "$*"; }
ok()   { _spin_stop; printf '  %s✓%s  %s\n' "$GREEN" "$RESET" "$*"; }

need() {
	command -v "$1" >/dev/null 2>&1 || fatal "missing required command: $1"
}

path_hint() {
	persist_path
}

# Persist the install dir on PATH for new terminals (curl + local). The
# running `sh scripts/install.sh` process cannot change the parent shell.
persist_path() {
	dir_expr='$HOME/.superopen/bin'
	marker='.superopen/bin'
	if [ "$SUPEROPEN_INSTALL_DIR" != "$HOME/.superopen/bin" ]; then
		dir_expr=$SUPEROPEN_INSTALL_DIR
		marker=$SUPEROPEN_INSTALL_DIR
	fi
	for name in .zprofile .zshrc .bash_profile .bashrc .profile; do
		file="$HOME/$name"
		if [ -f "$file" ] && grep -F "$marker" "$file" >/dev/null 2>&1; then
			continue
		fi
		if [ ! -f "$file" ]; then
			umask 022
			: > "$file"
		fi
		printf '\n# Superopen CLI\nexport PATH="%s:$PATH"\n' "$dir_expr" >> "$file"
	done
	fish_file="$HOME/.config/fish/config.fish"
	if command -v fish >/dev/null 2>&1 || [ -f "$fish_file" ]; then
		mkdir -p "$HOME/.config/fish"
		if [ ! -f "$fish_file" ] || ! grep -F "$marker" "$fish_file" >/dev/null 2>&1; then
			if [ ! -f "$fish_file" ]; then
				umask 022
				: > "$fish_file"
			fi
			printf '\n# Superopen CLI\nfish_add_path "%s"\n' "$dir_expr" >> "$fish_file"
		fi
	fi
	ok "PATH           open a new terminal, or: export PATH=\"$SUPEROPEN_INSTALL_DIR:\$PATH\""
}

# Install prefix is the parent of bin/: ~/.superopen or Homebrew's Cellar prefix.
# so dev looks only here — never in the repo you ran it from.
web_dst() {
	printf '%s\n' "$(dirname "$SUPEROPEN_INSTALL_DIR")/share/superopen/web"
}

# Local checkout: npm build, then install the same standalone tree curl users get.
stage_web_ui_from_source() {
	web_src=$1
	if [ ! -f "$web_src/package.json" ]; then
		fatal "web UI sources missing at $web_src"
	fi
	need npm
	step "Building the UI"
	log=$(mktemp)
	if ! (cd "$web_src" && npm install --ignore-scripts && npm run build) >"$log" 2>&1; then
		cat "$log" >&2
		rm -f "$log"
		fatal "UI build failed"
	fi
	rm -f "$log"
	dst=$(web_dst)
	sh "$SCRIPT_DIR/pack-web.sh" --from "$web_src" --dest "$dst"
	ok "UI             $dst"
}

# Release asset so-web.tar.gz: Next standalone output (no npm on the user machine).
install_web_tarball() {
	archive=$1
	dst=$(web_dst)
	rm -rf "$dst"
	mkdir -p "$dst"
	if ! tar -xzf "$archive" -C "$dst"; then
		fatal "extract failed; $archive may be corrupt"
	fi
	if [ ! -f "$dst/server.js" ]; then
		fatal "so-web.tar.gz is missing server.js (not a standalone UI bundle)"
	fi
	ok "UI             $dst"
}

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" 2>/dev/null && pwd || true)
if [ -n "$SCRIPT_DIR" ] && [ -f "$SCRIPT_DIR/../cmd/so/main.go" ] && command -v go >/dev/null 2>&1; then
	banner
	step "Building the CLI"
	mkdir -p "$SUPEROPEN_INSTALL_DIR"
	log=$(mktemp)
	if ! (cd "$SCRIPT_DIR/.." && if command -v clang >/dev/null 2>&1 || command -v gcc >/dev/null 2>&1; then
		CGO_ENABLED=1 go build -tags tsnative,sqlite_fts5 -o "$SUPEROPEN_INSTALL_DIR/so" ./cmd/so
	else
		go build -o "$SUPEROPEN_INSTALL_DIR/so" ./cmd/so
	fi) >"$log" 2>&1; then
		cat "$log" >&2
		rm -f "$log"
		fatal "CLI build failed"
	fi
	rm -f "$log"
	chmod +x "$SUPEROPEN_INSTALL_DIR/so"
	ok "CLI            $SUPEROPEN_INSTALL_DIR/so"
	stage_web_ui_from_source "$SCRIPT_DIR/../web"
	export PATH="$SUPEROPEN_INSTALL_DIR:$PATH"
	SUPEROPEN_INSTALLER=1 "$SUPEROPEN_INSTALL_DIR/so" install
	path_hint
	exit 0
fi

need curl
need tar
need uname
banner

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

step "Downloading the CLI"

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

ok "CLI            $target"

web_asset="so-web.tar.gz"
if [ "$SUPEROPEN_VERSION" = "latest" ]; then
	web_url="https://github.com/${SUPEROPEN_REPO}/releases/latest/download/${web_asset}"
else
	web_url="https://github.com/${SUPEROPEN_REPO}/releases/download/cli-${SUPEROPEN_VERSION}/${web_asset}"
fi
step "Downloading the UI"
if ! curl -fsSL --retry 3 --retry-delay 1 -o "$tmpdir/$web_asset" "$web_url"; then
	fatal "download failed: $web_url (UI bundle missing from this release?)"
fi
web_sha_url="${web_url}.sha256"
if curl -fsSL --retry 3 --retry-delay 1 -o "$tmpdir/$web_asset.sha256" "$web_sha_url" 2>/dev/null; then
	if [ -n "$sha256_cmd" ]; then
		expected=$(awk '{print $1}' "$tmpdir/$web_asset.sha256")
		# shellcheck disable=SC2086
		actual=$($sha256_cmd "$tmpdir/$web_asset" | awk '{print $1}')
		if [ -z "$expected" ] || [ "$expected" != "$actual" ]; then
			fatal "checksum mismatch for ${web_asset} - expected ${expected:-<empty>}, got ${actual}. Refusing to install."
		fi
	else
		warn "no sha256/shasum command found - skipping checksum verification"
	fi
else
	warn "sha256 sidecar not available at ${web_sha_url}; skipping checksum verification"
fi
install_web_tarball "$tmpdir/$web_asset"

export PATH="$SUPEROPEN_INSTALL_DIR:$PATH"
SUPEROPEN_INSTALLER=1 "$target" install
path_hint
