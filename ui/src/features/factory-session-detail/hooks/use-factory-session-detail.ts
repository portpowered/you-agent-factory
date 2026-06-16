import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";

import {
  type FactorySession,
  type FactorySessionDurableReadModel,
  type FactorySessionLiveResult,
  type FactorySessionPartialResult,
  type FactorySessionsAPIError,
  durableResultSurfacesFromResultsResponse,
  getFactorySession,
  getFactorySessionDurableResults,
  getFactorySessionPartialResult,
  getFactorySessionResult,
  listFactorySessionDispatches,
} from "../../../api/factory-sessions";
import {
  dispatchSummariesToFactoryDispatches,
  isDurableJavaScriptSession,
  shouldFetchDurablePartialResults,
} from "../../../api/factory-sessions/normalize-durable-inspection";
import { FactoryOrchestratorKind } from "../../../api/generated/openapi";

export const FACTORY_SESSION_DETAIL_QUERY_KEY = [
  "factory-session-detail",
] as const;

export interface FactorySessionDetailData {
  durableLifecycleStatus?: FactorySessionDurableReadModel["status"];
  partialResult?: FactorySessionPartialResult;
  result?: FactorySessionLiveResult;
  session: FactorySession;
}

export type FactorySessionDetailViewState =
  | { status: "idle" }
  | { data: FactorySessionDetailData; status: "success" }
  | { message?: string; status: "error" }
  | { status: "loading" }
  | { status: "not-found" };

export function useFactorySessionDetail(
  sessionID: string | null,
): FactorySessionDetailViewState {
  const query = useQuery<FactorySessionDetailData, FactorySessionsAPIError>({
    queryKey: [...FACTORY_SESSION_DETAIL_QUERY_KEY, sessionID ?? ""],
    queryFn: async () => {
      if (sessionID === null || sessionID.trim() === "") {
        throw new Error("Factory session detail requires a selected session id.");
      }

      const normalized = await getFactorySession(sessionID);
      let { durableLifecycleStatus, partialResult, result, resultSummary, session } =
        normalized;

      if (session.runtime.orchestratorKind !== FactoryOrchestratorKind.JAVASCRIPT) {
        return { durableLifecycleStatus, session };
      }

      const durableJavaScript = isDurableJavaScriptSession(
        session.id,
        session.runtime.orchestratorKind,
        durableLifecycleStatus,
      );
      const fetchDurablePartialResults =
        durableJavaScript &&
        shouldFetchDurablePartialResults({
          partialResult,
          result,
          resultSummary,
        });

      const [dispatchList, liveResult, livePartialResult, durableFinalResult, durablePartialResult] =
        await Promise.all([
          durableJavaScript
            ? listFactorySessionDispatches(sessionID).catch(() => undefined)
            : Promise.resolve(undefined),
          durableJavaScript
            ? Promise.resolve(undefined)
            : getFactorySessionResult(sessionID).catch(() => undefined),
          durableJavaScript
            ? Promise.resolve(undefined)
            : getFactorySessionPartialResult(sessionID).catch(() => undefined),
          durableJavaScript && !result
            ? getFactorySessionDurableResults(sessionID, "final").catch(() => undefined)
            : Promise.resolve(undefined),
          fetchDurablePartialResults
            ? getFactorySessionDurableResults(sessionID, "partial").catch(() => undefined)
            : Promise.resolve(undefined),
        ]);

      if (dispatchList && dispatchList.dispatches.length > 0) {
        session = {
          ...session,
          runtime: {
            ...session.runtime,
            dispatches: dispatchSummariesToFactoryDispatches(
              session.id,
              session.runtime.orchestratorKind,
              dispatchList.dispatches,
            ),
          },
        };
      }

      if (!result) {
        result =
          durableFinalResult === undefined
            ? liveResult
            : durableResultSurfacesFromResultsResponse(
                durableFinalResult,
                session.runtime.javascript?.phase,
              ).result;
      }

      if (!partialResult) {
        partialResult =
          durablePartialResult === undefined
            ? livePartialResult
            : durableResultSurfacesFromResultsResponse(
                durablePartialResult,
                session.runtime.javascript?.phase,
              ).partialResult;
      }

      return {
        durableLifecycleStatus,
        partialResult,
        result,
        session,
      };
    },
    enabled: sessionID !== null && sessionID.trim() !== "",
    gcTime: 0,
    refetchOnWindowFocus: false,
    retry: false,
  });

  return useMemo<FactorySessionDetailViewState>(() => {
    if (sessionID === null || sessionID.trim() === "") {
      return { status: "idle" };
    }

    if (query.isPending || query.isFetching) {
      return { status: "loading" };
    }

    if (query.error?.status === 404) {
      return { status: "not-found" };
    }

    if (query.error) {
      return { message: query.error.message, status: "error" };
    }

    if (!query.data) {
      return { status: "error" };
    }

    return { data: query.data, status: "success" };
  }, [
    query.data,
    query.error,
    query.isFetching,
    query.isPending,
    sessionID,
  ]);
}
