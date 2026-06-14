import { expect, test, vi } from "vitest";

import {
  browserIntegrationPhaseName,
  buildBrowserIntegrationVitestArgs,
  formatPhaseElapsed,
  phaseLogPrefix,
  runBrowserIntegration,
} from "./ui-integration-runner.mjs";

test("builds stable browser integration vitest args", () => {
  expect(buildBrowserIntegrationVitestArgs()).toEqual([
    "run",
    "integration",
    "--no-file-parallelism",
    "--maxWorkers",
    "1",
  ]);
});

test("formats browser integration phase elapsed output", () => {
  expect(formatPhaseElapsed(browserIntegrationPhaseName, 1500)).toBe(
    `${phaseLogPrefix} ${browserIntegrationPhaseName} elapsed: 1.50s`,
  );
});

test("runBrowserIntegration emits categorized slow-file summary", () => {
  const fixtureStdout = [
    " ✓ integration/event-stream-replay.integration.test.mjs (1 test) 90000ms",
    " ✓ integration/factory-import-second-session.integration.test.mjs (1 test) 120000ms",
  ].join("\n");
  const spawn = vi.fn(() => ({ status: 0, stdout: fixtureStdout }));
  const log = vi.spyOn(console, "log").mockImplementation(() => {});
  const exit = vi.spyOn(process, "exit").mockImplementation(() => {});

  runBrowserIntegration({ spawn });

  expect(spawn).toHaveBeenCalledWith(
    "vitest",
    buildBrowserIntegrationVitestArgs(),
    expect.objectContaining({
      encoding: "utf8",
      stdio: ["inherit", "pipe", "inherit"],
    }),
  );
  expect(log).toHaveBeenCalledWith(
    `${phaseLogPrefix} Browser integration slowest test files (top 2):`,
  );
  expect(log).toHaveBeenCalledWith(
    `${phaseLogPrefix}   integration/factory-import-second-session.integration.test.mjs 120.00s [import-export]`,
  );
  expect(exit).not.toHaveBeenCalled();

  log.mockRestore();
  exit.mockRestore();
});
