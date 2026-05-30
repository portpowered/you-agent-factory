import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test, vi } from "vitest";

import {
  buildUiCoveragePhases,
  defaultMainCoveredMaxWorkers,
  formatElapsedMs,
  formatPhaseElapsed,
  formatSlowFileSummaryLines,
  getMainCoveredMaxWorkers,
  mainCoveredPhaseName,
  parseVitestFileDurationsFromLog,
  phaseLogPrefix,
  rankSlowestTestFiles,
  runTimedPhase,
  uiCoveragePhases,
} from "./ui-coverage-runner.mjs";

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

test("keeps the React Flow coverage file isolated from the main covered pass", () => {
  const [mainCoveredPass, isolatedReactFlowPass] = buildUiCoveragePhases({
    mainCoveredMaxWorkers: defaultMainCoveredMaxWorkers,
  });
  const reactFlowCoverageFile =
    "src/features/workflow-activity/components/react-flow-current-activity-card.test.tsx";

  expect(mainCoveredPass.args).toEqual(
    expect.arrayContaining(["--exclude", reactFlowCoverageFile]),
  );
  expect(isolatedReactFlowPass.args).toContain(reactFlowCoverageFile);
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
  expect(formatSlowFileSummaryLines(slowFiles, { limit: 3 })).toEqual([
    `${phaseLogPrefix} Main covered pass slowest test files (top 3):`,
    `${phaseLogPrefix}   src/App.test.tsx 120.00s`,
    `${phaseLogPrefix}   src/features/timeline/state/factoryTimelineStore.test.ts 5.00s`,
    `${phaseLogPrefix}   src/i18n/formatters.test.ts 0.12s`,
  ]);
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
