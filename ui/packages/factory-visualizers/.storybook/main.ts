import path from "node:path";
import { fileURLToPath } from "node:url";
import type { StorybookConfig } from "@storybook/react-vite";
import react from "@vitejs/plugin-react-swc";
import { mergeConfig } from "vite";

const storybookRoot = path.dirname(fileURLToPath(import.meta.url));
const packageRoot = path.resolve(storybookRoot, "..");

const config: StorybookConfig = {
  stories: ["../src/**/*.stories.@(ts|tsx)"],
  framework: {
    name: "@storybook/react-vite",
    options: {},
  },
  async viteFinal(config) {
    return mergeConfig(config, {
      plugins: [react()],
      resolve: {
        alias: [
          {
            find: "@xyflow/react",
            replacement: path.resolve(
              packageRoot,
              "../../node_modules/@xyflow/react",
            ),
          },
          {
            find: "@you-agent-factory/client",
            replacement: path.resolve(packageRoot, "../client/src/index.ts"),
          },
          {
            find: "@you-agent-factory/components/graphs",
            replacement: path.resolve(
              packageRoot,
              "../components/src/graphs/index.ts",
            ),
          },
          {
            find: "@you-agent-factory/components/styles.css",
            replacement: path.resolve(
              packageRoot,
              "../components/src/styles.css",
            ),
          },
          {
            find: "@you-agent-factory/components",
            replacement: path.resolve(
              packageRoot,
              "../components/src/index.ts",
            ),
          },
          {
            find: "@you-agent-factory/factory-replay",
            replacement: path.resolve(
              packageRoot,
              "../factory-replay/src/index.ts",
            ),
          },
        ],
        dedupe: ["@xyflow/react", "react", "react-dom", "react/jsx-runtime"],
      },
    });
  },
};

export default config;
