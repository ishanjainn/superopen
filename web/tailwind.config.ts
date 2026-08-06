import type { Config } from "tailwindcss";

/** RGB channels only - pair with `/ <alpha-value>` for opacity modifiers. */
function scale(prefix: string) {
  const steps = [50, 100, 200, 300, 400, 500, 600, 700, 800, 900, 950] as const;
  return Object.fromEntries(
    steps.map((step) => [
      step,
      `rgb(var(--color-${prefix}-${step}) / <alpha-value>)`,
    ])
  );
}

const config: Config = {
  darkMode: "class",
  content: ["./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        white: "rgb(var(--color-white) / <alpha-value>)",
        black: "rgb(var(--color-black) / <alpha-value>)",
        neutral: scale("neutral"),
        stone: scale("stone"),
      },
    },
  },
  plugins: [],
};

export default config;
