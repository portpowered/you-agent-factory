import { existsSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, test, vi } from "vitest";

import {
  buildUiCoverageMergePhases,
  findMissingShardBlobIndices,
  formatMissingShardBlobSummary,
  logMergedShardSlowFileSummary,
  mainCoveredShardBlobPath,
  phaseLogPrefix,
  runUiCoverage,
  sanitizeVitestReportsDirForShardMerge,
  uiCoveragePhases,
  writeShardTimingArtifact,
} from "./ui-coverage-runner.mjs";

test("buildUiCoverageMergePhases runs follow-on phases without the main pass", () => {
  expect(buildUiCoverageMergePhases().map((phase) => phase.name)).toEqual([
    "Blob report merge pass",
    "Standalone script-style test",
  ]);
});

test("sanitizeVitestReportsDirForShardMerge removes stale monolithic and out-of-range shard blobs", () => {
  const reportsDir = mkdtempSync(join(tmpdir(), "ui-coverage-reports-"));
  writeFileSync(mainCoveredShardBlobPath(1, reportsDir), "{}");
  writeFileSync(
    join(reportsDir, "main-shard-1-timings.json"),
    JSON.stringify([{ durationMs: 1000, path: "src/a.test.ts" }]),
  );
  writeFileSync(join(reportsDir, "main.json"), "{}");
  writeFileSync(join(reportsDir, "main-shard-3.json"), "{}");
  writeFileSync(
    join(reportsDir, "react-flow-current-activity-card.json"),
    "{}",
  );

  const removed = sanitizeVitestReportsDirForShardMerge(2, { reportsDir });

  expect(removed.sort()).toEqual([
    "main-shard-1-timings.json",
    "main-shard-3.json",
    "main.json",
    "react-flow-current-activity-card.json",
  ]);
  expect(existsSync(mainCoveredShardBlobPath(1, reportsDir))).toBe(true);
  expect(existsSync(join(reportsDir, "main-shard-1-timings.json"))).toBe(false);
  expect(existsSync(join(reportsDir, "main.json"))).toBe(false);
  expect(existsSync(join(reportsDir, "main-shard-3.json"))).toBe(false);
});

test("runUiCoverageMerge ignores stale blobs before running merge phases", () => {
  const reportsDir = mkdtempSync(join(tmpdir(), "ui-coverage-reports-"));
  const timingReportsDir = mkdtempSync(join(tmpdir(), "ui-coverage-timings-"));
  writeFileSync(mainCoveredShardBlobPath(1, reportsDir), "{}");
  writeShardTimingArtifact(
    1,
    [{ durationMs: 1000, path: "src/slow.test.ts" }],
    timingReportsDir,
  );
  writeFileSync(join(reportsDir, "main.json"), "{}");
  writeFileSync(join(reportsDir, "main-shard-99.json"), "{}");
  writeFileSync(
    join(reportsDir, "main-shard-1-timings.json"),
    JSON.stringify([{ durationMs: 1, path: "src/stale.test.ts" }]),
  );
  const spawn = vi.fn(() => ({ status: 0 }));
  const exit = vi.spyOn(process, "exit").mockImplementation(() => {});
  const log = vi.spyOn(console, "log").mockImplementation(() => {});

  runUiCoverage(uiCoveragePhases, {
    env: { UI_COVERAGE_MERGE: "1", UI_COVERAGE_SHARD_TOTAL: "1" },
    reportsDir,
    timingReportsDir,
    spawn,
  });

  expect(existsSync(join(reportsDir, "main.json"))).toBe(false);
  expect(existsSync(join(reportsDir, "main-shard-99.json"))).toBe(false);
  expect(existsSync(join(reportsDir, "main-shard-1-timings.json"))).toBe(false);
  expect(spawn).toHaveBeenCalledTimes(2);
  expect(exit).not.toHaveBeenCalled();

  log.mockRestore();
  exit.mockRestore();
});

test("findMissingShardBlobIndices reports absent shard blobs", () => {
  const reportsDir = mkdtempSync(join(tmpdir(), "ui-coverage-reports-"));
  writeFileSync(mainCoveredShardBlobPath(1, reportsDir), "{}");

  expect(findMissingShardBlobIndices(2, { reportsDir })).toEqual([2]);
  expect(formatMissingShardBlobSummary([2], 2)).toContain("main-shard-2.json");
});

test("logMergedShardSlowFileSummary ranks merged shard timing artifacts", () => {
  const timingReportsDir = mkdtempSync(join(tmpdir(), "ui-coverage-timings-"));
  writeShardTimingArtifact(
    1,
    [{ durationMs: 1000, path: "src/a.test.ts" }],
    timingReportsDir,
  );
  writeShardTimingArtifact(
    2,
    [
      { durationMs: 5000, path: "src/App.test.tsx" },
      { durationMs: 2000, path: "src/a.test.ts" },
    ],
    timingReportsDir,
  );
  const log = vi.spyOn(console, "log").mockImplementation(() => {});

  logMergedShardSlowFileSummary(2, timingReportsDir);

  expect(log).toHaveBeenCalledWith(
    `${phaseLogPrefix} Merged main covered pass slowest test files (top 2):`,
  );
  expect(log).toHaveBeenCalledWith(
    `${phaseLogPrefix}   src/App.test.tsx 5.00s [app-shell-integration]`,
  );
  expect(log).toHaveBeenCalledWith(
    `${phaseLogPrefix}   src/a.test.ts 2.00s [uncategorized]`,
  );

  log.mockRestore();
});

test("runUiCoverage in merge mode runs follow-on phases when shard blobs exist", () => {
  const reportsDir = mkdtempSync(join(tmpdir(), "ui-coverage-reports-"));
  const timingReportsDir = mkdtempSync(join(tmpdir(), "ui-coverage-timings-"));
  writeFileSync(mainCoveredShardBlobPath(1, reportsDir), "{}");
  writeShardTimingArtifact(
    1,
    [{ durationMs: 1000, path: "src/slow.test.ts" }],
    timingReportsDir,
  );
  const spawn = vi.fn(() => ({ status: 0 }));
  const exit = vi.spyOn(process, "exit").mockImplementation(() => {});
  const log = vi.spyOn(console, "log").mockImplementation(() => {});

  runUiCoverage(uiCoveragePhases, {
    env: { UI_COVERAGE_MERGE: "1", UI_COVERAGE_SHARD_TOTAL: "1" },
    reportsDir,
    timingReportsDir,
    spawn,
  });

  expect(spawn).toHaveBeenCalledTimes(2);
  expect(log).toHaveBeenCalledWith(
    `${phaseLogPrefix} Merged main covered pass slowest test files (top 1):`,
  );
  expect(spawn.mock.calls.map(([, args]) => args)).toEqual(
    expect.arrayContaining([
      expect.arrayContaining([
        "--mergeReports",
        ".vitest-reports",
        "--coverage",
      ]),
    ]),
  );
  expect(exit).not.toHaveBeenCalled();

  log.mockRestore();
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
    expect.stringContaining("Missing UI coverage shard blobs"),
  );

  errorLog.mockRestore();
  exit.mockRestore();
});
