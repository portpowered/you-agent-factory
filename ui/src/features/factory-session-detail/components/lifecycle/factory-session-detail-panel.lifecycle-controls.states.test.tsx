import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { FactoryOrchestratorKind } from "../../../../api/generated/openapi";
import { FactorySessionDetailPanel } from "../factory-session-detail-panel";
import {
  createDeferred,
  jsonResponse,
  renderWithQueryClient,
} from "../test-support/factory-session-detail-panel.test-helpers";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: lifecycle state regressions share one mocked panel harness.
describe("factory session detail lifecycle control states", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("disables sibling lifecycle actions while one control request is pending", async () => {
    const pauseRequest = createDeferred<Response>();
    vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      const url = String(input);

      if (url.endsWith("/factory-sessions/dur-sess-js-running-001")) {
        return Promise.resolve(
          jsonResponse({
            dialect: "you-workflow-v1",
            lifecycle: {
              startedAt: "2026-06-08T14:00:00Z",
              updatedAt: "2026-06-08T14:05:00Z",
            },
            orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
            phase: "review",
            progress: {
              completedDispatches: 1,
              failedDispatches: 0,
              inFlightDispatches: 1,
              totalDispatches: 2,
            },
            resolvedSource: {
              kind: "WORKFLOW_NAME",
              sourceHash: "sha256:workflow-review",
              sourceRef: "workflow/review",
            },
            sessionId: "dur-sess-js-running-001",
            status: "RUNNING",
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
    const cancelButton = screen.getByRole("button", { name: "Cancel" });
    const terminateButton = screen.getByRole("button", { name: "Terminate" });

    await user.click(pauseButton);

    await waitFor(() => {
      expect(pauseButton.getAttribute("aria-busy")).toBe("true");
    });
    expect((pauseButton as HTMLButtonElement).disabled).toBe(true);
    expect((cancelButton as HTMLButtonElement).disabled).toBe(true);
    expect((terminateButton as HTMLButtonElement).disabled).toBe(true);
    expect(cancelButton.getAttribute("aria-busy")).toBeNull();
    expect(terminateButton.getAttribute("aria-busy")).toBeNull();

    pauseRequest.resolve(
      jsonResponse(
        {
          detail: "Pause request was queued.",
          operation: "PAUSE",
          outcome: "ACCEPTED",
          sessionId: "dur-sess-js-running-001",
          status: "PAUSED",
        },
        202,
      ),
    );

    await waitFor(() => {
      expect(pauseButton.getAttribute("aria-busy")).toBeNull();
    });
  });

  it("submits lifecycle controls from keyboard activation", async () => {
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
            phase: "review",
            progress: {
              completedDispatches: 1,
              failedDispatches: 0,
              inFlightDispatches: 1,
              totalDispatches: 2,
            },
            resolvedSource: {
              kind: "WORKFLOW_NAME",
              sourceHash: "sha256:workflow-review",
              sourceRef: "workflow/review",
            },
            sessionId: "dur-sess-js-running-001",
            status: "RUNNING",
            usage: { resources: [] },
          });
        }

        if (
          url.endsWith("/factory-sessions/dur-sess-js-running-001/pause") &&
          init?.method === "POST"
        ) {
          return jsonResponse(
            {
              detail: "Pause request was queued.",
              operation: "PAUSE",
              outcome: "ACCEPTED",
              sessionId: "dur-sess-js-running-001",
              status: "PAUSED",
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
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Pause" })).toBeTruthy();
    });

    const pauseButton = screen.getByRole("button", { name: "Pause" });
    pauseButton.focus();
    await user.keyboard("{Enter}");

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/factory-sessions/dur-sess-js-running-001/pause",
        expect.objectContaining({ method: "POST" }),
      );
      expect(screen.getByText("Pause accepted")).toBeTruthy();
    });
  });

  it("renders transport failures with bounded feedback while keeping detail content readable", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input, init) => {
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
            failedDispatches: 0,
            inFlightDispatches: 1,
            totalDispatches: 2,
          },
          resolvedSource: {
            kind: "WORKFLOW_NAME",
            sourceHash: "sha256:workflow-review",
            sourceRef: "workflow/review",
          },
          sessionId: "dur-sess-js-running-001",
          status: "RUNNING",
          usage: { resources: [] },
        });
      }

      if (
        url.endsWith("/factory-sessions/dur-sess-js-running-001/pause") &&
        init?.method === "POST"
      ) {
        return jsonResponse(
          {
            code: "INTERNAL_ERROR",
            message: "Lifecycle control service unavailable.",
          },
          500,
        );
      }

      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="dur-sess-js-running-001" />,
    );

    const user = userEvent.setup();
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Pause" })).toBeTruthy();
    });

    await user.click(screen.getByRole("button", { name: "Pause" }));

    await waitFor(() => {
      expect(screen.getByText("Request failed")).toBeTruthy();
      expect(screen.getByText("Pause could not be submitted.")).toBeTruthy();
      expect(
        screen.getByText("Lifecycle control service unavailable."),
      ).toBeTruthy();
      expect(screen.getByText("Lifecycle controls")).toBeTruthy();
      expect(screen.getByText("Phase")).toBeTruthy();
      expect(screen.getByRole("button", { name: "Pause" })).toBeTruthy();
      expect(screen.getByRole("button", { name: "Cancel" })).toBeTruthy();
      expect(screen.getByRole("alert")).toBeTruthy();
    });
  });
});
