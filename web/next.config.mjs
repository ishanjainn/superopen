/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  transpilePackages: ["three"],
  // Release tarball (`so-web.tar.gz`) is `.next/standalone` plus static/public.
  // Curl/brew users extract that; they do not run `next build`.
  output: "standalone",
  outputFileTracingExcludes: {
    "*": ["**/*.test.ts", "**/*.test.tsx"],
  },
  // UI is fully Next.js - no Go proxy. Data comes from `.so/` via route handlers.
  // Next.js 16 can auto-write AGENTS.md/CLAUDE.md with framework tips.
  // Disabled here so it does not collide with Superopen's own AGENTS.md /
  // CLAUDE.md injectors. This does NOT change Superopen harness behavior
  // (.so/, /so skills, SessionStart memory, hooks, etc.).
  agentRules: false,
  // Next 16: Turbopack is the default for `next dev` / `next build`.
  turbopack: {
    // Intentional dynamic reads of the local `.so/` workspace (not bundlable).
    ignoreIssue: [
      { path: "**/src/lib/so/nodeio.ts", title: /TP100[46]/ },
      { path: "**/src/lib/so/**", title: /TP100[46]/ },
      { path: "**/src/lib/so/**", description: /very dynamic/i },
    ],
  },
};

export default nextConfig;
