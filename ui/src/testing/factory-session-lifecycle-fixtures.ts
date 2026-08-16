import { FactoryOrchestratorKind } from "../api/generated/openapi";

export const pausedReplaySessionID = "dur-sess-js-paused-001";
export const failedPartialReplaySessionID = "dur-sess-js-failed-partial-001";

export function buildPausedDurableSession(sessionId = pausedReplaySessionID) {
  return {
    artifactRefs: [],
    dialect: "you-workflow-v1",
    lifecycle: {
      pausedAt: "2026-06-08T14:05:00Z",
      startedAt: "2026-06-08T14:00:00Z",
      updatedAt: "2026-06-08T14:05:00Z",
    },
    orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
    phase: "review",
    progress: {
      completedDispatches: 1,
      failedDispatches: 0,
      inFlightDispatches: 0,
      totalDispatches: 1,
    },
    resolvedSource: {
      kind: "WORKFLOW_NAME",
      sourceRef: "workflow/review",
      sourceHash: "sha256:workflow-review",
    },
    sessionId,
    status: "PAUSED",
    usage: { resources: [] },
  };
}

export function buildFailedPartialDurableSession(
  sessionId = failedPartialReplaySessionID,
) {
  return {
    artifactRefs: [
      {
        id: "artifact-release-verification-log",
        kind: "FINAL_RESULT",
        label: "Release verification log",
        visibility: "PRIVATE",
      },
    ],
    dialect: "you-workflow-v1",
    lifecycle: {
      finishedAt: "2026-06-08T14:05:00Z",
      startedAt: "2026-06-08T14:00:00Z",
      updatedAt: "2026-06-08T14:05:00Z",
    },
    orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
    phase: "verify",
    progress: {
      completedDispatches: 1,
      failedDispatches: 1,
      inFlightDispatches: 0,
      totalDispatches: 2,
    },
    resolvedSource: {
      kind: "WORKFLOW_NAME",
      sourceRef: "workflow/verify",
      sourceHash: "sha256:workflow-verify",
    },
    resultSummary: {
      artifactRefs: [
        {
          id: "artifact-release-verification-log",
          kind: "FINAL_RESULT",
          label: "Release verification log",
          visibility: "PRIVATE",
        },
      ],
      resultStatus: "FAILED_WITH_PARTIAL",
      summary: "Release verification failed after checkpoint recovery.",
    },
    sessionId,
    status: "FAILED",
    usage: { resources: [] },
  };
}

export function buildFailedPartialReplayDispatchList(
  sessionId = failedPartialReplaySessionID,
) {
  return {
    dispatches: [
      {
        dispatchKind: "JAVASCRIPT_AGENT",
        id: "dispatch-ok",
        orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
        sessionId,
        status: "COMPLETED",
      },
      {
        dispatchKind: "JAVASCRIPT_VERIFY",
        id: "dispatch-failed",
        javascript: {
          executionMode: "live",
          taskKind: "VERIFY",
          taskLabel: "Verify release manifest",
        },
        label: "Verify release manifest",
        orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
        providerSessionRefs: [
          {
            id: "provider-session-verify-1",
            kind: "session_id",
            provider: "codex",
          },
        ],
        sessionId,
        status: "FAILED",
      },
    ],
    sessionId,
  };
}
