# Superopen CLI is NOT in this image. Product benches bind-mount the locally
# built `so` (Linux ELF) at /usr/local/bin/so. Image = isolated Claude/OpenCode.
FROM golang:1.26-bookworm

ARG CLAUDE_CODE_VERSION=2.1.241
ARG OPENCODE_VERSION=1.18.21
ARG TARGETARCH

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates curl git sqlite3 python3 gcc libc6-dev \
    && rm -rf /var/lib/apt/lists/*

# Node 22 for Claude Code / OpenCode CLIs (pin >= 72h old; --ignore-scripts).
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

RUN npm install -g \
        "@anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}" \
        "opencode-ai@${OPENCODE_VERSION}" \
        --ignore-scripts \
    && if [ "$TARGETARCH" = "arm64" ]; then \
        npm install -g "@anthropic-ai/claude-code-linux-arm64@${CLAUDE_CODE_VERSION}" --ignore-scripts \
        && NPM_G="$(npm root -g)" \
        && native="$NPM_G/@anthropic-ai/claude-code-linux-arm64" \
        && dest="$NPM_G/@anthropic-ai/claude-code/bin/claude.exe" \
        && if [ -x "$native/claude" ]; then \
             cp "$native/claude" "$dest" \
             && chmod 755 "$dest"; \
           fi; \
       fi \
    && NPM_G="$(npm root -g)" \
    && chmod -R a+rX "$NPM_G/@anthropic-ai" "$NPM_G/opencode-ai" || true

RUN useradd -m -u 1000 -s /bin/bash eval \
    && mkdir -p /work /eval/home /eval/claude \
    && chown -R eval:eval /work /eval

USER eval
ENV HOME=/eval/home
ENV CLAUDE_CONFIG_DIR=/eval/claude
WORKDIR /work
