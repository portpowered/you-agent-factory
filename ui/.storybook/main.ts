import type { StorybookConfig } from "@storybook/react-vite";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { mergeConfig } from "vite";

const uiRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);

const config: StorybookConfig = {
  addons: ["@storybook/addon-vitest"],
  framework: {
    name: "@storybook/react-vite",
    options: {},
  },
  stories: ["../src/**/*.stories.@(ts|tsx)"],
  async viteFinal(config) {
    return mergeConfig(config, {
      resolve: {
        alias: [
          {
            find: "@xyflow/react",
            replacement: path.join(uiRoot, "node_modules/@xyflow/react"),
          },
        ],
        dedupe: ["@xyflow/react"],
      },
    });
  },
};

export default config;
