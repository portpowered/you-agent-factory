import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import type { FactoryEvent } from "../../../api/events";
import {
  type FactoryTimelineCheckpoint,
  persistTimelineCheckpoint,
  readFactoryTimelineDebugOptions,
  readTimelineCheckpoint,
  reconnectCursorFromCheckpoint,
  useFactoryTimelineStore,
} from "../../timeline/public";
import { useDashboardSession } from "../session/dashboard-session-provider";
import { useFactoryEventStream } from "./event-stream/useFactoryEventStream";
import { useDashboardSessionLifecycle } from "./useDashboardSessionLifecycle";
import { useDashboardTimelineMemoryDebug } from "./useDashboardTimelineMemoryDebug";
import { useDashboardWorldView } from "./useDashboardWorldView";

export interface UseDashboardSnapshotOptions {
  locale?: string | null;
  refreshToken?: number;
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
  const [checkpointHydratedSessionID, setCheckpointHydratedSessionID] =
    useState<string | null>(null);
  const [persistedCheckpoint, setPersistedCheckpoint] =
    useState<FactoryTimelineCheckpoint | null>(null);
  const checkpointHydrationKey = useMemo(
    () => (rawSessionID == null ? null : `${rawSessionID}::${refreshToken}`),
    [rawSessionID, refreshToken],
  );
  queuedAppendRef.current = appendEvents;

  useDashboardSessionLifecycle({
    locale,
    refreshToken,
    sessionID: rawSessionID,
  });

  const checkpointHydrated =
    checkpointHydratedSessionID === checkpointHydrationKey;
  const initialReconnectCursor = useMemo(
    () => reconnectCursorFromCheckpoint(persistedCheckpoint),
    [persistedCheckpoint],
  );

  useEffect(() => {
    let cancelled = false;

    setCheckpointHydratedSessionID(null);
    setPersistedCheckpoint(null);

    if (
      !rawSessionID ||
      typeof window === "undefined" ||
      debugOptions.disableTimelineCheckpoint
    ) {
      setCheckpointHydratedSessionID(checkpointHydrationKey);
      return;
    }

    void readTimelineCheckpoint(window.indexedDB, rawSessionID).then(
      (checkpoint) => {
        if (cancelled) {
          return;
        }
        if (checkpoint) {
          restoreCheckpoint(checkpoint);
        }
        setPersistedCheckpoint(checkpoint);
        setCheckpointHydratedSessionID(checkpointHydrationKey);
      },
    );

    return () => {
      cancelled = true;
    };
  }, [
    checkpointHydrationKey,
    debugOptions.disableTimelineCheckpoint,
    rawSessionID,
    restoreCheckpoint,
  ]);

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
        currentReplayCheckpoint,
      );
    }, 750);
    return () => {
      window.clearTimeout(persistHandle);
    };
  }, [
    currentReplayCheckpoint,
    debugOptions.disableTimelineCheckpoint,
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
    enabled: checkpointHydrated && rawSessionID != null && !isPaused,
    initialReconnectCursor,
    locale,
    onEvent: handleStreamEvent,
    onEvents: handleStreamEvents,
    refreshToken,
    sessionID: rawSessionID,
  });

  useDashboardTimelineMemoryDebug({ debugOptions, eventCount });

  return useMemo(
    () => ({
      snapshot,
      streamState,
      isInitialLoading,
      error,
    }),
    [error, snapshot, streamState, isInitialLoading],
  );
}
