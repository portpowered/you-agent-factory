import { useCallback, useEffect, useMemo, useRef } from "react";

import type { FactoryEvent } from "../../../api/events";
import {
  persistTimelineCheckpoint,
  readFactoryTimelineDebugOptions,
  useFactoryTimelineStore,
} from "../../timeline/public";
import { useDashboardSession } from "../session/dashboard-session-provider";
import { useDashboardCheckpointPreflight } from "./useDashboardCheckpointPreflight";
import { useFactoryEventStream } from "./event-stream/useFactoryEventStream";
import { useDashboardSessionLifecycle } from "./useDashboardSessionLifecycle";
import { useDashboardTimelineMemoryDebug } from "./useDashboardTimelineMemoryDebug";
import { useDashboardWorldView } from "./useDashboardWorldView";

export interface UseDashboardSnapshotOptions {
  locale?: string | null;
  refreshToken?: number;
}

function canOpenEventStream(
  preflightStatus: ReturnType<
    typeof useDashboardCheckpointPreflight
  >["preflightStatus"],
): boolean {
  return (
    preflightStatus === "success" || preflightStatus === "silent-recovery"
  );
}

export function useDashboardSnapshot({
  locale,
  refreshToken = 0,
}: UseDashboardSnapshotOptions = {}) {
  const appendEvents = useFactoryTimelineStore((state) => state.appendEvents);
  const currentReplayCheckpoint = useFactoryTimelineStore(
    (state) => state.currentReplayCheckpoint,
  );
  const eventCount = useFactoryTimelineStore((state) => state.events.length);
  const restoreCheckpoint = useFactoryTimelineStore(
    (state) => state.restoreCheckpoint,
  );
  const { error, isInitialLoading, snapshot, streamState } =
    useDashboardWorldView();
  const { isPaused, rawSessionID } = useDashboardSession();
  const debugOptions = useMemo(() => readFactoryTimelineDebugOptions(), []);
  const queuedAppendRef =
    useRef<(events: FactoryEvent[]) => void>(appendEvents);

  queuedAppendRef.current = appendEvents;

  useDashboardSessionLifecycle({
    locale,
    refreshToken,
    sessionID: rawSessionID,
  });

  const {
    checkpointHydrated,
    initialReconnectCursor,
    preflightStatus,
    recoveryState,
    persistedSyncIdentity,
  } = useDashboardCheckpointPreflight({
    checkpointRestoreEnabled: !debugOptions.disableTimelineCheckpoint,
    refreshToken,
    rawSessionID,
    restoreCheckpoint,
  });

  useEffect(() => {
    if (
      typeof window === "undefined" ||
      debugOptions.disableTimelineCheckpoint
    ) {
      return;
    }
    const persistHandle = window.setTimeout(() => {
      void persistTimelineCheckpoint(
        window.indexedDB,
        rawSessionID,
        currentReplayCheckpoint && persistedSyncIdentity
          ? {
              ...currentReplayCheckpoint,
              syncIdentity: persistedSyncIdentity,
            }
          : undefined,
      );
    }, 750);
    return () => {
      window.clearTimeout(persistHandle);
    };
  }, [
    currentReplayCheckpoint,
    debugOptions.disableTimelineCheckpoint,
    persistedSyncIdentity,
    rawSessionID,
  ]);

  const handleStreamEvent = useCallback((event: FactoryEvent) => {
    // Fallback for stream-hook callers that do not provide a batched callback.
    queuedAppendRef.current([event]);
  }, []);

  const handleStreamEvents = useCallback((events: FactoryEvent[]) => {
    queuedAppendRef.current(events);
  }, []);

  useFactoryEventStream({
    enabled:
      checkpointHydrated &&
      rawSessionID != null &&
      !isPaused &&
      canOpenEventStream(preflightStatus),
    initialReconnectCursor,
    locale,
    onEvent: handleStreamEvent,
    onEvents: handleStreamEvents,
    refreshToken,
    sessionID: rawSessionID,
  });

  useDashboardTimelineMemoryDebug({
    debugOptions,
    eventCount,
  });

  return useMemo(
    () => ({
      error,
      isInitialLoading,
      preflightRecovery: recoveryState,
      preflightStatus,
      snapshot,
      streamState,
    }),
    [error, isInitialLoading, preflightStatus, recoveryState, snapshot, streamState],
  );
}
