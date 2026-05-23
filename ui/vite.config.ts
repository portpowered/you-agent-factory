import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react-swc";
import { defineConfig } from "vite";
import monacoEditorPluginModule from "vite-plugin-monaco-editor";
import { coverageConfigDefaults } from "vitest/config";

const apiOrigin = process.env.AGENT_FACTORY_API_ORIGIN ?? "http://127.0.0.1:7437";
const monacoEditorPlugin =
  typeof monacoEditorPluginModule === "function"
    ? monacoEditorPluginModule
    : monacoEditorPluginModule.default;
const optimizedDeps = [
  "monaco-editor/esm/vs/editor/editor.api.js",
  "react",
  "react-dom",
  "react/jsx-runtime",
  "react/jsx-dev-runtime",
] as const;
const storybookInteropDeps = [
  "react",
  "react-dom",
  "react/jsx-runtime",
  "react/jsx-dev-runtime",
] as const;
const proxiedAPIPaths = [
  "/work",
  "^/factories/[^/]+/work$",
  "^/work-requests/[^/]+$",
  "^/factories/[^/]+/work-requests/[^/]+$",
  "^/work/[^/]+$",
  "^/factories/[^/]+/work/[^/]+$",
  "/events",
  "^/factories/[^/]+/events$",
  "/status",
  "^/factories/[^/]+/status$",
  "/provider-sessions/detail",
  "/factories",
  "/factory-sessions",
  "^/factory-sessions/[^/]+$",
  "/factory/~current",
  "^/factories/[^/]+/factory/~current$",
  "^/factories/[^/]+/factory/~current/editable-definition$",
  "^/factory/~current/workstations/[^/]+/prompt-template-contract$",
  "/factory/~current/editable-definition",
  "^/factory/~current/workstations/[^/]+/prompt-template-validation$",
] as const;
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
  optimizeDeps: {
    include: [...optimizedDeps],
    needsInterop: [...storybookInteropDeps],
  },
  plugins: [
    react(),
    tailwindcss(),
    monacoEditorPlugin({
      languageWorkers: ["editorWorkerService"],
    }),
  ],
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
    deps: {
      interopDefault: true,
    },
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/testing/vitest.setup.ts"],
    testTimeout: 15000,
    coverage: {
      provider: "v8",
      exclude: [
        ...coverageConfigDefaults.exclude,
        "**/*.jsonl",
        "scripts/**",
        "src/styles.css",
        "**/index.ts",
      ],
      thresholds: {
        statements: 93.1,
        branches: 80.4,
        functions: 94.9,
        lines: 93.1,
      },
    },
  },
});
