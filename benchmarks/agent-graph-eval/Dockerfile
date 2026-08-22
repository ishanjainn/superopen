FROM node:22-bookworm-slim

ARG CLAUDE_CODE_VERSION=2.1.234

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates git sqlite3 \
    && rm -rf /var/lib/apt/lists/*

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
