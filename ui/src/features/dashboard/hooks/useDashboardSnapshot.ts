import { useCallback, useEffect, useMemo, useRef } from "react";

import type { FactoryEvent } from "../../../api/events";
import {
  clearTimelineCheckpoint,
  type FactoryTimelineCheckpoint,
  persistTimelineCheckpoint,
  reconnectCursorFromCheckpoint,
  type TimelineCheckpointStreamIdentity,
  useFactoryTimelineStore,
} from "../../timeline/public";
import { useDashboardSession } from "../session/dashboard-session-provider";
import {
  recordSessionPersistenceInvalidation,
  silentReplayRecoveryDiagnostic,
} from "../lib/session-persistence/diagnostics";
import { useDashboardStreamStore } from "../state/dashboardStreamStore";
import { useFactoryEventStream } from "./event-stream/useFactoryEventStream";
import { useGuardedTimelineCheckpointBootstrap } from "./snapshot/useDashboardSnapshot.bootstrap";
import { useDashboardSessionLifecycle } from "./useDashboardSessionLifecycle";
import { useDashboardTimelineMemoryDebug } from "./useDashboardTimelineMemoryDebug";
import { useDashboardWorldView } from "./useDashboardWorldView";
import { readFactoryTimelineDebugOptions } from "../../timeline/public";

export interface UseDashboardSnapshotOptions {
  locale?: string | null;
  refreshToken?: number;
}

type DashboardPreflightStatus = "loading" | "non-recoverable" | "success";

function usePersistedTimelineCheckpoint({
  checkpoint,
  checkpointsDisabled,
  streamIdentity,
}: {
  checkpoint: FactoryTimelineCheckpoint | undefined;
  checkpointsDisabled: boolean;
  streamIdentity: TimelineCheckpointStreamIdentity | null;
}) {
  useEffect(() => {
    if (typeof window === "undefined" || checkpointsDisabled) {
      return;
    }
    const persistHandle = window.setTimeout(() => {
      void persistTimelineCheckpoint(
        window.indexedDB,
        checkpoint,
        streamIdentity,
      );
    }, 750);
    return () => {
      window.clearTimeout(persistHandle);
    };
  }, [checkpoint, checkpointsDisabled, streamIdentity]);
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: snapshot composition keeps preflight, checkpoint hydration, and stream wiring in one hook.
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
  const resetTimeline = useFactoryTimelineStore((state) => state.reset);
  const setResolvedStreamIdentity = useDashboardStreamStore(
    (state) => state.setResolvedStreamIdentity,
  );
  const setBackendRuntimeCacheScope = useDashboardStreamStore(
    (state) => state.setBackendRuntimeCacheScope,
  );
  const { error, isInitialLoading, snapshot, streamState } =
    useDashboardWorldView();
  const { isPaused, rawSessionID } = useDashboardSession();
  const debugOptions = useMemo(() => readFactoryTimelineDebugOptions(), []);
  const checkpointHydrationKey = useMemo(
    () => (rawSessionID == null ? null : `${rawSessionID}::${refreshToken}`),
    [rawSessionID, refreshToken],
  );
  const invalidatedReconnectCursorRef = useRef(false);
  const lastPersistedCheckpointRef =
    useRef<FactoryTimelineCheckpoint | null>(null);
  const lastSessionKeyRef = useRef<string | null>(null);
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
    preflightError,
    preflightRecovery,
    persistedCheckpoint,
    preflightReady,
    streamIdentity,
  } = useGuardedTimelineCheckpointBootstrap({
    checkpointHydrationKey,
    checkpointsDisabled: debugOptions.disableTimelineCheckpoint,
    rawSessionID,
    refreshToken,
    restoreCheckpoint,
    setBackendRuntimeCacheScope,
    setResolvedStreamIdentity,
  });

  if (
    lastPersistedCheckpointRef.current !== persistedCheckpoint ||
    lastSessionKeyRef.current !== checkpointHydrationKey
  ) {
    invalidatedReconnectCursorRef.current = false;
    lastPersistedCheckpointRef.current = persistedCheckpoint;
    lastSessionKeyRef.current = checkpointHydrationKey;
  }

  const initialReconnectCursor = useMemo(
    () =>
      invalidatedReconnectCursorRef.current
        ? undefined
        : reconnectCursorFromCheckpoint(persistedCheckpoint),
    [persistedCheckpoint],
  );

  const handleInvalidReconnectCursor = useCallback(() => {
    invalidatedReconnectCursorRef.current = true;
    if (streamIdentity && rawSessionID) {
      recordSessionPersistenceInvalidation(
        silentReplayRecoveryDiagnostic(
          {
            backendScopeID: streamIdentity.backendScopeID,
            factorySessionID: streamIdentity.factorySessionID,
            streamGenerationID: streamIdentity.streamGenerationID,
          },
          rawSessionID,
        ),
      );
    }
    resetTimeline();
    void clearTimelineCheckpoint(window.indexedDB, streamIdentity);
  }, [rawSessionID, resetTimeline, streamIdentity]);

  usePersistedTimelineCheckpoint({
    checkpoint: currentReplayCheckpoint,
    checkpointsDisabled: debugOptions.disableTimelineCheckpoint,
    streamIdentity,
  });

  const handleStreamEvent = useCallback((event: FactoryEvent) => {
    queuedAppendRef.current([event]);
  }, []);

  const handleStreamEvents = useCallback((events: FactoryEvent[]) => {
    queuedAppendRef.current(events);
  }, []);

  useFactoryEventStream({
    enabled:
      checkpointHydrated &&
      preflightReady &&
      rawSessionID != null &&
      !isPaused &&
      streamIdentity != null,
    initialReconnectCursor,
    locale,
    onEvent: handleStreamEvent,
    onEvents: handleStreamEvents,
    onInvalidReconnectCursor: handleInvalidReconnectCursor,
    refreshToken,
    sessionID: rawSessionID,
    streamIdentity,
  });

  useDashboardTimelineMemoryDebug({ debugOptions, eventCount });

  const preflightStatus: DashboardPreflightStatus = preflightRecovery
    ? "non-recoverable"
    : preflightError != null
      ? "non-recoverable"
      : checkpointHydrated && preflightReady
        ? "success"
        : "loading";

  return useMemo(
    () => ({
      error: preflightRecovery ? null : (preflightError ?? error),
      isInitialLoading:
        preflightRecovery == null &&
        preflightError == null &&
        (!checkpointHydrated || !preflightReady || isInitialLoading),
      preflightRecovery,
      preflightStatus,
      snapshot: preflightRecovery ? null : snapshot,
      streamState,
    }),
    [
      checkpointHydrated,
      error,
      isInitialLoading,
      preflightError,
      preflightRecovery,
      preflightReady,
      preflightStatus,
      snapshot,
      streamState,
    ],
  );
}
