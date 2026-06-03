import { mergeConfig } from "vite";
import baseConfig from "./vite.config";

export default mergeConfig(baseConfig, {
  test: {
    setupFiles: [
      "./src/testing/vitest.setup.ts",
      "./src/testing/warning-inventory-capture.setup.ts",
    ],
    testTimeout: 120_000,
  },
});
