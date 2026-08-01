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
const componentsPackageRoot = path.resolve(uiRoot, "packages/components/src");
const factoryEmulatorPackageRoot = path.resolve(
  uiRoot,
  "packages/factory-emulator/src",
);
const factoryReplayPackageRoot = path.resolve(
  uiRoot,
  "packages/factory-replay/src",
);
const factoryGraphPackageRoot = path.resolve(
  uiRoot,
  "packages/factory-graph/src",
);
const factoryVisualizersPackageRoot = path.resolve(
  uiRoot,
  "packages/factory-visualizers/src",
);
const isVitestRun =
  process.argv.includes("vitest") || process.env.VITEST === "true";

export function isDashboardUnitVitestRun(
  argv: readonly string[],
  env: { VITEST?: string } = process.env,
): boolean {
  const isVitestInvocation = argv.includes("vitest") || env.VITEST === "true";
  const projectSelectors: string[] = [];

  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index];
    if (argument.startsWith("--project=")) {
      projectSelectors.push(argument.slice("--project=".length));
      continue;
    }

    if (argument === "--project") {
      const selector = argv[index + 1];
      if (selector !== undefined && !selector.startsWith("--")) {
        projectSelectors.push(selector);
        index += 1;
      }
    }
  }

  return (
    isVitestInvocation &&
    projectSelectors.length > 0 &&
    projectSelectors.every((selector) => selector === "dashboard-unit")
  );
}

const isDashboardUnitRun = isDashboardUnitVitestRun(process.argv);
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
    find: "@you-agent-factory/factory-graph",
    replacement: path.join(factoryGraphPackageRoot, "index.ts"),
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
const testOnlyAliases = [
  {
    find: "@monaco-editor/react",
    replacement: path.join(uiRoot, "src/testing/mocks/monaco-react.ts"),
  },
  {
    find: "monaco-editor/esm/vs/editor/editor.all.js",
    replacement: path.join(uiRoot, "src/testing/mocks/monaco-editor-all.ts"),
  },
  {
    find: "monaco-editor/esm/vs/editor/editor.api.js",
    replacement: path.join(uiRoot, "src/testing/mocks/monaco-editor-api.ts"),
  },
] as const;
const isCoverageRun = process.argv.includes("--coverage");
const profileSourceMaps =
  process.env.AGENT_FACTORY_PROFILE_SOURCEMAPS === "true" ||
  process.env.AGENT_FACTORY_PROFILE_SOURCEMAPS === "1";
const monacoEditorPlugin =
  typeof monacoEditorPluginModule === "function"
    ? monacoEditorPluginModule
    : monacoEditorPluginModule.default;
const optimizedDeps = isDashboardUnitRun
  ? []
  : isVitestRun
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
const vitestServerDepsInline = isDashboardUnitRun
  ? []
  : ([
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
    ] as const);
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
    needsInterop: isDashboardUnitRun ? [] : [...storybookInteropDeps],
  },
  plugins: [
    ...(!isDashboardUnitRun ? [react()] : []),
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
      ...(isVitestRun ? testOnlyAliases : []),
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
        inline: [...vitestServerDepsInline],
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
    setupFiles: isDashboardUnitRun ? [] : ["./src/testing/vitest.setup.ts"],
    testTimeout: isCoverageRun ? 180000 : 30000,
    coverage: {
      provider: "v8",
      reporter: [...coverageConfigDefaults.reporter, "lcov"],
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
        // Node-only baseline measured 54.58/46.22/52.42/54.87 on 2026-07-26.
        // Component and browser confidence are intentionally outside coverage.
        statements: 54,
        branches: 46,
        functions: 52,
        lines: 54,
      },
    },
  },
});
