import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    environment: "node",
    fileParallelism: false,
    include: [
      "src/package-build.harness.test.ts",
      "src/package-pack.harness.test.ts",
    ],
  },
});
