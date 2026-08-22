FROM node:22-bookworm-slim

ARG CLAUDE_CODE_VERSION=2.1.234

RUN apt-get update \
    && apt-get install -y --no-install-recommends python3 python3-pip python3-venv ca-certificates git sqlite3 \
    && rm -rf /var/lib/apt/lists/*

# graphifyy 0.9.33 is ≥72h old (eval CLI query only; extract runs on the host).
RUN pip3 install --break-system-packages --no-cache-dir "graphifyy==0.9.33"

# Pin versions at least 72h old. Lifecycle scripts stay off (npm supply-chain policy).
# The wrapper's postinstall only copies the platform binary; we copy it ourselves.
RUN npm install -g \
        "@anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}" \
        "@anthropic-ai/claude-code-linux-arm64@${CLAUDE_CODE_VERSION}" \
        --ignore-scripts \
    && native=/usr/local/lib/node_modules/@anthropic-ai/claude-code-linux-arm64 \
    && ls -la "$native" \
    && cp "$native/claude" /usr/local/lib/node_modules/@anthropic-ai/claude-code/bin/claude.exe \
    && chmod 755 /usr/local/lib/node_modules/@anthropic-ai/claude-code/bin/claude.exe \
    && chmod -R a+rX /usr/local/lib/node_modules/@anthropic-ai

USER node
WORKDIR /work
