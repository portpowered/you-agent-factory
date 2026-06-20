import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";

import {
  type FactorySession,
  type FactorySessionDurableReadModel,
  type FactorySessionLiveResult,
  type FactorySessionPartialResult,
  type FactorySessionResult,
  type FactorySessionsAPIError,
  getDurableFactorySession,
  getDurableFactorySessionResult,
  getFactorySession,
  getFactorySessionPartialResult,
  getFactorySessionResult,
  isDurableFactorySessionID,
} from "../../../api/factory-sessions";
import { FactoryOrchestratorKind } from "../../../api/generated/openapi";

export const FACTORY_SESSION_DETAIL_QUERY_KEY = [
  "factory-session-detail",
] as const;

export type FactorySessionDetailData =
  | {
      durablePartialResult?: FactorySessionResult;
      durableResult?: FactorySessionResult;
      kind: "durable";
      session: FactorySessionDurableReadModel;
    }
  | {
      kind: "live";
      partialResult?: FactorySessionPartialResult;
      result?: FactorySessionLiveResult;
      session: FactorySession;
    };

export type FactorySessionDetailViewState =
  | { status: "idle" }
  | { data: FactorySessionDetailData; status: "success" }
  | { message?: string; status: "error" }
  | { status: "loading" }
  | { status: "not-found" };

async function loadLiveFactorySessionDetail(
  sessionID: string,
): Promise<FactorySessionDetailData> {
  const session = await getFactorySession(sessionID);
  if (session.runtime.orchestratorKind !== FactoryOrchestratorKind.JAVASCRIPT) {
    return { kind: "live", session };
  }

  const [result, partialResult] = await Promise.all([
    getFactorySessionResult(sessionID).catch(() => undefined),
    getFactorySessionPartialResult(sessionID).catch(() => undefined),
  ]);

  return {
    kind: "live",
    partialResult,
    result,
    session,
  };
}

async function loadDurableFactorySessionDetail(
  sessionID: string,
): Promise<FactorySessionDetailData> {
  const session = await getDurableFactorySession(sessionID);
  const [durableResult, durablePartialResult] = await Promise.all([
    getDurableFactorySessionResult(sessionID, { mode: "final" }).catch(
      () => undefined,
    ),
    getDurableFactorySessionResult(sessionID, { mode: "partial" }).catch(
      () => undefined,
    ),
  ]);

  return {
    durablePartialResult,
    durableResult,
    kind: "durable",
    session,
  };
}

export function useFactorySessionDetail(
  sessionID: string | null,
): FactorySessionDetailViewState {
  const query = useQuery<FactorySessionDetailData, FactorySessionsAPIError>({
    queryKey: [...FACTORY_SESSION_DETAIL_QUERY_KEY, sessionID ?? ""],
    queryFn: async () => {
      if (sessionID === null || sessionID.trim() === "") {
        throw new Error("Factory session detail requires a selected session id.");
      }

      if (isDurableFactorySessionID(sessionID)) {
        return loadDurableFactorySessionDetail(sessionID);
      }

      return loadLiveFactorySessionDetail(sessionID);
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
