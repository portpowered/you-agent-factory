import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react-swc";
import { defineConfig } from "vite";

const apiOrigin = process.env.AGENT_FACTORY_API_ORIGIN ?? "http://127.0.0.1:7437";
const proxiedAPIPaths = ["/events", "/factory", "/work"] as const;
const apiProxy = Object.fromEntries(
  proxiedAPIPaths.map((path) => [
    path,
    {
      target: apiOrigin,
      changeOrigin: true,
    },
  ]),
);

export default defineConfig({
  base: "/dashboard/ui/",
  build: {
    rollupOptions: {
      output: {
        assetFileNames: "assets/[name][extname]",
        chunkFileNames: "assets/[name].js",
        entryFileNames: "assets/[name].js",
      },
    },
  },
  esbuild: {
    jsxDev: false,
  },
  plugins: [react(), tailwindcss()],
  server: {
    host: true,
    port: 4173,
    proxy: apiProxy,
  },
  preview: {
    host: "127.0.0.1",
    port: 4173,
    proxy: apiProxy,
    strictPort: true,
  },
  test: {
    environment: "jsdom",
    globals: true,
    testTimeout: 15000,
    coverage: {
      provider: "v8",
      thresholds: {
        statements: 88.17,
        branches: 76.91,
        functions: 92.58,
        lines: 88,
      },
    },
  },
});
