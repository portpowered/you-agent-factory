import {
  durableResultSurfacesFromResultsResponse,
  type FactorySessionLiveResult,
  type FactorySessionPartialResult,
  getFactorySession,
  getFactorySessionDurableResults,
  getFactorySessionPartialResult,
  getFactorySessionResult,
  listFactorySessionDispatches,
} from "../../../api/factory-sessions";
import {
  dispatchSummariesToFactoryDispatches,
  durableSupplementalReadPlan,
  isDurableJavaScriptSession,
} from "../../../api/factory-sessions/normalize-durable-inspection";
import { FactoryOrchestratorKind } from "../../../api/generated/openapi";
import type { FactorySessionDetailData } from "./use-factory-session-detail";

export async function loadFactorySessionDetailData(
  sessionID: string,
): Promise<FactorySessionDetailData> {
  const normalized = await getFactorySession(sessionID);
  const {
    durableLifecycleStatus,
    durableProgress,
    durableReadModel,
    resultSummary,
    session,
  } = normalized;
  let { partialResult, result } = normalized;

  if (session.runtime.orchestratorKind !== FactoryOrchestratorKind.JAVASCRIPT) {
    return { durableLifecycleStatus, durableReadModel, session };
  }

  const durableJavaScript = isDurableJavaScriptSession(
    session.id,
    session.runtime.orchestratorKind,
    durableLifecycleStatus,
  );
  const supplementalReads = durableJavaScript
    ? durableSupplementalReadPlan({
        durableLifecycleStatus,
        partialResult,
        progress: durableProgress,
        result,
        resultSummary,
      })
    : {
        fetchDispatches: false,
        fetchFinalResults: false,
        fetchPartialResults: false,
      };
  const fetchDispatches = durableJavaScript
    ? supplementalReads.fetchDispatches
    : true;

  const [
    dispatchList,
    liveResult,
    livePartialResult,
    durableFinalResult,
    durablePartialResult,
  ] = await Promise.all([
    fetchDispatches
      ? listFactorySessionDispatches(sessionID)
      : Promise.resolve(undefined),
    durableJavaScript
      ? Promise.resolve(undefined)
      : getFactorySessionResult(sessionID),
    durableJavaScript
      ? Promise.resolve(undefined)
      : getFactorySessionPartialResult(sessionID),
    supplementalReads.fetchFinalResults
      ? getFactorySessionDurableResults(sessionID, "final")
      : Promise.resolve(undefined),
    supplementalReads.fetchPartialResults
      ? getFactorySessionDurableResults(sessionID, "partial")
      : Promise.resolve(undefined),
  ]);

  const dispatches = dispatchList
    ? dispatchSummariesToFactoryDispatches(
        session.id,
        session.runtime.orchestratorKind,
        dispatchList.dispatches,
      )
    : undefined;

  if (!result) {
    result = resolveFactorySessionResult(
      durableFinalResult,
      liveResult,
      session.runtime.javascript?.phase,
    );
  }

  if (!partialResult) {
    partialResult = resolveFactorySessionPartialResult(
      durablePartialResult,
      livePartialResult,
      session.runtime.javascript?.phase,
    );
  }

  return {
    dispatches,
    durableLifecycleStatus,
    durableReadModel,
    partialResult,
    result,
    session,
  };
}

function resolveFactorySessionResult(
  durableFinalResult:
    | Awaited<ReturnType<typeof getFactorySessionDurableResults>>
    | undefined,
  liveResult: FactorySessionLiveResult | undefined,
  fallbackPhase?: string,
): FactorySessionLiveResult | undefined {
  if (durableFinalResult === undefined) {
    return liveResult;
  }

  return durableResultSurfacesFromResultsResponse(
    durableFinalResult,
    fallbackPhase,
  ).result;
}

function resolveFactorySessionPartialResult(
  durablePartialResult:
    | Awaited<ReturnType<typeof getFactorySessionDurableResults>>
    | undefined,
  livePartialResult: FactorySessionPartialResult | undefined,
  fallbackPhase?: string,
): FactorySessionPartialResult | undefined {
  if (durablePartialResult === undefined) {
    return livePartialResult;
  }

  return durableResultSurfacesFromResultsResponse(
    durablePartialResult,
    fallbackPhase,
  ).partialResult;
}
