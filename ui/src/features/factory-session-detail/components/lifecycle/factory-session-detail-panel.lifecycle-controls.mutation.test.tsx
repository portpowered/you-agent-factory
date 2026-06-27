import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { FactoryOrchestratorKind } from "../../../../api/generated/openapi";
import { FactorySessionDetailPanel } from "../factory-session-detail-panel";
import {
  createDeferred,
  jsonResponse,
  renderWithQueryClient,
} from "../factory-session-detail-panel.test-helpers";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: lifecycle mutation regressions share one mocked panel harness.
describe("factory session detail lifecycle submissions", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("submits pause through the shared lifecycle route and prevents duplicate submission while pending", async () => {
    const pauseRequest = createDeferred<Response>();
    let sessionRequestCount = 0;
    const fetchMock = vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      const url = String(input);

      if (url.endsWith("/factory-sessions/dur-sess-js-running-001")) {
        sessionRequestCount += 1;
        return Promise.resolve(
          jsonResponse({
            dialect: "you-workflow-v1",
            lifecycle: {
              startedAt: "2026-06-08T14:00:00Z",
              updatedAt:
                sessionRequestCount > 1
                  ? "2026-06-08T14:06:00Z"
                  : "2026-06-08T14:05:00Z",
            },
            orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
            phase: "review",
            progress: {
              completedDispatches: 1,
              failedDispatches: 0,
              inFlightDispatches: sessionRequestCount > 1 ? 0 : 1,
              totalDispatches: 2,
            },
            resolvedSource: {
              kind: "WORKFLOW_NAME",
              sourceHash: "sha256:workflow-review",
              sourceRef: "workflow/review",
            },
            sessionId: "dur-sess-js-running-001",
            status: sessionRequestCount > 1 ? "PAUSED" : "RUNNING",
            usage: { resources: [] },
          }),
        );
      }

      if (
        url.endsWith("/factory-sessions/dur-sess-js-running-001/pause") &&
        init?.method === "POST"
      ) {
        return pauseRequest.promise;
      }

      return Promise.resolve(new Response("not found", { status: 404 }));
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="dur-sess-js-running-001" />,
    );

    const user = userEvent.setup();
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Pause" })).toBeTruthy();
    });

    const pauseButton = screen.getByRole("button", { name: "Pause" });
    await user.click(pauseButton);

    await waitFor(() => {
      expect(pauseButton.getAttribute("aria-busy")).toBe("true");
    });
    expect((pauseButton as HTMLButtonElement).disabled).toBe(true);

    await user.click(pauseButton);

    expect(fetchMock).toHaveBeenCalledTimes(2);

    pauseRequest.resolve(
      jsonResponse({
        detail: "Pause request was queued.",
        operation: "PAUSE",
        outcome: "ACCEPTED",
        sessionId: "dur-sess-js-running-001",
        session: {
          dialect: "you-workflow-v1",
          lifecycle: {
            pausedAt: "2026-06-08T14:06:00Z",
            startedAt: "2026-06-08T14:00:00Z",
            updatedAt: "2026-06-08T14:06:00Z",
          },
          orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
          phase: "review",
          progress: {
            completedDispatches: 1,
            failedDispatches: 0,
            inFlightDispatches: 0,
            totalDispatches: 2,
          },
          resolvedSource: {
            kind: "WORKFLOW_NAME",
            sourceHash: "sha256:workflow-review",
            sourceRef: "workflow/review",
          },
          sessionId: "dur-sess-js-running-001",
          status: "PAUSED",
          usage: { resources: [] },
        },
        status: "PAUSED",
      }, 202),
    );

    await waitFor(() => {
      expect(pauseButton.getAttribute("aria-busy")).toBeNull();
    });
    await waitFor(() => {
      expect(screen.getByText("Accepted")).toBeTruthy();
      expect(screen.getByText("Pause accepted")).toBeTruthy();
      expect(
        screen.getByText(
          "Pause request was queued. Current durable status: Paused.",
        ),
      ).toBeTruthy();
    });
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Resume" })).toBeTruthy();
      expect(screen.queryByRole("button", { name: "Pause" })).toBeNull();
      expect(screen.getByText("Pause accepted")).toBeTruthy();
    });
  });

  it("submits retry-dispatch with the currently selected failed dispatch", async () => {
    const fetchMock = vi.mocked(globalThis.fetch).mockImplementation(
      async (input, init) => {
        const url = String(input);

        if (url.endsWith("/factory-sessions/dur-sess-js-failed-partial-001")) {
          return jsonResponse({
            dialect: "you-workflow-v1",
            lifecycle: {
              finishedAt: "2026-06-08T14:05:00Z",
              startedAt: "2026-06-08T14:00:00Z",
            },
            orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
            phase: "verify",
            progress: {
              completedDispatches: 1,
              failedDispatches: 1,
              inFlightDispatches: 0,
              totalDispatches: 2,
            },
            resolvedSource: {
              kind: "WORKFLOW_NAME",
              sourceHash: "sha256:workflow-verify",
              sourceRef: "workflow/verify",
            },
            sessionId: "dur-sess-js-failed-partial-001",
            status: "FAILED",
            usage: { resources: [] },
          });
        }

        if (
          url.endsWith(
            "/factory-sessions/dur-sess-js-failed-partial-001/dispatches",
          )
        ) {
          return jsonResponse({
            dispatches: [
              {
                dispatchKind: "JAVASCRIPT_VERIFY",
                id: "dispatch-failed",
                orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
                sessionId: "dur-sess-js-failed-partial-001",
                status: "FAILED",
              },
            ],
            sessionId: "dur-sess-js-failed-partial-001",
          });
        }

        if (
          url.endsWith(
            "/factory-sessions/dur-sess-js-failed-partial-001/retry-dispatch",
          ) &&
          init?.method === "POST"
        ) {
          expect(init.body).toBe(
            JSON.stringify({
              dispatchId: "dispatch-failed",
              forceNewAttempt: false,
              resetAttemptCount: false,
            }),
          );

          return jsonResponse({
            dispatchId: "dispatch-failed",
            operation: "RETRY_DISPATCH",
            outcome: "ACCEPTED",
            retryDispatchId: "dispatch-retry-001",
            sessionId: "dur-sess-js-failed-partial-001",
            status: "RUNNING",
          }, 202);
        }

        return new Response("not found", { status: 404 });
      },
    );

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="dur-sess-js-failed-partial-001" />,
    );

    const user = userEvent.setup();
    await waitFor(() => {
      expect(
        screen.getByRole("button", {
          name: "Expand dispatch detail for dispatch-failed",
        }),
      ).toBeTruthy();
    });

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

    await user.click(screen.getByRole("button", { name: "Retry dispatch" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/factory-sessions/dur-sess-js-failed-partial-001/retry-dispatch",
        expect.objectContaining({
          body: JSON.stringify({
            dispatchId: "dispatch-failed",
            forceNewAttempt: false,
            resetAttemptCount: false,
          }),
          method: "POST",
        }),
      );
    });
  });

  // biome-ignore lint/complexity/noExcessiveLinesPerFunction: retry conflict coverage keeps one fetch seam readable.
  it("renders typed conflict feedback without collapsing the selected dispatch detail", async () => {
    const fetchMock = vi.mocked(globalThis.fetch).mockImplementation(
      async (input, init) => {
        const url = String(input);

        if (url.endsWith("/factory-sessions/dur-sess-js-failed-002")) {
          return jsonResponse({
            dialect: "you-workflow-v1",
            lifecycle: {
              finishedAt: "2026-06-08T14:04:00Z",
              startedAt: "2026-06-08T14:00:00Z",
              updatedAt: "2026-06-08T14:04:00Z",
            },
            orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
            phase: "verify",
            progress: {
              completedDispatches: 1,
              failedDispatches: 1,
              inFlightDispatches: 0,
              totalDispatches: 2,
            },
            resolvedSource: {
              kind: "WORKFLOW_NAME",
              sourceHash: "sha256:workflow-verify",
              sourceRef: "workflow/verify",
            },
            sessionId: "dur-sess-js-failed-002",
            status: "FAILED",
            usage: { resources: [] },
          });
        }

        if (url.endsWith("/factory-sessions/dur-sess-js-failed-002/dispatches")) {
          return jsonResponse({
            dispatches: [
              {
                dispatchKind: "JAVASCRIPT_VERIFY",
                id: "dispatch-failed",
                label: "Verify release manifest",
                orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
                sessionId: "dur-sess-js-failed-002",
                status: "FAILED",
              },
            ],
            sessionId: "dur-sess-js-failed-002",
          });
        }

        if (
          url.endsWith(
            "/factory-sessions/dur-sess-js-failed-002/dispatches/dispatch-failed",
          )
        ) {
          return jsonResponse({
            dispatchKind: "JAVASCRIPT_VERIFY",
            failureDetail: {
              errorClass: "verification_error",
              message: "Expected release manifest checksum.",
              reason: "VERIFY_ASSERTION_FAILED",
            },
            id: "dispatch-failed",
            javascript: {
              executionMode: "live",
              taskKind: "VERIFY",
              taskLabel: "Verify release manifest",
            },
            label: "Verify release manifest",
            orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
            sessionId: "dur-sess-js-failed-002",
            status: "FAILED",
            statusTransitions: ["QUEUED", "RUNNING", "FAILED"],
          });
        }

        if (
          url.endsWith("/factory-sessions/dur-sess-js-failed-002/retry-dispatch") &&
          init?.method === "POST"
        ) {
          return jsonResponse({
            detail: "Another retry request is still being applied.",
            dispatchId: "dispatch-failed",
            operation: "RETRY_DISPATCH",
            outcome: "CONFLICT",
            sessionId: "dur-sess-js-failed-002",
            status: "FAILED",
          }, 409);
        }

        return new Response("not found", { status: 404 });
      },
    );

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="dur-sess-js-failed-002" />,
    );

    const user = userEvent.setup();
    await waitFor(() => {
      expect(
        screen.getByRole("button", {
          name: "Expand dispatch detail for dispatch-failed",
        }),
      ).toBeTruthy();
    });

    await user.click(
      screen.getByRole("button", {
        name: "Expand dispatch detail for dispatch-failed",
      }),
    );

    await waitFor(() => {
      expect(screen.getByText("Failure detail")).toBeTruthy();
      expect(screen.getByRole("button", { name: "Retry dispatch" })).toBeTruthy();
    });

    await user.click(screen.getByRole("button", { name: "Retry dispatch" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/factory-sessions/dur-sess-js-failed-002/retry-dispatch",
        expect.objectContaining({
          body: JSON.stringify({
            dispatchId: "dispatch-failed",
            forceNewAttempt: false,
            resetAttemptCount: false,
          }),
          method: "POST",
        }),
      );
    });

    await waitFor(() => {
      expect(screen.getByText("Conflict")).toBeTruthy();
      expect(
        screen.getByText("Retry dispatch is blocked by another lifecycle change."),
      ).toBeTruthy();
      expect(
        screen.getByText(
          "Another retry request is still being applied. Current durable status: Failed.",
        ),
      ).toBeTruthy();
      expect(screen.getByText("Failure detail")).toBeTruthy();
      expect(screen.getByText("VERIFY_ASSERTION_FAILED")).toBeTruthy();
    });
  });
});
