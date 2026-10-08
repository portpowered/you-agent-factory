import { mkdtemp, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { expect, test, vi } from "vitest";
import { realBackendHarnessArtifactEnvironmentVariable } from "../integration/browser-test-harness.mjs";
import {
  browserIntegrationMaxWorkers,
  browserIntegrationPhaseName,
  buildBrowserIntegrationVitestArgs,
  buildFocusedBrowserIntegrationVitestArgs,
  formatPhaseElapsed,
  phaseLogPrefix,
  runBrowserIntegration,
  runBuiltBrowserIntegration,
  runFocusedBrowserIntegration,
} from "./ui-integration-runner.mjs";
import {
  durableSessionRealBackendIntegrationFiles,
  mockedBackendBrowserIntegrationFiles,
} from "./ui-integration-targets.mjs";

test("built browser entry starts only after ready output and fails without launching on missing or invalid output", async () => {
  const run = vi.fn();
  const cleanup = vi.fn();
  const buildHarness = vi.fn(async () => ({
    artifactPath: "built-harness",
    cleanup,
  }));
  const env = {};
  await runBuiltBrowserIntegration({
    ready: async () => true,
    run,
    buildHarness,
    env,
  });
  expect(run).toHaveBeenCalledWith({ prebuilt: true, exitOnFailure: false });
  expect(buildHarness).toHaveBeenCalledTimes(1);
  expect(cleanup).toHaveBeenCalledTimes(1);
  expect(env).toEqual({});
  run.mockClear();
  buildHarness.mockClear();
  await expect(
    runBuiltBrowserIntegration({
      ready: async () => false,
      run,
      buildHarness,
      env,
    }),
  ).rejects.toThrow(/missing or invalid/);
  expect(run).not.toHaveBeenCalled();
  expect(buildHarness).not.toHaveBeenCalled();
});

test("built browser entry supplies one harness artifact and restores ownership after a failed suite", async () => {
  const env = { [realBackendHarnessArtifactEnvironmentVariable]: "previous" };
  const cleanup = vi.fn();
  const publishBuildSuccess = vi.fn();
  const run = vi.fn(() => {
    expect(publishBuildSuccess).toHaveBeenCalledTimes(1);
    expect(env[realBackendHarnessArtifactEnvironmentVariable]).toBe(
      "built-harness",
    );
    throw new Error("browser assertion failed");
  });
  await expect(
    runBuiltBrowserIntegration({
      ready: async () => true,
      env,
      run,
      publishBuildSuccess,
      buildHarness: async () => ({ artifactPath: "built-harness", cleanup }),
    }),
  ).rejects.toThrow("browser assertion failed");
  expect(cleanup).toHaveBeenCalledTimes(1);
  expect(env[realBackendHarnessArtifactEnvironmentVariable]).toBe("previous");
});

test("harness build failure stays red without starting browser assertions", async () => {
  const run = vi.fn();
  const publishBuildSuccess = vi.fn();
  await expect(
    runBuiltBrowserIntegration({
      ready: async () => true,
      run,
      publishBuildSuccess,
      buildHarness: async () => {
        throw new Error("harness compile failed");
      },
    }),
  ).rejects.toThrow("harness compile failed");
  expect(run).not.toHaveBeenCalled();
  expect(publishBuildSuccess).not.toHaveBeenCalled();
});

test("build-success publication failure stays red and cleans the owned artifact", async () => {
  const cleanup = vi.fn();
  const run = vi.fn();
  const env = {};
  await expect(
    runBuiltBrowserIntegration({
      ready: async () => true,
      env,
      run,
      buildHarness: async () => ({ artifactPath: "built-harness", cleanup }),
      publishBuildSuccess: async () => {
        throw new Error("cannot publish build success");
      },
    }),
  ).rejects.toThrow("cannot publish build success");
  expect(run).not.toHaveBeenCalled();
  expect(cleanup).toHaveBeenCalledTimes(1);
  expect(env).toEqual({});
});

test("publishes compiler-cache eligibility before a later browser failure", async () => {
  const directory = await mkdtemp(
    path.join(tmpdir(), "frontend-build-output-"),
  );
  const output = path.join(directory, "output");
  const env = { GITHUB_OUTPUT: output };
  const cleanup = vi.fn();
  try {
    await expect(
      runBuiltBrowserIntegration({
        ready: async () => true,
        env,
        buildHarness: async () => ({ artifactPath: "built-harness", cleanup }),
        run: async () => {
          expect(await readFile(output, "utf8")).toBe(
            "harness-compiled=true\n",
          );
          throw new Error("browser assertion failed");
        },
      }),
    ).rejects.toThrow("browser assertion failed");
    expect(await readFile(output, "utf8")).toBe("harness-compiled=true\n");
    expect(cleanup).toHaveBeenCalledTimes(1);
    expect(env).toEqual({ GITHUB_OUTPUT: output });
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

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

test("runFocusedBrowserIntegration can throw failures for cleanup-safe callers", () => {
  const spawn = vi.fn(() => ({
    status: 1,
    stdout: "failed integration output",
  }));
  const exit = vi.spyOn(process, "exit").mockImplementation(() => {});

  expect(() =>
    runFocusedBrowserIntegration(durableSessionRealBackendIntegrationFiles, {
      exitOnFailure: false,
      spawn,
    }),
  ).toThrow(/Focused browser integration Vitest pass \(2 files\) failed/);
  expect(exit).not.toHaveBeenCalled();

  exit.mockRestore();
});
