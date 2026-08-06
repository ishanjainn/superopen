import nextConfig from "eslint-config-next";

/** @type {import("eslint").Linter.Config[]} */
const eslintConfig = [
  {
    ignores: [
      ".next/**",
      "node_modules/**",
      "out/**",
      "coverage/**",
      "next-env.d.ts",
      "vitest.config.mts",
    ],
  },
  ...nextConfig,
  {
    // React Compiler-oriented hooks rules are noisy on the existing map/UI code.
    // Keep them as warnings so CI stays green while we tighten over time.
    rules: {
      "react-hooks/refs": "warn",
      "react-hooks/set-state-in-effect": "warn",
      "react-hooks/immutability": "warn",
      "react-hooks/preserve-manual-memoization": "warn",
    },
  },
];

export default eslintConfig;
