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
  buildUiCoveragePhases,
  cleanCoverageArtifacts,
  defaultCapturedStdoutMaxBuffer,
  defaultMainCoveredMaxWorkers,
  formatElapsedMs,
  formatPhaseElapsed,
  getMainCoveredMaxWorkers,
  mainCoveredPhaseName,
  parseVitestFileDurationsFromLog,
  phaseLogPrefix,
  rankSlowestTestFiles,
  runTimedPhase,
  runUiCoverage,
  uiCoveragePhases,
  uiPerformanceTestPattern,
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

test("uses one covered pass followed by the standalone script-style check", () => {
  expect(uiCoveragePhases.map((phase) => phase.name)).toEqual([
    mainCoveredPhaseName,
    "Standalone script-style test",
  ]);
});

test("uses configured parallelism for the monolithic Node covered pass", () => {
  const [mainCoveredPass] = buildUiCoveragePhases({
    mainCoveredMaxWorkers: defaultMainCoveredMaxWorkers,
  });

  expect(mainCoveredPass.args).toContain(
    `--maxWorkers=${defaultMainCoveredMaxWorkers}`,
  );
  expect(mainCoveredPass.args).not.toEqual(
    expect.arrayContaining([expect.stringMatching(/^--shard=/)]),
  );
  expect(mainCoveredPass.args).not.toEqual(
    expect.arrayContaining([expect.stringMatching(/^--outputFile\.blob=/)]),
  );
});

test("allows the repo-owned coverage command to tune workers", () => {
  expect(
    getMainCoveredMaxWorkers({ UI_COVERAGE_MAIN_MAX_WORKERS: "50%" }),
  ).toBe("50%");
  expect(buildUiCoveragePhases({ env: {} })[0].args).toContain(
    `--maxWorkers=${defaultMainCoveredMaxWorkers}`,
  );
});

test("keeps browser, script-style, and performance tests outside unit coverage", () => {
  const [mainCoveredPass, standaloneScriptStyleTest] = buildUiCoveragePhases();

  expect(mainCoveredPass.args).toEqual(
    expect.arrayContaining([
      "--config=vitest.lanes.config.ts",
      "--project=dashboard-unit",
      "--coverage",
      "--exclude",
      "integration/*.integration.test.mjs",
      "--exclude",
      "scripts/dashboard-shell-storybook-responsive.test.mjs",
      "--exclude",
      uiPerformanceTestPattern,
    ]),
  );
  expect(standaloneScriptStyleTest.args).toEqual([
    "run",
    "scripts/dashboard-shell-storybook-responsive.test.mjs",
    "--maxWorkers=1",
  ]);
});

test("parses and ranks Vitest default-reporter file durations", () => {
  const parsed = parseVitestFileDurationsFromLog(fixtureLogSnippet);
  expect(parsed).toEqual([
    { durationMs: 3, path: "src/api/baseUrl.test.ts" },
    { durationMs: 26, path: "src/components/ui/formatters.test.ts" },
    { durationMs: 120_000, path: "src/App.test.tsx" },
    {
      durationMs: 5000,
      path: "src/features/timeline/state/factoryTimelineStore.test.ts",
    },
    { durationMs: 116, path: "src/i18n/formatters.test.ts" },
  ]);

  expect(rankSlowestTestFiles(parsed, 3)).toEqual([
    { durationMs: 120_000, path: "src/App.test.tsx" },
    {
      durationMs: 5000,
      path: "src/features/timeline/state/factoryTimelineStore.test.ts",
    },
    { durationMs: 116, path: "src/i18n/formatters.test.ts" },
  ]);
});

test("formats bounded slow-file summaries with stable categories", () => {
  const slowFiles = rankSlowestTestFiles(
    parseVitestFileDurationsFromLog(fixtureLogSnippet),
    3,
  );

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

test("cleanCoverageArtifacts removes legacy shard artifacts and recreates coverage temp", () => {
  const tempDir = mkdtempSync(join(tmpdir(), "ui-coverage-clean-"));

  try {
    mkdirSync(join(tempDir, "coverage/shard-1"), { recursive: true });
    mkdirSync(join(tempDir, ".vitest-reports"), { recursive: true });
    mkdirSync(join(tempDir, ".vitest-report-timings"), { recursive: true });

    cleanCoverageArtifacts(tempDir);

    expect(existsSync(join(tempDir, "coverage/.tmp"))).toBe(true);
    expect(existsSync(join(tempDir, ".vitest-reports"))).toBe(false);
    expect(existsSync(join(tempDir, ".vitest-report-timings"))).toBe(false);
  } finally {
    rmSync(tempDir, { force: true, recursive: true });
  }
});

test("runUiCoverage executes the monolithic pass and standalone check", () => {
  const rootDirectory = mkdtempSync(join(tmpdir(), "ui-coverage-run-"));
  const spawn = vi.fn(() => ({ status: 0, stdout: fixtureLogSnippet }));
  const log = vi.spyOn(console, "log").mockImplementation(() => {});
  const exit = vi.spyOn(process, "exit").mockImplementation(() => {});

  runUiCoverage(uiCoveragePhases, { rootDirectory, spawn });

  expect(spawn).toHaveBeenCalledTimes(2);
  expect(spawn.mock.calls[0][1]).not.toEqual(
    expect.arrayContaining([expect.stringMatching(/^--shard=/)]),
  );
  expect(log).toHaveBeenCalledWith(
    expect.stringContaining("Main covered pass slowest test files"),
  );
  expect(exit).not.toHaveBeenCalled();

  log.mockRestore();
  exit.mockRestore();
  rmSync(rootDirectory, { force: true, recursive: true });
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
