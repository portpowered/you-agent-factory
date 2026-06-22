// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: orchestrator-aware session detail states share one fetch harness and assertion seam.
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import userEvent from "@testing-library/user-event";
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
    const fetchCalls: string[] = [];
    let resolveArtifactResponse: (() => void) | null = null;
    vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
      const url = String(input);
      fetchCalls.push(url);
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
      if (url.endsWith("/factory-sessions/session-beta/artifacts/artifact-1")) {
        return await new Promise<Response>((resolve) => {
          resolveArtifactResponse = () => {
            resolve(
              jsonResponse({
                auditMode: "OFF",
                content: [{ text: "review output body", type: "output_text" }],
                createdAt: "2026-06-08T14:03:00Z",
                dispatchId: "dispatch-1",
                id: "artifact-1",
                kind: "CHILD_RESULT",
                label: "review output",
                sessionId: "session-beta",
                summary: "Captured during review",
                visibility: "CUSTOMER",
              }),
            );
          };
        });
      }
      return new Response("not found", { status: 404 });
    });

    const user = userEvent.setup();
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
    expect(
      fetchCalls.some((url) =>
        url.endsWith("/factory-sessions/session-beta/artifacts/artifact-1"),
      ),
    ).toBe(false);

    await user.click(
      screen.getByRole("button", { name: "View artifact artifact-1" }),
    );

    expect(screen.getByText("Loading artifact detail…")).toBeTruthy();
    resolveArtifactResponse?.();

    await waitFor(() => {
      expect(screen.getByText("Captured during review")).toBeTruthy();
    });

    expect(screen.getByText("Artifact detail")).toBeTruthy();
    expect(screen.getByText("output_text")).toBeTruthy();
    expect(screen.queryByText("rawCheckpointBody")).toBeNull();
  });

  it("opens artifact detail with keyboard interaction and shows download-only artifacts without a broken preview", async () => {
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
                id: "artifact-download",
                kind: "FINAL_RESULT",
                label: "bundle export",
                visibility: "CUSTOMER",
              },
            ],
            dispatches: [],
            javascript: {
              checkpoints: [],
              childDispatchCounts: {
                completed: 1,
                queued: 0,
                running: 0,
              },
              phases: ["finalize"],
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
        url.endsWith("/factory-sessions/session-beta/artifacts/artifact-download")
      ) {
        return jsonResponse({
          auditMode: "OFF",
          contentRef: {
            href: "/factory-sessions/session-beta/artifacts/artifact-download",
            label: "Download artifact",
          },
          createdAt: "2026-06-08T14:03:00Z",
          id: "artifact-download",
          kind: "FINAL_RESULT",
          label: "bundle export",
          sessionId: "session-beta",
          visibility: "CUSTOMER",
        });
      }
      return new Response("not found", { status: 404 });
    });

    const user = userEvent.setup();
    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="session-beta" />,
    );

    const toggle = await screen.findByRole("button", {
      name: "View artifact artifact-download",
    });
    toggle.focus();

    await user.keyboard("{Enter}");

    await waitFor(() => {
      expect(screen.getByRole("link", { name: "View artifact" })).toBeTruthy();
    });

    expect(
      screen.getByText("Inline preview is unavailable for this durable artifact."),
    ).toBeTruthy();
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

  it("shows an artifact detail error when the artifact read fails", async () => {
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
            dispatches: [],
            javascript: {
              checkpoints: [],
              childDispatchCounts: {
                completed: 1,
                queued: 0,
                running: 0,
              },
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
      if (url.endsWith("/factory-sessions/session-beta/partial-result")) {
        return new Response("not found", { status: 404 });
      }
      if (url.endsWith("/factory-sessions/session-beta/artifacts/artifact-1")) {
        return new Response(
          JSON.stringify({ code: "INTERNAL_ERROR", message: "artifact boom" }),
          {
            headers: { "Content-Type": "application/json" },
            status: 500,
          },
        );
      }
      return new Response("not found", { status: 404 });
    });

    const user = userEvent.setup();
    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="session-beta" />,
    );

    await user.click(
      await screen.findByRole("button", { name: "View artifact artifact-1" }),
    );

    await waitFor(() => {
      expect(screen.getByText("artifact boom")).toBeTruthy();
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
