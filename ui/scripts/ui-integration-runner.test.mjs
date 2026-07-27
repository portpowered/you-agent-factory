import { expect, test, vi } from "vitest";

import {
  browserIntegrationMaxWorkers,
  browserIntegrationPhaseName,
  buildBrowserIntegrationVitestArgs,
  buildFocusedBrowserIntegrationVitestArgs,
  formatPhaseElapsed,
  phaseLogPrefix,
  runBrowserIntegration,
  runFocusedBrowserIntegration,
} from "./ui-integration-runner.mjs";
import {
  durableSessionRealBackendIntegrationFiles,
  mockedBackendBrowserIntegrationFiles,
} from "./ui-integration-targets.mjs";

test("builds stable browser integration vitest args", () => {
  expect(buildBrowserIntegrationVitestArgs({})).toEqual([
    "run",
    ...mockedBackendBrowserIntegrationFiles,
    "--fileParallelism",
    "--maxWorkers",
    "3",
    "--reporter=verbose",
  ]);
});

test("accepts a measured mocked-browser worker override", () => {
  expect(
    browserIntegrationMaxWorkers({ UI_BROWSER_INTEGRATION_MAX_WORKERS: "3" }),
  ).toBe(3);
  expect(() =>
    browserIntegrationMaxWorkers({
      UI_BROWSER_INTEGRATION_MAX_WORKERS: "not-a-worker-count",
    }),
  ).toThrow(/must be a positive integer/);
});

test("builds focused browser integration vitest args for durable session proof", () => {
  expect(
    buildFocusedBrowserIntegrationVitestArgs(
      durableSessionRealBackendIntegrationFiles,
    ),
  ).toEqual([
    "run",
    ...durableSessionRealBackendIntegrationFiles,
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
      env: expect.objectContaining({
        AGENT_FACTORY_BROWSER_ARTIFACT_WORKER_ISOLATION: "true",
      }),
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

test("runFocusedBrowserIntegration runs only the requested integration files", () => {
  const fixtureStdout =
    " ✓ integration/durable-session-real-backend.integration.test.mjs (3 tests) 120000ms";
  const spawn = vi.fn(() => ({ status: 0, stdout: fixtureStdout }));
  const log = vi.spyOn(console, "log").mockImplementation(() => {});
  const exit = vi.spyOn(process, "exit").mockImplementation(() => {});

  runFocusedBrowserIntegration(durableSessionRealBackendIntegrationFiles, {
    spawn,
    phaseName: "Durable session real-backend browser integration Vitest pass",
  });

  expect(spawn).toHaveBeenCalledWith(
    "vitest",
    buildFocusedBrowserIntegrationVitestArgs(
      durableSessionRealBackendIntegrationFiles,
    ),
    expect.objectContaining({
      encoding: "utf8",
      stdio: ["inherit", "pipe", "inherit"],
    }),
  );
  expect(exit).not.toHaveBeenCalled();

  log.mockRestore();
  exit.mockRestore();
});
