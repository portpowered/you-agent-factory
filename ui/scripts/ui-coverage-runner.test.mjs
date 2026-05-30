import { expect, test, vi } from "vitest";

import {
  buildUiCoveragePhases,
  defaultMainCoveredMaxWorkers,
  formatElapsedMs,
  formatPhaseElapsed,
  getMainCoveredMaxWorkers,
  phaseLogPrefix,
  runTimedPhase,
  uiCoveragePhases,
} from "./ui-coverage-runner.mjs";
import { reactFlowCoverageTestPath } from "./bun-coverage-config.mjs";

test("formats stable elapsed output for comparable coverage phases", () => {
  expect(formatElapsedMs(1234)).toBe("1.23s");
  expect(formatPhaseElapsed("Main covered Vitest pass", 2500)).toBe(
    `${phaseLogPrefix} Main covered Vitest pass elapsed: 2.50s`,
  );
});

test("keeps coverage phase names stable and explicit", () => {
  expect(uiCoveragePhases.map((phase) => phase.name)).toEqual([
    "Main covered Vitest pass",
    "Isolated React Flow covered pass",
    "Blob report merge pass",
    "Standalone script-style test",
  ]);
});

test("runs the main covered pass through the Bun coverage orchestrator", () => {
  const [mainCoveredPass] = buildUiCoveragePhases({
    mainCoveredMaxWorkers: defaultMainCoveredMaxWorkers,
  });

  expect(mainCoveredPass).toEqual({
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

test("emits elapsed output before returning a failing phase status", () => {
  const log = vi.spyOn(console, "log").mockImplementation(() => {});
  const status = runTimedPhase(
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
