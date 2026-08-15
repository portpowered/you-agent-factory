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
  sessionAliasID?: string | null;
  sessionID: string | null;
  streamIdentity?: StreamDerivedCacheIdentity | null;
  validateReconnectCursor?: (
    sessionID?: string | null,
    reconnect?: FactoryEventReconnectCursor,
  ) => Promise<FactoryEventReconnectValidationResult>;
}

interface DashboardStreamConnectionOptions {
  isCurrentStreamTarget: (targetKey: string) => boolean;
  debugOptions: ReturnType<typeof readFactoryTimelineDebugOptions>;
  enabled: boolean;
  flushHandleRef: RefObject<number | null>;
  flushQueuedEvents: (expectedTargetKey?: string) => void;
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
  scheduleQueuedFlush: (targetKey: string) => void;
  sessionID: string | null;
  setStreamState: (
    streamState: ReturnType<
      typeof useDashboardStreamStore.getState
    >["streamState"],
  ) => void;
  streamIdentity: StreamDerivedCacheIdentity | null;
  streamSessionID: string;
  streamTargetKey: string;
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

function streamIdentityBelongsToSession(
  streamIdentity: StreamDerivedCacheIdentity | null,
  streamSessionID: string,
  sessionAliasID: string | null,
): boolean {
  if (!streamIdentity || streamIdentity.factorySessionID !== streamSessionID) {
    return streamIdentity == null;
  }
  const normalizedAlias = sessionAliasID?.trim() ?? "";
  return (
    normalizedAlias.length === 0 ||
    normalizedAlias === streamSessionID ||
    isDefaultFactorySessionID(normalizedAlias)
  );
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
  isCurrentStreamTarget,
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
  streamTargetKey,
  validateReconnectCursor,
}: DashboardStreamConnectionOptions) {
  const refs = useDashboardStreamConnectionRefs();
  const invalidReconnectRecoveryUsedRef = useRef(false);

  // biome-ignore lint/complexity/noExcessiveLinesPerFunction: the effect owns one stream lifecycle with coordinated reconnect paths.
  useEffect(() => {
    const capturedStreamTargetKey = streamTargetKey;
    const disposed = { current: false };
    const isActive = () =>
      !disposed.current && isCurrentStreamTarget(capturedStreamTargetKey);
    const applyStreamState = (
      streamState: Parameters<
        DashboardStreamConnectionOptions["setStreamState"]
      >[0],
    ) => {
      if (isActive()) {
        setStreamState(streamState);
      }
    };
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
        setStreamState: applyStreamState,
      })
    ) {
      return;
    }

    const handleStreamEvent = (event: FactoryEvent) => {
      if (!isActive()) {
        return;
      }
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
      scheduleQueuedFlush(capturedStreamTargetKey);
    };

    const openDashboardStream = (reconnect?: FactoryEventReconnectCursor) => {
      if (!isActive()) {
        return;
      }
      const validationAttempt = ++refs.reconnectValidationAttemptRef.current;
      void (async () => {
        if (reconnect != null) {
          const validation = await validateReconnectCursor(
            streamSessionID,
            reconnect,
          );
          if (
            !isActive() ||
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
            applyStreamState({
              message: validation.message,
              status: "offline",
            });
            return;
          }
        }

        if (!isActive()) {
          return;
        }
        refs.streamRef.current?.close();
        const stream = openStream(
          handleStreamEvent,
          (status, message) => applyStreamState({ status, message }),
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
          if (!isActive()) {
            return;
          }
          refs.cursorFreeReplayPendingRef.current = false;
          previousOnOpen?.call(stream, openEvent);
        };
        const previousOnError = stream.onerror;
        stream.onerror = (errorEvent) => {
          if (!isActive()) {
            return;
          }
          previousOnError?.call(stream, errorEvent);
          if (refs.cursorFreeReplayPendingRef.current) {
            refs.cursorFreeReplayPendingRef.current = false;
            refs.recoveringRef.current = false;
            applyStreamState(recoveryFailedStreamState(locale));
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
            setStreamState: applyStreamState,
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
      const targetRemainsCurrent = isCurrentStreamTarget(
        capturedStreamTargetKey,
      );
      disposed.current = true;
      refs.reconnectValidationAttemptRef.current += 1;
      clearReconnectTimeout(refs.reconnectTimeoutRef);
      clearQueuedFlush(flushHandleRef);
      if (targetRemainsCurrent) {
        flushQueuedEvents(capturedStreamTargetKey);
      } else {
        queuedEventsRef.current = [];
      }
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
    isCurrentStreamTarget,
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
    streamTargetKey,
    validateReconnectCursor,
  ]);
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: stream target identity and queued delivery wiring stay together at the hook boundary.
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
  sessionAliasID,
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
  const setSessionStreamState = useDashboardStreamStore(
    (state) => state.setSessionStreamState,
  );
  const queuedEventsRef = useRef<FactoryEvent[]>([]);
  const flushHandleRef = useRef<number | null>(null);
  const debugOptions = useMemo(() => readFactoryTimelineDebugOptions(), []);
  const streamSessionID = useMemo(
    () => resolveStreamSessionID(sessionID, streamIdentity),
    [sessionID, streamIdentity],
  );
  const streamTargetKey = useMemo(
    () =>
      JSON.stringify([
        sessionAliasID?.trim() ?? "",
        streamSessionID,
        refreshToken,
        streamIdentity
          ? [
              streamIdentity.backendScopeID,
              streamIdentity.factorySessionID,
              streamIdentity.logicalSessionKeyID,
              streamIdentity.streamGenerationID,
            ]
          : null,
      ]),
    [refreshToken, sessionAliasID, streamIdentity, streamSessionID],
  );
  const streamTargetKeyRef = useRef(streamTargetKey);
  streamTargetKeyRef.current = streamTargetKey;
  const isCurrentStreamTarget = useCallback(
    (targetKey: string) => streamTargetKeyRef.current === targetKey,
    [],
  );
  const setStreamState = useCallback(
    (
      streamState: ReturnType<
        typeof useDashboardStreamStore.getState
      >["streamState"],
    ) => {
      const streamStateSessionID = sessionAliasID ?? streamSessionID;
      const ownedStreamIdentity = streamIdentityBelongsToSession(
        streamIdentity,
        streamSessionID,
        sessionAliasID ?? null,
      )
        ? streamIdentity
        : null;
      setSessionStreamState(
        streamStateSessionID,
        ownedStreamIdentity,
        streamState,
        sessionAliasID && sessionAliasID !== streamStateSessionID
          ? [sessionAliasID]
          : undefined,
      );
    },
    [sessionAliasID, setSessionStreamState, streamIdentity, streamSessionID],
  );

  const flushQueuedEvents = useCallback(
    (expectedTargetKey?: string) => {
      if (
        expectedTargetKey != null &&
        !isCurrentStreamTarget(expectedTargetKey)
      ) {
        return;
      }
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
    },
    [isCurrentStreamTarget, onEvent, onEvents],
  );

  const scheduleQueuedFlush = useCallback(
    (expectedTargetKey: string) => {
      if (!isCurrentStreamTarget(expectedTargetKey)) {
        return;
      }
      if (flushHandleRef.current !== null) {
        return;
      }
      if (typeof window.requestAnimationFrame === "function") {
        flushHandleRef.current = window.requestAnimationFrame(() => {
          flushQueuedEvents(expectedTargetKey);
        });
        return;
      }
      flushHandleRef.current = window.setTimeout(() => {
        flushQueuedEvents(expectedTargetKey);
      }, 16);
    },
    [flushQueuedEvents, isCurrentStreamTarget],
  );

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
    isCurrentStreamTarget,
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
    streamTargetKey,
    validateReconnectCursor,
  });
}
