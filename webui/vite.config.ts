import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// The build output (dist/) is embedded into the Go binary via embed.FS
// - dev: vite dev on :43111 proxies to Go :43110
// - preview/e2e: vite preview on :5173 (spec: browser at 5173 reusing oc 33 rows)
// In dev, `vite` serves on :43111 and proxies /api to the
// Go web server (`pi-switch webui start`, default :43110).
export default defineConfig({
  plugins: [react(), tailwindcss()],
  base: "/",
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
  server: {
    port: 43111,
    strictPort: true,
    proxy: {
      "/api": {
        target: "http://127.0.0.1:43110",
        changeOrigin: true,
      },
    },
  },
  preview: {
    port: 5173,
    strictPort: true,
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test-setup.ts"],
    exclude: ["e2e/**", "**/node_modules/**", "**/dist/**"],
    env: {
      // Fixed timezone so DST-aware window tests are deterministic.
      TZ: "America/New_York",
    },
  },
});
