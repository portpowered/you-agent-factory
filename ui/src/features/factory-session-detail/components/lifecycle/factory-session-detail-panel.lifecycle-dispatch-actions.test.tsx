import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { FactoryOrchestratorKind } from "../../../../api/generated/openapi";
import {
  buildFailedPartialDurableSession,
  buildFailedPartialReplayDispatchList,
  failedPartialReplaySessionID,
} from "../../../../testing/factory-session-lifecycle-fixtures";
import { FactorySessionDetailPanel } from "../factory-session-detail-panel";
import {
  jsonResponse,
  renderWithQueryClient,
} from "../test-support/factory-session-detail-panel.test-helpers";

describe("failed dispatch retry actions", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows retry dispatch only for the selected failed dispatch", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith(`/factory-sessions/${failedPartialReplaySessionID}`)) {
        return jsonResponse(buildFailedPartialDurableSession());
      }
      if (
        url.endsWith(
          `/factory-sessions/${failedPartialReplaySessionID}/dispatches`,
        )
      ) {
        return jsonResponse(buildFailedPartialReplayDispatchList());
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={failedPartialReplaySessionID} />,
    );

    await waitFor(() => {
      expect(
        screen.getByRole("button", {
          name: "Expand dispatch detail for dispatch-failed",
        }),
      ).toBeTruthy();
    });

    expect(screen.queryByRole("button", { name: "Retry dispatch" })).toBeNull();
    expect(
      screen.getByText(
        "Select a running or failed dispatch to make interrupt or retry available on this detail surface.",
      ),
    ).toBeTruthy();

    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", {
        name: "Expand dispatch detail for dispatch-failed",
      }),
    );

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Retry dispatch" }),
      ).toBeTruthy();
    });

    expect(screen.getByText("Selected dispatch: dispatch-failed")).toBeTruthy();
  });

  it("keeps lifecycle controls hidden for non-durable failed dispatch drilldown", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith("/factory-sessions/session-beta")) {
        return jsonResponse({
          factoryDir: "/workspace/root/beta",
          folderPath: "/workspace/root",
          id: "session-beta",
          isDefault: false,
          project: "beta",
          runtime: {
            javascript: {
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
        });
      }
      if (url.endsWith("/factory-sessions/session-beta/result")) {
        return new Response("not found", { status: 404 });
      }
      if (url.endsWith("/factory-sessions/session-beta/dispatches")) {
        return jsonResponse({
          dispatches: [
            {
              dispatchKind: "JAVASCRIPT_VERIFY",
              id: "dispatch-failed",
              label: "Verify release manifest",
              status: "FAILED",
            },
          ],
          sessionId: "session-beta",
        });
      }
      if (url.endsWith("/factory-sessions/session-beta/partial-result")) {
        return new Response("not found", { status: 404 });
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="session-beta" />,
    );

    await waitFor(() => {
      expect(screen.getByText("Runtime")).toBeTruthy();
    });

    expect(screen.queryByText("Lifecycle controls")).toBeNull();
    expect(screen.queryByRole("button", { name: "Retry dispatch" })).toBeNull();
    expect(
      screen.queryByText(
        "Select a running or failed dispatch to make interrupt or retry available on this detail surface.",
      ),
    ).toBeNull();
  });
});

describe("active dispatch interrupt actions", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("selects a running dispatch and submits the dedicated interrupt route", async () => {
    const fetchMock = vi
      .mocked(globalThis.fetch)
      .mockImplementation(async (input, init) => {
        const url = String(input);
        if (url.endsWith("/factory-sessions/dur-sess-js-running-001")) {
          return jsonResponse({
            dialect: "you-workflow-v1",
            lifecycle: {
              startedAt: "2026-06-08T14:00:00Z",
              updatedAt: "2026-06-08T14:05:00Z",
            },
            orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
            phase: "execute",
            progress: {
              completedDispatches: 0,
              failedDispatches: 0,
              inFlightDispatches: 1,
              totalDispatches: 1,
            },
            resolvedSource: {
              kind: "WORKFLOW_NAME",
              sourceHash: "sha256:workflow-running",
              sourceRef: "workflow/running",
            },
            sessionId: "dur-sess-js-running-001",
            status: "RUNNING",
            usage: { resources: [] },
          });
        }
        if (
          url.endsWith("/factory-sessions/dur-sess-js-running-001/dispatches")
        ) {
          return jsonResponse({
            dispatches: [
              {
                dispatchKind: "JAVASCRIPT_AGENT",
                id: "dispatch-running",
                status: "RUNNING",
              },
            ],
            sessionId: "dur-sess-js-running-001",
          });
        }
        if (
          url.endsWith(
            "/factory-sessions/dur-sess-js-running-001/interrupt-dispatch",
          ) &&
          init?.method === "POST"
        ) {
          return jsonResponse(
            {
              dispatchId: "dispatch-running",
              operation: "INTERRUPT_DISPATCH",
              outcome: "ACCEPTED",
              sessionId: "dur-sess-js-running-001",
              status: "RUNNING",
            },
            202,
          );
        }
        return new Response("not found", { status: 404 });
      });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="dur-sess-js-running-001" />,
    );

    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", {
        name: "Expand dispatch detail for dispatch-running",
      }),
    );

    const interruptButton = await screen.findByRole("button", {
      name: "Interrupt dispatch",
    });
    await user.click(interruptButton);

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/factory-sessions/dur-sess-js-running-001/interrupt-dispatch",
        expect.objectContaining({
          body: JSON.stringify({ dispatchId: "dispatch-running" }),
          method: "POST",
        }),
      );
    });
    expect(
      screen.getByText("Selected dispatch: dispatch-running"),
    ).toBeTruthy();
  });
});

describe("terminal dispatch lifecycle actions", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows an empty lifecycle state when no actions are available", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith("/factory-sessions/dur-sess-js-success-001")) {
        return jsonResponse({
          dialect: "you-workflow-v1",
          lifecycle: {
            finishedAt: "2026-06-08T14:05:00Z",
            startedAt: "2026-06-08T14:00:00Z",
          },
          orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
          progress: {
            completedDispatches: 1,
            failedDispatches: 0,
            inFlightDispatches: 0,
            totalDispatches: 1,
          },
          resolvedSource: {
            kind: "WORKFLOW_NAME",
            sourceRef: "workflow/success",
            sourceHash: "sha256:workflow-success",
          },
          sessionId: "dur-sess-js-success-001",
          status: "SUCCEEDED",
          usage: { resources: [] },
        });
      }
      if (
        url.endsWith("/factory-sessions/dur-sess-js-success-001/dispatches")
      ) {
        return jsonResponse({
          dispatches: [
            {
              dispatchKind: "JAVASCRIPT_AGENT",
              id: "dispatch-success",
              orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
              sessionId: "dur-sess-js-success-001",
              status: "COMPLETED",
            },
          ],
          sessionId: "dur-sess-js-success-001",
        });
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="dur-sess-js-success-001" />,
    );

    await waitFor(() => {
      expect(screen.getByText("Lifecycle controls")).toBeTruthy();
    });

    expect(screen.queryByRole("button", { name: "Pause" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Resume" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Retry dispatch" })).toBeNull();
    expect(
      screen.getByText(
        "No lifecycle controls are available for this Factory Session state.",
      ),
    ).toBeTruthy();
  });
});
