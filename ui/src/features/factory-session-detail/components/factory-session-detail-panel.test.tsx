// biome-ignore-all lint/nursery/noExcessiveLinesPerFile: orchestrator-aware session detail states share one fetch harness and assertion seam.
// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: orchestrator-aware session detail states share one fetch harness and assertion seam.
// biome-ignore-all lint/nursery/noExcessiveLinesPerFile: non-success lifecycle coverage shares one panel fetch harness with durable detail cases.
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { FactoryOrchestratorKind } from "../../../api/generated/openapi";
import { FactorySessionDetailPanel } from "./factory-session-detail-panel";
import {
  BASELINE_DISPATCH_ID,
  BASELINE_SESSION_ID,
  mockJavaScriptSessionBetaFetchWithDeferredDispatch,
} from "./test-support/factory-session-detail-panel.baseline-fixtures";
import {
  jsonResponse,
  renderWithQueryClient,
} from "./test-support/factory-session-detail-panel.test-helpers";

describe("FactorySessionDetailPanel", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows loading and success states for a JavaScript factory session dispatch drilldown", async () => {
    const dispatchDetailResponse =
      mockJavaScriptSessionBetaFetchWithDeferredDispatch();

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID={BASELINE_SESSION_ID} />,
    );

    await waitFor(() => {
      expect(screen.getByText("Runtime")).toBeTruthy();
    });

    const user = userEvent.setup();
    const drilldownTrigger = screen.getByRole("button", {
      name: `Expand dispatch detail for ${BASELINE_DISPATCH_ID}`,
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

  it("shows a not-found state when the factory session is no longer available", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      new Response(
        JSON.stringify({
          code: "NOT_FOUND",
          message: "Factory session missing.",
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 404,
          statusText: "Not Found",
        },
      ),
    );

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="session-missing" />,
    );

    await waitFor(() => {
      expect(
        screen.getByText("This factory session is no longer available."),
      ).toBeTruthy();
    });

    expect(screen.queryByText("Loading factory session runtime…")).toBeNull();
    expect(screen.queryByText("Factory session missing.")).toBeNull();
  });

  it("shows canonical paused Factory Session lifecycle status from the API read model", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      jsonResponse({
        factoryDir: "/workspace/root/beta",
        folderPath: "/workspace/root",
        id: "session-beta",
        isDefault: false,
        project: "beta",
        runtime: {
          lifecycle: {
            startedAt: "2026-06-08T14:00:00Z",
            updatedAt: "2026-06-08T14:05:00Z",
          },
          lifecycleControlStatus: "PAUSED",
          orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
          progress: {
            categories: {},
            factoryState: "PAUSED",
            inFlightCount: 0,
            totalTokens: 0,
          },
          status: "ACTIVE",
          usage: { resources: [] },
        },
        target: { kind: "named", name: "beta" },
      }),
    );

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="session-beta" />,
    );

    await waitFor(() => {
      expect(screen.getByText("PAUSED")).toBeTruthy();
    });

    expect(screen.getByText("Factory Session lifecycle")).toBeTruthy();
  });

  it("shows running Factory Session lifecycle status after a canonical resume read", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      jsonResponse({
        factoryDir: "/workspace/root/beta",
        folderPath: "/workspace/root",
        id: "session-beta",
        isDefault: false,
        project: "beta",
        runtime: {
          lifecycle: {
            startedAt: "2026-06-08T14:00:00Z",
            updatedAt: "2026-06-08T14:10:00Z",
          },
          lifecycleControlStatus: "RUNNING",
          orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
          progress: {
            categories: {},
            factoryState: "RUNNING",
            inFlightCount: 1,
            totalTokens: 0,
          },
          status: "ACTIVE",
          usage: { resources: [] },
        },
        target: { kind: "named", name: "beta" },
      }),
    );

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="session-beta" />,
    );

    await waitFor(() => {
      expect(screen.getByText("RUNNING")).toBeTruthy();
    });

    expect(screen.getByText("Factory Session lifecycle")).toBeTruthy();
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
        screen.getByText(
          "Dispatch detail for dispatch-404 is no longer available.",
        ),
      ).toBeTruthy();
    });
    expect(screen.getByText("Dispatches")).toBeTruthy();
    expect(screen.getByText("Runtime")).toBeTruthy();
  });

  it("replaces dispatch detail when selecting a different dispatch summary row", async () => {
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
                id: "dispatch-alpha",
                label: "Alpha review task",
                orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
                sessionId: "session-beta",
                status: "COMPLETED",
              },
              {
                dispatchKind: "JAVASCRIPT_VERIFY",
                id: "dispatch-beta",
                label: "Beta verify task",
                orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
                sessionId: "session-beta",
                status: "FAILED",
              },
            ],
            javascript: {
              childDispatchCounts: {
                completed: 1,
                queued: 0,
                running: 0,
              },
              phase: "verify",
              phases: ["plan", "verify"],
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
        url.endsWith("/factory-sessions/session-beta/dispatches/dispatch-alpha")
      ) {
        return jsonResponse({
          artifactIds: ["artifact-alpha"],
          dispatchKind: "JAVASCRIPT_AGENT",
          id: "dispatch-alpha",
          javascript: {
            executionMode: "live",
            taskKind: "AGENT",
            taskLabel: "Alpha review task",
          },
          label: "Alpha review task",
          orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
          sessionId: "session-beta",
          status: "COMPLETED",
          statusTransitions: ["QUEUED", "RUNNING", "COMPLETED"],
        });
      }
      if (
        url.endsWith("/factory-sessions/session-beta/dispatches/dispatch-beta")
      ) {
        return jsonResponse({
          artifactIds: ["artifact-beta"],
          dispatchKind: "JAVASCRIPT_VERIFY",
          failureDetail: {
            errorClass: "verify_error",
            message: "Checksum mismatch on beta verify.",
            reason: "VERIFY_ASSERTION_FAILED",
          },
          id: "dispatch-beta",
          javascript: {
            executionMode: "live",
            taskKind: "VERIFY",
            taskLabel: "Beta verify task",
          },
          label: "Beta verify task",
          orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
          sessionId: "session-beta",
          status: "FAILED",
          statusTransitions: ["QUEUED", "RUNNING", "FAILED"],
        });
      }
      return new Response("not found", { status: 404 });
    });

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="session-beta" />,
    );

    await waitFor(() => {
      expect(screen.getByText("Runtime")).toBeTruthy();
    });

    expect(screen.getByText("JavaScript workflow")).toBeTruthy();
    expect(screen.getByText("verify")).toBeTruthy();

    const user = userEvent.setup();
    const alphaTrigger = screen.getByRole("button", {
      name: "Expand dispatch detail for dispatch-alpha",
    });
    const betaTrigger = screen.getByRole("button", {
      name: "Expand dispatch detail for dispatch-beta",
    });

    expect(alphaTrigger.getAttribute("aria-expanded")).toBe("false");
    expect(betaTrigger.getAttribute("aria-expanded")).toBe("false");

    await user.click(alphaTrigger);

    await waitFor(() => {
      expect(
        screen.getByRole("link", { name: "artifact-alpha" }),
      ).toBeTruthy();
    });
    expect(screen.getAllByText("COMPLETED").length).toBeGreaterThan(0);
    expect(alphaTrigger.getAttribute("aria-expanded")).toBe("true");
    expect(betaTrigger.getAttribute("aria-expanded")).toBe("false");
    expect(
      screen.queryByRole("link", { name: "artifact-beta" }),
    ).toBeNull();
    expect(screen.queryByText("Checksum mismatch on beta verify.")).toBeNull();

    await user.click(betaTrigger);

    await waitFor(() => {
      expect(
        screen.getByRole("link", { name: "artifact-beta" }),
      ).toBeTruthy();
    });
    expect(screen.getByText("Checksum mismatch on beta verify.")).toBeTruthy();
    expect(alphaTrigger.getAttribute("aria-expanded")).toBe("false");
    expect(betaTrigger.getAttribute("aria-expanded")).toBe("true");
    expect(
      screen.queryByRole("link", { name: "artifact-alpha" }),
    ).toBeNull();
    expect(screen.getByText("Runtime")).toBeTruthy();
    expect(screen.getByText("JavaScript workflow")).toBeTruthy();
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

    expect(screen.queryByText("Loading factory session runtime…")).toBeNull();
    expect(
      screen.queryByText("This factory session is no longer available."),
    ).toBeNull();
    expect(
      screen.getByRole("button", { name: "Retry loading dispatch detail" }),
    ).toBeTruthy();
    expect(screen.getByText("Dispatches")).toBeTruthy();
    expect(screen.getByText("Runtime")).toBeTruthy();
  });
});
