# Superopen CLI is NOT in this image. Product benches bind-mount the locally
# built `so` (Linux ELF) at /usr/local/bin/so. Image = isolated Claude/OpenCode/Cursor.
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

# Cursor CLI. Same tarball the official install script downloads; linked onto
# the container PATH because arm containers run as the host uid.
ARG CURSOR_CLI_VERSION=2026.10.22-27dcf5f
RUN arch="$(dpkg --print-architecture)" \
    && case "$arch" in \
         arm64) arch=arm64 ;; \
         amd64) arch=x64 ;; \
         *) echo "unsupported arch $arch" >&2; exit 1 ;; \
       esac \
    && curl -fSL "https://downloads.cursor.com/lab/${CURSOR_CLI_VERSION}/linux/${arch}/agent-cli-package.tar.gz" \
         -o /tmp/cursor-agent.tar.gz \
    && mkdir -p /opt/cursor-agent \
    && tar --strip-components=1 -xzf /tmp/cursor-agent.tar.gz -C /opt/cursor-agent \
    && ln -sf /opt/cursor-agent/cursor-agent /usr/local/bin/agent \
    && ln -sf /opt/cursor-agent/cursor-agent /usr/local/bin/cursor-agent \
    && chmod -R a+rX /opt/cursor-agent \
    && rm /tmp/cursor-agent.tar.gz

RUN useradd -m -u 1000 -s /bin/bash eval \
    && mkdir -p /work /eval/home /eval/claude \
    && chown -R eval:eval /work /eval

USER eval
ENV HOME=/eval/home
ENV CLAUDE_CONFIG_DIR=/eval/claude
WORKDIR /work
