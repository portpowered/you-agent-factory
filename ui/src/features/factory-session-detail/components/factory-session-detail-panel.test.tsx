// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction lint/nursery/noExcessiveLinesPerFile: orchestrator-aware session detail states share one fetch harness and assertion seam.
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
    expect(
      screen.getByText("cp-1 (plan) — saved plan checkpoint"),
    ).toBeTruthy();
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

    renderWithQueryClient(<FactorySessionDetailPanel sessionID="~default" />);

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
      new Response(
        JSON.stringify({ code: "INTERNAL_ERROR", message: "boom" }),
        {
          headers: { "Content-Type": "application/json" },
          status: 500,
        },
      ),
    );

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="session-beta" />,
    );

    await waitFor(() => {
      expect(screen.getByText("boom")).toBeTruthy();
    });
  });

  it("shows durable loading state with accessible Factory Session detail semantics", () => {
    vi.mocked(globalThis.fetch).mockImplementation(
      () => new Promise<Response>(() => {}),
    );

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="dur-sess-running-001" />,
    );

    const loadingRegions = screen.getAllByRole("status");
    const durableLoading = loadingRegions.find(
      (region) => region.getAttribute("aria-busy") === "true",
    );
    expect(durableLoading).toBeTruthy();
    expect(screen.getByText("Loading Factory Session detail")).toBeTruthy();
    expect(
      screen.getByText("Loading Factory Session detail from durable reads…"),
    ).toBeTruthy();
    expect(screen.queryByText("Loading factory session runtime…")).toBeNull();
  });

  it("shows a distinct durable not-found state", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      new Response(JSON.stringify({ code: "NOT_FOUND", message: "missing" }), {
        headers: { "Content-Type": "application/json" },
        status: 404,
      }),
    );

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="dur-sess-missing-001" />,
    );

    await waitFor(() => {
      expect(screen.getByText("Factory Session not found")).toBeTruthy();
    });

    expect(
      screen.getByText(
        "This Factory Session is not available. It may have been removed or the id is incorrect.",
      ),
    ).toBeTruthy();
    expect(
      screen.queryByText("This factory session is no longer available."),
    ).toBeNull();
  });

  it("shows a distinct durable error state separate from not-found", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      new Response(
        JSON.stringify({ code: "INTERNAL_ERROR", message: "boom" }),
        {
          headers: { "Content-Type": "application/json" },
          status: 500,
        },
      ),
    );

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="dur-sess-error-001" />,
    );

    await waitFor(() => {
      expect(
        screen.getByText("Factory Session detail unavailable"),
      ).toBeTruthy();
    });

    expect(screen.getByText("boom")).toBeTruthy();
    expect(screen.queryByText("Factory Session not found")).toBeNull();
  });

  it("shows durable partial-result inspection for in-progress sessions", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url === "/factory-sessions/dur-sess-js-run-n-001") {
        return jsonResponse({
          orchestratorKind: "JAVASCRIPT",
          phase: "review",
          progress: {
            completedDispatches: 1,
            inFlightDispatches: 1,
            totalDispatches: 3,
          },
          resolvedSource: {
            kind: "INLINE_WORKFLOW",
            sourceRef: "inline/review-flow",
          },
          resultSummary: {
            resultStatus: "PARTIAL",
            summary: "Review checkpoint saved.",
          },
          sessionId: "dur-sess-js-run-n-001",
          status: "RUNNING",
        });
      }
      if (url.endsWith("/results?mode=final")) {
        return new Response("not ready", { status: 404 });
      }
      if (url.endsWith("/results?mode=partial")) {
        return jsonResponse({
          artifactRefs: [
            {
              id: "artifact-partial",
              kind: "CHILD_RESULT",
              visibility: "CUSTOMER",
            },
          ],
          availability: {
            message: "Partial result is available while execution continues.",
          },
          mode: "partial",
          resultStatus: "PARTIAL",
          sessionId: "dur-sess-js-run-n-001",
        });
      }
      if (url.endsWith("/dispatches")) {
        return jsonResponse({
          dispatches: [],
          sessionId: "dur-sess-js-run-n-001",
        });
      }
      if (url.endsWith("/artifacts")) {
        return jsonResponse({
          artifacts: [],
          sessionId: "dur-sess-js-run-n-001",
        });
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="dur-sess-js-run-n-001" />,
    );

    await waitFor(() => {
      expect(screen.getByText("Factory Session in progress")).toBeTruthy();
    });

    expect(
      screen.getByText(
        "This Factory Session has not produced a final result yet. Partial inspection is shown below.",
      ),
    ).toBeTruthy();
    expect(screen.getByText("Review checkpoint saved.")).toBeTruthy();
    expect(
      screen.getByText(
        "Partial result is available while execution continues.",
      ),
    ).toBeTruthy();
    expect(screen.getByText("artifact-partial · CHILD_RESULT")).toBeTruthy();
    expect(screen.queryByText("Factory Session complete")).toBeNull();
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
      if (url.endsWith("/dispatches")) {
        return jsonResponse({
          dispatches: [
            {
              dispatchKind: "PETRI_TRANSITION",
              id: "disp-petri-success-001",
              label: "plan-task",
              status: "COMPLETED",
            },
          ],
          sessionId: "dur-sess-petri-success-001",
        });
      }
      if (url.endsWith("/artifacts")) {
        return jsonResponse({
          artifacts: [
            {
              dispatchId: "disp-petri-success-001",
              id: "art-petri-final-001",
              kind: "FINAL_RESULT",
              label: "Triage summary",
              retrievalRef: {
                href: "/factory-sessions/dur-sess-petri-success-001/artifacts/art-petri-final-001",
                method: "GET",
              },
              visibility: "PUBLIC",
            },
          ],
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

    expect(screen.getByText("Factory Session complete")).toBeTruthy();
    expect(
      screen.getByText(
        "This Factory Session reached a terminal state. Final inspection is shown below.",
      ),
    ).toBeTruthy();
    expect(screen.getByText("factory/customer-support-triage")).toBeTruthy();
    expect(screen.getByText("SUCCEEDED")).toBeTruthy();
    expect(screen.getByText("FINAL")).toBeTruthy();
    expect(screen.getByText("completed 1, total 1")).toBeTruthy();
    expect(
      screen.getByText(
        "disp-petri-success-001 · COMPLETED · PETRI_TRANSITION · plan-task",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText("art-petri-final-001 · FINAL_RESULT · Triage summary"),
    ).toBeTruthy();
    expect(screen.getByText("disp-petri-success-001")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Inspect Artifact" })).toBeTruthy();
    expect(screen.queryByText("Factory Session in progress")).toBeNull();
    expect(
      screen.queryByText("Dynamic workflow (JavaScript factory session)"),
    ).toBeNull();
  });

  it("shows durable dispatch and artifact empty states when lists are empty", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url === "/factory-sessions/dur-sess-empty-inspection-001") {
        return jsonResponse({
          orchestratorKind: "PETRI",
          resolvedSource: {
            kind: "FACTORY_ID",
            sourceRef: "factory/empty-inspection",
          },
          sessionId: "dur-sess-empty-inspection-001",
          status: "RUNNING",
        });
      }
      if (url.endsWith("/results?mode=final")) {
        return new Response("not ready", { status: 404 });
      }
      if (url.endsWith("/results?mode=partial")) {
        return jsonResponse({
          mode: "partial",
          resultStatus: "PARTIAL",
          sessionId: "dur-sess-empty-inspection-001",
        });
      }
      if (url.endsWith("/dispatches")) {
        return jsonResponse({
          dispatches: [],
          sessionId: "dur-sess-empty-inspection-001",
        });
      }
      if (url.endsWith("/artifacts")) {
        return jsonResponse({
          artifacts: [],
          sessionId: "dur-sess-empty-inspection-001",
        });
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="dur-sess-empty-inspection-001" />,
    );

    await waitFor(() => {
      expect(screen.getByText("Factory Session in progress")).toBeTruthy();
    });

    expect(
      screen.getByText("This Factory Session has no Dispatches yet."),
    ).toBeTruthy();
    expect(
      screen.getByText("This Factory Session has no Artifacts yet."),
    ).toBeTruthy();
  });

  it("shows durable Provider Session inspection links when dispatch refs are present", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url === "/factory-sessions/dur-sess-provider-session-001") {
        return jsonResponse({
          orchestratorKind: "PETRI",
          resolvedSource: {
            kind: "FACTORY_ID",
            sourceRef: "factory/provider-session",
          },
          sessionId: "dur-sess-provider-session-001",
          status: "RUNNING",
        });
      }
      if (url.endsWith("/results?mode=final")) {
        return new Response("not ready", { status: 404 });
      }
      if (url.endsWith("/results?mode=partial")) {
        return jsonResponse({
          mode: "partial",
          resultStatus: "PARTIAL",
          sessionId: "dur-sess-provider-session-001",
        });
      }
      if (url.endsWith("/dispatches")) {
        return jsonResponse({
          dispatches: [
            {
              dispatchKind: "PETRI_TRANSITION",
              id: "disp-provider-session-001",
              providerSessionRefs: [
                {
                  id: "prov-sess-disp-petri-001",
                  kind: "session_id",
                  provider: "codex",
                },
              ],
              status: "COMPLETED",
            },
          ],
          sessionId: "dur-sess-provider-session-001",
        });
      }
      if (url.endsWith("/artifacts")) {
        return jsonResponse({
          artifacts: [],
          sessionId: "dur-sess-provider-session-001",
        });
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="dur-sess-provider-session-001" />,
    );

    await waitFor(() => {
      expect(
        screen.getByRole("link", {
          name: "Inspect Provider Session: codex / session_id / prov-sess-disp-petri-001",
        }),
      ).toBeTruthy();
    });

    expect(
      screen
        .getByRole("link", {
          name: "Inspect Provider Session: codex / session_id / prov-sess-disp-petri-001",
        })
        .getAttribute("href"),
    ).toBe(
      "/provider-sessions/detail?id=prov-sess-disp-petri-001&kind=session_id&provider=codex",
    );
    expect(screen.queryByText("Provider Session")).toBeTruthy();
  });

  it("omits Provider Session subsection when durable dispatch refs are absent", async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      if (url === "/factory-sessions/dur-sess-no-provider-session-001") {
        return jsonResponse({
          orchestratorKind: "PETRI",
          resolvedSource: {
            kind: "FACTORY_ID",
            sourceRef: "factory/no-provider-session",
          },
          sessionId: "dur-sess-no-provider-session-001",
          status: "SUCCEEDED",
        });
      }
      if (url.endsWith("/results?mode=final")) {
        return jsonResponse({
          mode: "final",
          resultStatus: "FINAL",
          sessionId: "dur-sess-no-provider-session-001",
        });
      }
      if (url.endsWith("/results?mode=partial")) {
        return jsonResponse({
          mode: "partial",
          resultStatus: "PARTIAL",
          sessionId: "dur-sess-no-provider-session-001",
        });
      }
      if (url.endsWith("/dispatches")) {
        return jsonResponse({
          dispatches: [
            {
              dispatchKind: "PETRI_TRANSITION",
              id: "disp-no-provider-session-001",
              status: "COMPLETED",
            },
          ],
          sessionId: "dur-sess-no-provider-session-001",
        });
      }
      if (url.endsWith("/artifacts")) {
        return jsonResponse({
          artifacts: [],
          sessionId: "dur-sess-no-provider-session-001",
        });
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="dur-sess-no-provider-session-001" />,
    );

    await waitFor(() => {
      expect(
        screen.getByText(
          "disp-no-provider-session-001 · COMPLETED · PETRI_TRANSITION",
        ),
      ).toBeTruthy();
    });

    expect(screen.queryByText("Inspect Provider Session:")).toBeNull();
    expect(
      screen.queryByRole("link", { name: /Inspect Provider Session/ }),
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
