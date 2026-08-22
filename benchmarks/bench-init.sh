#!/usr/bin/env bash
# Record so init wall time, optional peak RSS, and published node/edge counts.
# Does not run the Linux kernel corpus; pass a repo root explicitly.
#
# Usage:
#   benchmarks/bench-init.sh [repo-root]
#   SO_BIN=./bin/so benchmarks/bench-init.sh /path/to/grafana
set -euo pipefail

ROOT="${1:-.}"
ROOT="$(cd "$ROOT" && pwd)"
SO_BIN="${SO_BIN:-}"
NATIVE_TAGS="${NATIVE_TAGS:-tsnative,sqlite_fts5}"
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"

if [[ -z "$SO_BIN" ]]; then
	if [[ -n "${SUPEROPEN_ROOT:-}" && -f "${SUPEROPEN_ROOT}/cmd/so/main.go" ]]; then
		echo "building native so from ${SUPEROPEN_ROOT}" >&2
		CGO_ENABLED=1 go build -tags "$NATIVE_TAGS" -o /tmp/so-bench "${SUPEROPEN_ROOT}/cmd/so"
		SO_BIN=/tmp/so-bench
	elif [[ -f "${SCRIPT_DIR}/../cmd/so/main.go" ]]; then
		echo "building native so from source" >&2
		(cd "${SCRIPT_DIR}/.." && CGO_ENABLED=1 go build -tags "$NATIVE_TAGS" -o /tmp/so-bench ./cmd/so)
		SO_BIN=/tmp/so-bench
	elif command -v so >/dev/null 2>&1; then
		SO_BIN="$(command -v so)"
	else
		echo "so binary not found; set SO_BIN or run from the Superopen checkout" >&2
		exit 1
	fi
fi

echo "repo=${ROOT}" >&2
echo "so=${SO_BIN}" >&2

START=$(date +%s)
set +e
"$SO_BIN" init --root "$ROOT" --force
STATUS=$?
set -e
END=$(date +%s)
ELAPSED=$((END - START))
if [[ $STATUS -ne 0 ]]; then
	echo "elapsed_sec=${ELAPSED}" >&2
	echo "so init failed with status ${STATUS}" >&2
	exit "$STATUS"
fi

JSON="$("$SO_BIN" --json graph status --root "$ROOT")"
echo "$JSON"
python3 - "$JSON" "$ELAPSED" <<'PY'
import json, sys
payload = json.loads(sys.argv[1])
print(f"elapsed_sec={sys.argv[2]}")
print(f"nodes={payload.get('node_count', '')}")
print(f"edges={payload.get('edge_count', '')}")
print(f"files={payload.get('file_count', '')}")
print(f"database={payload.get('database', '')}")
print(f"state={payload.get('state', '')}")
PY
