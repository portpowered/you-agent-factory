import { listComponentTestFiles } from "./component-test-files";

const ISOLATED_COMPONENT_TEST_MARKER = ".isolated.bun.component.test.tsx";
const PRELOAD_PATH = "./src/testing/bun/component.setup.ts";

const componentTests = listComponentTestFiles()
  .filter((file) => file.runner === "bun")
  .map((file) => file.path);
const isolatedTests = componentTests.filter((file) =>
  file.endsWith(ISOLATED_COMPONENT_TEST_MARKER),
);
const sharedProcessTests = componentTests.filter(
  (file) => !file.endsWith(ISOLATED_COMPONENT_TEST_MARKER),
);

console.log(
  `[ui-component] Bun owns ${componentTests.length} files (${sharedProcessTests.length} shared, ${isolatedTests.length} isolated).`,
);

function runComponentTests(files: string[]): void {
  if (files.length === 0) {
    return;
  }

  const result = Bun.spawnSync({
    cmd: [
      "bun",
      "test",
      "--preload",
      PRELOAD_PATH,
      "--reporter=dots",
      "--timeout=10000",
      ...files,
    ],
    cwd: process.cwd(),
    stderr: "inherit",
    stdout: "inherit",
  });

  if (result.exitCode !== 0) {
    process.exit(result.exitCode);
  }
}

runComponentTests(sharedProcessTests);

// Bun's module mocks are process-global. Files that replace application
// modules run separately so their mocks cannot leak into another feature's
// component tests. Ordinary DOM tests still share one fast process.
for (const isolatedTest of isolatedTests) {
  runComponentTests([isolatedTest]);
}
