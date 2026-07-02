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
