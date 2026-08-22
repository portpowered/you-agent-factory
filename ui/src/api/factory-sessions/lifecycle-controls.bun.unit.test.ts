import { afterEach, describe, expect, it } from "bun:test";

import { bunVi as vi } from "../../testing/bun/vi-compat";
import {
  approveFactorySession,
  interruptFactorySessionDispatch,
  pauseFactorySession,
  retryFactorySessionDispatch,
} from "./lifecycle-controls";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: lifecycle route coverage shares one transport seam harness.
describe("factory session lifecycle controls API", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("posts pause controls to the shared session lifecycle endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          operation: "PAUSE",
          outcome: "ACCEPTED",
          sessionId: "dur-sess-js-running-001",
          status: "PAUSED",
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 202,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      pauseFactorySession("dur-sess-js-running-001"),
    ).resolves.toEqual(
      expect.objectContaining({
        operation: "PAUSE",
        outcome: "ACCEPTED",
        sessionId: "dur-sess-js-running-001",
        status: "PAUSED",
      }),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/dur-sess-js-running-001/pause",
      expect.objectContaining({
        body: JSON.stringify({}),
        headers: {
          "Content-Type": "application/json",
        },
        method: "POST",
      }),
    );
  });

  it("posts approval through the typed durable approval endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          operation: "APPROVE",
          outcome: "ACCEPTED",
          sessionId: "dur-sess-js-awaiting-001",
          status: "RUNNING",
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      approveFactorySession("dur-sess-js-awaiting-001", {
        approvalPreviewId: "preview-001",
      }),
    ).resolves.toEqual(
      expect.objectContaining({
        operation: "APPROVE",
        outcome: "ACCEPTED",
      }),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/dur-sess-js-awaiting-001/approve",
      expect.objectContaining({
        body: JSON.stringify({
          approvalPreviewId: "preview-001",
        }),
        headers: {
          "Content-Type": "application/json",
        },
        method: "POST",
      }),
    );
  });

  it("posts retry-dispatch requests with the selected dispatch payload", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          dispatchId: "dispatch-failed-001",
          operation: "RETRY_DISPATCH",
          outcome: "ACCEPTED",
          retryDispatchId: "dispatch-retry-002",
          sessionId: "dur-sess-js-failed-001",
          status: "RUNNING",
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 202,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      retryFactorySessionDispatch("dur-sess-js-failed-001", {
        dispatchId: "dispatch-failed-001",
        forceNewAttempt: false,
        resetAttemptCount: false,
      }),
    ).resolves.toEqual(
      expect.objectContaining({
        dispatchId: "dispatch-failed-001",
        operation: "RETRY_DISPATCH",
        retryDispatchId: "dispatch-retry-002",
      }),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/dur-sess-js-failed-001/retry-dispatch",
      expect.objectContaining({
        body: JSON.stringify({
          dispatchId: "dispatch-failed-001",
          forceNewAttempt: false,
          resetAttemptCount: false,
        }),
        headers: {
          "Content-Type": "application/json",
        },
        method: "POST",
      }),
    );
  });

  it("posts interrupt-dispatch requests with the selected dispatch payload", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          dispatchId: "dispatch-running-001",
          operation: "INTERRUPT_DISPATCH",
          outcome: "ACCEPTED",
          sessionId: "dur-sess-js-running-001",
          status: "RUNNING",
        }),
        {
          headers: { "Content-Type": "application/json" },
          status: 202,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      interruptFactorySessionDispatch("dur-sess-js-running-001", {
        dispatchId: "dispatch-running-001",
      }),
    ).resolves.toEqual(
      expect.objectContaining({
        dispatchId: "dispatch-running-001",
        operation: "INTERRUPT_DISPATCH",
      }),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/dur-sess-js-running-001/interrupt-dispatch",
      expect.objectContaining({
        body: JSON.stringify({ dispatchId: "dispatch-running-001" }),
        method: "POST",
      }),
    );
  });

  it("returns typed conflict lifecycle outcomes instead of surfacing them as transport errors", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          detail: "Conflicting request id.",
          operation: "PAUSE",
          outcome: "CONFLICT",
          sessionId: "dur-sess-js-running-001",
          status: "RUNNING",
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 409,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      pauseFactorySession("dur-sess-js-running-001"),
    ).resolves.toEqual(
      expect.objectContaining({
        detail: "Conflicting request id.",
        operation: "PAUSE",
        outcome: "CONFLICT",
      }),
    );
  });
});
