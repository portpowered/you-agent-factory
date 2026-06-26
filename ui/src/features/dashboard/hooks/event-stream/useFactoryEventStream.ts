import { useQueryClient } from "@tanstack/react-query";
import { type RefObject, useCallback, useEffect, useMemo, useRef } from "react";

import type { FactoryEvent } from "../../../../api/events";
import {
  type FactoryEventReconnectCursor,
  openFactoryEventStream,
  probeFactoryEventStreamRecovery,
} from "../../../../api/events";
import {
  DEFAULT_FACTORY_SESSION_ID,
  isDefaultFactorySessionID,
} from "../../../../api/session-routing";
import { useFactoryTimelineStore } from "../../../timeline/public";
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
import {
  dashboardSessionKey,
} from "../../lib/dashboard-session-lifecycle";
import { useDashboardStreamStore } from "../../state/dashboardStreamStore";
import {
  type DashboardStreamConnectionRefs,
  hasReconnectCursor,
  reconnectAfterStreamError,
  useDashboardStreamConnectionRefs,
} from "./useFactoryEventStream.recovery";

export interface UseFactoryEventStreamOptions {
  enabled: boolean;
  initialReconnectCursor?: FactoryEventReconnectCursor;
  locale?: string | null;
  onEvent: (event: FactoryEvent) => void;
  onEvents?: (events: FactoryEvent[]) => void;
  openStream?: typeof openFactoryEventStream;
  probeRecovery?: typeof probeFactoryEventStreamRecovery;
  refreshToken?: number;
  sessionID: string | null;
}

interface DashboardStreamConnectionOptions {
  debugOptions: ReturnType<typeof readFactoryTimelineDebugOptions>;
  enabled: boolean;
  flushHandleRef: RefObject<number | null>;
  initialReconnectCursor?: FactoryEventReconnectCursor;
  flushQueuedEvents: () => void;
  locale?: string | null;
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
  streamSessionID: string;
}

function resolveStreamSessionID(sessionID: string | null): string {
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

function useDashboardStreamConnection({
  debugOptions,
  enabled,
  flushHandleRef,
  initialReconnectCursor,
  flushQueuedEvents,
  locale,
  openStream,
  probeRecovery,
  queryClient,
  queuedEventsRef,
  refreshToken,
  resetTimeline,
  scheduleQueuedFlush,
  sessionID,
  setStreamState,
  streamSessionID,
}: DashboardStreamConnectionOptions) {
  const refs = useDashboardStreamConnectionRefs();

  useDashboardStreamConnectionEffect({
    debugOptions,
    enabled,
    flushHandleRef,
    flushQueuedEvents,
    initialReconnectCursor,
    locale,
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
    streamSessionID,
  });
}

function useDashboardStreamConnectionEffect({
  debugOptions,
  enabled,
  flushHandleRef,
  flushQueuedEvents,
  initialReconnectCursor,
  locale,
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
  streamSessionID,
}: DashboardStreamConnectionOptions & {
  refs: DashboardStreamConnectionRefs;
}) {
  useEffect(() => {
    const sessionKey = dashboardSessionKey(sessionID, refreshToken);
    const previousSessionKey = refs.lastSessionKeyRef.current;
    const sessionSelectionChanged = sessionKey !== previousSessionKey;
    refs.lastSessionKeyRef.current = sessionKey;

    if (!sessionSelectionChanged && !enabled && sessionID != null) {
      setStreamState(pausedDashboardStreamState());
      return;
    }
    if (!sessionSelectionChanged && (!enabled || sessionID == null)) {
      return;
    }

    const shouldOpenStream = prepareDashboardStreamSession({
      hasOpenedStreamRef: refs.hasOpenedStreamRef,
      previousSessionKey: sessionSelectionChanged ? previousSessionKey : null,
      queuedEventsRef,
      refreshToken,
      selectedSessionID: sessionID,
    });
    if (!shouldOpenStream || sessionID == null || !enabled) {
      if (!enabled && sessionID != null) {
        setStreamState(pausedDashboardStreamState());
      }
      return;
    }

    const disposed = { current: false };
    const clearReconnectTimeout = () => {
      if (refs.reconnectTimeoutRef.current != null) {
        window.clearTimeout(refs.reconnectTimeoutRef.current);
        refs.reconnectTimeoutRef.current = null;
      }
    };
    const handleStreamEvent = (event: FactoryEvent) => {
      refs.staleCursorRecoveryAttemptedRef.current = false;
      refs.reconnectCursorRef.current = reconnectCursorFromEvent(event);
      syncCurrentFactoryDefinition(queryClient, event, streamSessionID);
      queuedEventsRef.current.push(
        compactFactoryEventForTimeline(event, debugOptions),
      );
      scheduleQueuedFlush();
    };
    const openDashboardStream = (reconnect?: FactoryEventReconnectCursor) => {
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
        return;
      }
      const previousOnError = stream.onerror;
      stream.onerror = (errorEvent) => {
        previousOnError?.call(stream, errorEvent);
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
          openDashboardStream,
          probeRecovery,
          queryClient,
          queuedEventsRef,
          refs,
          resetTimeline,
          setStreamState,
          streamSessionID,
        });
      };
    };

    refs.staleCursorRecoveryAttemptedRef.current = false;
    refs.reconnectCursorRef.current = initialReconnectCursor;
    openDashboardStream(initialReconnectCursor);
    return () => {
      disposed.current = true;
      clearReconnectTimeout();
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
    locale,
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
    streamSessionID,
  ]);
}

export function useFactoryEventStream({
  enabled,
  initialReconnectCursor,
  locale,
  onEvent,
  onEvents,
  openStream = openFactoryEventStream,
  probeRecovery = probeFactoryEventStreamRecovery,
  refreshToken = 0,
  sessionID,
}: UseFactoryEventStreamOptions) {
  const queryClient = useQueryClient();
  const resetTimeline = useFactoryTimelineStore((state) => state.reset);
  const setStreamState = useDashboardStreamStore(
    (state) => state.setStreamState,
  );
  const queuedEventsRef = useRef<FactoryEvent[]>([]);
  const flushHandleRef = useRef<number | null>(null);
  const debugOptions = useMemo(() => readFactoryTimelineDebugOptions(), []);
  const streamSessionID = useMemo(
    () => resolveStreamSessionID(sessionID),
    [sessionID],
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
    locale,
    openStream,
    probeRecovery,
    queryClient,
    queuedEventsRef,
    refreshToken,
    resetTimeline,
    scheduleQueuedFlush,
    sessionID,
    setStreamState,
    streamSessionID,
  });
}
