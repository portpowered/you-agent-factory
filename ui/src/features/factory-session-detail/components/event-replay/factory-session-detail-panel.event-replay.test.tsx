import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { FactoryOrchestratorKind } from "../../../../api/generated/openapi";
import { formatDateTime } from "../../../../i18n/formatters";
import {
  awaitingReplaySessionID,
  buildAwaitingDurableSession,
  buildAwaitingReplayEventStream,
  buildSuccessfulDurableSession,
  buildSuccessfulReplayEventStream,
  buildWarningDurableSession,
  buildWarningReplayEventStream,
  successfulReplaySessionID,
  warningReplaySessionID,
} from "../../../../testing/factory-session-event-replay-fixtures";
import { FactorySessionDetailPanel } from "../factory-session-detail-panel";
import {
  jsonResponse,
  renderWithQueryClient,
} from "../test-support/factory-session-detail-panel.test-helpers";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: focused event replay coverage keeps one fetch harness and assertion seam.
describe("FactorySessionDetailPanel event replay disclosure", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("reveals bounded durable Factory Event replay inline for durable JavaScript sessions", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith(`/factory-sessions/${successfulReplaySessionID}`)) {
        return jsonResponse(buildSuccessfulDurableSession());
      }
      if (
        url.endsWith(`/factory-sessions/${successfulReplaySessionID}/events`)
      ) {
        return new Response(buildSuccessfulReplayEventStream(), {
          headers: {
            "Content-Type": "text/event-stream",
          },
          status: 200,
        });
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={successfulReplaySessionID} />,
    );

    await waitFor(() => {
      expect(screen.getByText("JavaScript workflow")).toBeTruthy();
    });

    const user = userEvent.setup();
    const replayTrigger = screen.getByRole("button", {
      name: "Expand Factory Event replay",
    });

    expect(replayTrigger.getAttribute("aria-expanded")).toBe("false");

    await user.click(replayTrigger);

    await waitFor(() => {
      expect(screen.getByText("Showing 5 Factory Events.")).toBeTruthy();
    });

    expect(replayTrigger.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByText("Session started")).toBeTruthy();
    expect(screen.getByText("Phase changed")).toBeTruthy();
    expect(screen.getByText("Review work scheduled.")).toBeTruthy();
    expect(screen.getByText("Dispatch queued")).toBeTruthy();
    expect(
      screen.getByText("Draft release notes · Queue position 1"),
    ).toBeTruthy();
    expect(screen.getByText("Dispatch reconciled")).toBeTruthy();
    expect(screen.getByText("Dispatch status completed")).toBeTruthy();
    expect(
      screen.getAllByText(/artifact-release-notes/).length,
    ).toBeGreaterThan(0);
    expect(screen.getByText("Session completed")).toBeTruthy();
    expect(screen.getByText("Lifecycle status succeeded")).toBeTruthy();
    expect(screen.getByText("Dispatch Queued")).toBeTruthy();
    expect(
      screen.getByText(
        "Phase review · Dispatch dispatch-1 · 2 related work items",
      ),
    ).toBeTruthy();
    expect(screen.getByText("Session event 4 · Tick 4")).toBeTruthy();
    expect(
      screen.getAllByText(
        formatDateTime("2026-06-25T10:00:03Z", "en", {
          timeZone: "UTC",
        }),
      ).length,
    ).toBeTruthy();
  });

  it("keeps unfamiliar replay events in the neutral raw timeline", async () => {
    const futureEvent = {
      context: {
        eventTime: "2026-06-25T10:00:06Z",
        sequence: 6,
        sessionId: successfulReplaySessionID,
        sessionSequence: 6,
        tick: 6,
      },
      id: "evt-future-timeline",
      payload: { futurePayload: "payload-secret" },
      schemaVersion: "agent-factory.event.v1",
      type: "FUTURE_EVENT_TYPE",
    };
    const replayStream = `${buildSuccessfulReplayEventStream()}\ndata: ${JSON.stringify(
      futureEvent,
    )}\n\n`;

    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith(`/factory-sessions/${successfulReplaySessionID}`)) {
        return jsonResponse(buildSuccessfulDurableSession());
      }
      if (
        url.endsWith(`/factory-sessions/${successfulReplaySessionID}/events`)
      ) {
        return new Response(replayStream, {
          headers: {
            "Content-Type": "text/event-stream",
          },
          status: 200,
        });
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={successfulReplaySessionID} />,
    );

    await waitFor(() => {
      expect(screen.getByText("JavaScript workflow")).toBeTruthy();
    });
    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", { name: "Expand Factory Event replay" }),
    );

    await waitFor(() => {
      expect(screen.getByText("Showing 6 Factory Events.")).toBeTruthy();
    });

    expect(screen.getByText("Session completed")).toBeTruthy();
    const futureLabels = screen.getAllByText("FUTURE_EVENT_TYPE");
    expect(
      futureLabels.some((label) =>
        label.className.includes("bg-surface-container-low"),
      ),
    ).toBe(true);
  });

  it("surfaces failed and warning replay cues inside the bounded timeline", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith(`/factory-sessions/${warningReplaySessionID}`)) {
        return jsonResponse(buildWarningDurableSession());
      }
      if (url.endsWith(`/factory-sessions/${warningReplaySessionID}/events`)) {
        return new Response(buildWarningReplayEventStream(), {
          headers: {
            "Content-Type": "text/event-stream",
          },
          status: 200,
        });
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={warningReplaySessionID} />,
    );

    await waitFor(() => {
      expect(screen.getByText("JavaScript workflow")).toBeTruthy();
    });

    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", { name: "Expand Factory Event replay" }),
    );

    await waitFor(() => {
      expect(screen.getByText("Checkpoint recorded")).toBeTruthy();
    });

    expect(screen.getByText("Checkpoint before publish")).toBeTruthy();
    expect(screen.getByText("Dispatch interrupted")).toBeTruthy();
    expect(
      screen.getByText("Provider session timed out · Retry planned"),
    ).toBeTruthy();
    expect(screen.getByText("Session completed")).toBeTruthy();
    expect(screen.getByText("Release verification failed.")).toBeTruthy();
    expect(
      screen.getByText(
        "Phase verify · Checkpoint checkpoint-9 · Factory Artifact checkpoint-artifact-9",
      ),
    ).toBeTruthy();
  });

  it("reveals replay for durable awaiting-approval sessions before result artifacts exist", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith(`/factory-sessions/${awaitingReplaySessionID}`)) {
        return jsonResponse(buildAwaitingDurableSession());
      }
      if (url.endsWith(`/factory-sessions/${awaitingReplaySessionID}/events`)) {
        return new Response(buildAwaitingReplayEventStream(), {
          headers: {
            "Content-Type": "text/event-stream",
          },
          status: 200,
        });
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={awaitingReplaySessionID} />,
    );

    await waitFor(() => {
      expect(screen.getByText("Awaiting approval")).toBeTruthy();
    });

    const user = userEvent.setup();
    const replayTrigger = screen.getByRole("button", {
      name: "Expand Factory Event replay",
    });

    await user.click(replayTrigger);

    await waitFor(() => {
      expect(screen.getByText("Showing 2 Factory Events.")).toBeTruthy();
    });

    expect(screen.getByText("Session started")).toBeTruthy();
    expect(screen.getByText("Session result updated")).toBeTruthy();
    expect(screen.getByText("Result status not ready")).toBeTruthy();
    expect(screen.getAllByText("Phase approval").length).toBe(2);
  });

  it("keeps replay disclosure out of non-durable session detail views", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      jsonResponse({
        factoryDir: "/workspace/root/beta",
        folderPath: "/workspace/root",
        id: "session-beta",
        isDefault: false,
        project: "beta",
        runtime: {
          artifacts: [],
          javascript: {
            checkpoints: [],
            childDispatchCounts: {
              completed: 0,
              queued: 0,
              running: 0,
            },
            phase: "review",
            phases: ["review"],
            scriptStatus: "IDLE",
          },
          lifecycle: {
            startedAt: "2026-06-08T14:00:00Z",
            updatedAt: "2026-06-08T14:05:00Z",
          },
          orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
          progress: {
            categories: {},
            factoryState: "RUNNING",
            inFlightCount: 0,
            totalTokens: 0,
          },
          status: "IDLE",
          usage: { resources: [] },
        },
        target: { kind: "named", name: "beta" },
      }),
    );

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="session-beta" />,
    );

    await waitFor(() => {
      expect(screen.getByText("JavaScript workflow")).toBeTruthy();
    });

    expect(
      screen.queryByRole("button", { name: "Expand Factory Event replay" }),
    ).toBeNull();
  });
});
