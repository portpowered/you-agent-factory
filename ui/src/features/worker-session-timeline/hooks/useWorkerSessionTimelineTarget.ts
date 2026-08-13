import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import type { WorkerSessionObservation } from "../../../api/worker-sessions";
import {
  type ListWorkerSessionsForWorkOptions,
  listWorkerSessionsForWork,
  type WorkerSessionObservationAPIError,
} from "../../../api/worker-sessions";

export type WorkerSessionTimelineTargetLoader = (
  options: ListWorkerSessionsForWorkOptions,
) => Promise<WorkerSessionObservation[]>;

export type WorkerSessionTimelineTargetStatus =
  | "idle"
  | "loading"
  | "ready"
  | "error";

export interface UseWorkerSessionTimelineTargetOptions {
  factorySessionID: string | null;
  workID: string | null;
  enabled?: boolean;
  loadTargets?: WorkerSessionTimelineTargetLoader;
}

export interface UseWorkerSessionTimelineTargetResult {
  error: WorkerSessionObservationAPIError | null;
  observations: WorkerSessionObservation[];
  refetch: () => void;
  selectedWorkerSessionID: string | null;
  setSelectedWorkerSessionID: (workerSessionID: string) => void;
  status: WorkerSessionTimelineTargetStatus;
}

export function useWorkerSessionTimelineTarget({
  factorySessionID,
  workID,
  enabled = true,
  loadTargets = listWorkerSessionsForWork,
}: UseWorkerSessionTimelineTargetOptions): UseWorkerSessionTimelineTargetResult {
  const targetIsReady = enabled && factorySessionID !== null && workID !== null;
  const query = useQuery({
    enabled: targetIsReady,
    queryFn: ({ signal }) => {
      if (factorySessionID === null || workID === null) {
        throw new Error("Worker Session target scope is required.");
      }
      return loadTargets({ factorySessionID, signal, workID });
    },
    queryKey: ["worker-session-timeline-target", factorySessionID, workID],
    refetchOnWindowFocus: false,
    retry: false,
  });
  const [selectedWorkerSessionID, setSelectedWorkerSessionID] = useState<
    string | null
  >(null);

  useEffect(() => {
    const firstObservation = query.data?.[0];
    if (
      firstObservation === undefined ||
      query.data?.some(
        (observation) =>
          observation.workerSessionId === selectedWorkerSessionID,
      )
    ) {
      return;
    }
    setSelectedWorkerSessionID(firstObservation.workerSessionId);
  }, [query.data, selectedWorkerSessionID]);

  const status: WorkerSessionTimelineTargetStatus = !targetIsReady
    ? "idle"
    : query.isPending
      ? "loading"
      : query.isError
        ? "error"
        : "ready";
  const resolvedSelectedWorkerSessionID = targetIsReady
    ? (query.data?.find(
        (observation) =>
          observation.workerSessionId === selectedWorkerSessionID,
      )?.workerSessionId ??
      query.data?.[0]?.workerSessionId ??
      null)
    : null;

  return {
    error:
      query.error instanceof Error
        ? (query.error as WorkerSessionObservationAPIError)
        : null,
    observations: query.data ?? [],
    refetch: () => {
      void query.refetch();
    },
    selectedWorkerSessionID: resolvedSelectedWorkerSessionID,
    setSelectedWorkerSessionID,
    status,
  };
}
