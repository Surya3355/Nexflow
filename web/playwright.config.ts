import { defineConfig, devices } from "@playwright/test";

// Phone-width checks run against the built app served by Vite preview.
export default defineConfig({
  testDir: "./e2e",
  use: { baseURL: "http://localhost:4173" },
  webServer: {
    command: "npm run build && npx vite preview --port 4173 --strictPort",
    url: "http://localhost:4173",
    reuseExistingServer: true,
    timeout: 120_000,
  },
  projects: [
    { name: "phone", use: { ...devices["Pixel 5"], viewport: { width: 360, height: 740 } } },
  ],
});
