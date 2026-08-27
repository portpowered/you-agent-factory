// @vitest-environment node

import { EventEmitter } from "node:events";
import { existsSync, writeFileSync } from "node:fs";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { PassThrough } from "node:stream";

import { describe, expect, it } from "vitest";

import {
  buildRealBackendBrowserHarness,
  createRealBackendHarnessStartupTiming,
  findAvailablePort,
  formatRealBackendHarnessStartupTiming,
  startRealBackendBrowserHarness,
  waitForPortAvailable,
  waitForRealBackendHarnessReadiness,
} from "./browser-test-harness.mjs";

function fakeGoCache() {
  return {
    cacheReuse: {
      GOCACHE: "populated",
      GOMODCACHE: "populated",
      GOPATH: "available",
    },
    cacheReuseBeforeResolution: {
      GOCACHE: "populated",
      GOMODCACHE: "populated",
      GOPATH: "available",
    },
    elapsedMs: 2,
    environment: {
      GOCACHE: "C:\\cache\\go-build",
      GOMODCACHE: "C:\\cache\\gomodcache",
      GOPATH: "C:\\cache\\gopath",
    },
  };
}

function fakeProcess({ code = 0, stderr = "" } = {}) {
  return (_command, args) => {
    const child = new EventEmitter();
    child.stdout = new PassThrough();
    child.stderr = new PassThrough();
    child.exitCode = null;
    child.signalCode = null;
    if (code === 0) {
      const outputPath = args[args.indexOf("-o") + 1];
      writeFileSync(outputPath, "prebuilt browser api harness");
    }
    queueMicrotask(() => {
      if (stderr) {
        child.stderr.end(stderr);
      } else {
        child.stderr.end();
      }
      child.stdout.end();
      child.exitCode = code;
      child.emit("exit", code, null);
    });
    return child;
  };
}

function fakeHarnessProcess(spawnCalls) {
  return (command, args) => {
    spawnCalls.push({ args, command });
    const child = new EventEmitter();
    child.stdout = new PassThrough();
    child.stderr = new PassThrough();
    child.exitCode = null;
    child.signalCode = null;
    queueMicrotask(() => {
      child.emit("spawn");
      child.stderr.write("[browser-api-harness] phase=process-started\n");
      child.stdout.write(
        `${JSON.stringify({
          apiOrigin: "http://127.0.0.1:43123",
          sessionId: "dur-sess-prebuilt",
        })}\n`,
      );
      child.stderr.end();
      child.stdout.end();
      child.exitCode = 0;
    });
    return child;
  };
}

function fakeSpawnErrorHarnessProcess() {
  const child = new EventEmitter();
  child.stdout = new PassThrough();
  child.stderr = new PassThrough();
  child.exitCode = null;
  child.signalCode = null;
  queueMicrotask(() => {
    child.emit("error", new Error("spawn failed"));
  });
  return child;
}

async function fetchJSON(url) {
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`GET ${url} failed: ${response.status}`);
  }
  return response.json();
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: this focused suite owns all startup lifecycle paths and the one real setup proof.
describe("real backend durable session setup", () => {
  it("builds one prebuilt artifact and removes it through idempotent cleanup", async () => {
    const diagnostics = [];
    const artifact = await buildRealBackendBrowserHarness({
      diagnosticWriter: (line) => diagnostics.push(line),
      resolveGoCache: fakeGoCache,
      spawnProcess: fakeProcess({ stderr: "compiler setup complete" }),
    });

    expect(existsSync(artifact.artifactPath)).toBe(true);
    expect(artifact.buildTimings.harnessBuildMs).toEqual(expect.any(Number));
    expect(diagnostics.join(" ")).toContain("phase=harness-build complete");
    expect(diagnostics.join(" ")).toContain(
      "child-stderr compiler setup complete",
    );

    await artifact.cleanup();
    await artifact.cleanup();
    expect(existsSync(artifact.artifactPath)).toBe(false);
  });

  it("reports a build failure as harness-build and cleans the partial artifact", async () => {
    let artifactPath = null;
    const diagnostics = [];
    const spawnProcess = (command, args) => {
      artifactPath = args[args.indexOf("-o") + 1];
      return fakeProcess({
        code: 1,
        stderr: "compile failed at C:\\Users\\andre\\private\\token=secret",
      })(command, args);
    };

    await expect(
      buildRealBackendBrowserHarness({
        diagnosticWriter: (line) => diagnostics.push(line),
        resolveGoCache: fakeGoCache,
        spawnProcess,
      }),
    ).rejects.toThrow(
      /Real backend browser harness build failed[\s\S]*phase=harness-build[\s\S]*captured-output=child-stderr compile failed at <path>/,
    );
    expect(artifactPath).not.toBeNull();
    expect(existsSync(artifactPath)).toBe(false);
    expect(diagnostics.join(" ")).not.toContain("C:\\Users\\andre\\private");
  });

  it("launches a supplied prebuilt artifact without go run", async () => {
    const artifactDirectory = await mkdtemp(
      path.join(os.tmpdir(), "you-browser-api-harness-test-"),
    );
    const artifactPath = path.join(artifactDirectory, "browser_api_harness");
    await writeFile(artifactPath, "prebuilt browser api harness");
    const spawnCalls = [];
    const diagnostics = [];
    const apiPort = 43123;

    try {
      const backend = await startRealBackendBrowserHarness({
        apiPort,
        diagnosticWriter: (line) => diagnostics.push(line),
        harnessPath: artifactPath,
        resolveGoCache: fakeGoCache,
        spawnProcess: fakeHarnessProcess(spawnCalls),
        workflowFixture: "agent-run-fake-child.workflow.js",
        workflowName: "agent-run-fake-child",
      });

      expect(spawnCalls).toHaveLength(1);
      expect(spawnCalls[0].command).toBe(path.resolve(artifactPath));
      expect(spawnCalls[0].args).not.toContain("run");
      expect(backend.startupTimings).toMatchObject({
        executionMode: "prebuilt-artifact",
        harnessCompilationSetupMs: 0,
      });
      expect(diagnostics.join(" ")).toContain(
        "phase=process-launch started mode=prebuilt-artifact",
      );
      await backend.stop();
    } finally {
      await rm(artifactDirectory, { force: true, recursive: true });
    }
  });

  it("rejects promptly and cleans up the temporary home after a spawn error", async () => {
    const artifactDirectory = await mkdtemp(
      path.join(os.tmpdir(), "you-browser-api-harness-test-"),
    );
    const artifactPath = path.join(artifactDirectory, "browser_api_harness");
    await writeFile(artifactPath, "prebuilt browser api harness");
    let customerHome = null;
    const startPromise = startRealBackendBrowserHarness({
      apiPort: 43123,
      createTemporaryHome: async () => {
        customerHome = await mkdtemp(
          path.join(os.tmpdir(), "you-browser-backend-spawn-error-test-"),
        );
        return customerHome;
      },
      harnessPath: artifactPath,
      resolveGoCache: fakeGoCache,
      spawnProcess: fakeSpawnErrorHarnessProcess,
      workflowFixture: "agent-run-fake-child.workflow.js",
      workflowName: "agent-run-fake-child",
    });

    try {
      const failure = await new Promise((resolve, reject) => {
        const timeout = setTimeout(() => {
          reject(new Error("spawn-error cleanup did not settle promptly"));
        }, 1_000);
        startPromise.then(
          () => {
            clearTimeout(timeout);
            reject(new Error("spawn-error setup unexpectedly became ready"));
          },
          (error) => {
            clearTimeout(timeout);
            resolve(error);
          },
        );
      });

      expect(failure).toBeInstanceOf(Error);
      expect(failure.message).toMatch(
        /Real backend browser harness process error: Error: spawn failed[\s\S]*phase=harness-compilation\/setup/,
      );
      expect(customerHome).not.toBeNull();
      expect(existsSync(customerHome)).toBe(false);
    } finally {
      await rm(artifactDirectory, { force: true, recursive: true });
    }
  });

  it("reports phase timing when a valid readiness payload arrives", async () => {
    const child = new EventEmitter();
    const lineReader = new EventEmitter();
    const diagnostics = [];
    const timing = createRealBackendHarnessStartupTiming();
    timing.processSpawnedAt = timing.startedAt;
    timing.processStartedAt = timing.startedAt;

    const ready = waitForRealBackendHarnessReadiness({
      child,
      diagnosticWriter: (line) => diagnostics.push(line),
      lineReader,
      timing,
      timeoutMs: 100,
    });
    lineReader.emit(
      "line",
      JSON.stringify({
        apiOrigin: "http://127.0.0.1:43123",
        sessionId: "dur-sess-test",
      }),
    );

    await expect(ready).resolves.toMatchObject({
      apiOrigin: "http://127.0.0.1:43123",
      sessionId: "dur-sess-test",
    });
    expect(timing.firstReadyPayloadMs).toEqual(expect.any(Number));
    expect(timing.applicationStartupMs).toEqual(expect.any(Number));
    expect(diagnostics.join(" ")).toContain("first-ready-payload=");
    expect(formatRealBackendHarnessStartupTiming(timing)).toContain(
      "cache-resolution=unavailable",
    );
  });

  it("reports phase timing when the harness exits before readiness", async () => {
    const child = new EventEmitter();
    const lineReader = new EventEmitter();
    const timing = createRealBackendHarnessStartupTiming();
    timing.processSpawnedAt = timing.startedAt;

    const ready = waitForRealBackendHarnessReadiness({
      child,
      getCapturedStderr: () => "customer-home=C:\\Users\\andre\\secret",
      lineReader,
      timing,
      timeoutMs: 100,
    });
    child.emit("exit", 1, null);

    const failure = await ready.catch((error) => error);
    expect(failure).toBeInstanceOf(Error);
    expect(failure.message).toMatch(
      /exited before readiness:[\s\S]*phase=harness-compilation\/setup[\s\S]*cache-resolution=unavailable/,
    );
    expect(failure.message).not.toContain("C:\\Users\\andre\\secret");
  });

  it("reports phase timing when readiness reaches its deadline", async () => {
    const child = new EventEmitter();
    const lineReader = new EventEmitter();
    const timing = createRealBackendHarnessStartupTiming();
    timing.processSpawnedAt = timing.startedAt;

    await expect(
      waitForRealBackendHarnessReadiness({
        child,
        getCapturedStderr: () => "startup diagnostic",
        lineReader,
        timing,
        timeoutMs: 5,
      }),
    ).rejects.toThrow(
      /Timed out waiting for real backend browser harness readiness[\s\S]*phase=harness-compilation\/setup[\s\S]*captured-stderr=startup diagnostic/,
    );
  });

  it("reports malformed readiness output with phase timing", async () => {
    const child = new EventEmitter();
    const lineReader = new EventEmitter();
    const timing = createRealBackendHarnessStartupTiming();
    timing.processSpawnedAt = timing.startedAt;
    timing.processStartedAt = timing.startedAt;

    const ready = waitForRealBackendHarnessReadiness({
      child,
      lineReader,
      timing,
      timeoutMs: 100,
    });
    lineReader.emit("line", "not-json");

    await expect(ready).rejects.toThrow(
      /Failed to parse real backend browser harness ready payload[\s\S]*phase=application-startup/,
    );
  });

  it("starts a real backend and seeds one inspectable dur-sess-* JavaScript factory session", async () => {
    const apiPort = await findAvailablePort();
    const backend = await startRealBackendBrowserHarness({
      apiPort,
      startMode: "sync",
      workflowFixture: "agent-run-fake-child.workflow.js",
      workflowName: "agent-run-fake-child",
      requestID: `req-setup-${Date.now()}`,
    });

    try {
      expect(backend.sessionID).toMatch(/^dur-sess-/);
      expect(backend.apiOrigin).toBe(`http://127.0.0.1:${apiPort}`);

      const session = await fetchJSON(
        `${backend.apiOrigin}/factory-sessions/${encodeURIComponent(backend.sessionID)}`,
      );
      expect(session.sessionId).toBe(backend.sessionID);
      expect(session.dialect).toBe("javascript");

      const dispatches = await fetchJSON(
        `${backend.apiOrigin}/factory-sessions/${encodeURIComponent(backend.sessionID)}/dispatches`,
      );
      expect(dispatches.dispatches?.length).toBeGreaterThanOrEqual(1);
      expect(dispatches.dispatches[0].id).toBe("dispatch-1");
    } finally {
      await backend.stop();
    }

    await waitForPortAvailable("127.0.0.1", apiPort, 5_000, 50);
  }, 120_000);
});
