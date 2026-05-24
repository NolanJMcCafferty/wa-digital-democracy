import { defineConfig, devices } from "@playwright/test";

const WEB_PORT = Number(process.env.WADD_E2E_WEB_PORT ?? 13000);
const API_PORT = Number(process.env.WADD_E2E_API_PORT ?? 18080);
const INTERNAL_TOKEN = process.env.WADD_INTERNAL_API_TOKEN ?? "e2e-internal-token";
const API_BIN = process.env.WADD_API_BIN ?? "../../bin/wa-dd-api";
const DSN = process.env.WADD_E2E_DSN ?? "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable";

export default defineConfig({
  testDir: "./e2e",
  timeout: 30_000,
  expect: { timeout: 10_000 },
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI ? [["github"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL: `http://127.0.0.1:${WEB_PORT}`,
    trace: "on-first-retry",
  },
  projects: [
    {
      name: "chromium",
      use: {
        ...devices["Desktop Chrome"],
        channel: process.env.PLAYWRIGHT_CHROMIUM_CHANNEL,
      },
    },
  ],
  webServer: [
    {
      command: `${API_BIN} -addr 127.0.0.1:${API_PORT} -dsn '${DSN}'`,
      url: `http://127.0.0.1:${API_PORT}/healthz`,
      reuseExistingServer: !process.env.CI,
      timeout: 30_000,
      env: {
        WADD_INTERNAL_API_TOKEN: INTERNAL_TOKEN,
        WADD_E2E_ADDRESS_LOOKUP: "1",
      },
    },
    {
      command: `pnpm start --hostname 127.0.0.1 --port ${WEB_PORT}`,
      url: `http://127.0.0.1:${WEB_PORT}`,
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
      env: {
        WADD_API_URL: `http://127.0.0.1:${API_PORT}`,
        WADD_INTERNAL_API_TOKEN: INTERNAL_TOKEN,
      },
    },
  ],
});
