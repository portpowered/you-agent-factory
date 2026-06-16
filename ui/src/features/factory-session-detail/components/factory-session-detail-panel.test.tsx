// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: orchestrator-aware session detail states share one fetch harness and assertion seam.
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import { FactoryOrchestratorKind } from "../../../api/generated/openapi";
import { FactorySessionDetailPanel } from "./factory-session-detail-panel";

describe("FactorySessionDetailPanel", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows loading and success states for a JavaScript factory session", async () => {
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
            artifacts: [
              {
                id: "artifact-1",
                kind: "CHILD_RESULT",
                label: "review output",
                visibility: "CUSTOMER",
              },
            ],
            dispatches: [
              {
                dispatchKind: "JAVASCRIPT_AGENT",
                id: "dispatch-1",
                orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
                sessionId: "session-beta",
                status: "COMPLETED",
                warnings: [
                  {
                    code: "DISPATCH_WARNING",
                    message: "child agent retry scheduled",
                  },
                ],
              },
            ],
            javascript: {
              checkpoints: [
                {
                  id: "cp-1",
                  label: "plan",
                  summary: "saved plan checkpoint",
                },
              ],
              childDispatchCounts: {
                completed: 4,
                queued: 1,
                running: 2,
              },
              phase: "review",
              phases: ["plan", "review"],
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
        return jsonResponse({
          resultArtifactRef: {
            id: "artifact-final",
            kind: "FINAL_RESULT",
            visibility: "CUSTOMER",
          },
          sessionId: "session-beta",
          status: "IDLE",
        });
      }
      if (url.endsWith("/factory-sessions/session-beta/partial-result")) {
        return jsonResponse({
          partialResultArtifactRef: {
            id: "artifact-partial",
            kind: "CHILD_RESULT",
            visibility: "CUSTOMER",
          },
          phase: "review",
          sessionId: "session-beta",
        });
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="session-beta" />,
    );

    expect(screen.getByRole("status").textContent).toContain(
      "Loading factory session runtime…",
    );

    await waitFor(() => {
      expect(screen.getByText("Runtime")).toBeTruthy();
    });

    expect(screen.getByText("JavaScript workflow")).toBeTruthy();
    expect(screen.getAllByText("Idle").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("review")).toBeTruthy();
    expect(screen.getByText("cp-1 (plan) — saved plan checkpoint")).toBeTruthy();
    expect(screen.getByText("child agent retry scheduled")).toBeTruthy();
    expect(screen.getByText("artifact-final · FINAL_RESULT")).toBeTruthy();
    expect(screen.getByText("artifact-partial · CHILD_RESULT")).toBeTruthy();
    expect(screen.queryByText("rawCheckpointBody")).toBeNull();
  });

  it("shows durable JavaScript session summary from shared typed session data", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith("/factory-sessions/dur-sess-js-run-n-001")) {
        return jsonResponse({
          dialect: "you-workflow-v1",
          lifecycle: {
            startedAt: "2026-06-08T14:00:00Z",
            updatedAt: "2026-06-08T14:05:00Z",
          },
          orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
          phase: "verify",
          phaseSummaries: [{ dispatchCount: 1, phase: "plan" }],
          progress: {
            completedDispatches: 1,
            failedDispatches: 0,
            inFlightDispatches: 1,
            totalDispatches: 3,
          },
          resolvedSource: {
            kind: "WORKFLOW_NAME",
            sourceRef: "workflow/release-train",
            sourceHash: "sha256:js-workflow-release-train",
          },
          sessionId: "dur-sess-js-run-n-001",
          status: "RUNNING",
          usage: { resources: [] },
        });
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="dur-sess-js-run-n-001" />,
    );

    await waitFor(() => {
      expect(screen.getByText("JavaScript workflow")).toBeTruthy();
    });

    expect(screen.getAllByText("Running").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("verify")).toBeTruthy();
    expect(screen.queryAllByText("Idle")).toHaveLength(0);
  });

  it("shows durable JavaScript dispatch, warning, artifact, and result inspection details", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith("/factory-sessions/dur-sess-js-success-002")) {
        return jsonResponse({
          artifactRefs: [
            {
              id: "art-js-success-001",
              kind: "FINAL_RESULT",
              label: "Docs refresh output",
              visibility: "PUBLIC",
            },
          ],
          dialect: "you-workflow-v1",
          lifecycle: {
            finishedAt: "2026-06-08T13:10:00Z",
            startedAt: "2026-06-08T13:00:02Z",
          },
          orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
          progress: {
            completedDispatches: 2,
            failedDispatches: 0,
            inFlightDispatches: 0,
            totalDispatches: 2,
          },
          resolvedSource: {
            kind: "WORKFLOW_FILE",
            sourceRef: "workflow/.claude/workflows/docs-refresh.yaml",
            sourceHash: "sha256:js-workflow-docs-refresh",
          },
          resultSummary: {
            resultStatus: "FINAL",
            summary: "Documentation refresh complete.",
          },
          sessionId: "dur-sess-js-success-002",
          status: "SUCCEEDED",
          usage: { resources: [] },
        });
      }
      if (url.endsWith("/factory-sessions/dur-sess-js-success-002/dispatches")) {
        return jsonResponse({
          dispatches: [
            {
              attempt: 1,
              dispatchKind: "JAVASCRIPT_AGENT",
              id: "disp-js-success-001",
              label: "draft-docs",
              status: "COMPLETED",
            },
            {
              attempt: 1,
              dispatchKind: "JAVASCRIPT_VERIFY",
              id: "disp-js-success-002",
              label: "verify-docs",
              outputArtifactIds: ["art-js-success-001"],
              status: "COMPLETED",
              warnings: [
                {
                  code: "DISPATCH_WARNING",
                  message: "child output truncated for display",
                },
              ],
            },
          ],
          sessionId: "dur-sess-js-success-002",
        });
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="dur-sess-js-success-002" />,
    );

    await waitFor(() => {
      expect(screen.getByText("Child dispatch activity")).toBeTruthy();
    });

    expect(
      screen.getByText("disp-js-success-002 (verify-docs) · COMPLETED"),
    ).toBeTruthy();
    expect(screen.getByText("child output truncated for display")).toBeTruthy();
    expect(screen.getByText("FINAL_RESULT")).toBeTruthy();
    expect(screen.getByText("art-js-success-001 · FINAL_RESULT")).toBeTruthy();
    expect(screen.getByText("queued 0, running 0, completed 2")).toBeTruthy();
  });

  it("shows Petri marking and enabled transitions without dynamic workflow shorthand", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      jsonResponse({
        factoryDir: "/workspace/root",
        folderPath: "/workspace/root",
        id: "~default",
        isDefault: true,
        project: "root",
        runtime: {
          lifecycle: {
            startedAt: "2026-06-08T14:00:00Z",
            updatedAt: "2026-06-08T14:05:00Z",
          },
          orchestratorKind: FactoryOrchestratorKind.PETRI,
          petri: {
            enabledTransitions: [
              {
                transitionId: "tr-process",
                workerType: "worker-a",
              },
            ],
            marking: [{ id: "tok-1" }],
          },
          progress: {
            categories: {},
            factoryState: "RUNNING",
            inFlightCount: 0,
            totalTokens: 1,
          },
          status: "IDLE",
          usage: { resources: [] },
        },
        target: { kind: "default" },
      }),
    );

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="~default" />,
    );

    await waitFor(() => {
      expect(screen.getByText("1 token")).toBeTruthy();
    });

    expect(screen.getByText("tr-process (worker-a)")).toBeTruthy();
    expect(
      screen.queryByText("Dynamic workflow (JavaScript factory session)"),
    ).toBeNull();
  });

  it("shows an error state when the factory session API fails", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      new Response(JSON.stringify({ code: "INTERNAL_ERROR", message: "boom" }), {
        headers: { "Content-Type": "application/json" },
        status: 500,
      }),
    );

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="session-beta" />,
    );

    await waitFor(() => {
      expect(screen.getByRole("alert").textContent).toContain("boom");
    });
    expect(screen.getByText("session-beta")).toBeTruthy();
  });

  it("shows a not-found state when the durable JavaScript session is missing", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      new Response(
        JSON.stringify({
          code: "NOT_FOUND",
          message: "Factory session not found.",
        }),
        {
          headers: { "Content-Type": "application/json" },
          status: 404,
          statusText: "Not Found",
        },
      ),
    );

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="dur-sess-js-missing-001" />,
    );

    expect(screen.getByRole("status").textContent).toContain(
      "Loading factory session runtime…",
    );
    expect(screen.getByText("dur-sess-js-missing-001")).toBeTruthy();

    await waitFor(() => {
      expect(screen.getByRole("status").textContent).toContain(
        "This factory session is no longer available.",
      );
    });
    expect(screen.queryByText("Runtime")).toBeNull();
  });

  it("renders zh-CN durable JavaScript loading and missing states", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      new Response(
        JSON.stringify({
          code: "NOT_FOUND",
          message: "Factory session not found.",
        }),
        {
          headers: { "Content-Type": "application/json" },
          status: 404,
          statusText: "Not Found",
        },
      ),
    );

    renderWithQueryClient(
      <FactorySessionDetailPanel
        locale="zh-CN"
        sessionID="dur-sess-js-missing-001"
      />,
    );

    expect(screen.getByRole("status").textContent).toContain(
      "正在加载工厂会话运行时…",
    );

    await waitFor(() => {
      expect(screen.getByRole("status").textContent).toContain(
        "此工厂会话已不可用。",
      );
    });
  });
});

function renderWithQueryClient(children: ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>,
  );
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    headers: { "Content-Type": "application/json" },
    status,
  });
}
