import { useQuery } from "@tanstack/react-query";
import { useCallback, useEffect, useRef, useState } from "react";
import type { WorkerSessionObservation } from "../../../api/worker-sessions";
import {
  type ListWorkerSessionsForWorkOptions,
  listWorkerSessionsForWork,
  type WorkerSessionObservationAPIError,
} from "../../../api/worker-sessions";
import {
  getWorkerSessionTimelineSelectionStorageKey,
  readWorkerSessionTimelineSelection,
  writeWorkerSessionTimelineSelection,
} from "../lib/worker-session-timeline-selection";

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
  const selectionStorageKey = getWorkerSessionTimelineSelectionStorageKey(
    factorySessionID,
    workID,
  );
  const previousSelectionStorageKey = useRef<string | null>(null);

  useEffect(() => {
    if (!targetIsReady || query.data === undefined) {
      return;
    }
    const scopeChanged =
      previousSelectionStorageKey.current !== selectionStorageKey;
    previousSelectionStorageKey.current = selectionStorageKey;
    const persistedWorkerSessionID =
      readWorkerSessionTimelineSelection(selectionStorageKey);
    const selectedFromCurrentState = scopeChanged
      ? undefined
      : query.data.find(
          (observation) =>
            observation.workerSessionId === selectedWorkerSessionID,
        )?.workerSessionId;
    const nextWorkerSessionID =
      selectedFromCurrentState ??
      query.data.find(
        (observation) =>
          observation.workerSessionId === persistedWorkerSessionID,
      )?.workerSessionId ??
      query.data[0]?.workerSessionId ??
      null;

    if (nextWorkerSessionID !== selectedWorkerSessionID) {
      setSelectedWorkerSessionID(nextWorkerSessionID);
    }
    writeWorkerSessionTimelineSelection(
      selectionStorageKey,
      nextWorkerSessionID,
    );
  }, [query.data, selectedWorkerSessionID, selectionStorageKey, targetIsReady]);

  const selectWorkerSession = useCallback(
    (workerSessionID: string) => {
      setSelectedWorkerSessionID(workerSessionID);
      writeWorkerSessionTimelineSelection(selectionStorageKey, workerSessionID);
    },
    [selectionStorageKey],
  );

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
    setSelectedWorkerSessionID: selectWorkerSession,
    status,
  };
}
