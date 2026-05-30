import { spawnSync } from "node:child_process";
import { rmSync } from "node:fs";
import { performance } from "node:perf_hooks";
import {
  bunCoverageReportsDir,
  coverageTestTimeoutMs,
  reactFlowCoverageReportsDir,
  reactFlowCoverageTestPath,
} from "./bun-coverage-config.mjs";

export const phaseLogPrefix = "[ui-coverage]";
export const defaultMainCoveredMaxWorkers = "2";

export function getMainCoveredMaxWorkers(env = process.env) {
  return env.UI_COVERAGE_MAIN_MAX_WORKERS || defaultMainCoveredMaxWorkers;
}

export function buildUiCoveragePhases(options = {}) {
  const mainCoveredMaxWorkers =
    options.mainCoveredMaxWorkers ?? getMainCoveredMaxWorkers(options.env);

  return [
    {
      name: "Main covered Vitest pass",
      command: "node",
      args: ["scripts/run-bun-coverage-main.mjs"],
    },
    {
      name: "Isolated React Flow covered pass",
      command: "bun",
      args: [
        "test",
        reactFlowCoverageTestPath,
        "--coverage",
        "--coverage-reporter=lcov",
        `--coverage-dir=${reactFlowCoverageReportsDir}`,
        "--timeout",
        String(coverageTestTimeoutMs),
        "--parallel=1",
        "--isolate",
      ],
    },
    {
      name: "Blob report merge pass",
      command: "node",
      args: ["scripts/merge-bun-coverage-thresholds.mjs"],
    },
    {
      name: "Standalone script-style test",
      command: "vitest",
      args: [
        "run",
        "scripts/dashboard-shell-storybook-responsive.test.mjs",
        "--maxWorkers=1",
      ],
    },
  ];
}

export const uiCoveragePhases = buildUiCoveragePhases();

export function formatElapsedMs(elapsedMs) {
  return `${(elapsedMs / 1000).toFixed(2)}s`;
}

export function formatPhaseElapsed(phaseName, elapsedMs) {
  return `${phaseLogPrefix} ${phaseName} elapsed: ${formatElapsedMs(elapsedMs)}`;
}

export function cleanCoverageArtifacts() {
  rmSync("coverage", { force: true, recursive: true });
  rmSync(".vitest-reports", { force: true, recursive: true });
  rmSync(bunCoverageReportsDir, { force: true, recursive: true });
}

export function runTimedPhase(phase, spawn = spawnSync) {
  const startedAt = performance.now();
  const result = spawn(phase.command, phase.args, {
    shell: process.platform === "win32",
    stdio: "inherit",
  });
  const elapsedMs = performance.now() - startedAt;

  console.log(formatPhaseElapsed(phase.name, elapsedMs));

  if (result.error) {
    throw result.error;
  }

  return result.status ?? 1;
}

export function runUiCoverage(phases = uiCoveragePhases) {
  cleanCoverageArtifacts();

  for (const phase of phases) {
    const status = runTimedPhase(phase);
    if (status !== 0) {
      process.exit(status);
    }
  }
}
