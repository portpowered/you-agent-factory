import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { FactoryOrchestratorKind } from "../../../../api/generated/openapi";
import { FactorySessionDetailPanel } from "../factory-session-detail-panel";
import {
  jsonResponse,
  renderWithQueryClient,
} from "../factory-session-detail-panel.test-helpers";

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
            sourceRef: "workflow/verify",
            sourceHash: "sha256:workflow-verify",
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
              dispatchKind: "JAVASCRIPT_AGENT",
              id: "dispatch-ok",
              orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
              sessionId: "dur-sess-js-failed-partial-001",
              status: "COMPLETED",
            },
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
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="dur-sess-js-failed-partial-001" />,
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
        "Select a failed dispatch to make retry available on this detail surface.",
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
