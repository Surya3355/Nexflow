/// <reference types="vitest/config" />
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

// The app talks to the Go API under /api. In development the Vite server
// forwards /api to the Go server on port 8080.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: { "/api": "http://localhost:8080" },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["src/test/setup.ts"],
    globals: false,
    include: ["src/**/*.test.{ts,tsx}"],
  },
});
