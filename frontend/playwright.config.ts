import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "list",
  outputDir: "test-results/playwright",
  use: {
	baseURL: process.env.E2E_BASE_URL || "http://localhost:5173",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
	video: "off",
  },
  projects: [
    {
      name: "chromium",
	  use: {
		...devices["Desktop Chrome"],
		channel: process.env.E2E_BROWSER_CHANNEL as "msedge" | "chrome" | undefined,
	  },
    },
  ],
});
