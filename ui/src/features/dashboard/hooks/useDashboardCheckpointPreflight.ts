import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import type { FactoryEventReconnectCursor } from "../../../api/events";
import { getFactorySessionSyncPreflight } from "../../../api/factory-sessions";
import {
  clearTimelineCheckpoint,
  type FactoryTimelineCheckpoint,
  type FactoryTimelineSyncIdentity,
  readTimelineCheckpoint,
} from "../../timeline/public";
import { clearDashboardSessionScopedQueries } from "../lib/dashboard-session-lifecycle";
import {
  buildDashboardSessionPreflightFailureRecoveryState,
  buildDashboardSessionRestorePlan,
  type DashboardSessionPreflightStatus,
  type DashboardSessionRecoveryState,
} from "../lib/dashboard-session-sync-preflight";

export interface UseDashboardCheckpointPreflightOptions {
  checkpointRestoreEnabled: boolean;
  rawSessionID: string | null;
  restoreCheckpoint: (checkpoint: FactoryTimelineCheckpoint) => void;
}

export interface DashboardCheckpointPreflightState {
  checkpointHydrated: boolean;
  initialReconnectCursor?: FactoryEventReconnectCursor;
  preflightStatus: DashboardSessionPreflightStatus;
  recoveryState: DashboardSessionRecoveryState | null;
  persistedSyncIdentity: FactoryTimelineSyncIdentity | null;
}

export function useDashboardCheckpointPreflight({
  checkpointRestoreEnabled,
  rawSessionID,
  restoreCheckpoint,
}: UseDashboardCheckpointPreflightOptions): DashboardCheckpointPreflightState {
  const queryClient = useQueryClient();
  const [checkpointHydratedSessionID, setCheckpointHydratedSessionID] =
    useState<string | null>(null);
  const [initialReconnectCursor, setInitialReconnectCursor] =
    useState<FactoryEventReconnectCursor | undefined>(undefined);
  const [persistedSyncIdentity, setPersistedSyncIdentity] =
    useState<FactoryTimelineSyncIdentity | null>(null);
  const [preflightStatus, setPreflightStatus] =
    useState<DashboardSessionPreflightStatus>("idle");
  const [recoveryState, setRecoveryState] =
    useState<DashboardSessionRecoveryState | null>(null);

  useEffect(() => {
    let cancelled = false;

    setCheckpointHydratedSessionID(null);
    setInitialReconnectCursor(undefined);
    setPersistedSyncIdentity(null);
    setPreflightStatus("idle");
    setRecoveryState(null);

    if (
      !rawSessionID ||
      typeof window === "undefined" ||
      !checkpointRestoreEnabled
    ) {
      setPreflightStatus("success");
      setCheckpointHydratedSessionID(rawSessionID);
      return;
    }

    const sessionID = rawSessionID;
    setPreflightStatus("loading");

    void hydrateDashboardCheckpoint().catch(() => {
      if (cancelled) {
        return;
      }
      setPreflightStatus("non-recoverable");
      setRecoveryState(
        buildDashboardSessionPreflightFailureRecoveryState(sessionID),
      );
      setCheckpointHydratedSessionID(sessionID);
    });

    return () => {
      cancelled = true;
    };

    async function hydrateDashboardCheckpoint(): Promise<void> {
      const checkpoint = await readTimelineCheckpoint(
        window.indexedDB,
        sessionID,
      );
      const preflight = await getFactorySessionSyncPreflight(
        sessionID,
        checkpoint == null
          ? undefined
          : {
              afterEventId: checkpoint.afterEventId,
              afterSequence: checkpoint.afterSequence,
            },
      );

      if (cancelled) {
        return;
      }

      const restorePlan = buildDashboardSessionRestorePlan(
        checkpoint,
        preflight,
      );
      if (restorePlan.clearCheckpoint) {
        await clearTimelineCheckpoint(window.indexedDB, sessionID);
      }
      if (cancelled) {
        return;
      }
      if (restorePlan.clearQueryCache) {
        clearDashboardSessionScopedQueries(queryClient, sessionID);
      }
      if (restorePlan.restoreCheckpoint) {
        restoreCheckpoint(restorePlan.restoreCheckpoint);
      }
      setInitialReconnectCursor(restorePlan.reconnectCursor);
      setPreflightStatus(restorePlan.preflightStatus);
      setRecoveryState(restorePlan.recoveryState);
      setPersistedSyncIdentity(restorePlan.syncIdentity);
      setCheckpointHydratedSessionID(sessionID);
    }
  }, [checkpointRestoreEnabled, queryClient, rawSessionID, restoreCheckpoint]);

  return {
    checkpointHydrated: checkpointHydratedSessionID === rawSessionID,
    initialReconnectCursor,
    preflightStatus,
    recoveryState,
    persistedSyncIdentity,
  };
}
