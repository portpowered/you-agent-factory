import {
  type FactorySession,
  type FactorySessionLiveResult,
  type FactorySessionPartialResult,
  durableResultSurfacesFromResultsResponse,
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
  let {
    durableLifecycleStatus,
    durableProgress,
    partialResult,
    result,
    resultSummary,
    session,
  } = normalized;

  if (session.runtime.orchestratorKind !== FactoryOrchestratorKind.JAVASCRIPT) {
    return { durableLifecycleStatus, session };
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

  const [dispatchList, liveResult, livePartialResult, durableFinalResult, durablePartialResult] =
    await Promise.all([
      supplementalReads.fetchDispatches
        ? listFactorySessionDispatches(sessionID).catch(() => undefined)
        : Promise.resolve(undefined),
      durableJavaScript
        ? Promise.resolve(undefined)
        : getFactorySessionResult(sessionID).catch(() => undefined),
      durableJavaScript
        ? Promise.resolve(undefined)
        : getFactorySessionPartialResult(sessionID).catch(() => undefined),
      supplementalReads.fetchFinalResults
        ? getFactorySessionDurableResults(sessionID, "final").catch(() => undefined)
        : Promise.resolve(undefined),
      supplementalReads.fetchPartialResults
        ? getFactorySessionDurableResults(sessionID, "partial").catch(() => undefined)
        : Promise.resolve(undefined),
    ]);

  if (dispatchList && dispatchList.dispatches.length > 0) {
    session = mergeDispatchSummaries(session, dispatchList.dispatches);
  }

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
    durableLifecycleStatus,
    partialResult,
    result,
    session,
  };
}

function mergeDispatchSummaries(
  session: FactorySession,
  dispatches: Parameters<typeof dispatchSummariesToFactoryDispatches>[2],
): FactorySession {
  return {
    ...session,
    runtime: {
      ...session.runtime,
      dispatches: dispatchSummariesToFactoryDispatches(
        session.id,
        session.runtime.orchestratorKind,
        dispatches,
      ),
    },
  };
}

function resolveFactorySessionResult(
  durableFinalResult: Awaited<ReturnType<typeof getFactorySessionDurableResults>> | undefined,
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
  durablePartialResult: Awaited<ReturnType<typeof getFactorySessionDurableResults>> | undefined,
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
