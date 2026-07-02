// biome-ignore-all lint/nursery/noExcessiveLinesPerFile: dispatch drilldown boundary states share one fetch harness.
// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: dispatch drilldown boundary states share one fetch harness.
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { FactoryOrchestratorKind } from "../../../api/generated/openapi";
import { FactorySessionDetailPanel } from "./factory-session-detail-panel";
import {
  jsonResponse,
  renderWithQueryClient,
} from "./test-support/factory-session-detail-panel.test-helpers";

const SESSION_ID = "session-beta";
const PRIMARY_PROVIDER_SESSION =
  "Provider session: codex / session_id / provider-session-1";

function mockBoundaryFetch() {
  vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
    const url = String(input);
    if (url.endsWith(`/factory-sessions/${SESSION_ID}`)) {
      return jsonResponse({
        factoryDir: "/workspace/root/beta",
        folderPath: "/workspace/root",
        id: SESSION_ID,
        isDefault: false,
        project: "beta",
        runtime: {
          dispatches: [
            {
              dispatchKind: "JAVASCRIPT_AGENT",
              id: "dispatch-success",
              javascript: {
                executionMode: "live",
                taskKind: "AGENT",
                taskLabel: "Review child task",
              },
              orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
              providerSessionRefs: [
                {
                  id: "provider-session-1",
                  kind: "session_id",
                  provider: "codex",
                },
              ],
              sessionId: SESSION_ID,
              status: "COMPLETED",
              warnings: [
                {
                  code: "DISPATCH_WARNING",
                  message: "Token budget was nearly exhausted.",
                },
              ],
            },
            {
              dispatchKind: "JAVASCRIPT_AGENT",
              id: "dispatch-missing",
              javascript: {
                executionMode: "live",
                taskKind: "AGENT",
                taskLabel: "Missing child detail",
              },
              orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
              providerSessionRefs: [
                {
                  id: "provider-session-2",
                  kind: "session_id",
                  provider: "codex",
                },
              ],
              sessionId: SESSION_ID,
              status: "FAILED",
            },
            {
              dispatchKind: "JAVASCRIPT_AGENT",
              id: "dispatch-error",
              javascript: {
                executionMode: "live",
                taskKind: "AGENT",
                taskLabel: "Errored child detail",
              },
              orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
              providerSessionRefs: [
                {
                  id: "provider-session-3",
                  kind: "session_id",
                  provider: "codex",
                },
              ],
              sessionId: SESSION_ID,
              status: "FAILED",
            },
          ],
          javascript: {
            childDispatchCounts: {
              completed: 1,
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
    if (url.endsWith(`/factory-sessions/${SESSION_ID}/result`)) {
      return jsonResponse({
        resultArtifactRef: {
          id: "artifact-final",
          kind: "FINAL_RESULT",
          visibility: "CUSTOMER",
        },
        sessionId: SESSION_ID,
        status: "IDLE",
      });
    }
    if (url.endsWith(`/factory-sessions/${SESSION_ID}/partial-result`)) {
      return new Response("not found", { status: 404 });
    }
    if (url.endsWith("/dispatches/dispatch-missing")) {
      return new Response("not found", { status: 404 });
    }
    if (url.endsWith("/dispatches/dispatch-error")) {
      return new Response(
        JSON.stringify({ code: "INTERNAL_ERROR", message: "dispatch boom" }),
        {
          headers: { "Content-Type": "application/json" },
          status: 500,
        },
      );
    }
    return new Response("not found", { status: 404 });
  });
}

function expectSummaryState() {
  expect(screen.getAllByText("Execution mode: live").length).toBeGreaterThan(0);
  expect(screen.getByText(PRIMARY_PROVIDER_SESSION)).toBeTruthy();
  expect(screen.getByText("artifact-final · FINAL_RESULT")).toBeTruthy();
}

describe("FactorySessionDetailPanel dispatch detail failure payload", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("renders successful dispatch detail with status, execution mode, provider sessions, and artifacts", async () => {
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
                dispatchKind: "JAVASCRIPT_AGENT",
                id: "dispatch-success",
                label: "Draft response",
                orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
                sessionId: "session-beta",
                status: "COMPLETED",
              },
            ],
            javascript: {
              childDispatchCounts: {
                completed: 1,
                queued: 0,
                running: 0,
              },
              phases: [],
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
      if (url.endsWith("/factory-sessions/session-beta/partial-result")) {
        return new Response("not found", { status: 404 });
      }
      if (
        url.endsWith(
          "/factory-sessions/session-beta/dispatches/dispatch-success",
        )
      ) {
        return jsonResponse({
          artifactIds: ["artifact-final-1", "artifact-log-2"],
          attempt: 2,
          dispatchKind: "JAVASCRIPT_AGENT",
          id: "dispatch-success",
          javascript: {
            executionMode: "live",
            taskKind: "AGENT",
            taskLabel: "Draft response",
          },
          label: "Draft response",
          model: "gpt-5.5",
          orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
          phase: "deliver",
          provider: "openai",
          providerSessionRefs: [
            {
              id: "sess_codex_1",
              kind: "session_id",
              provider: "codex",
            },
          ],
          relatedWorkIds: ["work-alpha"],
          runnerId: "runner-web-1",
          sessionId: "session-beta",
          status: "COMPLETED",
          statusTransitions: ["QUEUED", "RUNNING", "COMPLETED"],
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
        name: "Expand dispatch detail for dispatch-success",
      }),
    );

    await waitFor(() => {
      expect(screen.getByText("JavaScript task")).toBeTruthy();
    });

    expect(screen.getAllByText("COMPLETED").length).toBeGreaterThan(0);
    expect(screen.getByText("live")).toBeTruthy();
    expect(screen.getByText("Provider sessions")).toBeTruthy();
    expect(screen.getByText("session_id · sess_codex_1")).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "artifact-final-1" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "artifact-log-2" }),
    ).toBeTruthy();
    expect(screen.getByText("Runtime")).toBeTruthy();
    expect(screen.getByText("JavaScript workflow")).toBeTruthy();
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
    expect(screen.getByText("Failure detail")).toBeTruthy();
  });

  it("renders warning dispatch detail with typed warning data", async () => {
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
                id: "dispatch-warning",
                orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
                sessionId: "session-beta",
                status: "COMPLETED",
                warnings: [
                  {
                    code: "DISPATCH_WARNING",
                    message: "Verification completed with non-blocking warnings.",
                  },
                ],
              },
            ],
            javascript: {
              childDispatchCounts: {
                completed: 1,
                queued: 0,
                running: 0,
              },
              phases: [],
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
      if (url.endsWith("/factory-sessions/session-beta/partial-result")) {
        return new Response("not found", { status: 404 });
      }
      if (
        url.endsWith(
          "/factory-sessions/session-beta/dispatches/dispatch-warning",
        )
      ) {
        return jsonResponse({
          artifactIds: ["artifact-warning-log"],
          dispatchKind: "JAVASCRIPT_VERIFY",
          id: "dispatch-warning",
          javascript: {
            executionMode: "live",
            taskKind: "VERIFY",
            taskLabel: "Verify docs",
          },
          orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
          sessionId: "session-beta",
          status: "COMPLETED",
          warnings: [
            {
              code: "DISPATCH_WARNING",
              message: "Verification completed with non-blocking warnings.",
            },
          ],
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
        name: "Expand dispatch detail for dispatch-warning",
      }),
    );

    await waitFor(() => {
      expect(screen.getByText("DISPATCH_WARNING")).toBeTruthy();
    });

    expect(
      screen.getAllByText("Dispatch warnings").length,
    ).toBeGreaterThanOrEqual(1);

    expect(
      screen.getAllByText(
        "Verification completed with non-blocking warnings.",
      ).length,
    ).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("DISPATCH_WARNING")).toBeTruthy();
    expect(screen.queryByText("Failure detail")).toBeNull();
    expect(screen.getByText("Runtime")).toBeTruthy();
  });
});

describe("FactorySessionDetailPanel dispatch detail boundary states", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("preserves live-provider summary details while dispatch detail hits missing and error states", async () => {
    mockBoundaryFetch();

    renderWithQueryClient(<FactorySessionDetailPanel sessionID={SESSION_ID} />);

    await waitFor(() => {
      expect(screen.getByText("Dispatches")).toBeTruthy();
    });

    expectSummaryState();
    expect(screen.getByText("Token budget was nearly exhausted.")).toBeTruthy();

    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", {
        name: "Expand dispatch detail for dispatch-missing",
      }),
    );

    await waitFor(() => {
      expect(
        screen.getByText(
          "Dispatch detail for dispatch-missing is no longer available.",
        ),
      ).toBeTruthy();
    });

    expectSummaryState();

    await user.click(
      screen.getByRole("button", {
        name: "Expand dispatch detail for dispatch-error",
      }),
    );

    await waitFor(() => {
      expect(screen.getByText("dispatch boom")).toBeTruthy();
    });

    expectSummaryState();
  });
});
