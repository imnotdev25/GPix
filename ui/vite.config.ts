import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The build output lands in pkg/web/dist, which the Go binary embeds
// (see pkg/web/spa.go). During `npm run dev`, API calls are proxied to a
// locally running gpix so the SPA can be developed with hot reload.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "../pkg/web/dist",
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      "/api": { target: "http://localhost:8080", changeOrigin: true },
      "/thumb": { target: "http://localhost:8080", changeOrigin: true },
      "/raw": { target: "http://localhost:8080", changeOrigin: true },
      "/stream": { target: "http://localhost:8080", changeOrigin: true },
      "/auth": { target: "http://localhost:8080", changeOrigin: true },
    },
  },
});
