import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import type { FactoryEventReconnectCursor } from "../../../../api/events";
import type {
  FactoryTimelineCheckpoint,
  TimelineCheckpointStreamIdentity,
} from "../../../timeline/public";
import type { DashboardSessionRecoveryState } from "../../lib/preflight/dashboard-session-sync-preflight";
import { runDashboardCheckpointPreflight } from "../../lib/preflight/run-dashboard-checkpoint-preflight";
import { useRemapDashboardSelectedSession } from "../../session/dashboard-session-provider";
import { useDashboardStreamStore } from "../../state/dashboardStreamStore";

export interface UseDashboardCheckpointPreflightResult {
  checkpointHydrated: boolean;
  initialReconnectCursor?: FactoryEventReconnectCursor;
  preflightError: Error | null;
  preflightRecovery: DashboardSessionRecoveryState | null;
  preflightReady: boolean;
  persistedCheckpoint: FactoryTimelineCheckpoint | null;
  resolvedSessionID: string | null;
  streamIdentity: TimelineCheckpointStreamIdentity | null;
}

function resetDashboardCheckpointPreflightState(
  setCheckpointHydratedKey: (value: string | null) => void,
  setPersistedCheckpoint: (value: FactoryTimelineCheckpoint | null) => void,
  setPreflightError: (value: Error | null) => void,
  setPreflightRecovery: (value: DashboardSessionRecoveryState | null) => void,
  setPreflightReadyKey: (value: string | null) => void,
  setInitialReconnectCursor: (
    value: FactoryEventReconnectCursor | undefined,
  ) => void,
  setResolvedSessionID: (value: string | null) => void,
  setStreamIdentity: (value: TimelineCheckpointStreamIdentity | null) => void,
): void {
  setCheckpointHydratedKey(null);
  setPersistedCheckpoint(null);
  setPreflightError(null);
  setPreflightRecovery(null);
  setPreflightReadyKey(null);
  setInitialReconnectCursor(undefined);
  setResolvedSessionID(null);
  setStreamIdentity(null);
}

export function useDashboardCheckpointPreflight({
  checkpointHydrationKey,
  checkpointsDisabled,
  rawSessionID,
  restoreCheckpoint,
}: {
  checkpointHydrationKey: string | null;
  checkpointsDisabled: boolean;
  rawSessionID: string | null;
  refreshToken: number;
  restoreCheckpoint: (checkpoint: FactoryTimelineCheckpoint) => void;
}): UseDashboardCheckpointPreflightResult {
  const queryClient = useQueryClient();
  const remapSelectedSessionID = useRemapDashboardSelectedSession();
  const setStreamState = useDashboardStreamStore(
    (state) => state.setStreamState,
  );
  const [checkpointHydratedKey, setCheckpointHydratedKey] = useState<
    string | null
  >(null);
  const [preflightReadyKey, setPreflightReadyKey] = useState<string | null>(
    null,
  );
  const [preflightError, setPreflightError] = useState<Error | null>(null);
  const [preflightRecovery, setPreflightRecovery] =
    useState<DashboardSessionRecoveryState | null>(null);
  const [persistedCheckpoint, setPersistedCheckpoint] =
    useState<FactoryTimelineCheckpoint | null>(null);
  const [initialReconnectCursor, setInitialReconnectCursor] = useState<
    FactoryEventReconnectCursor | undefined
  >(undefined);
  const [resolvedSessionID, setResolvedSessionID] = useState<string | null>(
    null,
  );
  const [streamIdentity, setStreamIdentity] =
    useState<TimelineCheckpointStreamIdentity | null>(null);

  const checkpointHydrated = checkpointHydratedKey === checkpointHydrationKey;
  const preflightReady = preflightReadyKey === checkpointHydrationKey;

  useEffect(() => {
    let cancelled = false;
    const abortController = new AbortController();

    resetDashboardCheckpointPreflightState(
      setCheckpointHydratedKey,
      setPersistedCheckpoint,
      setPreflightError,
      setPreflightRecovery,
      setPreflightReadyKey,
      setInitialReconnectCursor,
      setResolvedSessionID,
      setStreamIdentity,
    );

    if (
      checkpointHydrationKey == null ||
      !rawSessionID ||
      typeof window === "undefined" ||
      checkpointsDisabled
    ) {
      setResolvedSessionID(rawSessionID);
      setPreflightReadyKey(checkpointHydrationKey);
      setCheckpointHydratedKey(checkpointHydrationKey);
      return;
    }

    void (async () => {
      const hydration = await runDashboardCheckpointPreflight({
        isCurrent: () => !cancelled,
        onRemapSessionID: remapSelectedSessionID,
        onStreamOffline: (message) => {
          setStreamState({
            message,
            status: "offline",
          });
        },
        queryClient,
        rawSessionID,
        restoreCheckpoint,
        signal: abortController.signal,
      });
      if (cancelled) {
        return;
      }

      setPreflightRecovery(hydration.preflightRecovery);
      setPreflightError(hydration.preflightError);

      if (hydration.preflightRecovery != null) {
        setPreflightReadyKey(checkpointHydrationKey);
        setCheckpointHydratedKey(checkpointHydrationKey);
        return;
      }

      if (hydration.preflightError != null) {
        setCheckpointHydratedKey(checkpointHydrationKey);
        return;
      }

      setResolvedSessionID(hydration.resolvedSessionID);
      setStreamIdentity(hydration.streamIdentity);
      setInitialReconnectCursor(hydration.initialReconnectCursor);
      setPersistedCheckpoint(hydration.persistedCheckpoint);
      setPreflightReadyKey(checkpointHydrationKey);
      setCheckpointHydratedKey(checkpointHydrationKey);
    })();

    return () => {
      cancelled = true;
      abortController.abort();
    };
  }, [
    checkpointHydrationKey,
    checkpointsDisabled,
    queryClient,
    rawSessionID,
    remapSelectedSessionID,
    restoreCheckpoint,
    setStreamState,
  ]);

  return {
    checkpointHydrated,
    initialReconnectCursor,
    preflightError,
    preflightRecovery,
    preflightReady,
    persistedCheckpoint,
    resolvedSessionID,
    streamIdentity,
  };
}
