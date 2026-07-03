import { vi } from "vitest";

import { FactoryOrchestratorKind } from "../../../../api/generated/openapi";
import { BASELINE_SESSION_ID } from "./factory-session-detail-panel.baseline-fixtures";
import { jsonResponse } from "./factory-session-detail-panel.test-helpers";

export const DISPATCH_DETAIL_SESSION_ID = BASELINE_SESSION_ID;
export const DISPATCH_SUCCESS_ID = "dispatch-success";
export const DISPATCH_FAILED_ID = "dispatch-failed";
export const DISPATCH_WARNING_ID = "dispatch-warning";
export const DISPATCH_MISSING_ID = "dispatch-missing";
export const DISPATCH_ERROR_ID = "dispatch-error";

export const PRIMARY_PROVIDER_SESSION =
  "Provider session: codex / session_id / provider-session-1";

function createIdleJavaScriptSessionRuntime(
  dispatches: unknown[],
  overrides?: { scriptStatus?: string; status?: string },
) {
  return {
    dispatches,
    javascript: {
      childDispatchCounts: {
        completed: dispatches.length,
        queued: 0,
        running: 0,
      },
      phases: [],
      scriptStatus: overrides?.scriptStatus ?? "IDLE",
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
    status: overrides?.status ?? "IDLE",
    usage: { resources: [] },
  };
}

function createSessionPayload(runtime: unknown) {
  return {
    factoryDir: "/workspace/root/beta",
    folderPath: "/workspace/root",
    id: DISPATCH_DETAIL_SESSION_ID,
    isDefault: false,
    project: "beta",
    runtime,
    target: { kind: "named", name: "beta" },
  };
}

export function mockSuccessfulDispatchDetailFetch() {
  vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
    const url = String(input);
    if (url.endsWith(`/factory-sessions/${DISPATCH_DETAIL_SESSION_ID}`)) {
      return jsonResponse(
        createSessionPayload(
          createIdleJavaScriptSessionRuntime([
            {
              dispatchKind: "JAVASCRIPT_AGENT",
              id: DISPATCH_SUCCESS_ID,
              label: "Draft response",
              orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
              sessionId: DISPATCH_DETAIL_SESSION_ID,
              status: "COMPLETED",
            },
          ]),
        ),
      );
    }
    if (
      url.endsWith(`/factory-sessions/${DISPATCH_DETAIL_SESSION_ID}/result`)
    ) {
      return new Response("not found", { status: 404 });
    }
    if (
      url.endsWith(
        `/factory-sessions/${DISPATCH_DETAIL_SESSION_ID}/partial-result`,
      )
    ) {
      return new Response("not found", { status: 404 });
    }
    if (
      url.endsWith(
        `/factory-sessions/${DISPATCH_DETAIL_SESSION_ID}/dispatches/${DISPATCH_SUCCESS_ID}`,
      )
    ) {
      return jsonResponse({
        artifactIds: ["artifact-final-1", "artifact-log-2"],
        attempt: 2,
        dispatchKind: "JAVASCRIPT_AGENT",
        id: DISPATCH_SUCCESS_ID,
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
        sessionId: DISPATCH_DETAIL_SESSION_ID,
        status: "COMPLETED",
        statusTransitions: ["QUEUED", "RUNNING", "COMPLETED"],
      });
    }
    return new Response("not found", { status: 404 });
  });
}

export function mockFailedDispatchDetailFetch() {
  vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
    const url = String(input);
    if (url.endsWith(`/factory-sessions/${DISPATCH_DETAIL_SESSION_ID}`)) {
      return jsonResponse(
        createSessionPayload(
          createIdleJavaScriptSessionRuntime(
            [
              {
                dispatchKind: "JAVASCRIPT_VERIFY",
                id: DISPATCH_FAILED_ID,
                orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
                sessionId: DISPATCH_DETAIL_SESSION_ID,
                status: "FAILED",
              },
            ],
            { scriptStatus: "FAILED", status: "FAILED" },
          ),
        ),
      );
    }
    if (
      url.endsWith(`/factory-sessions/${DISPATCH_DETAIL_SESSION_ID}/result`)
    ) {
      return new Response("not found", { status: 404 });
    }
    if (
      url.endsWith(
        `/factory-sessions/${DISPATCH_DETAIL_SESSION_ID}/partial-result`,
      )
    ) {
      return new Response("not found", { status: 404 });
    }
    if (
      url.endsWith(
        `/factory-sessions/${DISPATCH_DETAIL_SESSION_ID}/dispatches/${DISPATCH_FAILED_ID}`,
      )
    ) {
      return jsonResponse({
        artifactIds: ["artifact-failure-log"],
        dispatchKind: "JAVASCRIPT_VERIFY",
        failureDetail: {
          errorClass: " verification_error ",
          message: " Expected release manifest checksum. ",
          reason: " VERIFY_ASSERTION_FAILED ",
        },
        id: DISPATCH_FAILED_ID,
        javascript: {
          executionMode: " live ",
          taskKind: "VERIFY",
          taskLabel: " verify docs ",
        },
        orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
        relatedWorkIds: ["work-failed-1"],
        sessionId: DISPATCH_DETAIL_SESSION_ID,
        status: "FAILED",
        statusTransitions: ["QUEUED", "RUNNING", "FAILED"],
      });
    }
    return new Response("not found", { status: 404 });
  });
}

export function mockWarningDispatchDetailFetch() {
  vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
    const url = String(input);
    if (url.endsWith(`/factory-sessions/${DISPATCH_DETAIL_SESSION_ID}`)) {
      return jsonResponse(
        createSessionPayload(
          createIdleJavaScriptSessionRuntime([
            {
              dispatchKind: "JAVASCRIPT_VERIFY",
              id: DISPATCH_WARNING_ID,
              orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
              sessionId: DISPATCH_DETAIL_SESSION_ID,
              status: "COMPLETED",
              warnings: [
                {
                  code: "DISPATCH_WARNING",
                  message: "Verification completed with non-blocking warnings.",
                },
              ],
            },
          ]),
        ),
      );
    }
    if (
      url.endsWith(`/factory-sessions/${DISPATCH_DETAIL_SESSION_ID}/result`)
    ) {
      return new Response("not found", { status: 404 });
    }
    if (
      url.endsWith(
        `/factory-sessions/${DISPATCH_DETAIL_SESSION_ID}/partial-result`,
      )
    ) {
      return new Response("not found", { status: 404 });
    }
    if (
      url.endsWith(
        `/factory-sessions/${DISPATCH_DETAIL_SESSION_ID}/dispatches/${DISPATCH_WARNING_ID}`,
      )
    ) {
      return jsonResponse({
        artifactIds: ["artifact-warning-log"],
        dispatchKind: "JAVASCRIPT_VERIFY",
        id: DISPATCH_WARNING_ID,
        javascript: {
          executionMode: "live",
          taskKind: "VERIFY",
          taskLabel: "Verify docs",
        },
        orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
        sessionId: DISPATCH_DETAIL_SESSION_ID,
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
}

export function mockDispatchDetailBoundaryFetch() {
  vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
    const url = String(input);
    if (url.endsWith(`/factory-sessions/${DISPATCH_DETAIL_SESSION_ID}`)) {
      return jsonResponse(
        createSessionPayload({
          dispatches: [
            {
              dispatchKind: "JAVASCRIPT_AGENT",
              id: DISPATCH_SUCCESS_ID,
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
              sessionId: DISPATCH_DETAIL_SESSION_ID,
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
              id: DISPATCH_MISSING_ID,
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
              sessionId: DISPATCH_DETAIL_SESSION_ID,
              status: "FAILED",
            },
            {
              dispatchKind: "JAVASCRIPT_AGENT",
              id: DISPATCH_ERROR_ID,
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
              sessionId: DISPATCH_DETAIL_SESSION_ID,
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
        }),
      );
    }
    if (
      url.endsWith(`/factory-sessions/${DISPATCH_DETAIL_SESSION_ID}/result`)
    ) {
      return jsonResponse({
        resultArtifactRef: {
          id: "artifact-final",
          kind: "FINAL_RESULT",
          visibility: "CUSTOMER",
        },
        sessionId: DISPATCH_DETAIL_SESSION_ID,
        status: "IDLE",
      });
    }
    if (
      url.endsWith(
        `/factory-sessions/${DISPATCH_DETAIL_SESSION_ID}/partial-result`,
      )
    ) {
      return new Response("not found", { status: 404 });
    }
    if (url.endsWith(`/dispatches/${DISPATCH_MISSING_ID}`)) {
      return new Response("not found", { status: 404 });
    }
    if (url.endsWith(`/dispatches/${DISPATCH_ERROR_ID}`)) {
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
