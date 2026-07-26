import { spawnSync } from "node:child_process";
import {
  existsSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { join } from "node:path";
import { performance } from "node:perf_hooks";

import {
  defaultCapturedStdoutMaxBuffer,
  defaultSlowFileSummaryLimit,
  logSlowFileSummary as emitSlowFileSummary,
  formatElapsedMs,
  formatSlowFileSummaryLines,
  mergeFileDurations,
  parseVitestFileDurationsFromLog,
  rankSlowestTestFiles,
} from "./ui-test-cost-report.mjs";

export const phaseLogPrefix = "[ui-coverage]";
export const mainCoveredPhaseName = "Main covered Vitest pass";
export const defaultMainCoveredMaxWorkers = "4";
export const defaultShardMainCoveredMaxWorkers = "1";
export { defaultCapturedStdoutMaxBuffer, defaultSlowFileSummaryLimit };
export const defaultUiCoverageShardTotal = 4;
export const defaultTimingReportsDir = ".vitest-report-timings";
export const uiPerformanceTestPattern = "**/performance/*.test.ts";

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

export function mainCoveredShardBlobPath(
  shardIndex,
  reportsDir = ".vitest-reports",
) {
  return join(reportsDir, `main-shard-${shardIndex}.json`);
}

export function mainCoveredShardTimingBlobPath(
  shardIndex,
  timingReportsDir = defaultTimingReportsDir,
) {
  return join(timingReportsDir, `main-shard-${shardIndex}-timings.json`);
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

export function allowedShardReportBasenames(shardTotal) {
  const allowed = new Set();

  for (let index = 1; index <= shardTotal; index += 1) {
    allowed.add(`main-shard-${index}.json`);
  }

  return allowed;
}

export function sanitizeVitestReportsDirForShardMerge(
  shardTotal,
  { reportsDir = ".vitest-reports" } = {},
) {
  if (!existsSync(reportsDir)) {
    mkdirSync(reportsDir, { recursive: true });
    return [];
  }

  const allowed = allowedShardReportBasenames(shardTotal);
  const removed = [];

  for (const entry of readdirSync(reportsDir)) {
    if (allowed.has(entry)) {
      continue;
    }

    rmSync(join(reportsDir, entry), { force: true, recursive: true });
    removed.push(entry);
  }

  return removed;
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
  const shard = options.shard ?? null;
  const mainCoveredMaxWorkers =
    options.mainCoveredMaxWorkers ??
    getMainCoveredMaxWorkers(options.env, { shard: Boolean(shard) });
  const blobPath = options.blobPath ?? ".vitest-reports/main.json";
  const coverageCleanFlag = shard
    ? "--coverage.clean=true"
    : "--coverage.clean=false";
  const args = [
    "run",
    "--config=vitest.lanes.config.ts",
    "--project=dashboard-unit",
    "--coverage",
    coverageCleanFlag,
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
    "scripts/ui-coverage-runner.shard-merge.test.mjs",
    "--exclude",
    uiPerformanceTestPattern,
  ];

  if (options.shard) {
    args.push(`--shard=${options.shard.index}/${options.shard.total}`);
    args.push(
      `--coverage.reportsDirectory=coverage/shard-${options.shard.index}`,
    );
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
      name: "Blob report merge pass",
      command: "vitest",
      args: [
        "--mergeReports",
        ".vitest-reports",
        "--coverage",
        "--passWithNoTests",
      ],
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

export function writeShardTimingArtifact(
  shardIndex,
  fileDurations,
  timingReportsDir = defaultTimingReportsDir,
) {
  mkdirSync(timingReportsDir, { recursive: true });
  writeFileSync(
    mainCoveredShardTimingBlobPath(shardIndex, timingReportsDir),
    `${JSON.stringify(fileDurations, null, 2)}\n`,
  );
}

export function readShardTimingArtifacts(
  shardTotal,
  timingReportsDir = defaultTimingReportsDir,
) {
  const fileDurations = [];

  for (let index = 1; index <= shardTotal; index += 1) {
    const timingPath = mainCoveredShardTimingBlobPath(index, timingReportsDir);
    if (!existsSync(timingPath)) {
      continue;
    }

    fileDurations.push(...JSON.parse(readFileSync(timingPath, "utf8")));
  }

  return fileDurations;
}

export function formatPhaseElapsed(phaseName, elapsedMs) {
  return `${phaseLogPrefix} ${phaseName} elapsed: ${formatElapsedMs(elapsedMs)}`;
}

export function cleanCoverageArtifacts(rootDirectory = ".") {
  const coverageDirectory = join(rootDirectory, "coverage");
  const reportsDirectory = join(rootDirectory, ".vitest-reports");

  rmSync(coverageDirectory, { force: true, recursive: true });
  rmSync(reportsDirectory, { force: true, recursive: true });
  rmSync(join(rootDirectory, defaultTimingReportsDir), {
    force: true,
    recursive: true,
  });
  mkdirSync(join(coverageDirectory, ".tmp"), { recursive: true });
  mkdirSync(reportsDirectory, { recursive: true });
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

export function logMergedShardSlowFileSummary(
  shardTotal,
  timingReportsDir = defaultTimingReportsDir,
) {
  const merged = mergeFileDurations(
    readShardTimingArtifacts(shardTotal, timingReportsDir),
  );
  const slowFiles = rankSlowestTestFiles(merged);

  for (const line of formatSlowFileSummaryLines(slowFiles, {
    logPrefix: phaseLogPrefix,
    summaryTitle: "Merged main covered pass slowest test files",
  })) {
    console.log(line);
  }
}

export function runUiCoverageShard(shard, options = {}) {
  const spawn = options.spawn ?? spawnSync;
  rmSync(`coverage/shard-${shard.index}`, { force: true, recursive: true });
  const phase = buildMainCoveredShardPhase(shard, options);
  const { capturedStdout, status } = runTimedPhase(phase, spawn, {
    captureStdout: true,
  });

  if (status !== 0) {
    process.exit(status);
  }

  const fileDurations = parseVitestFileDurationsFromLog(capturedStdout);
  writeShardTimingArtifact(
    shard.index,
    fileDurations,
    options.timingReportsDir ?? defaultTimingReportsDir,
  );
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

  sanitizeVitestReportsDirForShardMerge(shardTotal, { reportsDir });

  rmSync("coverage", { force: true, recursive: true });

  for (const phase of buildUiCoverageMergePhases(options)) {
    const { status } = runTimedPhase(phase, spawn);

    if (status !== 0) {
      process.exit(status);
    }
  }

  logMergedShardSlowFileSummary(
    shardTotal,
    options.timingReportsDir ?? defaultTimingReportsDir,
  );
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
