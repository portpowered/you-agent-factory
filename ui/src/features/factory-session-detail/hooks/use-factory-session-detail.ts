import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";

import type {
  FactorySession,
  FactorySessionDurableReadModel,
  FactorySessionLiveResult,
  FactorySessionPartialResult,
  FactorySessionsAPIError,
} from "../../../api/factory-sessions";
import { loadFactorySessionDetailData } from "./load-factory-session-detail-data";

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

      return loadFactorySessionDetailData(sessionID);
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

    if (query.isPending || (query.isFetching && !query.data)) {
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
