import { vi } from "vitest";

import { FactoryOrchestratorKind } from "../../../../api/generated/openapi";
import { BASELINE_SESSION_ID } from "./factory-session-detail-panel.baseline-fixtures";
import { jsonResponse } from "./factory-session-detail-panel.test-helpers";

export const DISPATCH_NOT_FOUND_ID = "dispatch-404";
export const DISPATCH_REPLACEMENT_ALPHA_ID = "dispatch-alpha";
export const DISPATCH_REPLACEMENT_BETA_ID = "dispatch-beta";
export const DISPATCH_API_ERROR_ID = "dispatch-500";

export function createBaselineDispatchDetailPayload() {
  return {
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
    sessionId: BASELINE_SESSION_ID,
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
  };
}

function createFailedSessionRuntime() {
  return {
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
  };
}

function failedDispatchSummary(dispatchId: string) {
  return {
    dispatchKind: "JAVASCRIPT_AGENT",
    id: dispatchId,
    status: "FAILED",
  };
}

function createSessionBetaPayload(
  runtime: Record<string, unknown>,
): Record<string, unknown> {
  return {
    factoryDir: "/workspace/root/beta",
    folderPath: "/workspace/root",
    id: BASELINE_SESSION_ID,
    isDefault: false,
    project: "beta",
    runtime,
    target: { kind: "named", name: "beta" },
  };
}

function mockSessionBetaResultNotFound() {
  return {
    sessionResult: new Response("not found", { status: 404 }),
    partialResult: new Response("not found", { status: 404 }),
  };
}

export function mockDispatchNotFoundFetch() {
  const { sessionResult, partialResult } = mockSessionBetaResultNotFound();

  vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
    const url = String(input);
    if (url.endsWith(`/factory-sessions/${BASELINE_SESSION_ID}`)) {
      return jsonResponse(
        createSessionBetaPayload(createFailedSessionRuntime()),
      );
    }
    if (url.endsWith(`/factory-sessions/${BASELINE_SESSION_ID}/result`)) {
      return sessionResult;
    }
    if (url.endsWith(`/factory-sessions/${BASELINE_SESSION_ID}/dispatches`)) {
      return jsonResponse({
        dispatches: [failedDispatchSummary(DISPATCH_NOT_FOUND_ID)],
        sessionId: BASELINE_SESSION_ID,
      });
    }
    if (
      url.endsWith(`/factory-sessions/${BASELINE_SESSION_ID}/partial-result`)
    ) {
      return partialResult;
    }
    if (
      url.endsWith(
        `/factory-sessions/${BASELINE_SESSION_ID}/dispatches/${DISPATCH_NOT_FOUND_ID}`,
      )
    ) {
      return new Response("not found", { status: 404 });
    }
    return new Response("not found", { status: 404 });
  });
}

export function mockDispatchReplacementFetch() {
  const { sessionResult, partialResult } = mockSessionBetaResultNotFound();
  const dispatches = [
    {
      dispatchKind: "JAVASCRIPT_AGENT",
      id: DISPATCH_REPLACEMENT_ALPHA_ID,
      label: "Alpha review task",
      status: "COMPLETED",
    },
    {
      dispatchKind: "JAVASCRIPT_VERIFY",
      id: DISPATCH_REPLACEMENT_BETA_ID,
      label: "Beta verify task",
      status: "FAILED",
    },
  ];

  vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
    const url = String(input);
    if (url.endsWith(`/factory-sessions/${BASELINE_SESSION_ID}`)) {
      return jsonResponse(
        createSessionBetaPayload({
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
        }),
      );
    }
    if (url.endsWith(`/factory-sessions/${BASELINE_SESSION_ID}/result`)) {
      return sessionResult;
    }
    if (url.endsWith(`/factory-sessions/${BASELINE_SESSION_ID}/dispatches`)) {
      return jsonResponse({ dispatches, sessionId: BASELINE_SESSION_ID });
    }
    if (
      url.endsWith(`/factory-sessions/${BASELINE_SESSION_ID}/partial-result`)
    ) {
      return partialResult;
    }
    if (
      url.endsWith(
        `/factory-sessions/${BASELINE_SESSION_ID}/dispatches/${DISPATCH_REPLACEMENT_ALPHA_ID}`,
      )
    ) {
      return jsonResponse({
        artifactIds: ["artifact-alpha"],
        dispatchKind: "JAVASCRIPT_AGENT",
        id: DISPATCH_REPLACEMENT_ALPHA_ID,
        javascript: {
          executionMode: "live",
          taskKind: "AGENT",
          taskLabel: "Alpha review task",
        },
        label: "Alpha review task",
        orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
        sessionId: BASELINE_SESSION_ID,
        status: "COMPLETED",
        statusTransitions: ["QUEUED", "RUNNING", "COMPLETED"],
      });
    }
    if (
      url.endsWith(
        `/factory-sessions/${BASELINE_SESSION_ID}/dispatches/${DISPATCH_REPLACEMENT_BETA_ID}`,
      )
    ) {
      return jsonResponse({
        artifactIds: ["artifact-beta"],
        dispatchKind: "JAVASCRIPT_VERIFY",
        failureDetail: {
          message: "Checksum mismatch on beta verify.",
          reason: "VERIFY_ASSERTION_FAILED",
        },
        id: DISPATCH_REPLACEMENT_BETA_ID,
        javascript: {
          executionMode: "live",
          taskKind: "VERIFY",
          taskLabel: "Beta verify task",
        },
        label: "Beta verify task",
        orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
        sessionId: BASELINE_SESSION_ID,
        status: "FAILED",
        statusTransitions: ["QUEUED", "RUNNING", "FAILED"],
      });
    }
    return new Response("not found", { status: 404 });
  });
}

export function mockDispatchApiErrorFetch() {
  const { sessionResult, partialResult } = mockSessionBetaResultNotFound();

  vi.mocked(globalThis.fetch).mockImplementation(async (input) => {
    const url = String(input);
    if (url.endsWith(`/factory-sessions/${BASELINE_SESSION_ID}`)) {
      return jsonResponse(
        createSessionBetaPayload(createFailedSessionRuntime()),
      );
    }
    if (url.endsWith(`/factory-sessions/${BASELINE_SESSION_ID}/result`)) {
      return sessionResult;
    }
    if (url.endsWith(`/factory-sessions/${BASELINE_SESSION_ID}/dispatches`)) {
      return jsonResponse({
        dispatches: [failedDispatchSummary(DISPATCH_API_ERROR_ID)],
        sessionId: BASELINE_SESSION_ID,
      });
    }
    if (
      url.endsWith(`/factory-sessions/${BASELINE_SESSION_ID}/partial-result`)
    ) {
      return partialResult;
    }
    if (
      url.endsWith(
        `/factory-sessions/${BASELINE_SESSION_ID}/dispatches/${DISPATCH_API_ERROR_ID}`,
      )
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
}
