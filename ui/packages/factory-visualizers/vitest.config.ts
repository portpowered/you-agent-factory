import path from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vitest/config";

const packageRoot = path.dirname(fileURLToPath(import.meta.url));
const uiRoot = path.resolve(packageRoot, "../..");
const uiNodeModules = path.join(uiRoot, "node_modules");

export default defineConfig({
  resolve: {
    alias: {
      "@testing-library/jest-dom/vitest": path.join(
        uiNodeModules,
        "@testing-library/jest-dom/vitest",
      ),
      "@testing-library/react": path.join(
        uiNodeModules,
        "@testing-library/react",
      ),
      "@you-agent-factory/components/graphs": path.resolve(
        packageRoot,
        "../components/src/graphs/index.ts",
      ),
      "@you-agent-factory/components": path.resolve(
        packageRoot,
        "../components/src/index.ts",
      ),
      "@you-agent-factory/client": path.resolve(
        packageRoot,
        "../client/src/index.ts",
      ),
      "@xyflow/react": path.join(uiNodeModules, "@xyflow/react"),
      "@you-agent-factory/factory-replay": path.resolve(
        packageRoot,
        "../factory-replay/src/index.ts",
      ),
      react: path.join(uiNodeModules, "react"),
      "react-dom": path.join(uiNodeModules, "react-dom"),
      "react/jsx-runtime": path.join(uiNodeModules, "react/jsx-runtime"),
    },
    dedupe: ["react", "react-dom", "react/jsx-runtime"],
  },
  test: {
    environment: "happy-dom",
    include: ["src/**/*.test.ts", "src/**/*.test.tsx"],
    setupFiles: ["./src/testing/vitest.setup.ts"],
    server: {
      deps: {
        moduleDirectories: [uiNodeModules],
        inline: [
          "react",
          "react-dom",
          "react-dom/client",
          "react/jsx-runtime",
          "@testing-library/react",
        ],
      },
    },
  },
});
