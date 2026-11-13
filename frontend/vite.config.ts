import path from "node:path";
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import wails from "@wailsio/runtime/plugins/vite";

// https://vitejs.dev/config/
export default defineConfig({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    css: false,
    // Los tests que recorren el redactor o un diálogo entero rondan los 2-3 s:
    // con la máquina cargada (o en CI) el tope de 5 s los cortaba de a ratos.
    testTimeout: 15_000,
  },
  plugins: [react(), tailwindcss(), wails("./bindings")],
});
