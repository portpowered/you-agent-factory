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
import { buildDashboardSessionRestorePlan } from "../lib/dashboard-session-sync-preflight";

export interface UseDashboardCheckpointPreflightOptions {
  checkpointRestoreEnabled: boolean;
  rawSessionID: string | null;
  restoreCheckpoint: (checkpoint: FactoryTimelineCheckpoint) => void;
}

export interface DashboardCheckpointPreflightState {
  checkpointHydrated: boolean;
  initialReconnectCursor?: FactoryEventReconnectCursor;
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

  useEffect(() => {
    let cancelled = false;

    setCheckpointHydratedSessionID(null);
    setInitialReconnectCursor(undefined);
    setPersistedSyncIdentity(null);

    if (
      !rawSessionID ||
      typeof window === "undefined" ||
      !checkpointRestoreEnabled
    ) {
      setCheckpointHydratedSessionID(rawSessionID);
      return;
    }

    const sessionID = rawSessionID;

    void hydrateDashboardCheckpoint().catch(() => {
      if (cancelled) {
        return;
      }
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
      setPersistedSyncIdentity(restorePlan.syncIdentity);
      setCheckpointHydratedSessionID(sessionID);
    }
  }, [checkpointRestoreEnabled, queryClient, rawSessionID, restoreCheckpoint]);

  return {
    checkpointHydrated: checkpointHydratedSessionID === rawSessionID,
    initialReconnectCursor,
    persistedSyncIdentity,
  };
}
