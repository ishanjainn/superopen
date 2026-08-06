import { defineConfig } from "vitest/config";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL(".", import.meta.url));

export default defineConfig({
  test: {
    environment: "node",
    include: ["src/**/*.test.ts"],
    reporters: ["default"],
  },
  resolve: {
    alias: {
      "@": resolve(root, "./src"),
    },
  },
});
