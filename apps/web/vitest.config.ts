import { defineConfig } from "vitest/config";
import path from "path";

// Vitest is scoped to PURE LOGIC: helpers, parsers, formatters, reducers,
// data shaping. Anything that renders React or touches the DOM belongs
// in Playwright (apps/web/e2e). This keeps the unit suite fast, dependency-
// light, and free of overlap with the e2e coverage. See AGENTS.md.
export default defineConfig({
  test: {
    environment: "node",
    include: ["src/**/*.{test,spec}.ts"],
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "src"),
    },
  },
});
