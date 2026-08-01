import { spawnSync } from "node:child_process";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

export const dashboardUnitVitestArgs = [
  "run",
  "--config=vitest.lanes.config.ts",
  "--project=dashboard-unit",
  "--maxWorkers=4",
  "--retry=0",
];

export function buildUiUnitPhases(forwardedArgs = []) {
  return [
    {
      command: "bun",
      args: ["run", "test:unit:bun"],
      name: "Bun native unit",
    },
    {
      command: "vitest",
      args: [...dashboardUnitVitestArgs, ...forwardedArgs],
      name: "Vitest dashboard unit",
    },
  ];
}

export const uiUnitPhases = buildUiUnitPhases();

export function runUiUnit({ phases = uiUnitPhases, spawn = spawnSync } = {}) {
  for (const phase of phases) {
    console.log(`[ui-unit] ${phase.name}`);
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
  process.exitCode = runUiUnit({
    phases: buildUiUnitPhases(process.argv.slice(2)),
  });
}
