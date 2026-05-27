import { spawnSync } from "node:child_process";
import { rmSync } from "node:fs";
import { performance } from "node:perf_hooks";

export const phaseLogPrefix = "[ui-coverage]";

export const uiCoveragePhases = [
  {
    name: "Main covered Vitest pass",
    command: "vitest",
    args: [
      "run",
      "--coverage",
      "--coverage.clean=false",
      "--maxWorkers=1",
      "--coverage.thresholds.lines=0",
      "--coverage.thresholds.functions=0",
      "--coverage.thresholds.statements=0",
      "--coverage.thresholds.branches=0",
      "--reporter=default",
      "--reporter=blob",
      "--outputFile.blob=.vitest-reports/main.json",
      "--exclude",
      "integration/*.integration.test.mjs",
      "--exclude",
      "scripts/dashboard-shell-storybook-responsive.test.mjs",
      "--exclude",
      "src/features/workflow-activity/components/react-flow-current-activity-card.test.tsx",
    ],
  },
  {
    name: "Isolated React Flow covered pass",
    command: "vitest",
    args: [
      "run",
      "--coverage",
      "--coverage.clean=false",
      "--maxWorkers=1",
      "--coverage.thresholds.lines=0",
      "--coverage.thresholds.functions=0",
      "--coverage.thresholds.statements=0",
      "--coverage.thresholds.branches=0",
      "--reporter=default",
      "--reporter=blob",
      "--outputFile.blob=.vitest-reports/react-flow-current-activity-card.json",
      "src/features/workflow-activity/components/react-flow-current-activity-card.test.tsx",
    ],
  },
  {
    name: "Blob report merge pass",
    command: "vitest",
    args: ["--mergeReports", ".vitest-reports", "--coverage"],
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

export function formatElapsedMs(elapsedMs) {
  return `${(elapsedMs / 1000).toFixed(2)}s`;
}

export function formatPhaseElapsed(phaseName, elapsedMs) {
  return `${phaseLogPrefix} ${phaseName} elapsed: ${formatElapsedMs(elapsedMs)}`;
}

export function cleanCoverageArtifacts() {
  rmSync("coverage", { force: true, recursive: true });
  rmSync(".vitest-reports", { force: true, recursive: true });
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
