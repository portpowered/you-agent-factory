import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";

import {
  getProviderSessionDetails,
  type ProviderSessionDetailsAPIError,
  type ProviderSessionDetailResponse,
} from "../../api/provider-session-details";
import type { LoadableProviderSessionRef } from "./provider-session-details";

export const PROVIDER_SESSION_DETAIL_QUERY_KEY = [
  "current-selection-provider-session-detail",
] as const;

export type ProviderSessionDetailViewState =
  | { status: "idle" }
  | { sessionDetail: ProviderSessionDetailResponse; status: "empty" }
  | { message?: string; status: "error" }
  | { status: "loading" }
  | { status: "not-found" }
  | { sessionDetail: ProviderSessionDetailResponse; status: "parse-error" }
  | { sessionDetail: ProviderSessionDetailResponse; status: "success" };

export function useProviderSessionDetail(
  session: LoadableProviderSessionRef | null,
): ProviderSessionDetailViewState {
  const query = useQuery<
    ProviderSessionDetailResponse,
    ProviderSessionDetailsAPIError
  >({
    queryKey: [
      ...PROVIDER_SESSION_DETAIL_QUERY_KEY,
      session?.provider ?? "",
      session?.kind ?? "",
      session?.id ?? "",
    ],
    queryFn: () => {
      if (session === null) {
        throw new Error("Provider-session detail query requires a selected session.");
      }

      return getProviderSessionDetails(session);
    },
    enabled: session !== null,
    gcTime: 0,
    refetchOnWindowFocus: false,
    retry: false,
  });

  return useMemo<ProviderSessionDetailViewState>(() => {
    if (session === null) {
      return { status: "idle" };
    }

    if (query.isPending || query.isFetching) {
      return { status: "loading" };
    }

    if (query.error?.code === "NOT_FOUND") {
      return { status: "not-found" };
    }

    if (query.error) {
      return { message: query.error.message, status: "error" };
    }

    if (!query.data) {
      return {
        status: "error",
      };
    }

    if (
      query.data.parse.eventCount === 0 &&
      query.data.parse.lineCount === 0 &&
      query.data.parse.parseErrors.length === 0
    ) {
      return {
        sessionDetail: query.data,
        status: "empty",
      };
    }

    if (
      query.data.parse.eventCount === 0 &&
      query.data.parse.parseErrors.length > 0
    ) {
      return {
        sessionDetail: query.data,
        status: "parse-error",
      };
    }

    return {
      sessionDetail: query.data,
      status: "success",
    };
  }, [
    query.data,
    query.error,
    query.isFetching,
    query.isPending,
    session,
  ]);
}
