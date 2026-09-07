#!/usr/bin/env bash
# Run the Superopen harness inside the so-bench image.
# Developer HOME is never mounted. Locally built Linux `so` is bind-mounted.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
IMAGE="${SO_BENCH_IMAGE:-so-bench}"
DOCKERFILE="$ROOT/benchmarks/docker/Dockerfile"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker-run.sh needs Docker on PATH." >&2
  exit 1
fi
if ! docker info >/dev/null 2>&1; then
  echo "Docker daemon is not running." >&2
  exit 1
fi

docker build \
  --build-arg "CLAUDE_CODE_VERSION=${CLAUDE_CODE_VERSION:-2.1.241}" \
  --build-arg "OPENCODE_VERSION=${OPENCODE_VERSION:-1.18.21}" \
  -t "$IMAGE" \
  -f "$DOCKERFILE" \
  "$ROOT/benchmarks/docker"

SO_LINUX="$(python3 - <<'PY'
from pathlib import Path
import sys
sys.path.insert(0, str(Path("benchmarks")))
from docker import linux_so_path, prepare_guest_so
repo = Path(".").resolve()
host = repo / "bin" / "so"
print(prepare_guest_so(str(host) if host.is_file() else str(linux_so_path(repo)), repo))
PY
)"

AUTH="$(mktemp -d "${TMPDIR:-/tmp}/so-bench-auth.XXXXXX")"
cleanup() { rm -rf "$AUTH"; }
trap cleanup EXIT
mkdir -p "$AUTH/claude" "$AUTH/home/.claude" "$AUTH/home/.config/opencode"
# Auth files only — never mount host HOME or ~/.claude as a volume.
for src in \
  "$HOME/.claude/.credentials.json" \
  "$HOME/.claude/credentials.json" \
  "$HOME/.claude.json"
do
  if [ -f "$src" ]; then
    base="$(basename "$src")"
    cp "$src" "$AUTH/claude/$base"
    cp "$src" "$AUTH/home/.claude/$base"
    chmod 600 "$AUTH/claude/$base" "$AUTH/home/.claude/$base" || true
  fi
done
if [ -f "$HOME/.config/opencode/auth.json" ]; then
  cp "$HOME/.config/opencode/auth.json" "$AUTH/home/.config/opencode/auth.json"
  chmod 600 "$AUTH/home/.config/opencode/auth.json" || true
fi

UID_GID="$(id -u):$(id -g)"
# Isolation is this container. Do not nest docker isolate (would need the socket).
FILTERED=()
skip_next=0
for arg in "$@"; do
  if [ "$skip_next" = 1 ]; then
    skip_next=0
    continue
  fi
  case "$arg" in
    --isolate) skip_next=1 ;;
    --isolate=*) ;;
    --so-bin) skip_next=1 ;;
    --so-bin=*) ;;
    *) FILTERED+=("$arg") ;;
  esac
done

run=(
  docker run --rm
  --user "$UID_GID"
  --add-host host.docker.internal:host-gateway
  -e HOME=/eval/home
  -e CLAUDE_CONFIG_DIR=/eval/claude
  -e SUPEROPEN_SO_BIN=/usr/local/bin/so
  -e "ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY:-}"
  -e "OPENAI_API_KEY=${OPENAI_API_KEY:-}"
  -e "OPENCODE_API_KEY=${OPENCODE_API_KEY:-}"
  -e "SUPEROPEN_LOCOMO_URL=${SUPEROPEN_LOCOMO_URL:-}"
  -e "SUPEROPEN_LME_URL=${SUPEROPEN_LME_URL:-}"
  -v "$ROOT:/src"
  -v "$SO_LINUX:/usr/local/bin/so:ro"
  -v "$AUTH/home:/eval/home"
  -v "$AUTH/claude:/eval/claude"
  -w /src
  "$IMAGE"
  python3 benchmarks/run.py
  --isolate host
  --so-bin /usr/local/bin/so
)
exec "${run[@]}" ${FILTERED[@]+"${FILTERED[@]}"}
