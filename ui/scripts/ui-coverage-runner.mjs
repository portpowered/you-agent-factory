import { spawnSync } from "node:child_process";
import { existsSync, rmSync } from "node:fs";
import { join } from "node:path";
import { performance } from "node:perf_hooks";

export const phaseLogPrefix = "[ui-coverage]";
export const mainCoveredPhaseName = "Main covered Vitest pass";
export const defaultMainCoveredMaxWorkers = "2";
export const defaultSlowFileSummaryLimit = 15;
export const defaultUiCoverageShardTotal = 10;

const vitestFileDurationLinePattern =
  /^\s*[✓×]\s+(\S+\.(?:test|spec)\.(?:tsx?|mjs|cjs))\s+\([^)]+\)\s+(\d+(?:\.\d+)?)ms(?:\s+\d+ MB heap used)?/gm;

const ansiEscapePattern = new RegExp(`${String.fromCharCode(27)}\\[[0-9;]*m`, "g");

export function getMainCoveredMaxWorkers(env = process.env) {
  return env.UI_COVERAGE_MAIN_MAX_WORKERS || defaultMainCoveredMaxWorkers;
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

export function mainCoveredShardBlobPath(shardIndex, reportsDir = ".vitest-reports") {
  return join(reportsDir, `main-shard-${shardIndex}.json`);
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

export function findMissingShardBlobIndices(
  shardTotal,
  { reportsDir = ".vitest-reports" } = {},
) {
  const missing = [];

  for (let index = 1; index <= shardTotal; index += 1) {
    if (!existsSync(mainCoveredShardBlobPath(index, reportsDir))) {
      missing.push(index);
    }
  }

  return missing;
}

export function formatMissingShardBlobSummary(missingIndices, shardTotal) {
  const missingList = missingIndices.join(", ");
  const expectedPaths = missingIndices
    .map((index) => mainCoveredShardBlobPath(index))
    .join(", ");
  return `Missing UI coverage shard blobs (${missingList}/${shardTotal}): expected ${expectedPaths}`;
}

export function buildUiCoverageMergePhases(options = {}) {
  return buildUiCoveragePhases(options).slice(1);
}

export function buildMainCoveredVitestArgs(options = {}) {
  const mainCoveredMaxWorkers =
    options.mainCoveredMaxWorkers ?? getMainCoveredMaxWorkers(options.env);
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
    "src/features/workflow-activity/components/react-flow-current-activity-card.test.tsx",
  ];

  if (options.shard) {
    args.push(`--shard=${options.shard.index}/${options.shard.total}`);
  }

  return args;
}

export function buildMainCoveredShardPhase(shard, options = {}) {
  return {
    kind: mainCoveredPassKind,
    name: `${mainCoveredPhaseName} (shard ${shard.label})`,
    command: "vitest",
    args: buildMainCoveredVitestArgs({
      ...options,
      blobPath: mainCoveredShardBlobPath(shard.index),
      shard,
    }),
  };
}

export function buildUiCoveragePhases(options = {}) {
  const mainCoveredMaxWorkers =
    options.mainCoveredMaxWorkers ?? getMainCoveredMaxWorkers(options.env);

  return [
    {
      kind: mainCoveredPassKind,
      name: mainCoveredPhaseName,
      command: "vitest",
      args: buildMainCoveredVitestArgs({
        env: options.env,
        mainCoveredMaxWorkers,
      }),
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
  const reportsDir = options.reportsDir ?? ".vitest-reports";
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
