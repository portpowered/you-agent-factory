import { spawnSync } from "node:child_process";
import { mkdirSync, rmSync } from "node:fs";
import { join } from "node:path";
import { performance } from "node:perf_hooks";

import {
  defaultCapturedStdoutMaxBuffer,
  defaultSlowFileSummaryLimit,
  logSlowFileSummary as emitSlowFileSummary,
  formatElapsedMs,
  formatSlowFileSummaryLines,
  parseVitestFileDurationsFromLog,
  rankSlowestTestFiles,
} from "./ui-test-cost-report.mjs";

export const phaseLogPrefix = "[ui-coverage]";
export const mainCoveredPhaseName = "Main covered Vitest pass";
export const defaultMainCoveredMaxWorkers = "4";
export { defaultCapturedStdoutMaxBuffer, defaultSlowFileSummaryLimit };
export const uiPerformanceTestPattern = "**/performance/*.test.ts";

export function getMainCoveredMaxWorkers(env = process.env) {
  return env.UI_COVERAGE_MAIN_MAX_WORKERS ?? defaultMainCoveredMaxWorkers;
}

const mainCoveredPassKind = "main-covered";

export function buildMainCoveredVitestArgs(options = {}) {
  const mainCoveredMaxWorkers =
    options.mainCoveredMaxWorkers ?? getMainCoveredMaxWorkers(options.env);

  return [
    "run",
    "--config=vitest.lanes.config.ts",
    "--project=dashboard-unit",
    "--coverage",
    `--maxWorkers=${mainCoveredMaxWorkers}`,
    "--reporter=default",
    "--exclude",
    "integration/*.integration.test.mjs",
    "--exclude",
    "scripts/dashboard-shell-storybook-responsive.test.mjs",
    "--exclude",
    "scripts/ui-coverage-runner.test.mjs",
    "--exclude",
    uiPerformanceTestPattern,
  ];
}

export function buildUiCoveragePhases(options = {}) {
  return [
    {
      kind: mainCoveredPassKind,
      name: mainCoveredPhaseName,
      command: "vitest",
      args: buildMainCoveredVitestArgs(options),
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

export {
  formatElapsedMs,
  formatSlowFileSummaryLines,
  parseVitestFileDurationsFromLog,
  rankSlowestTestFiles,
};

export function formatPhaseElapsed(phaseName, elapsedMs) {
  return `${phaseLogPrefix} ${phaseName} elapsed: ${formatElapsedMs(elapsedMs)}`;
}

export function cleanCoverageArtifacts(rootDirectory = ".") {
  const coverageDirectory = join(rootDirectory, "coverage");

  rmSync(coverageDirectory, { force: true, recursive: true });
  rmSync(join(rootDirectory, ".vitest-reports"), {
    force: true,
    recursive: true,
  });
  rmSync(join(rootDirectory, ".vitest-report-timings"), {
    force: true,
    recursive: true,
  });
  mkdirSync(join(coverageDirectory, ".tmp"), { recursive: true });
}

export function runTimedPhase(phase, spawn = spawnSync, options = {}) {
  const captureStdout = options.captureStdout === true;
  const maxBuffer =
    options.maxBuffer ??
    (captureStdout ? defaultCapturedStdoutMaxBuffer : undefined);
  const startedAt = performance.now();
  const result = spawn(phase.command, phase.args, {
    shell: process.platform === "win32",
    stdio: captureStdout ? ["inherit", "pipe", "inherit"] : "inherit",
    encoding: captureStdout ? "utf8" : undefined,
    ...(maxBuffer === undefined ? {} : { maxBuffer }),
  });
  const elapsedMs = performance.now() - startedAt;

  if (captureStdout && result.stdout) {
    process.stdout.write(result.stdout);
  }

  console.log(formatPhaseElapsed(phase.name, elapsedMs));

  if (result.error) {
    throw result.error;
  }

  return {
    capturedStdout: captureStdout ? (result.stdout ?? "") : "",
    status: result.status ?? 1,
  };
}

export function logSlowFileSummary(capturedStdout, summaryTitle) {
  emitSlowFileSummary(capturedStdout, {
    logPrefix: phaseLogPrefix,
    summaryTitle: summaryTitle ?? "Main covered pass slowest test files",
  });
}

export function runUiCoverage(phases = uiCoveragePhases, options = {}) {
  const spawn = options.spawn ?? spawnSync;
  cleanCoverageArtifacts(options.rootDirectory ?? ".");

  for (const phase of phases) {
    const captureStdout = phase.kind === mainCoveredPassKind;
    const { capturedStdout, status } = runTimedPhase(phase, spawn, {
      captureStdout,
    });

    if (status !== 0) {
      process.exit(status);
    }

    if (captureStdout) {
      logSlowFileSummary(capturedStdout);
    }
  }
}
