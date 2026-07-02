import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react-swc";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";
import { createComponentsPackageAliases } from "./packages/components/src/vite-aliases";
import monacoEditorPluginModule from "vite-plugin-monaco-editor";
import { coverageConfigDefaults } from "vitest/config";

const apiOrigin =
  process.env.AGENT_FACTORY_API_ORIGIN ?? "http://127.0.0.1:7437";
const componentsPackageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "packages/components/src",
);
const isCoverageRun = process.argv.includes("--coverage");
const profileSourceMaps =
  process.env.AGENT_FACTORY_PROFILE_SOURCEMAPS === "true" ||
  process.env.AGENT_FACTORY_PROFILE_SOURCEMAPS === "1";
const isVitestRun =
  process.argv.includes("vitest") || process.env.VITEST === "true";
const monacoEditorPlugin =
  typeof monacoEditorPluginModule === "function"
    ? monacoEditorPluginModule
    : monacoEditorPluginModule.default;
const optimizedDeps = isVitestRun
  ? ([
      "@radix-ui/react-slot",
      "react",
      "react-dom",
      "react/jsx-runtime",
      "react/jsx-dev-runtime",
    ] as const)
  : ([
      "@radix-ui/react-slot",
      "monaco-editor/esm/vs/editor/editor.api.js",
      "react",
      "react-dom",
      "react/jsx-runtime",
      "react/jsx-dev-runtime",
    ] as const);
const storybookInteropDeps = [
  "react",
  "react-dom",
  "react/jsx-runtime",
  "react/jsx-dev-runtime",
] as const;
const currentFactoryPromptTemplateProxyPaths = [
  "^/factory-sessions/[^/]+/factory/workstations/[^/]+/prompt-template-contract$",
  "^/factory-sessions/[^/]+/factory/workstations/[^/]+/prompt-template-validation$",
] as const;
const proxiedAPIPaths = [
  "/work",
  "^/factory-sessions/[^/]+/work$",
  "^/factory-sessions/[^/]+/invocations$",
  "^/work-requests/[^/]+$",
  "^/factory-sessions/[^/]+/work-requests/[^/]+$",
  "^/work/[^/]+$",
  "^/factory-sessions/[^/]+/work/[^/]+$",
  // Compatibility-only: retain process-global /events proxying for legacy tooling.
  "/events",
  "^/factory-sessions/[^/]+/events$",
  "/status",
  "^/factory-sessions/[^/]+/status$",
  "/provider-sessions/detail",
  "/factories",
  "/factory-sessions",
  "^/factory-sessions/[^/]+$",
  "/factory-sessions/~default/factory",
  "^/factory-sessions/[^/]+/factory$",
  ...currentFactoryPromptTemplateProxyPaths,
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
    sourcemap: profileSourceMaps,
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
    ...(!isVitestRun ? [tailwindcss()] : []),
    ...(!isVitestRun
      ? [
          monacoEditorPlugin({
            languageWorkers: ["editorWorkerService"],
          }),
        ]
      : []),
  ],
  resolve: {
    alias: createComponentsPackageAliases(componentsPackageRoot),
  },
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
    exclude: [
      "packages/components/src/**/*.test.ts",
      "packages/components/src/**/*.test.tsx",
      "packages/components/src/**/*.harness.test.ts",
    ],
    globals: true,
    setupFiles: ["./src/testing/vitest.setup.ts"],
    testTimeout: isCoverageRun ? 180000 : 30000,
    coverage: {
      provider: "v8",
      exclude: [
        ...coverageConfigDefaults.exclude,
        "src/api/generated/**",
        "**/*.jsonl",
        "src/api/generated/**",
        "scripts/**",
        "src/testing/app-shell-test-graph-layout.ts",
        "src/testing/app-shell-work-outcome-stub.tsx",
        "src/testing/app-shell-workflow-activity-stub.tsx",
        "src/testing/guarded-suite-console.setup.ts",
        "src/testing/replay-harness.ts",
        "src/styles.css",
        "**/index.ts",
        // Browser integration harness code is validated in the browser lane, not jsdom coverage.
        "integration/**",
      ],
      thresholds: {
        // Mergeability: sharded mergeReports on ubuntu-latest measured ~92.97% lines (PR #820 run 27600146245).
        statements: 92.97,
        branches: 80.4,
        functions: 94.9,
        lines: 92.97,
      },
    },
  },
});
