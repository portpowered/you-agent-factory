import { FactoryOrchestratorKind } from "../api/generated/openapi";

export const successfulReplaySessionID = "dur-sess-js-success-002";
export const warningReplaySessionID = "dur-sess-js-warning-003";
export const awaitingReplaySessionID = "dur-sess-js-awaiting-001";
export const unavailableReplaySessionID = "dur-sess-js-unavailable-003";
export const emptyReplaySessionID = "dur-sess-js-empty-004";
export const errorReplaySessionID = "dur-sess-js-error-005";

export function buildSuccessfulDurableSession(
  sessionId = successfulReplaySessionID,
) {
  return {
    artifactRefs: [
      {
        id: "art-js-success-001",
        kind: "FINAL_RESULT",
        label: "Docs refresh output",
        visibility: "PUBLIC",
      },
    ],
    dialect: "you-workflow-v1",
    lifecycle: {
      finishedAt: "2026-06-08T13:10:00Z",
      startedAt: "2026-06-08T13:00:02Z",
    },
    orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
    progress: {
      completedDispatches: 2,
      failedDispatches: 0,
      inFlightDispatches: 0,
      totalDispatches: 2,
    },
    resolvedSource: {
      kind: "WORKFLOW_FILE",
      sourceRef: "workflow/.claude/workflows/docs-refresh.yaml",
      sourceHash: "sha256:js-workflow-docs-refresh",
    },
    resultSummary: {
      resultStatus: "FINAL",
      summary: "Documentation refresh complete.",
    },
    sessionId,
    status: "SUCCEEDED",
    usage: { resources: [] },
  };
}

export function buildWarningDurableSession(sessionId = warningReplaySessionID) {
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
      finishedAt: "2026-06-25T11:10:00Z",
      startedAt: "2026-06-25T11:00:00Z",
    },
    orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
    progress: {
      completedDispatches: 0,
      failedDispatches: 1,
      inFlightDispatches: 0,
      totalDispatches: 1,
    },
    resolvedSource: {
      kind: "WORKFLOW_FILE",
      sourceRef: "workflow/.claude/workflows/release-verify.yaml",
      sourceHash: "sha256:release-verify",
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

export function buildAwaitingDurableSession(
  sessionId = awaitingReplaySessionID,
) {
  return {
    artifactRefs: [],
    dialect: "you-workflow-v1",
    lifecycle: {
      awaitingApprovalAt: "2026-06-08T15:00:01Z",
      queuedAt: "2026-06-08T15:00:00Z",
    },
    orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
    progress: {
      completedDispatches: 0,
      failedDispatches: 0,
      inFlightDispatches: 0,
      totalDispatches: 0,
    },
    resolvedSource: {
      kind: "WORKFLOW_FILE",
      sourceRef: "workflow/.claude/workflows/policy-gated-release.yaml",
      sourceHash: "sha256:js-workflow-policy-gated-release",
    },
    resultSummary: {
      resultStatus: "NOT_READY",
      summary: "Awaiting orchestrator policy approval before execution.",
    },
    sessionId,
    status: "AWAITING_APPROVAL",
    usage: { resources: [] },
  };
}

export function buildSuccessfulReplayEventStream(
  sessionId = successfulReplaySessionID,
) {
  return [
    `data: ${JSON.stringify({
      id: "evt-1",
      type: "SESSION_STARTED",
      context: {
        sequence: 1,
        tick: 1,
        eventTime: "2026-06-25T10:00:00Z",
        sessionId,
        sessionSequence: 1,
        phaseName: "plan",
      },
      payload: { startedAt: "2026-06-25T10:00:00Z" },
    })}`,
    "",
    `data: ${JSON.stringify({
      id: "evt-2",
      type: "ORCHESTRATOR_PHASE_CHANGED",
      context: {
        sequence: 2,
        tick: 2,
        eventTime: "2026-06-25T10:00:01Z",
        sessionId,
        sessionSequence: 2,
        phaseName: "review",
      },
      payload: {
        phase: "review",
        progressSummary: "Review work scheduled.",
      },
    })}`,
    "",
    `data: ${JSON.stringify({
      id: "evt-3",
      type: "DISPATCH_QUEUED",
      context: {
        sequence: 3,
        tick: 3,
        eventTime: "2026-06-25T10:00:02Z",
        sessionId,
        sessionSequence: 3,
        phaseName: "review",
        dispatchId: "dispatch-1",
        workIds: ["work-1", "work-2"],
      },
      payload: {
        dispatchKind: "JAVASCRIPT_AGENT",
        label: "Draft release notes",
        queuePosition: 1,
      },
    })}`,
    "",
    `data: ${JSON.stringify({
      id: "evt-4",
      type: "DISPATCH_RECONCILED",
      context: {
        sequence: 4,
        tick: 4,
        eventTime: "2026-06-25T10:00:03Z",
        sessionId,
        sessionSequence: 4,
        phaseName: "review",
        dispatchId: "dispatch-1",
      },
      payload: {
        reconciledStatus: "COMPLETED",
        resultArtifactRef: {
          id: "artifact-release-notes",
          kind: "FINAL_RESULT",
          label: "Release notes",
        },
        artifactIds: ["artifact-release-notes"],
      },
    })}`,
    "",
    `data: ${JSON.stringify({
      id: "evt-5",
      type: "SESSION_COMPLETED",
      context: {
        sequence: 5,
        tick: 5,
        eventTime: "2026-06-25T10:00:05Z",
        sessionId,
        sessionSequence: 5,
        phaseName: "review",
      },
      payload: {
        finalStatus: "SUCCEEDED",
        completedAt: "2026-06-25T10:00:05Z",
        artifactIds: ["artifact-release-notes"],
      },
    })}`,
    "",
  ].join("\n");
}

export function buildWarningReplayEventStream(
  sessionId = warningReplaySessionID,
) {
  return [
    `data: ${JSON.stringify({
      id: "evt-w1",
      type: "JAVASCRIPT_CHECKPOINT_REF",
      context: {
        sequence: 1,
        tick: 1,
        eventTime: "2026-06-25T11:00:01Z",
        sessionId,
        sessionSequence: 1,
        phaseName: "verify",
        checkpointId: "checkpoint-9",
      },
      payload: {
        label: "Checkpoint before publish",
        warnings: [
          {
            code: "CHECKPOINT_STALE",
            message: "Checkpoint is older than the latest source hash.",
          },
        ],
      },
    })}`,
    "",
    `data: ${JSON.stringify({
      id: "evt-w2",
      type: "DISPATCH_INTERRUPTED",
      context: {
        sequence: 2,
        tick: 2,
        eventTime: "2026-06-25T11:00:02Z",
        sessionId,
        sessionSequence: 2,
        phaseName: "verify",
        dispatchId: "dispatch-verify",
      },
      payload: {
        reason: "Provider session timed out",
        observedStatus: "RUNNING",
        interruptedAt: "2026-06-25T11:00:02Z",
        retryPlanned: true,
      },
    })}`,
    "",
    `data: ${JSON.stringify({
      id: "evt-w3",
      type: "SESSION_COMPLETED",
      context: {
        sequence: 3,
        tick: 3,
        eventTime: "2026-06-25T11:00:05Z",
        sessionId,
        sessionSequence: 3,
        phaseName: "verify",
      },
      payload: {
        finalStatus: "FAILED",
        completedAt: "2026-06-25T11:00:05Z",
        failureDetail: {
          message: "Release verification failed.",
        },
      },
    })}`,
    "",
  ].join("\n");
}

export function buildEmptyReplayEventStream() {
  return [": keepalive", "", "event: ping", "data: ignored", ""].join("\n");
}

export function buildAwaitingReplayEventStream(
  sessionId = awaitingReplaySessionID,
) {
  return [
    `data: ${JSON.stringify({
      id: "session-started/dur-sess-js-awaiting-001",
      type: "SESSION_STARTED",
      context: {
        sequence: 1,
        tick: 1,
        eventTime: "2026-06-08T15:00:00Z",
        sessionId,
        sessionSequence: 0,
        phaseName: "approval",
      },
      payload: {
        sourceRef: "workflow/.claude/workflows/policy-gated-release.yaml",
        sourceHash: "sha256:js-workflow-policy-gated-release",
        policyHash: "eff-policy-gated-release",
        startedAt: "2026-06-08T15:00:00Z",
      },
    })}`,
    "",
    `data: ${JSON.stringify({
      id: "session-result-updated/dur-sess-js-awaiting-001",
      type: "SESSION_RESULT_UPDATED",
      context: {
        sequence: 2,
        tick: 2,
        eventTime: "2026-06-08T15:00:01Z",
        sessionId,
        sessionSequence: 1,
        phaseName: "approval",
      },
      payload: {
        resultStatus: "NOT_READY",
      },
    })}`,
    "",
  ].join("\n");
}

export function buildSuccessfulReplayDispatchList(
  sessionId = successfulReplaySessionID,
) {
  return {
    dispatches: [
      {
        dispatchKind: "JAVASCRIPT_AGENT",
        id: "dispatch-1",
        outputArtifactIds: [],
        phase: "review",
        status: "COMPLETED",
      },
    ],
    sessionId,
  };
}

export function buildWarningReplayDispatchList(
  sessionId = warningReplaySessionID,
) {
  return {
    dispatches: [
      {
        dispatchKind: "JAVASCRIPT_VERIFY",
        id: "dispatch-verify",
        outputArtifactIds: ["artifact-release-verification-log"],
        phase: "verify",
        status: "FAILED",
      },
    ],
    sessionId,
  };
}
