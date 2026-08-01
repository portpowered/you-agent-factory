import { spawnSync } from "node:child_process";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

export const uiTestPhases = [
  { command: "bun", args: ["run", "test:unit"], name: "Unit tests" },
  {
    command: "bun",
    args: ["run", "test:component"],
    name: "Component tests",
  },
  {
    command: "bun",
    args: ["run", "test:integration"],
    name: "Browser integration tests",
  },
];

export function runUiTest({ phases = uiTestPhases, spawn = spawnSync } = {}) {
  for (const phase of phases) {
    console.log(`[ui-test] ${phase.name}`);
    const result = spawn(phase.command, phase.args, {
      env: process.env,
      shell: process.platform === "win32",
      stdio: "inherit",
    });

    if (result.error) {
      throw result.error;
    }

    const status = result.status ?? 1;
    if (status !== 0) {
      return status;
    }
  }

  return 0;
}

const isMain = process.argv[1]
  ? resolve(process.argv[1]) === fileURLToPath(import.meta.url)
  : false;

if (isMain) {
  process.exitCode = runUiTest();
}
