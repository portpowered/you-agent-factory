import { spawnSync } from "node:child_process";
import { existsSync, readdirSync, rmSync, statSync } from "node:fs";
import { join } from "node:path";
import { performance } from "node:perf_hooks";
import {
  bunCoverageReportsDir,
  coverageTestTimeoutMs,
  reactFlowCoverageReportsDir,
  reactFlowCoverageTestPath,
} from "./bun-coverage-config.mjs";

export const phaseLogPrefix = "[ui-coverage]";
export const mainCoveredPhaseName = "Main covered Vitest pass";
export const defaultMainCoveredMaxWorkers = "2";
export const defaultShardMainCoveredMaxWorkers = "1";
export const defaultSlowFileSummaryLimit = 15;
export const defaultUiCoverageShardTotal = 10;

const vitestFileDurationLinePattern =
  /^\s*[✓×]\s+(\S+\.(?:test|spec)\.(?:tsx?|mjs|cjs))\s+\([^)]+\)\s+(\d+(?:\.\d+)?)ms(?:\s+\d+ MB heap used)?/gm;

const ansiEscapePattern = new RegExp(`${String.fromCharCode(27)}\\[[0-9;]*m`, "g");

export function getMainCoveredMaxWorkers(env = process.env, options = {}) {
  if (env.UI_COVERAGE_MAIN_MAX_WORKERS) {
    return env.UI_COVERAGE_MAIN_MAX_WORKERS;
  }
  if (options.shard) {
    return defaultShardMainCoveredMaxWorkers;
  }
  return defaultMainCoveredMaxWorkers;
}

const mainCoveredPassKind = "main-covered";

export function parseUiCoverageShard(env = process.env) {
  const raw = env.UI_COVERAGE_SHARD?.trim();
  if (!raw) {
    return null;
  }

  const match = /^(\d+)\/(\d+)$/.exec(raw);
  if (!match) {
    throw new Error(
      `Invalid UI_COVERAGE_SHARD "${raw}"; expected format index/total (e.g. 3/10)`,
    );
  }

  const index = Number(match[1]);
  const total = Number(match[2]);
  if (!Number.isInteger(index) || !Number.isInteger(total) || total < 1) {
    throw new Error(
      `Invalid UI_COVERAGE_SHARD "${raw}"; total must be a positive integer`,
    );
  }
  if (index < 1 || index > total) {
    throw new Error(
      `Invalid UI_COVERAGE_SHARD "${raw}"; index must be between 1 and ${total}`,
    );
  }

  return { index, label: raw, total };
}

export function mainCoveredShardReportDir(
  shardIndex,
  reportsDir = bunCoverageReportsDir,
) {
  return join(reportsDir, `main-shard-${shardIndex}`);
}

/** @deprecated Vitest blob path; Bun shards write lcov under {@link mainCoveredShardReportDir}. */
export function mainCoveredShardBlobPath(shardIndex, reportsDir = bunCoverageReportsDir) {
  return join(mainCoveredShardReportDir(shardIndex, reportsDir), "lcov.info");
}

export function parseUiCoverageMerge(env = process.env) {
  const raw = env.UI_COVERAGE_MERGE?.trim().toLowerCase();
  if (!raw) {
    return false;
  }
  if (raw === "0" || raw === "false" || raw === "no") {
    return false;
  }
  return true;
}

export function getUiCoverageShardTotal(env = process.env) {
  const raw = env.UI_COVERAGE_SHARD_TOTAL?.trim();
  if (!raw) {
    return defaultUiCoverageShardTotal;
  }

  const total = Number(raw);
  if (!Number.isInteger(total) || total < 1) {
    throw new Error(
      `Invalid UI_COVERAGE_SHARD_TOTAL "${raw}"; expected a positive integer`,
    );
  }

  return total;
}

export function shardReportHasCoverage(shardIndex, reportsDir = bunCoverageReportsDir) {
  const shardDir = mainCoveredShardReportDir(shardIndex, reportsDir);
  if (!existsSync(shardDir)) {
    return false;
  }

  const stack = [shardDir];
  while (stack.length > 0) {
    const current = stack.pop();
    for (const entry of readdirSync(current)) {
      const fullPath = join(current, entry);
      if (statSync(fullPath).isDirectory()) {
        stack.push(fullPath);
        continue;
      }
      if (entry === "lcov.info") {
        return true;
      }
    }
  }

  return false;
}

export function findMissingShardBlobIndices(
  shardTotal,
  { reportsDir = bunCoverageReportsDir } = {},
) {
  const missing = [];

  for (let index = 1; index <= shardTotal; index += 1) {
    if (!shardReportHasCoverage(index, reportsDir)) {
      missing.push(index);
    }
  }

  return missing;
}

export function formatMissingShardBlobSummary(missingIndices, shardTotal) {
  const missingList = missingIndices.join(", ");
  const expectedPaths = missingIndices
    .map((index) => mainCoveredShardReportDir(index))
    .join(", ");
  return `Missing UI coverage shard reports (${missingList}/${shardTotal}): expected lcov under ${expectedPaths}`;
}

export function buildUiCoverageMergePhases(options = {}) {
  return buildUiCoveragePhases(options).slice(1);
}

export function buildMainCoveredVitestArgs(options = {}) {
  const shard = options.shard ?? null;
  const mainCoveredMaxWorkers =
    options.mainCoveredMaxWorkers ??
    getMainCoveredMaxWorkers(options.env, { shard: Boolean(shard) });
  const blobPath =
    options.blobPath ?? ".vitest-reports/main.json";
  const args = [
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
    `--outputFile.blob=${blobPath}`,
    "--exclude",
    "integration/*.integration.test.mjs",
    "--exclude",
    "scripts/dashboard-shell-storybook-responsive.test.mjs",
    "--exclude",
    "scripts/ui-coverage-runner.test.mjs",
    "--exclude",
    "src/features/workflow-activity/components/react-flow-current-activity-card.test.tsx",
  ];

  if (options.shard) {
    args.push(`--shard=${options.shard.index}/${options.shard.total}`);
  }

  return args;
}

export function buildMainCoveredShardPhase(shard, _options = {}) {
  return {
    kind: mainCoveredPassKind,
    name: `${mainCoveredPhaseName} (shard ${shard.label})`,
    command: "node",
    args: ["scripts/run-bun-coverage-main.mjs"],
  };
}

export function buildUiCoveragePhases(options = {}) {
  return [
    {
      kind: mainCoveredPassKind,
      name: mainCoveredPhaseName,
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
        "--config",
        "vitest.config.ts",
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
  rmSync(bunCoverageReportsDir, { force: true, recursive: true });
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

export function runUiCoverageShard(shard, options = {}) {
  const spawn = options.spawn ?? spawnSync;
  const phase = buildMainCoveredShardPhase(shard, options);
  const { capturedStdout, status } = runTimedPhase(phase, spawn, {
    captureStdout: true,
  });

  if (status !== 0) {
    process.exit(status);
  }

  logSlowFileSummary(capturedStdout);
}

export function runUiCoverageMerge(options = {}) {
  const env = options.env ?? process.env;
  const spawn = options.spawn ?? spawnSync;
  const reportsDir = options.reportsDir ?? bunCoverageReportsDir;
  const shardTotal = getUiCoverageShardTotal(env);
  const missing = findMissingShardBlobIndices(shardTotal, { reportsDir });

  if (missing.length > 0) {
    console.error(
      `${phaseLogPrefix} ${formatMissingShardBlobSummary(missing, shardTotal)}`,
    );
    process.exit(1);
  }

  rmSync("coverage", { force: true, recursive: true });

  for (const phase of buildUiCoverageMergePhases(options)) {
    const { status } = runTimedPhase(phase, spawn);

    if (status !== 0) {
      process.exit(status);
    }
  }
}

export function runUiCoverage(phases = uiCoveragePhases, options = {}) {
  const env = options.env ?? process.env;
  const spawn = options.spawn ?? spawnSync;
  const shard = parseUiCoverageShard(env);
  const merge = parseUiCoverageMerge(env);

  if (shard && merge) {
    throw new Error(
      "UI_COVERAGE_SHARD and UI_COVERAGE_MERGE cannot both be set",
    );
  }

  if (shard) {
    runUiCoverageShard(shard, options);
    return;
  }

  if (merge) {
    runUiCoverageMerge(options);
    return;
  }

  cleanCoverageArtifacts();

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
