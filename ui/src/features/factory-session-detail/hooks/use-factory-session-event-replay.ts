import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import type { FactoryEvent } from "../../../api/events";
import {
  type FactorySessionsAPIError,
  listFactorySessionEventReplay,
} from "../../../api/factory-sessions";

export const FACTORY_SESSION_EVENT_REPLAY_QUERY_KEY = [
  "factory-session-event-replay",
] as const;

export type FactorySessionEventReplayViewState =
  | { status: "idle" }
  | { status: "loading" }
  | { status: "unavailable"; message?: string }
  | { status: "error"; message?: string }
  | { status: "success"; events: FactoryEvent[] };

export function useFactorySessionEventReplay(
  sessionID: string | null,
  enabled: boolean,
): FactorySessionEventReplayViewState {
  const query = useQuery<FactoryEvent[], FactorySessionsAPIError>({
    queryKey: [...FACTORY_SESSION_EVENT_REPLAY_QUERY_KEY, sessionID ?? ""],
    queryFn: async () => {
      if (sessionID === null || sessionID.trim() === "") {
        throw new Error("Factory session event replay requires a session id.");
      }
      return listFactorySessionEventReplay(sessionID);
    },
    enabled: enabled && sessionID !== null && sessionID.trim() !== "",
    gcTime: 0,
    refetchOnWindowFocus: false,
    retry: false,
  });

  return useMemo<FactorySessionEventReplayViewState>(() => {
    if (!enabled || sessionID === null || sessionID.trim() === "") {
      return { status: "idle" };
    }

    if (query.isPending || query.isFetching) {
      return { status: "loading" };
    }

    if (query.error) {
      if (query.error.status === 404) {
        return { status: "unavailable" };
      }
      return { message: query.error.message, status: "error" };
    }

    return { events: query.data ?? [], status: "success" };
  }, [
    enabled,
    query.data,
    query.error,
    query.isFetching,
    query.isPending,
    sessionID,
  ]);
}
