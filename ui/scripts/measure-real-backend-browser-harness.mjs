// @vitest-environment node

import { spawn } from "node:child_process";
import { mkdir, mkdtemp, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

import {
  findAvailablePort,
  startRealBackendBrowserHarness,
  waitForPortAvailable,
} from "../integration/browser-test-harness.mjs";

const dirname = path.dirname(fileURLToPath(import.meta.url));
const repositoryRoot = path.resolve(dirname, "..", "..");
const cacheDirectoryPrefix = "you-browser-harness-measurement-";
const defaultRunsPerCondition = 1;
// One extra Go runner matches the lane's normal shared-runner pressure while
// leaving the measurement process enough CPU to report its own startup phases.
const defaultContentionJobs = 1;
const measurementConditions = ["cold", "warm", "contended"];
const measurementEnvironmentNames = [
  "GOCACHE",
  "GOMODCACHE",
  "GOPATH",
  "GOTOOLCHAIN",
];

function numericOption(args, name, fallback, minimum) {
  const prefix = `--${name}=`;
  const value = args
    .find((arg) => arg.startsWith(prefix))
    ?.slice(prefix.length);
  if (value === undefined) {
    return fallback;
  }

  const parsed = Number(value);
  if (!Number.isInteger(parsed) || parsed < minimum) {
    throw new Error(`--${name} must be an integer >= ${minimum}.`);
  }
  return parsed;
}

function selectedConditions(args) {
  const requested = args
    .find((arg) => arg.startsWith("--conditions="))
    ?.slice("--conditions=".length)
    .split(",")
    .filter(Boolean);
  const conditions = requested ?? measurementConditions;
  const invalid = conditions.filter(
    (condition) => !measurementConditions.includes(condition),
  );
  if (invalid.length > 0) {
    throw new Error(
      `--conditions contains unsupported values: ${invalid.join(", ")}.`,
    );
  }
  return conditions;
}

function environmentWithMeasurementCaches(cacheRoot) {
  return {
    ...process.env,
    GOCACHE: path.join(cacheRoot, "go-build"),
    GOMODCACHE: path.join(cacheRoot, "gomodcache"),
    GOPATH: path.join(cacheRoot, "gopath"),
    GOTOOLCHAIN: "auto",
  };
}

function restoreEnvironment(originalEnvironment) {
  for (const name of measurementEnvironmentNames) {
    const originalValue = originalEnvironment[name];
    if (originalValue === undefined) {
      delete process.env[name];
    } else {
      process.env[name] = originalValue;
    }
  }
}

function applyEnvironment(measurementEnvironment) {
  for (const name of measurementEnvironmentNames) {
    process.env[name] = measurementEnvironment[name];
  }
}

function spawnContentionJob(measurementEnvironment) {
  return new Promise((resolve) => {
    const child = spawn(
      "go",
      [
        "test",
        "./tests/functional/internal/support/cmd/browser_api_harness",
        "-run",
        "^$",
        "-count=1",
      ],
      {
        cwd: repositoryRoot,
        env: measurementEnvironment,
        shell: false,
        stdio: "ignore",
      },
    );
    child.once("error", (error) => resolve({ error }));
    child.once("exit", (code, signal) => resolve({ code, signal }));
  });
}

async function runMeasurement(condition, runNumber, contentionJobs) {
  const cacheRoot = process.env.GO_BROWSER_HARNESS_MEASUREMENT_ROOT;
  if (condition === "cold") {
    await rm(cacheRoot, { force: true, recursive: true });
    await mkdir(cacheRoot, { recursive: true });
  }
  const measurementEnvironment = environmentWithMeasurementCaches(cacheRoot);
  applyEnvironment(measurementEnvironment);

  const contention =
    condition === "contended"
      ? Array.from({ length: contentionJobs }, () =>
          spawnContentionJob(measurementEnvironment),
        )
      : [];
  const apiPort = await findAvailablePort();
  let backend;
  try {
    backend = await startRealBackendBrowserHarness({
      apiPort,
      requestID: `req-browser-harness-measurement-${condition}-${runNumber}`,
      startMode: "sync",
      workflowFixture: "agent-run-fake-child.workflow.js",
      workflowName: "agent-run-fake-child",
    });
    console.log(
      `[real-backend-browser-harness-measurement] condition=${condition} run=${runNumber} ${JSON.stringify(backend.startupTimings)}`,
    );
  } finally {
    await backend?.stop().catch(() => {});
    await waitForPortAvailable("127.0.0.1", apiPort, 5_000, 50).catch(() => {});
    await Promise.all(contention);
  }
}

async function main() {
  const args = process.argv.slice(2);
  const conditions = selectedConditions(args);
  const runs = numericOption(args, "runs", defaultRunsPerCondition, 1);
  const contentionJobs = numericOption(
    args,
    "contention-jobs",
    defaultContentionJobs,
    1,
  );
  const originalEnvironment = {
    GOCACHE: process.env.GOCACHE,
    GOMODCACHE: process.env.GOMODCACHE,
    GOPATH: process.env.GOPATH,
    GOTOOLCHAIN: process.env.GOTOOLCHAIN,
  };
  const cacheRoot = await mkdtemp(path.join(os.tmpdir(), cacheDirectoryPrefix));
  process.env.GO_BROWSER_HARNESS_MEASUREMENT_ROOT = cacheRoot;

  try {
    for (const condition of conditions) {
      if (condition === "warm" && !conditions.includes("cold")) {
        console.log(
          "[real-backend-browser-harness-measurement] warm condition uses an initially empty isolated cache; include cold first for a populated-cache comparison.",
        );
      }
      for (let runNumber = 1; runNumber <= runs; runNumber += 1) {
        await runMeasurement(condition, runNumber, contentionJobs);
      }
    }
  } finally {
    restoreEnvironment(originalEnvironment);
    delete process.env.GO_BROWSER_HARNESS_MEASUREMENT_ROOT;
    await rm(cacheRoot, { force: true, recursive: true });
  }
}

main().catch((error) => {
  console.error(
    `[real-backend-browser-harness-measurement] failed: ${error instanceof Error ? error.message : String(error)}`,
  );
  process.exitCode = 1;
});
