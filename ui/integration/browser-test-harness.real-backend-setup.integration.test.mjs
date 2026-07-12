// @vitest-environment node

import { describe, expect, it } from "vitest";

import {
  findAvailablePort,
  startRealBackendBrowserHarness,
  waitForPortAvailable,
} from "./browser-test-harness.mjs";

async function fetchJSON(url) {
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`GET ${url} failed: ${response.status}`);
  }
  return response.json();
}

describe("real backend durable session setup", () => {
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
