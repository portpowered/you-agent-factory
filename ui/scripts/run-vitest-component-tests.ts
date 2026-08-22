import { listComponentTestFiles } from "./component-test-files";
import { buildVitestComponentArgs } from "./component-test-workers";

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
    ...buildVitestComponentArgs({
      componentTests,
      forwardedArgs: process.argv.slice(2),
    }),
  ],
  cwd: process.cwd(),
  stderr: "inherit",
  stdout: "inherit",
});

process.exit(result.exitCode);
