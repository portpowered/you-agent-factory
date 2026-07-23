import path from "node:path";
import { fileURLToPath } from "node:url";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react-swc";
import { defineConfig } from "vite";
import monacoEditorPluginModule from "vite-plugin-monaco-editor";
import { configDefaults, coverageConfigDefaults } from "vitest/config";
import { createComponentsPackageAliases } from "./packages/components/src/vite-aliases";

const apiOrigin =
  process.env.AGENT_FACTORY_API_ORIGIN ?? "http://127.0.0.1:7437";
const uiRoot = path.dirname(fileURLToPath(import.meta.url));
const packagedFactoriesPackageRoot = path.resolve(
  uiRoot,
  "../packages/packaged-factories",
);
const componentsPackageRoot = path.resolve(uiRoot, "packages/components/src");
const factoryEmulatorPackageRoot = path.resolve(
  uiRoot,
  "packages/factory-emulator/src",
);
const factoryReplayPackageRoot = path.resolve(
  uiRoot,
  "packages/factory-replay/src",
);
const factoryVisualizersPackageRoot = path.resolve(
  uiRoot,
  "packages/factory-visualizers/src",
);
const sharedReactAliases = [
  {
    find: "@you-agent-factory/factory-visualizers/styles.css",
    replacement: path.join(factoryVisualizersPackageRoot, "styles.css"),
  },
  {
    find: "@you-agent-factory/factory-visualizers",
    replacement: path.join(factoryVisualizersPackageRoot, "index.ts"),
  },
  {
    find: "@you-agent-factory/factory-emulator",
    replacement: path.join(factoryEmulatorPackageRoot, "index.ts"),
  },
  {
    find: "@you-agent-factory/factory-replay",
    replacement: path.join(factoryReplayPackageRoot, "index.ts"),
  },
  {
    find: "@testing-library/jest-dom/vitest",
    replacement: path.join(
      uiRoot,
      "node_modules/@testing-library/jest-dom/vitest",
    ),
  },
  {
    find: "@testing-library/react",
    replacement: path.join(uiRoot, "node_modules/@testing-library/react"),
  },
  {
    find: "@testing-library/user-event",
    replacement: path.join(uiRoot, "node_modules/@testing-library/user-event"),
  },
  {
    find: "@you-agent-factory/client",
    replacement: path.join(uiRoot, "packages/client/src/index.ts"),
  },
  {
    find: "@you-agent-factory/factory-replay",
    replacement: path.join(uiRoot, "packages/factory-replay/src/index.ts"),
  },
  {
    find: "react",
    replacement: path.join(uiRoot, "node_modules/react"),
  },
  {
    find: "react-dom",
    replacement: path.join(uiRoot, "node_modules/react-dom"),
  },
  {
    find: "react/jsx-runtime",
    replacement: path.join(uiRoot, "node_modules/react/jsx-runtime"),
  },
  {
    find: "react/jsx-dev-runtime",
    replacement: path.join(uiRoot, "node_modules/react/jsx-dev-runtime"),
  },
  {
    find: "@radix-ui/react-dialog",
    replacement: path.join(uiRoot, "node_modules/@radix-ui/react-dialog"),
  },
  {
    find: "@radix-ui/react-popover",
    replacement: path.join(uiRoot, "node_modules/@radix-ui/react-popover"),
  },
  {
    find: "@radix-ui/react-collapsible",
    replacement: path.join(uiRoot, "node_modules/@radix-ui/react-collapsible"),
  },
  {
    find: "@radix-ui/react-scroll-area",
    replacement: path.join(uiRoot, "node_modules/@radix-ui/react-scroll-area"),
  },
  {
    find: "@radix-ui/react-select",
    replacement: path.join(uiRoot, "node_modules/@radix-ui/react-select"),
  },
  {
    find: "@radix-ui/react-slot",
    replacement: path.join(uiRoot, "node_modules/@radix-ui/react-slot"),
  },
  {
    find: "@radix-ui/react-compose-refs",
    replacement: path.join(uiRoot, "node_modules/@radix-ui/react-compose-refs"),
  },
  {
    find: "@xyflow/react",
    replacement: path.join(uiRoot, "node_modules/@xyflow/react"),
  },
  {
    find: "@xyflow/system",
    replacement: path.join(uiRoot, "node_modules/@xyflow/system"),
  },
  {
    find: "recharts",
    replacement: path.join(uiRoot, "node_modules/recharts"),
  },
  {
    find: "react-redux",
    replacement: path.join(uiRoot, "node_modules/react-redux"),
  },
] as const;
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
      "@radix-ui/react-collapsible",
      "@radix-ui/react-dialog",
      "@radix-ui/react-popover",
      "@radix-ui/react-scroll-area",
      "@radix-ui/react-select",
      "@radix-ui/react-slot",
      "@xyflow/react",
      "react-redux",
      "recharts",
      "react",
      "react-dom",
      "react/jsx-runtime",
      "react/jsx-dev-runtime",
    ] as const)
  : ([
      "@xyflow/react",
      "@radix-ui/react-slot",
      "react-redux",
      "recharts",
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
  "^/factory-sessions/[^/]+/events$",
  "/status",
  "^/factory-sessions/[^/]+/status$",
  "^/factory-sessions/[^/]+/sync-preflight$",
  "^/factory-sessions/[^/]+/dispatches$",
  "^/factory-sessions/[^/]+/dispatches/[^/]+$",
  "^/factory-sessions/[^/]+/artifacts$",
  "^/factory-sessions/[^/]+/artifacts/[^/]+$",
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
            customDistPath: (root, buildOutDir) =>
              path.resolve(root, buildOutDir, "monacoeditorwork"),
            languageWorkers: ["editorWorkerService"],
          }),
        ]
      : []),
  ],
  resolve: {
    alias: [
      ...sharedReactAliases,
      ...createComponentsPackageAliases(componentsPackageRoot),
    ],
    dedupe: [
      "@radix-ui/react-collapsible",
      "@radix-ui/react-compose-refs",
      "@radix-ui/react-dialog",
      "@radix-ui/react-popover",
      "@radix-ui/react-scroll-area",
      "@radix-ui/react-select",
      "@radix-ui/react-slot",
      "@xyflow/react",
      "@xyflow/system",
      "react-redux",
      "recharts",
      "react",
      "react-dom",
      "react/jsx-runtime",
      "react/jsx-dev-runtime",
    ],
  },
  server: {
    fs: {
      allow: [uiRoot, packagedFactoriesPackageRoot],
    },
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
    server: {
      deps: {
        moduleDirectories: [path.join(uiRoot, "node_modules")],
        inline: [
          "recharts",
          "@radix-ui/react-collapsible",
          "@radix-ui/react-compose-refs",
          "@radix-ui/react-dialog",
          "@radix-ui/react-popover",
          "@radix-ui/react-scroll-area",
          "@radix-ui/react-select",
          "@radix-ui/react-slot",
          "@xyflow/react",
          "@xyflow/system",
          "react",
          "react-dom",
          "react/jsx-runtime",
          "react/jsx-dev-runtime",
        ],
      },
    },
    environment: "jsdom",
    exclude: [
      ...configDefaults.exclude,
      "packages/components/src/**/*.test.ts",
      "packages/components/src/**/*.test.tsx",
      "packages/components/src/**/*.harness.test.ts",
      "packages/factory-emulator/src/**/*.test.ts",
      "packages/factory-emulator/src/**/*.test.tsx",
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
        "src/features/**/test-support/**",
        "src/styles.css",
        "**/index.ts",
        // The emulator package owns its Node test lane; dashboard coverage owns only its website adapter.
        "packages/factory-emulator/src/**",
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
