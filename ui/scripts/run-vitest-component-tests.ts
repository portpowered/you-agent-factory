import { listComponentTestFiles } from "./component-test-files";

const componentTests = listComponentTestFiles()
  .filter((file) => file.runner === "vitest")
  .map((file) => file.path);

if (componentTests.length === 0) {
  process.exit(0);
}

console.log(
  `[ui-component] Vitest compatibility owns ${componentTests.length} files.`,
);

const result = Bun.spawnSync({
  cmd: [
    "bunx",
    "vitest",
    "run",
    "--config",
    "vitest.lanes.config.ts",
    "--project=dashboard-component",
    "--maxWorkers=2",
    "--retry=0",
    ...process.argv.slice(2),
    ...componentTests,
  ],
  cwd: process.cwd(),
  stderr: "inherit",
  stdout: "inherit",
});

process.exit(result.exitCode);
