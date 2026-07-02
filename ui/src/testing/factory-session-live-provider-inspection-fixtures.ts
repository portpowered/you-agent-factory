import { FactoryOrchestratorKind } from "../api/generated/openapi";

export const successfulLiveProviderSessionID = "dur-sess-js-success-002";
export const failedBridgedChildSessionID = "dur-sess-js-failed-partial-001";

export const successfulLiveProviderDispatchID = "disp-js-success-002";
export const failedBridgedChildDispatchID = "disp-js-fail-002";

export const successfulLiveProviderSessionRef = {
  id: "resp-docs-refresh-001",
  kind: "session_id" as const,
  provider: "codex",
};

export const failedBridgedChildProviderSessionRef = {
  id: "resp-verify-failed-001",
  kind: "session_id" as const,
  provider: "codex",
};

export function buildSuccessfulLiveProviderDispatchSummary() {
  return {
    attempt: 1,
    dispatchKind: "JAVASCRIPT_VERIFY",
    id: successfulLiveProviderDispatchID,
    javascript: {
      executionMode: "live",
      taskKind: "VERIFY",
      taskLabel: "verify-docs",
    },
    label: "verify-docs",
    orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
    outputArtifactIds: ["art-js-success-001"],
    providerSessionRefs: [successfulLiveProviderSessionRef],
    sessionId: successfulLiveProviderSessionID,
    status: "COMPLETED",
  };
}

export function buildSuccessfulLiveProviderDispatchList() {
  return {
    dispatches: [buildSuccessfulLiveProviderDispatchSummary()],
    sessionId: successfulLiveProviderSessionID,
  };
}

export function buildSuccessfulLiveProviderDispatchDetail() {
  return {
    artifactIds: ["art-js-success-001"],
    attempt: 1,
    dispatchKind: "JAVASCRIPT_VERIFY",
    id: successfulLiveProviderDispatchID,
    javascript: {
      executionMode: "live",
      taskKind: "VERIFY",
      taskLabel: "verify-docs",
    },
    label: "verify-docs",
    orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
    providerSessionRefs: [successfulLiveProviderSessionRef],
    relatedWorkIds: ["work-docs-refresh-001"],
    sessionId: successfulLiveProviderSessionID,
    status: "COMPLETED",
    statusTransitions: ["QUEUED", "RUNNING", "COMPLETED"],
  };
}

export function buildFailedBridgedChildDurableSession(
  sessionId = failedBridgedChildSessionID,
) {
  return {
    dialect: "you-workflow-v1",
    lifecycle: {
      finishedAt: "2026-06-08T14:10:00Z",
      startedAt: "2026-06-08T14:00:00Z",
    },
    orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
    progress: {
      completedDispatches: 1,
      failedDispatches: 1,
      inFlightDispatches: 0,
      totalDispatches: 2,
    },
    resolvedSource: {
      kind: "INLINE_WORKFLOW",
      sourceRef: "inline-workflow/req-js-failed-partial-001",
      sourceHash: "sha256:inline-workflow-001",
    },
    resultSummary: {
      resultStatus: "FAILED_WITH_PARTIAL",
      summary: "Partial synthesis available before verify failed.",
    },
    sessionId,
    status: "FAILED",
    usage: { resources: [] },
  };
}

export function buildFailedBridgedChildDispatchSummary() {
  return {
    attempt: 1,
    dispatchKind: "JAVASCRIPT_VERIFY",
    failureDetail: {
      errorClass: "verification_error",
      message: "Expected release manifest checksum.",
      reason: "VERIFY_ASSERTION_FAILED",
    },
    id: failedBridgedChildDispatchID,
    javascript: {
      executionMode: "live",
      taskKind: "VERIFY",
      taskLabel: "verify",
    },
    label: "verify",
    orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
    providerSessionRefs: [failedBridgedChildProviderSessionRef],
    sessionId: failedBridgedChildSessionID,
    status: "FAILED",
  };
}

export function buildFailedBridgedChildDispatchList() {
  return {
    dispatches: [
      {
        attempt: 1,
        dispatchKind: "JAVASCRIPT_SYNTHESIZE",
        id: "disp-js-fail-001",
        label: "synthesize",
        sessionId: failedBridgedChildSessionID,
        status: "COMPLETED",
      },
      buildFailedBridgedChildDispatchSummary(),
    ],
    sessionId: failedBridgedChildSessionID,
  };
}

export function buildFailedBridgedChildDispatchDetail() {
  return {
    attempt: 1,
    dispatchKind: "JAVASCRIPT_VERIFY",
    failureDetail: {
      errorClass: "verification_error",
      message: "Expected release manifest checksum.",
      reason: "VERIFY_ASSERTION_FAILED",
    },
    id: failedBridgedChildDispatchID,
    javascript: {
      executionMode: "live",
      taskKind: "VERIFY",
      taskLabel: "verify",
    },
    label: "verify",
    orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
    providerSessionRefs: [failedBridgedChildProviderSessionRef],
    relatedWorkIds: [],
    sessionId: failedBridgedChildSessionID,
    status: "FAILED",
    statusTransitions: ["QUEUED", "RUNNING", "FAILED"],
  };
}
