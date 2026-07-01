import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import type { FactoryEvent } from "../../../api/events";
import {
  clearTimelineCheckpoint,
  type FactoryTimelineCheckpoint,
  persistTimelineCheckpoint,
  purgeLegacyTimelineCheckpoints,
  reconnectCursorFromCheckpoint,
  type TimelineCheckpointStreamIdentity,
  useFactoryTimelineStore,
} from "../../timeline/public";
import {
  useDashboardSession,
  useSetDashboardSessionID,
} from "../session/dashboard-session-provider";
import {
  bootstrapDashboardSessionSyncPreflight,
  type DashboardSessionRecoveryState,
} from "../lib/dashboard-session-sync-preflight";
import { useDashboardStreamStore } from "../state/dashboardStreamStore";
import { useFactoryEventStream } from "./event-stream/useFactoryEventStream";
import { useDashboardSessionLifecycle } from "./useDashboardSessionLifecycle";
import { useDashboardTimelineMemoryDebug } from "./useDashboardTimelineMemoryDebug";
import { useDashboardWorldView } from "./useDashboardWorldView";
import { readFactoryTimelineDebugOptions } from "../../timeline/state/factoryTimelineDebug";

export interface UseDashboardSnapshotOptions {
  locale?: string | null;
  refreshToken?: number;
}

type DashboardPreflightStatus = "loading" | "non-recoverable" | "success";

function useGuardedTimelineCheckpointBootstrap({
  checkpointHydrationKey,
  checkpointsDisabled,
  rawSessionID,
  refreshToken,
  restoreCheckpoint,
  setResolvedStreamIdentity,
}: {
  checkpointHydrationKey: string | null;
  checkpointsDisabled: boolean;
  rawSessionID: string | null;
  refreshToken: number;
  restoreCheckpoint: (checkpoint: FactoryTimelineCheckpoint) => void;
  setResolvedStreamIdentity: (
    streamIdentity: TimelineCheckpointStreamIdentity | null,
  ) => void;
}) {
  const setSelectedSessionID = useSetDashboardSessionID();
  const setStreamState = useDashboardStreamStore((state) => state.setStreamState);
  const [checkpointHydratedKey, setCheckpointHydratedKey] =
    useState<string | null>(null);
  const [persistedCheckpoint, setPersistedCheckpoint] =
    useState<FactoryTimelineCheckpoint | null>(null);
  const [preflightReadyKey, setPreflightReadyKey] = useState<string | null>(
    null,
  );
  const [preflightError, setPreflightError] = useState<Error | null>(null);
  const [preflightRecovery, setPreflightRecovery] =
    useState<DashboardSessionRecoveryState | null>(null);
  const [streamIdentity, setStreamIdentity] =
    useState<TimelineCheckpointStreamIdentity | null>(null);

  const checkpointHydrated = checkpointHydratedKey === checkpointHydrationKey;
  const preflightReady = preflightReadyKey === checkpointHydrationKey;

  useEffect(() => {
    let cancelled = false;

    setCheckpointHydratedKey(null);
    setPersistedCheckpoint(null);
    setPreflightError(null);
    setPreflightRecovery(null);
    setPreflightReadyKey(null);
    setStreamIdentity(null);
    setResolvedStreamIdentity(null);

    if (
      checkpointHydrationKey == null ||
      !rawSessionID ||
      typeof window === "undefined" ||
      checkpointsDisabled
    ) {
      setPreflightReadyKey(checkpointHydrationKey);
      setCheckpointHydratedKey(checkpointHydrationKey);
      return;
    }

    void bootstrapDashboardSessionSyncPreflight({
      indexedDB: window.indexedDB,
      refreshToken,
      sessionID: rawSessionID,
    })
      .then(async (outcome) => {
        if (cancelled) {
          return;
        }
        if (outcome.kind === "error") {
          setPreflightError(outcome.error);
          setStreamState({
            message: outcome.error.message,
            status: "offline",
          });
          setCheckpointHydratedKey(checkpointHydrationKey);
          return;
        }
        if (outcome.kind === "recovery") {
          setPreflightRecovery(outcome.recovery);
          setCheckpointHydratedKey(checkpointHydrationKey);
          return;
        }

        const {
          checkpoint,
          reconnectCursor: _reconnectCursor,
          remappedFactorySessionId,
          streamIdentity: resolvedStreamIdentity,
        } = outcome.result;
        setStreamIdentity(resolvedStreamIdentity);
        setResolvedStreamIdentity(resolvedStreamIdentity);
        if (
          remappedFactorySessionId != null &&
          remappedFactorySessionId !== rawSessionID
        ) {
          setSelectedSessionID(remappedFactorySessionId);
        }
        await purgeLegacyTimelineCheckpoints(window.indexedDB);
        setPreflightReadyKey(checkpointHydrationKey);
        if (checkpoint) {
          restoreCheckpoint(checkpoint);
        }
        setPersistedCheckpoint(checkpoint);
        setCheckpointHydratedKey(checkpointHydrationKey);
      })
      .catch((preflightError: unknown) => {
        if (cancelled) {
          return;
        }
        const message =
          preflightError instanceof Error && preflightError.message.trim() !== ""
            ? preflightError.message
            : "The dashboard could not load the selected session.";
        setPreflightError(new Error(message));
        setStreamState({
          message,
          status: "offline",
        });
        setCheckpointHydratedKey(checkpointHydrationKey);
      });

    return () => {
      cancelled = true;
    };
  }, [
    checkpointHydrationKey,
    checkpointsDisabled,
    rawSessionID,
    refreshToken,
    restoreCheckpoint,
    setResolvedStreamIdentity,
    setSelectedSessionID,
    setStreamState,
  ]);

  return {
    checkpointHydrated,
    preflightError,
    preflightRecovery,
    persistedCheckpoint,
    preflightReady,
    streamIdentity,
  };
}

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
    resetTimeline();
    void clearTimelineCheckpoint(window.indexedDB, streamIdentity);
  }, [resetTimeline, streamIdentity]);

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
