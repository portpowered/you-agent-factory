// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: orchestrator-aware session detail states share one fetch harness and assertion seam.
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { FactoryOrchestratorKind } from "../../../api/generated/openapi";
import { FactorySessionDetailPanel } from "./factory-session-detail-panel";
import {
  createDeferred,
  jsonResponse,
  renderWithQueryClient,
} from "./factory-session-detail-panel.test-helpers";

describe("FactorySessionDetailPanel", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows loading and success states for a JavaScript factory session", async () => {
    const dispatchDetailResponse = createDeferred<Response>();
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
                javascript: {
                  executionMode: " live ",
                  taskKind: "AGENT",
                  taskLabel: " Review child task ",
                },
                orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
                providerSessionRefs: [
                  {
                    id: "provider-session-1",
                    kind: "session_id",
                    provider: "codex",
                  },
                ],
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
      if (
        url.endsWith("/factory-sessions/session-beta/dispatches/dispatch-1")
      ) {
        return dispatchDetailResponse.promise;
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
    expect(
      screen.getByText("cp-1 (plan) — saved plan checkpoint"),
    ).toBeTruthy();
    expect(screen.getByText("child agent retry scheduled")).toBeTruthy();
    expect(screen.getByText("artifact-final · FINAL_RESULT")).toBeTruthy();
    expect(screen.getByText("artifact-partial · CHILD_RESULT")).toBeTruthy();
    expect(screen.getByText("Execution mode: live")).toBeTruthy();
    expect(
      screen.getByText(
        "Provider session: codex / session_id / provider-session-1",
      ),
    ).toBeTruthy();
    expect(screen.queryByText("rawCheckpointBody")).toBeNull();

    const user = userEvent.setup();
    const drilldownTrigger = screen.getByRole("button", {
      name: "Expand dispatch detail for dispatch-1",
    });

    expect(drilldownTrigger.getAttribute("aria-expanded")).toBe("false");

    await user.click(drilldownTrigger);

    expect(screen.getByText("Loading dispatch detail…")).toBeTruthy();
    expect(drilldownTrigger.getAttribute("aria-expanded")).toBe("true");

    dispatchDetailResponse.resolve(
      jsonResponse({
        artifactIds: ["artifact-dispatch-1"],
        attempt: 2,
        dispatchKind: "JAVASCRIPT_AGENT",
        id: "dispatch-1",
        javascript: {
          executionMode: " live ",
          taskKind: "AGENT",
          taskLabel: " Review child task ",
        },
        label: " Review child task ",
        model: " gpt-5.5 ",
        orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
        phase: " review ",
        promptDigest: " sha256:prompt-1 ",
        provider: " openai ",
        providerSessionRefs: [
          {
            id: "provider-session-1",
            kind: "session_id",
            provider: "codex",
          },
        ],
        relatedWorkIds: ["work-123"],
        runnerId: " runner-a ",
        schemaDigest: " sha256:schema-1 ",
        sessionId: "session-beta",
        status: "COMPLETED",
        statusTransitions: ["QUEUED", "RUNNING", "COMPLETED"],
        usage: {
          costUsd: 0.21,
          durationMillis: 4400,
          inputTokens: 120,
          outputTokens: 80,
          retryCount: 1,
          totalTokens: 200,
        },
        warnings: [
          {
            code: "DISPATCH_WARNING",
            message: "Token budget was nearly exhausted.",
          },
        ],
      }),
    );

    await waitFor(() => {
      expect(screen.getByText("JavaScript task")).toBeTruthy();
    });

    expect(screen.getAllByText("COMPLETED").length).toBeGreaterThan(0);
    expect(screen.getAllByText("JAVASCRIPT_AGENT").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Review child task").length).toBeGreaterThan(1);
    expect(screen.getByText("live")).toBeTruthy();
    expect(screen.getByText("session_id · provider-session-1")).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "artifact-dispatch-1" }),
    ).toBeTruthy();
    expect(screen.getByText("QUEUED")).toBeTruthy();
    expect(screen.getByText("RUNNING")).toBeTruthy();
    expect(screen.getByText("$0.21")).toBeTruthy();
    expect(screen.getByText("4,400 ms")).toBeTruthy();
    expect(screen.getByText("Token budget was nearly exhausted.")).toBeTruthy();
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
      expect(screen.getByRole("alert").textContent).toContain("boom");
    });
    expect(screen.getByText("session-beta")).toBeTruthy();
  });

  it("shows unavailable dispatch detail when the durable dispatch read returns not found", async () => {
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
                id: "dispatch-404",
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
        url.endsWith("/factory-sessions/session-beta/dispatches/dispatch-404")
      ) {
        return new Response("not found", { status: 404 });
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="session-beta" />,
    );

    await waitFor(() => {
      expect(screen.getByText("Dispatches")).toBeTruthy();
    });

    const user = userEvent.setup();
    const drilldownTrigger = screen.getByRole("button", {
      name: "Expand dispatch detail for dispatch-404",
    });
    drilldownTrigger.focus();
    await user.keyboard("{Enter}");

    await waitFor(() => {
      expect(
        screen.getByText("This dispatch detail is no longer available."),
      ).toBeTruthy();
    });
  });

  it("shows a dispatch-detail error state when the durable dispatch read fails", async () => {
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
                id: "dispatch-500",
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
        url.endsWith("/factory-sessions/session-beta/dispatches/dispatch-500")
      ) {
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

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="session-beta" />,
    );

    await waitFor(() => {
      expect(screen.getByText("Dispatches")).toBeTruthy();
    });

    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", {
        name: "Expand dispatch detail for dispatch-500",
      }),
    );

    await waitFor(() => {
      expect(screen.getByText("dispatch boom")).toBeTruthy();
    });
  });

});
