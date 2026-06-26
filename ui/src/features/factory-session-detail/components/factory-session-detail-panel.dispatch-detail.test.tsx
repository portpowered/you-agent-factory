import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { FactoryOrchestratorKind } from "../../../api/generated/openapi";
import { FactorySessionDetailPanel } from "./factory-session-detail-panel";
import {
  jsonResponse,
  renderWithQueryClient,
} from "./factory-session-detail-panel.test-helpers";

describe("FactorySessionDetailPanel dispatch detail", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("renders failed dispatch detail with typed failure data and artifact links", async () => {
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
            dispatches: [
              {
                dispatchKind: "JAVASCRIPT_VERIFY",
                id: "dispatch-failed",
                orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
                sessionId: "session-beta",
                status: "FAILED",
              },
            ],
            javascript: {
              childDispatchCounts: {
                completed: 0,
                queued: 0,
                running: 0,
              },
              phases: [],
              scriptStatus: "FAILED",
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
            status: "FAILED",
            usage: { resources: [] },
          },
          target: { kind: "named", name: "beta" },
        });
      }
      if (url.endsWith("/factory-sessions/session-beta/result")) {
        return new Response("not found", { status: 404 });
      }
      if (url.endsWith("/factory-sessions/session-beta/partial-result")) {
        return new Response("not found", { status: 404 });
      }
      if (
        url.endsWith("/factory-sessions/session-beta/dispatches/dispatch-failed")
      ) {
        return jsonResponse({
          artifactIds: ["artifact-failure-log"],
          dispatchKind: "JAVASCRIPT_VERIFY",
          failureDetail: {
            errorClass: " verification_error ",
            message: " Expected release manifest checksum. ",
            reason: " VERIFY_ASSERTION_FAILED ",
          },
          id: "dispatch-failed",
          javascript: {
            executionMode: " live ",
            taskKind: "VERIFY",
            taskLabel: " verify docs ",
          },
          orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
          relatedWorkIds: ["work-failed-1"],
          sessionId: "session-beta",
          status: "FAILED",
          statusTransitions: ["QUEUED", "RUNNING", "FAILED"],
        });
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(<FactorySessionDetailPanel sessionID="session-beta" />);

    await waitFor(() => {
      expect(screen.getByText("Dispatches")).toBeTruthy();
    });

    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", {
        name: "Expand dispatch detail for dispatch-failed",
      }),
    );

    await waitFor(() => {
      expect(screen.getByText("Failure detail")).toBeTruthy();
    });

    expect(screen.getByText("VERIFY_ASSERTION_FAILED")).toBeTruthy();
    expect(screen.getByText("verification_error")).toBeTruthy();
    expect(
      screen.getByText("Expected release manifest checksum."),
    ).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "artifact-failure-log" }),
    ).toBeTruthy();
    expect(screen.getAllByText("FAILED").length).toBeGreaterThan(1);
  });
});
