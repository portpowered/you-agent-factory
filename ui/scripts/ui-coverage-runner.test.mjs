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

test("emits elapsed output before returning a failing phase status", () => {
  const log = vi.spyOn(console, "log").mockImplementation(() => {});
  const status = runTimedPhase(
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
