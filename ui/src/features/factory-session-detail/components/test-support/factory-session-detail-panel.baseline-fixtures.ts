import { vi } from "vitest";

import { FactoryOrchestratorKind } from "../../../../api/generated/openapi";
import { jsonResponse } from "./factory-session-detail-panel.test-helpers";

export const BASELINE_SESSION_ID = "session-beta";
export const BASELINE_DISPATCH_ID = "dispatch-1";

export function createJavaScriptSessionBetaDispatches() {
  return [
    {
      dispatchKind: "JAVASCRIPT_AGENT",
      id: BASELINE_DISPATCH_ID,
      javascript: {
        executionMode: " live ",
        taskKind: "AGENT",
        taskLabel: " Review child task ",
      },
      providerSessionRefs: [
        {
          id: "provider-session-1",
          kind: "session_id",
          provider: "codex",
        },
      ],
      status: "COMPLETED",
      warnings: [
        {
          code: "DISPATCH_WARNING",
          message: "child agent retry scheduled",
        },
      ],
    },
  ];
}

export function createJavaScriptSessionBetaPayload() {
  return {
    factoryDir: "/workspace/root/beta",
    folderPath: "/workspace/root",
    id: BASELINE_SESSION_ID,
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
  };
}

export function mockJavaScriptSessionBetaFetch() {
  vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
    const url = String(input);
    if (url.endsWith(`/factory-sessions/${BASELINE_SESSION_ID}`)) {
      return jsonResponse(createJavaScriptSessionBetaPayload());
    }
    if (url.endsWith(`/factory-sessions/${BASELINE_SESSION_ID}/dispatches`)) {
      return jsonResponse({
        dispatches: createJavaScriptSessionBetaDispatches(),
        sessionId: BASELINE_SESSION_ID,
      });
    }
    if (url.endsWith(`/factory-sessions/${BASELINE_SESSION_ID}/result`)) {
      return jsonResponse({
        resultArtifactRef: {
          id: "artifact-final",
          kind: "FINAL_RESULT",
          visibility: "CUSTOMER",
        },
        sessionId: BASELINE_SESSION_ID,
        status: "IDLE",
      });
    }
    if (
      url.endsWith(`/factory-sessions/${BASELINE_SESSION_ID}/partial-result`)
    ) {
      return jsonResponse({
        partialResultArtifactRef: {
          id: "artifact-partial",
          kind: "CHILD_RESULT",
          visibility: "CUSTOMER",
        },
        phase: "review",
        sessionId: BASELINE_SESSION_ID,
      });
    }
    return new Response("not found", { status: 404 });
  });
}

export function mockPendingSessionFetch() {
  let resolveFetch: (value: Response) => void = () => undefined;
  const pendingResponse = new Promise<Response>((resolve) => {
    resolveFetch = resolve;
  });
  vi.mocked(globalThis.fetch).mockReturnValue(pendingResponse);

  return {
    resolveWithPetriSession: () =>
      resolveFetch(
        jsonResponse({
          factoryDir: "/workspace/root/beta",
          folderPath: "/workspace/root",
          id: BASELINE_SESSION_ID,
          isDefault: false,
          project: "beta",
          runtime: {
            lifecycle: {
              startedAt: "2026-06-08T14:00:00Z",
              updatedAt: "2026-06-08T14:05:00Z",
            },
            orchestratorKind: FactoryOrchestratorKind.PETRI,
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
        }),
      ),
  };
}
