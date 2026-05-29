import { spawnSync } from "node:child_process";
import { rmSync } from "node:fs";
import { performance } from "node:perf_hooks";

export const phaseLogPrefix = "[ui-coverage]";
export const mainCoveredPhaseName = "Main covered Vitest pass";
export const defaultMainCoveredMaxWorkers = "2";
export const defaultSlowFileSummaryLimit = 15;

const vitestFileDurationLinePattern =
  /^\s*[✓×]\s+(\S+\.(?:test|spec)\.(?:tsx?|mjs|cjs))\s+\([^)]+\)\s+(\d+(?:\.\d+)?)ms(?:\s+\d+ MB heap used)?/gm;

const ansiEscapePattern = /\u001b\[[0-9;]*m/g;

export function getMainCoveredMaxWorkers(env = process.env) {
  return env.UI_COVERAGE_MAIN_MAX_WORKERS || defaultMainCoveredMaxWorkers;
}

export function buildUiCoveragePhases(options = {}) {
  const mainCoveredMaxWorkers =
    options.mainCoveredMaxWorkers ?? getMainCoveredMaxWorkers(options.env);

  return [
    {
      name: mainCoveredPhaseName,
      command: "vitest",
      args: [
        "run",
        "--coverage",
        "--coverage.clean=false",
        `--maxWorkers=${mainCoveredMaxWorkers}`,
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
}

export const uiCoveragePhases = buildUiCoveragePhases();

export function stripAnsi(text) {
  return text.replace(ansiEscapePattern, "");
}

export function parseVitestFileDurationsFromLog(logText) {
  const strippedLog = stripAnsi(logText);
  const durationsByPath = new Map();

  for (const match of strippedLog.matchAll(vitestFileDurationLinePattern)) {
    const [, filePath, durationMsText] = match;
    const durationMs = Number(durationMsText);
    durationsByPath.set(
      filePath,
      Math.max(durationsByPath.get(filePath) ?? 0, durationMs),
    );
  }

  return [...durationsByPath.entries()].map(([path, durationMs]) => ({
    path,
    durationMs,
  }));
}

export function rankSlowestTestFiles(fileDurations, limit = defaultSlowFileSummaryLimit) {
  return [...fileDurations]
    .sort((left, right) => right.durationMs - left.durationMs)
    .slice(0, limit);
}

export function formatSlowFileSummaryLines(
  slowFiles,
  { limit = defaultSlowFileSummaryLimit } = {},
) {
  if (slowFiles.length === 0) {
    return [`${phaseLogPrefix} Main covered pass slowest test files: none reported`];
  }

  const lines = [
    `${phaseLogPrefix} Main covered pass slowest test files (top ${Math.min(slowFiles.length, limit)}):`,
  ];

  for (const { path, durationMs } of slowFiles) {
    lines.push(`${phaseLogPrefix}   ${path} ${formatElapsedMs(durationMs)}`);
  }

  return lines;
}

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

export function runTimedPhase(phase, spawn = spawnSync, options = {}) {
  const captureStdout = options.captureStdout === true;
  const startedAt = performance.now();
  const result = spawn(phase.command, phase.args, {
    shell: process.platform === "win32",
    stdio: captureStdout ? ["inherit", "pipe", "inherit"] : "inherit",
    encoding: captureStdout ? "utf8" : undefined,
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

export function logSlowFileSummary(capturedStdout) {
  const slowFiles = rankSlowestTestFiles(
    parseVitestFileDurationsFromLog(capturedStdout),
  );

  for (const line of formatSlowFileSummaryLines(slowFiles)) {
    console.log(line);
  }
}

export function runUiCoverage(phases = uiCoveragePhases) {
  cleanCoverageArtifacts();

  for (const phase of phases) {
    const captureStdout = phase.name === mainCoveredPhaseName;
    const { capturedStdout, status } = runTimedPhase(phase, spawnSync, {
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
