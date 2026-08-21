#!/usr/bin/env sh
# Assemble the Next.js standalone UI (after `npm run build` in web/).
#
# Usage:
#   sh scripts/pack-web.sh --from web --dest "$HOME/.superopen/share/superopen/web"
#   sh scripts/pack-web.sh --from web --tar dist/so-web.tar.gz
#
# Copies `.next/static` and `public` into `.next/standalone` (Next does not
# do this itself), then installs that tree or tars it as so-web.tar.gz.

set -eu

FROM=""
DEST=""
TAR=""

fatal() { printf 'pack-web: error: %s\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
	case "$1" in
		--from)
			[ $# -ge 2 ] || fatal "missing value for --from"
			FROM=$2
			shift 2
			;;
		--dest)
			[ $# -ge 2 ] || fatal "missing value for --dest"
			DEST=$2
			shift 2
			;;
		--tar)
			[ $# -ge 2 ] || fatal "missing value for --tar"
			TAR=$2
			shift 2
			;;
		-h|--help)
			sed -n '2,9p' "$0"
			exit 0
			;;
		*)
			fatal "unknown argument: $1"
			;;
	esac
done

[ -n "$FROM" ] || fatal "required: --from <web dir>"
[ -n "$DEST" ] || [ -n "$TAR" ] || fatal "required: --dest <dir> and/or --tar <file>"

FROM=$(CDPATH= cd -- "$FROM" && pwd)
standalone="$FROM/.next/standalone"
[ -f "$standalone/server.js" ] || fatal "missing $standalone/server.js; set output: 'standalone' and run npm run build in $FROM"

if [ -d "$FROM/.next/static" ]; then
	mkdir -p "$standalone/.next/static"
	cp -R "$FROM/.next/static/." "$standalone/.next/static/"
fi
if [ -f "$FROM/.next/BUILD_ID" ]; then
	mkdir -p "$standalone/.next"
	cp "$FROM/.next/BUILD_ID" "$standalone/.next/BUILD_ID"
fi
if [ -d "$FROM/public" ]; then
	mkdir -p "$standalone/public"
	cp -R "$FROM/public/." "$standalone/public/"
fi

stage=${DEST}
if [ -z "$stage" ]; then
	stage=$(mktemp -d 2>/dev/null || mktemp -d -t so-web)
	trap 'rm -rf "$stage"' EXIT INT TERM
fi

if [ -n "$DEST" ]; then
	rm -rf "$DEST"
	mkdir -p "$DEST"
fi
# DEST may equal stage; copy standalone contents into it.
cp -R "$standalone/." "$stage/"
[ -f "$stage/server.js" ] || fatal "failed to assemble standalone UI into $stage"
# File tracing can pull unit tests into standalone; do not ship them.
find "$stage" \( -name '*.test.ts' -o -name '*.test.tsx' \) -type f -delete

if [ -n "$TAR" ]; then
	mkdir -p "$(dirname "$TAR")"
	TAR=$(CDPATH= cd -- "$(dirname "$TAR")" && pwd)/$(basename "$TAR")
	(cd "$stage" && tar -czf "$TAR" .)
	[ -f "$TAR" ] || fatal "failed to write $TAR"
fi
