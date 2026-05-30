import { defineConfig, mergeConfig } from "vitest/config";

import viteConfig from "./vite.config";

/** Vitest-owned lanes (integration, Storybook script verifiers, coverage standalone phase). */
export default mergeConfig(
  viteConfig,
  defineConfig({
    resolve: {
      alias: [{ find: "bun:test", replacement: "vitest" }],
    },
    test: {
      exclude: ["**/*.bun.test.{ts,tsx}"],
      deps: {
        interopDefault: true,
      },
      environment: "jsdom",
      globals: true,
      setupFiles: ["./src/testing/vitest.setup.ts"],
      testTimeout: 30_000,
    },
  }),
);
