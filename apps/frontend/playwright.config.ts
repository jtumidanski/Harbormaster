import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  timeout: 60_000,
  use: {
    baseURL: process.env["HARBORMASTER_E2E_URL"] ?? "http://localhost:8080",
    headless: true,
    viewport: { width: 1280, height: 800 },
  },
  retries: 1,
});
