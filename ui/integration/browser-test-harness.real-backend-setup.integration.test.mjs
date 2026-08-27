// @vitest-environment node

import { EventEmitter } from "node:events";

import { describe, expect, it } from "vitest";

import {
  createRealBackendHarnessStartupTiming,
  findAvailablePort,
  formatRealBackendHarnessStartupTiming,
  startRealBackendBrowserHarness,
  waitForPortAvailable,
  waitForRealBackendHarnessReadiness,
} from "./browser-test-harness.mjs";

async function fetchJSON(url) {
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`GET ${url} failed: ${response.status}`);
  }
  return response.json();
}

describe("real backend durable session setup", () => {
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
