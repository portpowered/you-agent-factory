import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import type { FactoryEvent } from "../../../api/events";
import { getFactorySession } from "../../../api/factory-sessions";
import {
  clearTimelineCheckpoint,
  type FactoryTimelineCheckpoint,
  persistTimelineCheckpoint,
  readFactoryTimelineDebugOptions,
  readTimelineCheckpoint,
  reconnectCursorFromCheckpoint,
  type TimelineCheckpointStreamIdentity,
  useFactoryTimelineStore,
} from "../../timeline/public";
import { useDashboardSession } from "../session/dashboard-session-provider";
import { useDashboardStreamStore } from "../state/dashboardStreamStore";
import { useFactoryEventStream } from "./event-stream/useFactoryEventStream";
import { useDashboardSessionLifecycle } from "./useDashboardSessionLifecycle";
import { useDashboardTimelineMemoryDebug } from "./useDashboardTimelineMemoryDebug";
import { useDashboardWorldView } from "./useDashboardWorldView";

export interface UseDashboardSnapshotOptions {
  locale?: string | null;
  refreshToken?: number;
}

interface DashboardSessionRecoveryState {
  reasonCode: string;
  requestedSessionId: string;
}

function useGuardedTimelineCheckpointBootstrap({
  checkpointsDisabled,
  rawSessionID,
  restoreCheckpoint,
}: {
  checkpointsDisabled: boolean;
  rawSessionID: string | null;
  restoreCheckpoint: (checkpoint: FactoryTimelineCheckpoint) => void;
}) {
  const setStreamState = useDashboardStreamStore((state) => state.setStreamState);
  const [checkpointHydratedSessionID, setCheckpointHydratedSessionID] =
    useState<string | null>(null);
  const [persistedCheckpoint, setPersistedCheckpoint] =
    useState<FactoryTimelineCheckpoint | null>(null);
  const [preflightReadySessionID, setPreflightReadySessionID] =
    useState<string | null>(null);
  const [streamIdentity, setStreamIdentity] =
    useState<TimelineCheckpointStreamIdentity | null>(null);

  const checkpointHydrated = checkpointHydratedSessionID === rawSessionID;
  const preflightReady = preflightReadySessionID === rawSessionID;

  useEffect(() => {
    let cancelled = false;

    setCheckpointHydratedSessionID(null);
    setPersistedCheckpoint(null);
    setPreflightReadySessionID(null);
    setStreamIdentity(null);

    if (
      !rawSessionID ||
      typeof window === "undefined" ||
      checkpointsDisabled
    ) {
      setPreflightReadySessionID(rawSessionID);
      setCheckpointHydratedSessionID(rawSessionID);
      return;
    }

    void getFactorySession(rawSessionID)
      .then(async (response) => {
        if (cancelled) {
          return;
        }
        const checkpointStreamIdentity = streamIdentityFromSessionResponse(
          response.session,
        );
        setStreamIdentity(checkpointStreamIdentity);
        setPreflightReadySessionID(rawSessionID);
        const checkpoint = await readTimelineCheckpoint(
          window.indexedDB,
          rawSessionID,
          checkpointStreamIdentity,
        );
        if (cancelled) {
          return;
        }
        if (checkpoint) {
          restoreCheckpoint(checkpoint);
        }
        setPersistedCheckpoint(checkpoint);
        setCheckpointHydratedSessionID(rawSessionID);
      })
      .catch((preflightError: unknown) => {
        if (cancelled) {
          return;
        }
        const message =
          preflightError instanceof Error && preflightError.message.trim() !== ""
            ? preflightError.message
            : "The dashboard could not load the selected session.";
        setStreamState({
          message,
          status: "offline",
        });
        setCheckpointHydratedSessionID(rawSessionID);
      });

    return () => {
      cancelled = true;
    };
  }, [checkpointsDisabled, rawSessionID, restoreCheckpoint, setStreamState]);

  return {
    checkpointHydrated,
    persistedCheckpoint,
    preflightReady,
    streamIdentity,
  };
}

function usePersistedTimelineCheckpoint({
  checkpoint,
  checkpointsDisabled,
  rawSessionID,
  streamIdentity,
}: {
  checkpoint: FactoryTimelineCheckpoint | undefined;
  checkpointsDisabled: boolean;
  rawSessionID: string | null;
  streamIdentity: TimelineCheckpointStreamIdentity | null;
}) {
  useEffect(() => {
    if (typeof window === "undefined" || checkpointsDisabled) {
      return;
    }
    const persistHandle = window.setTimeout(() => {
      void persistTimelineCheckpoint(
        window.indexedDB,
        rawSessionID,
        checkpoint,
        streamIdentity,
      );
    }, 750);
    return () => {
      window.clearTimeout(persistHandle);
    };
  }, [checkpoint, checkpointsDisabled, rawSessionID, streamIdentity]);
}

function streamIdentityFromSessionResponse(session: {
  id: string;
  runtime?: {
    lifecycle?: {
      startedAt?: string;
    };
    streamIdentity?: {
      backendScopeID?: string;
      factorySessionID?: string;
      streamGenerationID?: string;
    };
  };
}): TimelineCheckpointStreamIdentity | null {
  const identity = session.runtime?.streamIdentity;
  if (
    typeof identity?.backendScopeID !== "string" ||
    typeof identity.factorySessionID !== "string" ||
    typeof identity.streamGenerationID !== "string"
  ) {
    return null;
  }
  return {
    backendScopeID: identity.backendScopeID,
    factorySessionID: identity.factorySessionID,
    streamGenerationID: identity.streamGenerationID,
  };
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
  const resetTimeline = useFactoryTimelineStore((state) => state.reset);
  const { error, isInitialLoading, snapshot, streamState } =
    useDashboardWorldView();
  const { isPaused, rawSessionID } = useDashboardSession();
  const debugOptions = useMemo(() => readFactoryTimelineDebugOptions(), []);
  const invalidatedReconnectCursorRef = useRef(false);
  const lastPersistedCheckpointRef =
    useRef<FactoryTimelineCheckpoint | null>(null);
  const lastSessionIDRef = useRef<string | null>(null);
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
    persistedCheckpoint,
    preflightReady,
    streamIdentity,
  } = useGuardedTimelineCheckpointBootstrap({
    checkpointsDisabled: debugOptions.disableTimelineCheckpoint,
    rawSessionID,
    restoreCheckpoint,
  });
  if (
    lastPersistedCheckpointRef.current !== persistedCheckpoint ||
    lastSessionIDRef.current !== rawSessionID
  ) {
    invalidatedReconnectCursorRef.current = false;
    lastPersistedCheckpointRef.current = persistedCheckpoint;
    lastSessionIDRef.current = rawSessionID;
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
    rawSessionID,
    streamIdentity,
  });

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
      preflightReady &&
      rawSessionID != null &&
      !isPaused,
    initialReconnectCursor,
    locale,
    onEvent: handleStreamEvent,
    onEvents: handleStreamEvents,
    onInvalidReconnectCursor: handleInvalidReconnectCursor,
    refreshToken,
    sessionID: rawSessionID,
  });

  useDashboardTimelineMemoryDebug({
    debugOptions,
    eventCount,
  });

  return useMemo(
    () => ({
      preflightRecovery:
        null as DashboardSessionRecoveryState | null,
      snapshot,
      streamState,
      isInitialLoading,
      error,
    }),
    [error, snapshot, streamState, isInitialLoading],
  );
}
