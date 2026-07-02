import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { FactoryOrchestratorKind } from "../../../api/generated/openapi";
import { FactorySessionDetailPanel } from "./factory-session-detail-panel";
import { jsonResponse, renderWithQueryClient } from "./test-support/factory-session-detail-panel.test-helpers";

const failedDispatchSummaryFixture = {
  dispatchKind: "JAVASCRIPT_VERIFY",
  id: "dispatch-failed",
  javascript: {
    executionMode: "live",
    taskKind: "VERIFY",
    taskLabel: "Verify release manifest",
  },
  label: "Verify release manifest",
  orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
  providerSessionRefs: [
    {
      id: "provider-session-verify-1",
      kind: "session_id",
      provider: "codex",
    },
  ],
  sessionId: "session-failed",
  status: "FAILED",
  warnings: [
    {
      code: "DISPATCH_WARNING",
      message: "Provider returned a partial verification trace.",
    },
  ],
};

const failedDispatchDetailFixture = {
  artifactIds: ["artifact-failure-log"],
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
  providerSessionRefs: failedDispatchSummaryFixture.providerSessionRefs,
  relatedWorkIds: ["work-verify-1"],
  sessionId: "session-failed",
  status: "FAILED",
  statusTransitions: ["QUEUED", "RUNNING", "FAILED"],
  warnings: failedDispatchSummaryFixture.warnings,
};

function installFailedDispatchFetchMock() {
  vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
    const url = String(input);
    if (url.endsWith("/factory-sessions/session-failed")) {
      return jsonResponse({
        factoryDir: "/workspace/root/beta",
        folderPath: "/workspace/root",
        id: "session-failed",
        isDefault: false,
        project: "beta",
        runtime: {
          dispatches: [failedDispatchSummaryFixture],
          javascript: {
            childDispatchCounts: {
              completed: 0,
              queued: 0,
              running: 0,
            },
            phase: "review",
            phases: ["review"],
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
          status: "IDLE",
          usage: { resources: [] },
        },
        target: { kind: "named", name: "beta" },
      });
    }
    if (url.endsWith("/factory-sessions/session-failed/result")) {
      return new Response(null, { status: 404 });
    }
    if (url.endsWith("/factory-sessions/session-failed/partial-result")) {
      return new Response(null, { status: 404 });
    }
    if (url.endsWith("/factory-sessions/session-failed/dispatches/dispatch-failed")) {
      return jsonResponse(failedDispatchDetailFixture);
    }
    return new Response("not found", { status: 404 });
  });
}

describe("FactorySessionDetailPanel failure drilldown", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows typed failure detail for a failed live-provider child dispatch", async () => {
    installFailedDispatchFetchMock();

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="session-failed" />,
    );

    await waitFor(() => {
      expect(screen.getByText("Verify release manifest")).toBeTruthy();
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
    expect(screen.getByText("Expected release manifest checksum.")).toBeTruthy();
    expect(screen.getByText("live")).toBeTruthy();
    expect(screen.getByText("codex")).toBeTruthy();
    expect(screen.getByText("session_id · provider-session-verify-1")).toBeTruthy();
    expect(
      screen.getAllByText("Provider returned a partial verification trace."),
    ).toHaveLength(2);
  });
});
