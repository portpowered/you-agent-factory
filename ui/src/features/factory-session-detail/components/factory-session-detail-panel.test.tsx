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

    expect(screen.getByText("Loading factory session runtime…")).toBeTruthy();

    await waitFor(() => {
      expect(
        screen.getByText("Dynamic workflow (JavaScript factory session)"),
      ).toBeTruthy();
    });

    expect(screen.getByText("review")).toBeTruthy();
    expect(screen.getByText("cp-1 (plan) — saved plan checkpoint")).toBeTruthy();
    expect(screen.getByText("child agent retry scheduled")).toBeTruthy();
    expect(screen.getByText("artifact-final · FINAL_RESULT")).toBeTruthy();
    expect(screen.getByText("artifact-partial · CHILD_RESULT")).toBeTruthy();
    expect(screen.queryByText("rawCheckpointBody")).toBeNull();
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
      expect(screen.getByText("boom")).toBeTruthy();
    });
  });

  it("shows durable factory session summary fields from the durable read model", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url === "/factory-sessions/dur-sess-petri-success-001") {
        return jsonResponse({
          orchestratorKind: "PETRI",
          progress: {
            completedDispatches: 1,
            totalDispatches: 1,
          },
          resolvedSource: {
            kind: "FACTORY_ID",
            sourceRef: "factory/customer-support-triage",
          },
          resultSummary: {
            resultStatus: "FINAL",
            summary: "Ticket triaged and resolved.",
          },
          sessionId: "dur-sess-petri-success-001",
          status: "SUCCEEDED",
        });
      }
      if (url.endsWith("/results?mode=final")) {
        return jsonResponse({
          mode: "final",
          resultStatus: "FINAL",
          sessionId: "dur-sess-petri-success-001",
        });
      }
      if (url.endsWith("/results?mode=partial")) {
        return jsonResponse({
          mode: "partial",
          resultStatus: "PARTIAL",
          sessionId: "dur-sess-petri-success-001",
        });
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="dur-sess-petri-success-001" />,
    );

    await waitFor(() => {
      expect(screen.getByText("Ticket triaged and resolved.")).toBeTruthy();
    });

    expect(screen.getByText("factory/customer-support-triage")).toBeTruthy();
    expect(screen.getByText("SUCCEEDED")).toBeTruthy();
    expect(screen.getByText("FINAL")).toBeTruthy();
    expect(screen.getByText("completed 1, total 1")).toBeTruthy();
    expect(
      screen.queryByText("Dynamic workflow (JavaScript factory session)"),
    ).toBeNull();
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
