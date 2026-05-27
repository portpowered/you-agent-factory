import { expect, test, vi } from "vitest";

import {
  formatElapsedMs,
  formatPhaseElapsed,
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
