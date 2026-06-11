import type { LoadedInspectionFixtureScenario } from "./builtin-scenarios";
import {
  createInspectionFixtureScenarioIndex,
  loadJavaScriptInspectionFixtureScenarios,
} from "./fixture-scenarios";
import type {
  DurableSessionListInspectionData,
  FactoryDispatch,
  FactorySessionArtifactDetail,
  FactorySessionDurableReadModel,
  FactorySessionResult,
  FixtureAdapterRequestOptions,
  InspectionAdapterOutcome,
  ListFactorySessionArtifactsResponse,
  ListFactorySessionDispatchesResponse,
} from "./types";

const DEFAULT_ADAPTER_ERROR_MESSAGE =
  "Fixture-backed durable session inspection is unavailable.";

export interface FixtureBackedDurableSessionInspectionAdapter {
  listSessions: (
    options?: FixtureAdapterRequestOptions,
  ) => Promise<InspectionAdapterOutcome<DurableSessionListInspectionData>>;
  getSession: (
    sessionId: string,
    options?: FixtureAdapterRequestOptions,
  ) => Promise<InspectionAdapterOutcome<FactorySessionDurableReadModel>>;
  getResult: (
    sessionId: string,
    options?: FixtureAdapterRequestOptions,
  ) => Promise<InspectionAdapterOutcome<FactorySessionResult>>;
  listDispatches: (
    sessionId: string,
    options?: FixtureAdapterRequestOptions,
  ) => Promise<InspectionAdapterOutcome<ListFactorySessionDispatchesResponse>>;
  getDispatch: (
    sessionId: string,
    dispatchId: string,
    options?: FixtureAdapterRequestOptions,
  ) => Promise<InspectionAdapterOutcome<FactoryDispatch>>;
  listArtifacts: (
    sessionId: string,
    options?: FixtureAdapterRequestOptions,
  ) => Promise<InspectionAdapterOutcome<ListFactorySessionArtifactsResponse>>;
  getArtifact: (
    sessionId: string,
    artifactId: string,
    options?: FixtureAdapterRequestOptions,
  ) => Promise<InspectionAdapterOutcome<FactorySessionArtifactDetail>>;
}

export interface CreateFixtureBackedDurableSessionInspectionAdapterOptions {
  scenarios?: LoadedInspectionFixtureScenario[];
}

export function createFixtureBackedDurableSessionInspectionAdapter(
  options: CreateFixtureBackedDurableSessionInspectionAdapterOptions = {},
): FixtureBackedDurableSessionInspectionAdapter {
  const index = createInspectionFixtureScenarioIndex(
    options.scenarios ?? loadJavaScriptInspectionFixtureScenarios(),
  );

  return {
    listSessions: (requestOptions) =>
      Promise.resolve(listSessionsOutcome(index.scenarios, requestOptions)),
    getSession: (sessionId, requestOptions) =>
      Promise.resolve(
        getSessionOutcome(index.bySessionId, sessionId, requestOptions),
      ),
    getResult: (sessionId, requestOptions) =>
      Promise.resolve(
        getResultOutcome(index.bySessionId, sessionId, requestOptions),
      ),
    listDispatches: (sessionId, requestOptions) =>
      Promise.resolve(
        listDispatchesOutcome(index.bySessionId, sessionId, requestOptions),
      ),
    getDispatch: (sessionId, dispatchId, requestOptions) =>
      Promise.resolve(
        getDispatchOutcome(
          index.bySessionId,
          sessionId,
          dispatchId,
          requestOptions,
        ),
      ),
    listArtifacts: (sessionId, requestOptions) =>
      Promise.resolve(
        listArtifactsOutcome(index.bySessionId, sessionId, requestOptions),
      ),
    getArtifact: (sessionId, artifactId, requestOptions) =>
      Promise.resolve(
        getArtifactOutcome(
          index.bySessionId,
          sessionId,
          artifactId,
          requestOptions,
        ),
      ),
  };
}

function listSessionsOutcome(
  scenarios: LoadedInspectionFixtureScenario[],
  options?: FixtureAdapterRequestOptions,
): InspectionAdapterOutcome<DurableSessionListInspectionData> {
  const simulated =
    resolveSimulatedOutcome<DurableSessionListInspectionData>(options);
  if (simulated) {
    return simulated;
  }

  const sessions = scenarios
    .map((scenario) => scenario.listSummary ?? summaryFromSession(scenario))
    .filter(
      (summary): summary is NonNullable<typeof summary> =>
        summary !== undefined,
    )
    .filter((summary) => summary.orchestratorKind === "JAVASCRIPT");

  if (sessions.length === 0) {
    return { status: "empty" };
  }

  return {
    data: {
      scope: "persisted",
      sessions,
    },
    status: "success",
  };
}

function getSessionOutcome(
  scenariosBySessionId: Map<string, LoadedInspectionFixtureScenario>,
  sessionId: string,
  options?: FixtureAdapterRequestOptions,
): InspectionAdapterOutcome<FactorySessionDurableReadModel> {
  const simulated =
    resolveSimulatedOutcome<FactorySessionDurableReadModel>(options);
  if (simulated) {
    return simulated;
  }

  const scenario = scenariosBySessionId.get(sessionId);
  if (!scenario?.session) {
    return {
      code: "NOT_FOUND",
      message: `Durable factory session ${sessionId} was not found in fixtures.`,
      status: "error",
    };
  }

  return {
    data: scenario.session,
    status: "success",
  };
}

function getResultOutcome(
  scenariosBySessionId: Map<string, LoadedInspectionFixtureScenario>,
  sessionId: string,
  options?: FixtureAdapterRequestOptions,
): InspectionAdapterOutcome<FactorySessionResult> {
  const simulated = resolveSimulatedOutcome<FactorySessionResult>(options);
  if (simulated) {
    return simulated;
  }

  const scenario = scenariosBySessionId.get(sessionId);
  if (!scenario) {
    return {
      code: "NOT_FOUND",
      message: `Durable factory session ${sessionId} was not found in fixtures.`,
      status: "error",
    };
  }

  if (!scenario.result) {
    return { status: "empty" };
  }

  return {
    data: scenario.result,
    status: "success",
  };
}

function listDispatchesOutcome(
  scenariosBySessionId: Map<string, LoadedInspectionFixtureScenario>,
  sessionId: string,
  options?: FixtureAdapterRequestOptions,
): InspectionAdapterOutcome<ListFactorySessionDispatchesResponse> {
  const simulated =
    resolveSimulatedOutcome<ListFactorySessionDispatchesResponse>(options);
  if (simulated) {
    return simulated;
  }

  const scenario = scenariosBySessionId.get(sessionId);
  if (!scenario) {
    return {
      code: "NOT_FOUND",
      message: `Durable factory session ${sessionId} was not found in fixtures.`,
      status: "error",
    };
  }

  const dispatches = scenario.dispatches;
  if (dispatches.length === 0) {
    return { status: "empty" };
  }

  return {
    data: {
      dispatches,
      sessionId,
    },
    status: "success",
  };
}

function getDispatchOutcome(
  scenariosBySessionId: Map<string, LoadedInspectionFixtureScenario>,
  sessionId: string,
  dispatchId: string,
  options?: FixtureAdapterRequestOptions,
): InspectionAdapterOutcome<FactoryDispatch> {
  const simulated = resolveSimulatedOutcome<FactoryDispatch>(options);
  if (simulated) {
    return simulated;
  }

  const scenario = scenariosBySessionId.get(sessionId);
  if (!scenario) {
    return {
      code: "NOT_FOUND",
      message: `Durable factory session ${sessionId} was not found in fixtures.`,
      status: "error",
    };
  }

  const dispatch =
    scenario.dispatchDetail?.id === dispatchId
      ? scenario.dispatchDetail
      : scenario.dispatches.find((row) => row.id === dispatchId);

  if (!dispatch) {
    return {
      code: "NOT_FOUND",
      message: `Dispatch ${dispatchId} was not found for session ${sessionId}.`,
      status: "error",
    };
  }

  if ("sessionId" in dispatch) {
    return {
      data: dispatch,
      status: "success",
    };
  }

  return {
    data: {
      ...dispatch,
      orchestratorKind: scenario.session?.orchestratorKind ?? "JAVASCRIPT",
      sessionId,
    },
    status: "success",
  };
}

function listArtifactsOutcome(
  scenariosBySessionId: Map<string, LoadedInspectionFixtureScenario>,
  sessionId: string,
  options?: FixtureAdapterRequestOptions,
): InspectionAdapterOutcome<ListFactorySessionArtifactsResponse> {
  const simulated =
    resolveSimulatedOutcome<ListFactorySessionArtifactsResponse>(options);
  if (simulated) {
    return simulated;
  }

  const scenario = scenariosBySessionId.get(sessionId);
  if (!scenario) {
    return {
      code: "NOT_FOUND",
      message: `Durable factory session ${sessionId} was not found in fixtures.`,
      status: "error",
    };
  }

  if (scenario.artifacts.length === 0) {
    return { status: "empty" };
  }

  return {
    data: {
      artifacts: scenario.artifacts,
      sessionId,
    },
    status: "success",
  };
}

function getArtifactOutcome(
  scenariosBySessionId: Map<string, LoadedInspectionFixtureScenario>,
  sessionId: string,
  artifactId: string,
  options?: FixtureAdapterRequestOptions,
): InspectionAdapterOutcome<FactorySessionArtifactDetail> {
  const simulated =
    resolveSimulatedOutcome<FactorySessionArtifactDetail>(options);
  if (simulated) {
    return simulated;
  }

  const scenario = scenariosBySessionId.get(sessionId);
  if (!scenario) {
    return {
      code: "NOT_FOUND",
      message: `Durable factory session ${sessionId} was not found in fixtures.`,
      status: "error",
    };
  }

  if (
    scenario.artifactDetail?.id === artifactId &&
    scenario.artifactDetail.sessionId === sessionId
  ) {
    return {
      data: scenario.artifactDetail,
      status: "success",
    };
  }

  const summary = scenario.artifacts.find(
    (artifact) => artifact.id === artifactId,
  );
  if (!summary) {
    return {
      code: "NOT_FOUND",
      message: `Artifact ${artifactId} was not found for session ${sessionId}.`,
      status: "error",
    };
  }

  return {
    data: {
      ...summary,
      sessionId,
    },
    status: "success",
  };
}

function resolveSimulatedOutcome<T>(
  options?: FixtureAdapterRequestOptions,
): InspectionAdapterOutcome<T> | undefined {
  switch (options?.simulate) {
    case "loading":
      return { status: "loading" };
    case "empty":
      return { status: "empty" };
    case "error":
      return {
        code: options.errorCode,
        message: options.errorMessage ?? DEFAULT_ADAPTER_ERROR_MESSAGE,
        status: "error",
      };
    default:
      return undefined;
  }
}

function summaryFromSession(
  scenario: LoadedInspectionFixtureScenario,
): LoadedInspectionFixtureScenario["listSummary"] {
  if (scenario.listSummary) {
    return scenario.listSummary;
  }

  if (!scenario.session) {
    return undefined;
  }

  return {
    artifactCount: scenario.artifacts.length,
    orchestratorKind: scenario.session.orchestratorKind,
    phase: scenario.session.phase,
    progress: scenario.session.progress,
    resolvedSource: scenario.session.resolvedSource,
    resultSummary: scenario.session.resultSummary,
    sessionId: scenario.session.sessionId,
    sourceHash: scenario.session.sourceHash,
    staleLease: scenario.session.staleLease,
    status: scenario.session.status,
  };
}
