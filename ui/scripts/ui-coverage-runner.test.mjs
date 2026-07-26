import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test, vi } from "vitest";

import {
  buildMainCoveredShardPhase,
  buildUiCoveragePhases,
  cleanCoverageArtifacts,
  defaultCapturedStdoutMaxBuffer,
  defaultMainCoveredMaxWorkers,
  defaultShardMainCoveredMaxWorkers,
  defaultUiCoverageShardTotal,
  formatElapsedMs,
  formatPhaseElapsed,
  getMainCoveredMaxWorkers,
  getUiCoverageShardTotal,
  isolatedReactFlowCoverageFiles,
  mainCoveredPhaseName,
  mainCoveredShardBlobPath,
  parseUiCoverageMerge,
  parseUiCoverageShard,
  parseVitestFileDurationsFromLog,
  phaseLogPrefix,
  rankSlowestTestFiles,
  runTimedPhase,
  runUiCoverage,
  uiCoveragePhases,
} from "./ui-coverage-runner.mjs";
import { formatSlowFileSummaryLines } from "./ui-test-cost-report.mjs";

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

test("uses safe parallelism for the main covered pass only", () => {
  const [mainCoveredPass, isolatedReactFlowPass] = buildUiCoveragePhases({
    mainCoveredMaxWorkers: defaultMainCoveredMaxWorkers,
  });

  expect(mainCoveredPass.args).toContain(
    `--maxWorkers=${defaultMainCoveredMaxWorkers}`,
  );
  expect(mainCoveredPass.args).not.toContain("--maxWorkers=1");
  expect(isolatedReactFlowPass.args).toContain("--maxWorkers=1");
});

test("allows repo-owned coverage command to tune main covered pass workers", () => {
  expect(
    getMainCoveredMaxWorkers({ UI_COVERAGE_MAIN_MAX_WORKERS: "50%" }),
  ).toBe("50%");
  expect(buildUiCoveragePhases({ env: {} })[0].args).toContain(
    `--maxWorkers=${defaultMainCoveredMaxWorkers}`,
  );
});

test("keeps browser-backed and standalone script-style tests outside the main covered pass", () => {
  const [mainCoveredPass, , , standaloneScriptStyleTest] =
    buildUiCoveragePhases({
      mainCoveredMaxWorkers: defaultMainCoveredMaxWorkers,
    });

  expect(mainCoveredPass.args).toEqual(
    expect.arrayContaining([
      "--exclude",
      "integration/*.integration.test.mjs",
      "--exclude",
      "scripts/dashboard-shell-storybook-responsive.test.mjs",
    ]),
  );
  expect(standaloneScriptStyleTest.args).toEqual([
    "run",
    "scripts/dashboard-shell-storybook-responsive.test.mjs",
    "--maxWorkers=1",
  ]);
});

test("keeps the React Flow coverage files isolated from the main covered pass", () => {
  const [mainCoveredPass, isolatedReactFlowPass] = buildUiCoveragePhases({
    mainCoveredMaxWorkers: defaultMainCoveredMaxWorkers,
  });

  for (const reactFlowCoverageFile of isolatedReactFlowCoverageFiles) {
    expect(mainCoveredPass.args).toEqual(
      expect.arrayContaining(["--exclude", reactFlowCoverageFile]),
    );
    expect(isolatedReactFlowPass.args).toContain(reactFlowCoverageFile);
  }
  expect(isolatedReactFlowPass.args).toContain("--maxWorkers=1");
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
  expect(
    formatSlowFileSummaryLines(slowFiles, {
      limit: 3,
      logPrefix: phaseLogPrefix,
      summaryTitle: "Main covered pass slowest test files",
    }),
  ).toEqual([
    `${phaseLogPrefix} Main covered pass slowest test files (top 3):`,
    `${phaseLogPrefix}   src/App.test.tsx 120.00s [app-shell-integration]`,
    `${phaseLogPrefix}   src/features/timeline/state/factoryTimelineStore.test.ts 5.00s [replay-timeline]`,
    `${phaseLogPrefix}   src/i18n/formatters.test.ts 0.12s [uncategorized]`,
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

test("builds shard main pass with vitest shard flag and unique blob output", () => {
  const shard = { index: 3, label: "3/10", total: 10 };
  const phase = buildMainCoveredShardPhase(shard);

  expect(phase.name).toBe(`${mainCoveredPhaseName} (shard 3/10)`);
  expect(phase.args).toContain("--shard=3/10");
  expect(phase.args).toContain("--coverage.clean=true");
  expect(phase.args).not.toContain("--coverage.clean=false");
  expect(phase.args).toContain("--coverage.reportsDirectory=coverage/shard-3");
  expect(phase.args).toContain(
    `--outputFile.blob=${mainCoveredShardBlobPath(3)}`,
  );
  expect(phase.args).not.toContain(
    "--outputFile.blob=.vitest-reports/main.json",
  );
  expect(phase.args).toContain(
    `--maxWorkers=${defaultShardMainCoveredMaxWorkers}`,
  );
  expect(phase.args).toEqual(
    expect.arrayContaining([
      "--exclude",
      "integration/*.integration.test.mjs",
      "--exclude",
      "scripts/dashboard-shell-storybook-responsive.test.mjs",
      "--exclude",
      "scripts/ui-coverage-runner.test.mjs",
      "--exclude",
      "scripts/ui-coverage-runner.shard-merge.test.mjs",
      ...isolatedReactFlowCoverageFiles.flatMap((file) => ["--exclude", file]),
    ]),
  );
});

test("allows UI_COVERAGE_MAIN_MAX_WORKERS to override shard worker default", () => {
  const shard = { index: 1, label: "1/10", total: 10 };
  const phase = buildMainCoveredShardPhase(shard, {
    env: { UI_COVERAGE_MAIN_MAX_WORKERS: "2" },
  });

  expect(phase.args).toContain("--maxWorkers=2");
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
  expect(() =>
    getUiCoverageShardTotal({ UI_COVERAGE_SHARD_TOTAL: "0" }),
  ).toThrow(/positive integer/);
});

test("cleanCoverageArtifacts recreates coverage temp and blob report directories", () => {
  const tempDir = mkdtempSync(join(tmpdir(), "ui-coverage-clean-"));

  try {
    mkdirSync(join(tempDir, "coverage/old"), { recursive: true });
    mkdirSync(join(tempDir, ".vitest-reports/old"), { recursive: true });

    cleanCoverageArtifacts(tempDir);

    expect(existsSync(join(tempDir, "coverage/.tmp"))).toBe(true);
    expect(existsSync(join(tempDir, ".vitest-reports"))).toBe(true);
  } finally {
    rmSync(tempDir, { force: true, recursive: true });
  }
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
  expect(spawn.mock.calls[0][1]).toContain("--shard=2/10");
  expect(spawn.mock.calls[0][1]).toContain(
    `--outputFile.blob=${mainCoveredShardBlobPath(2)}`,
  );
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
      command: "fake-vitest",
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

test("uses a larger buffer when capturing noisy Vitest output", () => {
  const spawn = vi.fn(() => ({ status: 0, stdout: fixtureLogSnippet }));
  const log = vi.spyOn(console, "log").mockImplementation(() => {});

  runTimedPhase(
    {
      args: ["--coverage"],
      command: "vitest",
      name: "Main covered Vitest pass",
    },
    spawn,
    { captureStdout: true },
  );

  expect(spawn.mock.calls[0][2]).toMatchObject({
    encoding: "utf8",
    maxBuffer: defaultCapturedStdoutMaxBuffer,
    stdio: ["inherit", "pipe", "inherit"],
  });

  log.mockRestore();
});
