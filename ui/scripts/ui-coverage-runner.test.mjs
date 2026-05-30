import { mkdirSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test, vi } from "vitest";

import {
  buildMainCoveredShardPhase,
  buildUiCoverageMergePhases,
  buildUiCoveragePhases,
  defaultMainCoveredMaxWorkers,
  defaultShardMainCoveredMaxWorkers,
  defaultUiCoverageShardTotal,
  findMissingShardBlobIndices,
  formatElapsedMs,
  formatMissingShardBlobSummary,
  formatPhaseElapsed,
  formatSlowFileSummaryLines,
  getMainCoveredMaxWorkers,
  getUiCoverageShardTotal,
  mainCoveredPhaseName,
  mainCoveredShardBlobPath,
  mainCoveredShardReportDir,
  parseUiCoverageMerge,
  parseUiCoverageShard,
  parseVitestFileDurationsFromLog,
  phaseLogPrefix,
  rankSlowestTestFiles,
  runTimedPhase,
  runUiCoverage,
  uiCoveragePhases,
} from "./ui-coverage-runner.mjs";
import { reactFlowCoverageTestPath } from "./bun-coverage-config.mjs";

const fixtureLogSnippet = readFileSync(
  join(
    dirname(fileURLToPath(import.meta.url)),
    "fixtures/vitest-main-pass-log-snippet.txt",
  ),
  "utf8",
);

test("formats stable elapsed output for comparable coverage phases", () => {
  expect(formatElapsedMs(1234)).toBe("1.23s");
  expect(formatPhaseElapsed("Main covered Vitest pass", 2500)).toBe(
    `${phaseLogPrefix} Main covered Vitest pass elapsed: 2.50s`,
  );
});

test("keeps coverage phase names stable and explicit", () => {
  expect(uiCoveragePhases.map((phase) => phase.name)).toEqual([
    mainCoveredPhaseName,
    "Isolated React Flow covered pass",
    "Blob report merge pass",
    "Standalone script-style test",
  ]);
});

test("runs the main covered pass through the Bun coverage orchestrator", () => {
  const [mainCoveredPass] = buildUiCoveragePhases({
    mainCoveredMaxWorkers: defaultMainCoveredMaxWorkers,
  });

  expect(mainCoveredPass).toMatchObject({
    kind: "main-covered",
    name: "Main covered Vitest pass",
    command: "node",
    args: ["scripts/run-bun-coverage-main.mjs"],
  });
});

test("uses safe parallelism for the isolated React Flow covered pass only", () => {
  const [, isolatedReactFlowPass] = buildUiCoveragePhases({
    mainCoveredMaxWorkers: defaultMainCoveredMaxWorkers,
  });

  expect(isolatedReactFlowPass.command).toBe("bun");
  expect(isolatedReactFlowPass.args).toContain("--parallel=1");
  expect(isolatedReactFlowPass.args).toContain(reactFlowCoverageTestPath);
});

test("allows repo-owned coverage command to tune main covered pass workers", () => {
  expect(
    getMainCoveredMaxWorkers({ UI_COVERAGE_MAIN_MAX_WORKERS: "50%" }),
  ).toBe("50%");
  expect(getMainCoveredMaxWorkers({})).toBe(defaultMainCoveredMaxWorkers);
});

test("keeps browser-backed and standalone script-style tests outside the main covered pass", () => {
  const [, , , standaloneScriptStyleTest] = buildUiCoveragePhases({
    mainCoveredMaxWorkers: defaultMainCoveredMaxWorkers,
  });

  expect(standaloneScriptStyleTest.args).toEqual([
    "run",
    "--config",
    "vitest.config.ts",
    "scripts/dashboard-shell-storybook-responsive.test.mjs",
    "--maxWorkers=1",
  ]);
});

test("keeps the React Flow coverage file isolated from the main covered pass", () => {
  const [, isolatedReactFlowPass] = buildUiCoveragePhases({
    mainCoveredMaxWorkers: defaultMainCoveredMaxWorkers,
  });

  expect(isolatedReactFlowPass.args).toContain(reactFlowCoverageTestPath);
  expect(isolatedReactFlowPass.args).toContain("--parallel=1");
});

test("merges Bun lcov reports and enforces thresholds in the merge pass", () => {
  const [, , mergePass] = buildUiCoveragePhases({
    mainCoveredMaxWorkers: defaultMainCoveredMaxWorkers,
  });

  expect(mergePass).toEqual({
    name: "Blob report merge pass",
    command: "node",
    args: ["scripts/merge-bun-coverage-thresholds.mjs"],
  });
});

test("parses vitest default reporter file durations from a fixture log snippet", () => {
  expect(parseVitestFileDurationsFromLog(fixtureLogSnippet)).toEqual([
    { durationMs: 3, path: "src/api/baseUrl.test.ts" },
    { durationMs: 26, path: "src/components/ui/formatters.test.ts" },
    { durationMs: 120_000, path: "src/App.test.tsx" },
    {
      durationMs: 5000,
      path: "src/features/timeline/state/factoryTimelineStore.test.ts",
    },
    { durationMs: 116, path: "src/i18n/formatters.test.ts" },
  ]);
});

test("formats a bounded slow-file summary with stable labels", () => {
  const slowFiles = rankSlowestTestFiles(
    parseVitestFileDurationsFromLog(fixtureLogSnippet),
    3,
  );

  expect(slowFiles).toEqual([
    { durationMs: 120_000, path: "src/App.test.tsx" },
    {
      durationMs: 5000,
      path: "src/features/timeline/state/factoryTimelineStore.test.ts",
    },
    { durationMs: 116, path: "src/i18n/formatters.test.ts" },
  ]);
  expect(formatSlowFileSummaryLines(slowFiles, { limit: 3 })).toEqual([
    `${phaseLogPrefix} Main covered pass slowest test files (top 3):`,
    `${phaseLogPrefix}   src/App.test.tsx 120.00s`,
    `${phaseLogPrefix}   src/features/timeline/state/factoryTimelineStore.test.ts 5.00s`,
    `${phaseLogPrefix}   src/i18n/formatters.test.ts 0.12s`,
  ]);
});

test("parses UI_COVERAGE_SHARD index/total pairs", () => {
  expect(parseUiCoverageShard({ UI_COVERAGE_SHARD: "3/10" })).toEqual({
    index: 3,
    label: "3/10",
    total: 10,
  });
  expect(parseUiCoverageShard({ UI_COVERAGE_SHARD: " 1/1 " })).toEqual({
    index: 1,
    label: "1/1",
    total: 1,
  });
  expect(parseUiCoverageShard({})).toBeNull();
});

test("rejects invalid UI_COVERAGE_SHARD values", () => {
  expect(() => parseUiCoverageShard({ UI_COVERAGE_SHARD: "shard-3" })).toThrow(
    /expected format index\/total/,
  );
  expect(() => parseUiCoverageShard({ UI_COVERAGE_SHARD: "0/10" })).toThrow(
    /index must be between 1 and 10/,
  );
  expect(() => parseUiCoverageShard({ UI_COVERAGE_SHARD: "11/10" })).toThrow(
    /index must be between 1 and 10/,
  );
});

test("builds shard main pass through the Bun coverage orchestrator", () => {
  const shard = { index: 3, label: "3/10", total: 10 };
  const phase = buildMainCoveredShardPhase(shard);

  expect(phase).toEqual({
    kind: "main-covered",
    name: `${mainCoveredPhaseName} (shard 3/10)`,
    command: "node",
    args: ["scripts/run-bun-coverage-main.mjs"],
  });
  expect(mainCoveredShardReportDir(3)).toBe(".bun-coverage-reports/main-shard-3");
});

test("parses UI_COVERAGE_MERGE truthy values", () => {
  expect(parseUiCoverageMerge({ UI_COVERAGE_MERGE: "1" })).toBe(true);
  expect(parseUiCoverageMerge({ UI_COVERAGE_MERGE: "true" })).toBe(true);
  expect(parseUiCoverageMerge({})).toBe(false);
  expect(parseUiCoverageMerge({ UI_COVERAGE_MERGE: "0" })).toBe(false);
});

test("defaults and validates UI_COVERAGE_SHARD_TOTAL for merge mode", () => {
  expect(getUiCoverageShardTotal({})).toBe(defaultUiCoverageShardTotal);
  expect(getUiCoverageShardTotal({ UI_COVERAGE_SHARD_TOTAL: "3" })).toBe(3);
  expect(() => getUiCoverageShardTotal({ UI_COVERAGE_SHARD_TOTAL: "0" })).toThrow(
    /positive integer/,
  );
});

test("buildUiCoverageMergePhases runs follow-on phases without the main pass", () => {
  expect(buildUiCoverageMergePhases().map((phase) => phase.name)).toEqual([
    "Isolated React Flow covered pass",
    "Blob report merge pass",
    "Standalone script-style test",
  ]);
});

function writeShardLcov(reportsDir, shardIndex) {
  const shardDir = mainCoveredShardReportDir(shardIndex, reportsDir);
  mkdirSync(shardDir, { recursive: true });
  writeFileSync(join(shardDir, "lcov.info"), "TN:\nend_of_record\n");
}

test("findMissingShardBlobIndices reports absent shard reports", () => {
  const reportsDir = mkdtempSync(join(tmpdir(), "ui-coverage-reports-"));
  writeShardLcov(reportsDir, 1);

  expect(findMissingShardBlobIndices(2, { reportsDir })).toEqual([2]);
  expect(formatMissingShardBlobSummary([2], 2)).toContain("main-shard-2");
});

test("runUiCoverage in merge mode runs follow-on phases when shard reports exist", () => {
  const reportsDir = mkdtempSync(join(tmpdir(), "ui-coverage-reports-"));
  writeShardLcov(reportsDir, 1);
  const spawn = vi.fn(() => ({ status: 0 }));
  const exit = vi.spyOn(process, "exit").mockImplementation(() => {});

  runUiCoverage(uiCoveragePhases, {
    env: { UI_COVERAGE_MERGE: "1", UI_COVERAGE_SHARD_TOTAL: "1" },
    reportsDir,
    spawn,
  });

  expect(spawn).toHaveBeenCalledTimes(3);
  expect(spawn.mock.calls.map(([, args]) => args)).toEqual(
    expect.arrayContaining([
      ["scripts/merge-bun-coverage-thresholds.mjs"],
    ]),
  );
  expect(exit).not.toHaveBeenCalled();

  exit.mockRestore();
});

test("runUiCoverage in merge mode exits before phases when shard blobs are missing", () => {
  const reportsDir = mkdtempSync(join(tmpdir(), "ui-coverage-reports-"));
  const spawn = vi.fn(() => ({ status: 0 }));
  const exit = vi.spyOn(process, "exit").mockImplementation((code) => {
    throw new Error(`process.exit:${code}`);
  });
  const errorLog = vi.spyOn(console, "error").mockImplementation(() => {});

  expect(() =>
    runUiCoverage(uiCoveragePhases, {
      env: { UI_COVERAGE_MERGE: "1", UI_COVERAGE_SHARD_TOTAL: "2" },
      reportsDir,
      spawn,
    }),
  ).toThrow(/process.exit:1/);

  expect(spawn).not.toHaveBeenCalled();
  expect(errorLog).toHaveBeenCalledWith(
    expect.stringContaining("Missing UI coverage shard reports"),
  );

  errorLog.mockRestore();
  exit.mockRestore();
});

test("rejects setting UI_COVERAGE_SHARD and UI_COVERAGE_MERGE together", () => {
  expect(() =>
    runUiCoverage(uiCoveragePhases, {
      env: { UI_COVERAGE_SHARD: "1/10", UI_COVERAGE_MERGE: "1" },
      spawn: vi.fn(() => ({ status: 0 })),
    }),
  ).toThrow(/cannot both be set/);
});

test("runUiCoverage in shard mode runs only the shard main pass", () => {
  const spawn = vi.fn(() => ({ status: 0, stdout: fixtureLogSnippet }));
  const log = vi.spyOn(console, "log").mockImplementation(() => {});
  const exit = vi.spyOn(process, "exit").mockImplementation(() => {});

  runUiCoverage(uiCoveragePhases, {
    env: { UI_COVERAGE_SHARD: "2/10" },
    spawn,
  });

  expect(spawn).toHaveBeenCalledTimes(1);
  expect(spawn.mock.calls[0]).toEqual([
    "node",
    ["scripts/run-bun-coverage-main.mjs"],
    expect.any(Object),
  ]);
  expect(log).toHaveBeenCalledWith(
    expect.stringContaining("Main covered Vitest pass (shard 2/10) elapsed:"),
  );
  expect(exit).not.toHaveBeenCalled();

  log.mockRestore();
  exit.mockRestore();
});

test("emits elapsed output before returning a failing phase status", () => {
  const log = vi.spyOn(console, "log").mockImplementation(() => {});
  const { status } = runTimedPhase(
    {
      args: ["--fails"],
      command: "fake-runner",
      name: "Failing covered pass",
    },
    () => ({ status: 7 }),
  );

  expect(status).toBe(7);
  expect(log).toHaveBeenCalledWith(
    expect.stringContaining("Failing covered pass elapsed:"),
  );

  log.mockRestore();
});
