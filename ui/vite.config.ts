import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react-swc";
import { defineConfig } from "vite";
import monacoEditorPluginModule from "vite-plugin-monaco-editor";

const apiOrigin = process.env.AGENT_FACTORY_API_ORIGIN ?? "http://127.0.0.1:7437";
const monacoEditorPlugin =
  typeof monacoEditorPluginModule === "function"
    ? monacoEditorPluginModule
    : monacoEditorPluginModule.default;
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
  "^/work-requests/[^/]+$",
  "^/factory-sessions/[^/]+/work-requests/[^/]+$",
  "^/work/[^/]+$",
  "^/factory-sessions/[^/]+/work/[^/]+$",
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
  },
  esbuild: {
    jsxDev: false,
  },
  optimizeDeps: {
    include: [
      "@radix-ui/react-slot",
      "monaco-editor/esm/vs/editor/editor.api.js",
      "react",
      "react-dom",
      "react/jsx-runtime",
      "react/jsx-dev-runtime",
    ],
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
});
