import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";

import type { FactorySessionsAPIError } from "../../../api/factory-sessions/api";
import { getFactorySessionDispatchDetail } from "../../../api/factory-sessions/dispatch-detail";
import {
  type FactorySessionDispatchDrilldownModel,
  normalizeFactorySessionDispatchDetail,
} from "../lib/factory-session-dispatch-detail";

export const FACTORY_SESSION_DISPATCH_DETAIL_QUERY_KEY = [
  "factory-session-dispatch-detail",
] as const;

export type FactorySessionDispatchDetailViewState =
  | { status: "idle" }
  | { data: FactorySessionDispatchDrilldownModel; status: "success" }
  | { message?: string; status: "error" }
  | { status: "loading" }
  | { status: "not-found" };

export function useFactorySessionDispatchDetail(
  sessionID: string,
  dispatchID: string | null,
): FactorySessionDispatchDetailViewState {
  const query = useQuery<
    FactorySessionDispatchDrilldownModel,
    FactorySessionsAPIError
  >({
    queryKey: [
      ...FACTORY_SESSION_DISPATCH_DETAIL_QUERY_KEY,
      sessionID,
      dispatchID ?? "",
    ],
    queryFn: async () => {
      if (dispatchID === null || dispatchID.trim() === "") {
        throw new Error(
          "Factory session dispatch detail requires a selected dispatch id.",
        );
      }

      const dispatch = await getFactorySessionDispatchDetail({
        dispatch_id: dispatchID,
        session_id: sessionID,
      });
      return normalizeFactorySessionDispatchDetail(dispatch);
    },
    enabled: dispatchID !== null && dispatchID.trim() !== "",
    gcTime: 0,
    refetchOnWindowFocus: false,
    retry: false,
  });

  return useMemo<FactorySessionDispatchDetailViewState>(() => {
    if (dispatchID === null || dispatchID.trim() === "") {
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
  }, [dispatchID, query.data, query.error, query.isFetching, query.isPending]);
}
