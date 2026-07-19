import path from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vitest/config";

const packageRoot = path.dirname(fileURLToPath(import.meta.url));
const uiRoot = path.resolve(packageRoot, "../..");
const uiNodeModules = path.join(uiRoot, "node_modules");

export default defineConfig({
  resolve: {
    alias: {
      "@you-agent-factory/components": path.resolve(
        packageRoot,
        "../components/src/index.ts",
      ),
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
  },
});
