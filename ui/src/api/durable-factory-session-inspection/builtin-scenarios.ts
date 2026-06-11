import type {
  FactoryDispatch,
  FactorySessionDispatchSummary,
  FactorySessionDurableReadModel,
  FactorySessionDurableSummary,
  FactorySessionResult,
} from "./types";

export interface LoadedInspectionFixtureScenario {
  artifactDetail?: import("./types").FactorySessionArtifactDetail;
  artifacts: import("./types").FactorySessionArtifactSummary[];
  dispatchDetail?: FactoryDispatch;
  dispatches: FactorySessionDispatchSummary[];
  id: string;
  listSummary?: FactorySessionDurableSummary;
  result?: FactorySessionResult;
  session?: FactorySessionDurableReadModel;
}

export function builtinInterruptedRecoverableScenario(): LoadedInspectionFixtureScenario {
  const sessionId = "dur-sess-js-interrupted-001";
  const session: FactorySessionDurableReadModel = {
    dialect: "you-workflow-v1",
    lifecycle: {
      interruptedAt: "2026-06-08T10:05:00Z",
      startedAt: "2026-06-08T10:00:00Z",
      updatedAt: "2026-06-08T10:05:00Z",
    },
    links: {
      events: `/factory-sessions/${sessionId}/events`,
      results: `/factory-sessions/${sessionId}/results`,
      session: `/factory-sessions/${sessionId}`,
      status: `/factory-sessions/${sessionId}`,
    },
    orchestratorKind: "JAVASCRIPT",
    phase: "audit",
    progress: {
      completedDispatches: 1,
      failedDispatches: 0,
      inFlightDispatches: 0,
      totalDispatches: 2,
    },
    resolvedSource: {
      dialect: "you-workflow-v1",
      kind: "WORKFLOW_NAME",
      sourceHash: "sha256:js-workflow-recoverable-audit",
      sourceRef: "workflow/recoverable-audit",
    },
    resultSummary: {
      resultStatus: "PARTIAL",
      summary: "Interrupted after partial audit progress.",
    },
    sessionId,
    sourceHash: "sha256:js-workflow-recoverable-audit",
    staleLease: true,
    status: "INTERRUPTED",
    usage: {
      resources: [],
    },
  };
  const listSummary: FactorySessionDurableSummary = {
    actions: {
      canResume: true,
      canTerminate: true,
    },
    artifactCount: 0,
    orchestratorKind: "JAVASCRIPT",
    phase: "audit",
    progress: {
      completedDispatches: 1,
      failedDispatches: 0,
      inFlightDispatches: 0,
      totalDispatches: 2,
    },
    recoverable: true,
    resolvedSource: session.resolvedSource,
    resultSummary: session.resultSummary,
    sessionId,
    sourceHash: session.sourceHash,
    staleLease: true,
    status: "INTERRUPTED",
  };
  const dispatches: FactorySessionDispatchSummary[] = [
    {
      attempt: 1,
      dispatchKind: "JAVASCRIPT_AGENT",
      id: "disp-js-interrupted-001",
      label: "plan-audit",
      phase: "plan",
      status: "COMPLETED",
    },
    {
      attempt: 1,
      dispatchKind: "JAVASCRIPT_AGENT",
      id: "disp-js-interrupted-002",
      label: "audit",
      phase: "audit",
      status: "FAILED",
    },
  ];
  const dispatchDetail: FactoryDispatch = {
    attempt: 1,
    dispatchKind: "JAVASCRIPT_AGENT",
    id: "disp-js-interrupted-002",
    javascript: {
      taskKind: "AGENT",
      taskLabel: "audit",
    },
    label: "audit",
    orchestratorKind: "JAVASCRIPT",
    sessionId,
    status: "FAILED",
  };
  const result: FactorySessionResult = {
    mode: "partial",
    primaryResult: [
      {
        text: "Partial audit notes before interruption.",
        type: "text",
      },
    ],
    resultStatus: "PARTIAL",
    sessionId,
    sessionStatus: "INTERRUPTED",
  };

  return {
    artifacts: [],
    dispatches,
    dispatchDetail,
    id: "javascript-interrupted-recoverable",
    listSummary,
    result,
    session,
  };
}
