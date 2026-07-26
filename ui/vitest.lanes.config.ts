import { defineConfig, mergeConfig } from "vitest/config";

import viteConfig from "./vite.config";

export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      projects: [
        {
          extends: true,
          test: {
            environment: "node",
            exclude: [
              "src/**/*.component.test.ts",
              "src/**/performance/*.test.ts",
            ],
            include: ["src/**/*.test.ts", "src/**/*.unit.test.mts"],
            name: "dashboard-unit",
            setupFiles: [],
          },
        },
        {
          extends: true,
          test: {
            environment: "jsdom",
            include: ["src/**/*.component.test.ts", "src/**/*.test.tsx"],
            exclude: ["src/**/performance/*.test.tsx"],
            name: "dashboard-component",
          },
        },
      ],
    },
  }),
);
