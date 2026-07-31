import { performance } from "node:perf_hooks";

import {
  assertComponentTestDuration,
  formatComponentDuration,
  getComponentTestMaxDurationMs,
} from "./component-test-duration";

async function runLane(scriptName: string) {
  const startedAt = performance.now();
  const child = Bun.spawn(["bun", "run", scriptName], {
    env: process.env,
    stderr: "inherit",
    stdout: "inherit",
  });
  const exitCode = await child.exited;
  const elapsedMs = performance.now() - startedAt;

  console.log(
    `[ui-component] ${scriptName} elapsed: ${formatComponentDuration(elapsedMs)}`,
  );

  if (exitCode !== 0) {
    process.exit(exitCode);
  }
}

const startedAt = performance.now();
await Promise.all([
  runLane("test:component:bun"),
  runLane("test:component:vitest"),
]);

const totalDurationMs = performance.now() - startedAt;
const maxDurationMs = getComponentTestMaxDurationMs();
console.log(
  `[ui-component] total elapsed: ${formatComponentDuration(totalDurationMs)} (budget ${formatComponentDuration(maxDurationMs)})`,
);
assertComponentTestDuration(totalDurationMs, maxDurationMs);
