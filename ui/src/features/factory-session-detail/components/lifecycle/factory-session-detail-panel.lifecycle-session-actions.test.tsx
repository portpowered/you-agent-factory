import { screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { FactoryOrchestratorKind } from "../../../../api/generated/openapi";
import {
  awaitingReplaySessionID,
  buildAwaitingDurableSession,
} from "../../../../testing/factory-session-event-replay-fixtures";
import {
  buildPausedDurableSession,
  pausedReplaySessionID,
} from "../../../../testing/factory-session-lifecycle-fixtures";
import { FactorySessionDetailPanel } from "../factory-session-detail-panel";
import {
  jsonResponse,
  renderWithQueryClient,
} from "../test-support/factory-session-detail-panel.test-helpers";

describe("running durable session actions", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows pause, cancel, and terminate for a running durable JavaScript session", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith("/factory-sessions/dur-sess-js-running-001")) {
        return jsonResponse({
          dialect: "you-workflow-v1",
          lifecycle: {
            startedAt: "2026-06-08T14:00:00Z",
            updatedAt: "2026-06-08T14:05:00Z",
          },
          orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
          phase: "review",
          progress: {
            completedDispatches: 1,
            failedDispatches: 1,
            inFlightDispatches: 1,
            totalDispatches: 3,
          },
          resolvedSource: {
            kind: "WORKFLOW_NAME",
            sourceRef: "workflow/review",
            sourceHash: "sha256:workflow-review",
          },
          sessionId: "dur-sess-js-running-001",
          status: "RUNNING",
          usage: { resources: [] },
        });
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="dur-sess-js-running-001" />,
    );

    await waitFor(() => {
      expect(screen.getByText("Lifecycle controls")).toBeTruthy();
    });

    expect(screen.getByRole("button", { name: "Pause" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Terminate" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Resume" })).toBeNull();
  });
});

describe("paused durable session actions", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows resume controls for a paused durable JavaScript session", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith(`/factory-sessions/${pausedReplaySessionID}`)) {
        return jsonResponse(buildPausedDurableSession());
      }
      if (
        url.endsWith(`/factory-sessions/${pausedReplaySessionID}/dispatches`)
      ) {
        return jsonResponse({
          dispatches: [],
          sessionId: pausedReplaySessionID,
        });
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={pausedReplaySessionID} />,
    );

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Resume" })).toBeTruthy();
    });

    expect(screen.queryByRole("button", { name: "Pause" })).toBeNull();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Terminate" })).toBeTruthy();
  });
});

describe("awaiting-approval durable session actions", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows approve controls for an awaiting-approval durable JavaScript session", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith(`/factory-sessions/${awaitingReplaySessionID}`)) {
        return jsonResponse(buildAwaitingDurableSession());
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={awaitingReplaySessionID} />,
    );

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Approve" })).toBeTruthy();
    });

    expect(screen.queryByRole("button", { name: "Pause" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Resume" })).toBeNull();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Terminate" })).toBeTruthy();
  });
});
