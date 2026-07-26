// biome-ignore-all lint/style/noExcessiveLinesPerFile: stream lifecycle wiring stays colocated with its single owning hook.
import { useQueryClient } from "@tanstack/react-query";
import { type RefObject, useCallback, useEffect, useMemo, useRef } from "react";

import type { FactoryEvent } from "../../../../api/events";
import {
  type FactoryEventReconnectCursor,
  type FactoryEventReconnectValidationResult,
  openFactoryEventStream,
  probeFactoryEventStreamRecovery,
  validateFactoryEventReconnectCursor,
} from "../../../../api/events";
import {
  DEFAULT_FACTORY_SESSION_ID,
  isDefaultFactorySessionID,
} from "../../../../api/session-routing";
import { useFactoryTimelineStore } from "../../../timeline/public/store";
import {
  normalizeStreamDerivedCacheIdentity,
  type StreamDerivedCacheIdentity,
} from "../../../timeline/public/stream-identity";
import {
  compactFactoryEventForTimeline,
  readFactoryTimelineDebugOptions,
} from "../../../timeline/state/factoryTimelineDebug";
import {
  clearQueuedFlush,
  pausedDashboardStreamState,
  prepareDashboardStreamSession,
  syncCurrentFactoryDefinition,
} from "../../lib/dashboard-event-stream";
import { dashboardSessionKey } from "../../lib/dashboard-session-lifecycle";
import { useDashboardStreamStore } from "../../state/dashboardStreamStore";
import {
  hasReconnectCursor,
  reconnectAfterStreamError,
  recordCursorFreeReplayFallbackDiagnostic,
  recordStaleCursorDiagnostic,
  recoveryFailedStreamState,
  useDashboardStreamConnectionRefs,
} from "./useFactoryEventStream.recovery";

export interface UseFactoryEventStreamOptions {
  enabled: boolean;
  initialReconnectCursor?: FactoryEventReconnectCursor;
  initialCursorFreeReplayCorrelationToken?: string | null;
  locale?: string | null;
  onEvent: (event: FactoryEvent) => void;
  onEvents?: (events: FactoryEvent[]) => void;
  onInvalidReconnectCursor?: () => void;
  openStream?: typeof openFactoryEventStream;
  probeRecovery?: typeof probeFactoryEventStreamRecovery;
  refreshToken?: number;
  sessionID: string | null;
  streamIdentity?: StreamDerivedCacheIdentity | null;
  validateReconnectCursor?: (
    sessionID?: string | null,
    reconnect?: FactoryEventReconnectCursor,
  ) => Promise<FactoryEventReconnectValidationResult>;
}

interface DashboardStreamConnectionOptions {
  debugOptions: ReturnType<typeof readFactoryTimelineDebugOptions>;
  enabled: boolean;
  flushHandleRef: RefObject<number | null>;
  flushQueuedEvents: () => void;
  initialReconnectCursor?: FactoryEventReconnectCursor;
  initialCursorFreeReplayCorrelationToken?: string | null;
  locale?: string | null;
  onInvalidReconnectCursor?: () => void;
  openStream: typeof openFactoryEventStream;
  probeRecovery: typeof probeFactoryEventStreamRecovery;
  queryClient: ReturnType<typeof useQueryClient>;
  queuedEventsRef: RefObject<FactoryEvent[]>;
  refreshToken: number;
  resetTimeline: () => void;
  scheduleQueuedFlush: () => void;
  sessionID: string | null;
  setStreamState: (
    streamState: ReturnType<
      typeof useDashboardStreamStore.getState
    >["streamState"],
  ) => void;
  streamIdentity: StreamDerivedCacheIdentity | null;
  streamSessionID: string;
  validateReconnectCursor: (
    sessionID?: string | null,
    reconnect?: FactoryEventReconnectCursor,
  ) => Promise<FactoryEventReconnectValidationResult>;
}

function resolveStreamSessionID(
  sessionID: string | null,
  streamIdentity?: StreamDerivedCacheIdentity | null,
): string {
  const resolvedIdentity = normalizeStreamDerivedCacheIdentity(streamIdentity);
  if (resolvedIdentity) {
    return resolvedIdentity.factorySessionID;
  }
  if (sessionID == null) {
    return DEFAULT_FACTORY_SESSION_ID;
  }
  return isDefaultFactorySessionID(sessionID)
    ? DEFAULT_FACTORY_SESSION_ID
    : sessionID;
}

function reconnectCursorFromEvent(
  event: FactoryEvent,
): FactoryEventReconnectCursor {
  return {
    afterEventId: event.id,
    afterSequence: event.context.sessionSequence ?? event.context.sequence,
  };
}

function clearReconnectTimeout(reconnectTimeoutRef: {
  current: number | null;
}) {
  if (reconnectTimeoutRef.current != null) {
    window.clearTimeout(reconnectTimeoutRef.current);
    reconnectTimeoutRef.current = null;
  }
}

function resolveStreamOpenPermission({
  enabled,
  hasOpenedStreamRef,
  previousSessionKey,
  queuedEventsRef,
  refreshToken,
  sessionID,
  setStreamState,
}: {
  enabled: boolean;
  hasOpenedStreamRef: RefObject<boolean>;
  previousSessionKey: string | null;
  queuedEventsRef: RefObject<FactoryEvent[]>;
  refreshToken: number;
  sessionID: string | null;
  setStreamState: DashboardStreamConnectionOptions["setStreamState"];
}): boolean {
  if (!enabled) {
    if (sessionID != null) {
      setStreamState(pausedDashboardStreamState());
    }
    return false;
  }
  if (sessionID == null) {
    return false;
  }
  return prepareDashboardStreamSession({
    hasOpenedStreamRef,
    previousSessionKey,
    queuedEventsRef,
    refreshToken,
    selectedSessionID: sessionID,
  });
}

function shouldRecoverFromInvalidCursor(
  validation: Exclude<FactoryEventReconnectValidationResult, { ok: true }>,
): validation is Extract<
  FactoryEventReconnectValidationResult,
  { ok: false; reason: "stale_cursor" }
> {
  return validation.reason === "stale_cursor";
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: stream connection setup keeps validation, stale-cursor recovery, and cleanup in one hook.
function useDashboardStreamConnection({
  debugOptions,
  enabled,
  flushHandleRef,
  flushQueuedEvents,
  initialReconnectCursor,
  initialCursorFreeReplayCorrelationToken,
  locale,
  onInvalidReconnectCursor,
  openStream,
  probeRecovery,
  queryClient,
  queuedEventsRef,
  refreshToken,
  resetTimeline,
  scheduleQueuedFlush,
  sessionID,
  setStreamState,
  streamIdentity,
  streamSessionID,
  validateReconnectCursor,
}: DashboardStreamConnectionOptions) {
  const refs = useDashboardStreamConnectionRefs();
  const invalidReconnectRecoveryUsedRef = useRef(false);

  // biome-ignore lint/complexity/noExcessiveLinesPerFunction: the effect owns one stream lifecycle with coordinated reconnect paths.
  useEffect(() => {
    const sessionKey = dashboardSessionKey(sessionID, refreshToken);
    const previousSessionKey = refs.lastSessionKeyRef.current;
    const sessionSelectionChanged = sessionKey !== previousSessionKey;
    refs.lastSessionKeyRef.current = sessionKey;

    if (
      !resolveStreamOpenPermission({
        enabled,
        hasOpenedStreamRef: refs.hasOpenedStreamRef,
        previousSessionKey: sessionSelectionChanged ? previousSessionKey : null,
        queuedEventsRef,
        refreshToken,
        sessionID,
        setStreamState,
      })
    ) {
      return;
    }

    const disposed = { current: false };
    const handleStreamEvent = (event: FactoryEvent) => {
      refs.cursorFreeReplayPendingRef.current = false;
      refs.staleCursorRecoveryAttemptedRef.current = false;
      invalidReconnectRecoveryUsedRef.current = false;
      refs.reconnectCursorRef.current = reconnectCursorFromEvent(event);
      syncCurrentFactoryDefinition(
        queryClient,
        event,
        streamSessionID,
        streamIdentity,
      );
      queuedEventsRef.current.push(
        compactFactoryEventForTimeline(event, debugOptions),
      );
      scheduleQueuedFlush();
    };

    const openDashboardStream = (reconnect?: FactoryEventReconnectCursor) => {
      const validationAttempt = ++refs.reconnectValidationAttemptRef.current;
      void (async () => {
        if (reconnect != null) {
          const validation = await validateReconnectCursor(
            streamSessionID,
            reconnect,
          );
          if (
            validationAttempt !== refs.reconnectValidationAttemptRef.current
          ) {
            return;
          }
          if (!validation.ok) {
            refs.reconnectCursorRef.current = undefined;
            if (
              shouldRecoverFromInvalidCursor(validation) &&
              !invalidReconnectRecoveryUsedRef.current
            ) {
              invalidReconnectRecoveryUsedRef.current = true;
              const correlationToken = recordStaleCursorDiagnostic(
                streamIdentity,
                streamSessionID,
              );
              refs.cursorFreeReplayPendingRef.current =
                correlationToken != null;
              refs.cursorFreeReplayCorrelationTokenRef.current =
                correlationToken;
              onInvalidReconnectCursor?.();
              openDashboardStream(undefined);
              return;
            }
            setStreamState({
              message: validation.message,
              status: "offline",
            });
            return;
          }
        }

        if (disposed.current) {
          return;
        }
        refs.streamRef.current?.close();
        const stream = openStream(
          handleStreamEvent,
          (status, message) => setStreamState({ status, message }),
          streamSessionID,
          reconnect,
        );
        refs.streamRef.current = stream;
        if (!stream) {
          refs.cursorFreeReplayPendingRef.current = false;
          refs.cursorFreeReplayCorrelationTokenRef.current = null;
          return;
        }
        const replayCorrelationToken =
          refs.cursorFreeReplayCorrelationTokenRef.current;
        if (reconnect == null && replayCorrelationToken) {
          recordCursorFreeReplayFallbackDiagnostic(replayCorrelationToken);
          refs.cursorFreeReplayCorrelationTokenRef.current = null;
        }
        const previousOnOpen = stream.onopen;
        stream.onopen = (openEvent) => {
          refs.cursorFreeReplayPendingRef.current = false;
          previousOnOpen?.call(stream, openEvent);
        };
        const previousOnError = stream.onerror;
        stream.onerror = (errorEvent) => {
          previousOnError?.call(stream, errorEvent);
          if (refs.cursorFreeReplayPendingRef.current) {
            refs.cursorFreeReplayPendingRef.current = false;
            refs.recoveringRef.current = false;
            setStreamState(recoveryFailedStreamState(locale));
            return;
          }
          const cursor = refs.reconnectCursorRef.current;
          if (
            refs.recoveringRef.current ||
            refs.reconnectTimeoutRef.current != null ||
            !hasReconnectCursor(cursor)
          ) {
            return;
          }
          void reconnectAfterStreamError({
            cursor,
            disposed,
            locale,
            onInvalidReconnectCursor,
            openDashboardStream,
            probeRecovery,
            queryClient,
            queuedEventsRef,
            refs,
            resetTimeline,
            setStreamState,
            streamIdentity,
            streamSessionID,
          });
        };
      })();
    };

    refs.staleCursorRecoveryAttemptedRef.current = false;
    refs.cursorFreeReplayPendingRef.current = false;
    refs.cursorFreeReplayCorrelationTokenRef.current =
      initialCursorFreeReplayCorrelationToken ?? null;
    refs.cursorFreeReplayPendingRef.current =
      initialReconnectCursor == null &&
      refs.cursorFreeReplayCorrelationTokenRef.current != null;
    refs.reconnectCursorRef.current = initialReconnectCursor;
    invalidReconnectRecoveryUsedRef.current = false;
    openDashboardStream(initialReconnectCursor);
    return () => {
      disposed.current = true;
      refs.reconnectValidationAttemptRef.current += 1;
      clearReconnectTimeout(refs.reconnectTimeoutRef);
      clearQueuedFlush(flushHandleRef);
      flushQueuedEvents();
      refs.streamRef.current?.close();
      refs.streamRef.current = null;
    };
  }, [
    debugOptions,
    enabled,
    flushHandleRef,
    flushQueuedEvents,
    initialReconnectCursor,
    initialCursorFreeReplayCorrelationToken,
    locale,
    onInvalidReconnectCursor,
    openStream,
    probeRecovery,
    queryClient,
    queuedEventsRef,
    refreshToken,
    refs,
    resetTimeline,
    scheduleQueuedFlush,
    sessionID,
    setStreamState,
    streamIdentity,
    streamSessionID,
    validateReconnectCursor,
  ]);
}

export function useFactoryEventStream({
  enabled,
  initialReconnectCursor,
  initialCursorFreeReplayCorrelationToken,
  locale,
  onEvent,
  onEvents,
  onInvalidReconnectCursor,
  openStream = openFactoryEventStream,
  probeRecovery = probeFactoryEventStreamRecovery,
  refreshToken = 0,
  sessionID,
  streamIdentity = null,
  validateReconnectCursor = validateFactoryEventReconnectCursor,
}: UseFactoryEventStreamOptions) {
  const queryClient = useQueryClient();
  const resetAllTimelines = useFactoryTimelineStore((state) => state.reset);
  const resetTimelineEntry = useFactoryTimelineStore(
    (state) => state.resetEntry,
  );
  const resetTimeline = useCallback(() => {
    if (streamIdentity) {
      resetTimelineEntry(streamIdentity);
      return;
    }
    resetAllTimelines();
  }, [resetAllTimelines, resetTimelineEntry, streamIdentity]);
  const setStreamState = useDashboardStreamStore(
    (state) => state.setStreamState,
  );
  const queuedEventsRef = useRef<FactoryEvent[]>([]);
  const flushHandleRef = useRef<number | null>(null);
  const debugOptions = useMemo(() => readFactoryTimelineDebugOptions(), []);
  const streamSessionID = useMemo(
    () => resolveStreamSessionID(sessionID, streamIdentity),
    [sessionID, streamIdentity],
  );

  const flushQueuedEvents = useCallback(() => {
    flushHandleRef.current = null;
    if (queuedEventsRef.current.length === 0) {
      return;
    }
    const events = queuedEventsRef.current;
    queuedEventsRef.current = [];
    if (onEvents) {
      onEvents(events);
      return;
    }
    for (const event of events) {
      onEvent(event);
    }
  }, [onEvent, onEvents]);

  const scheduleQueuedFlush = useCallback(() => {
    if (flushHandleRef.current !== null) {
      return;
    }
    if (typeof window.requestAnimationFrame === "function") {
      flushHandleRef.current = window.requestAnimationFrame(() => {
        flushQueuedEvents();
      });
      return;
    }
    flushHandleRef.current = window.setTimeout(() => {
      flushQueuedEvents();
    }, 16);
  }, [flushQueuedEvents]);

  useEffect(() => {
    return () => {
      clearQueuedFlush(flushHandleRef);
    };
  }, []);

  useDashboardStreamConnection({
    debugOptions,
    enabled,
    flushHandleRef,
    flushQueuedEvents,
    initialReconnectCursor,
    initialCursorFreeReplayCorrelationToken,
    locale,
    onInvalidReconnectCursor,
    openStream,
    probeRecovery,
    queryClient,
    queuedEventsRef,
    refreshToken,
    resetTimeline,
    scheduleQueuedFlush,
    sessionID,
    setStreamState,
    streamIdentity,
    streamSessionID,
    validateReconnectCursor,
  });
}
