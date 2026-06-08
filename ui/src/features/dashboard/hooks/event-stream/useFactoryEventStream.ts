import { useQueryClient } from "@tanstack/react-query";
import { type RefObject, useCallback, useEffect, useMemo, useRef } from "react";

import type { FactoryEvent } from "../../../../api/events";
import {
  type FactoryEventReconnectCursor,
  openFactoryEventStream,
} from "../../../../api/events";
import {
  DEFAULT_FACTORY_SESSION_ID,
  isDefaultFactorySessionID,
} from "../../../../api/session-routing";
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

export interface UseFactoryEventStreamOptions {
  enabled: boolean;
  locale?: string | null;
  onEvent: (event: FactoryEvent) => void;
  openStream?: typeof openFactoryEventStream;
  refreshToken?: number;
  sessionID: string | null;
}

function resolveStreamSessionID(sessionID: string | null): string {
  if (sessionID == null) {
    return DEFAULT_FACTORY_SESSION_ID;
  }
  return isDefaultFactorySessionID(sessionID)
    ? DEFAULT_FACTORY_SESSION_ID
    : sessionID;
}

interface DashboardStreamConnectionOptions {
  debugOptions: ReturnType<typeof readFactoryTimelineDebugOptions>;
  enabled: boolean;
  flushHandleRef: RefObject<number | null>;
  flushQueuedEvents: () => void;
  openStream: typeof openFactoryEventStream;
  queryClient: ReturnType<typeof useQueryClient>;
  queuedEventsRef: RefObject<FactoryEvent[]>;
  refreshToken: number;
  scheduleQueuedFlush: () => void;
  sessionID: string | null;
  setStreamState: (
    streamState: ReturnType<
      typeof useDashboardStreamStore.getState
    >["streamState"],
  ) => void;
  streamSessionID: string;
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
  flushQueuedEvents,
  openStream,
  queryClient,
  queuedEventsRef,
  refreshToken,
  scheduleQueuedFlush,
  sessionID,
  setStreamState,
  streamSessionID,
}: DashboardStreamConnectionOptions) {
  const hasOpenedStreamRef = useRef(false);
  const lastSessionKeyRef = useRef<string | null>(null);
  const reconnectCursorRef = useRef<FactoryEventReconnectCursor | undefined>(
    undefined,
  );
  const reconnectTimeoutRef = useRef<number | null>(null);
  const streamRef = useRef<ReturnType<typeof openFactoryEventStream>>(null);

  useEffect(() => {
    const sessionKey = dashboardSessionKey(sessionID, refreshToken);
    const previousSessionKey = lastSessionKeyRef.current;
    const sessionSelectionChanged = sessionKey !== previousSessionKey;
    lastSessionKeyRef.current = sessionKey;

    if (!sessionSelectionChanged && !enabled && sessionID != null) {
      setStreamState(pausedDashboardStreamState());
      return;
    }

    if (!sessionSelectionChanged && !enabled && sessionID == null) {
      return;
    }

    const shouldOpenStream = prepareDashboardStreamSession({
      hasOpenedStreamRef,
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

    const clearReconnectTimeout = () => {
      if (reconnectTimeoutRef.current != null) {
        window.clearTimeout(reconnectTimeoutRef.current);
        reconnectTimeoutRef.current = null;
      }
    };

    const handleStreamEvent = (event: FactoryEvent) => {
      reconnectCursorRef.current = reconnectCursorFromEvent(event);
      syncCurrentFactoryDefinition(queryClient, event, streamSessionID);
      queuedEventsRef.current.push(
        compactFactoryEventForTimeline(event, debugOptions),
      );
      scheduleQueuedFlush();
    };

    const openDashboardStream = (
      reconnect?: FactoryEventReconnectCursor,
    ) => {
      streamRef.current?.close();
      const stream = openStream(
        handleStreamEvent,
        (status, message) => {
          setStreamState({ status, message });
        },
        streamSessionID,
        reconnect,
      );
      streamRef.current = stream;
      if (!stream) {
        return;
      }
      const previousOnError = stream.onerror;
      stream.onerror = (errorEvent) => {
        previousOnError?.call(stream, errorEvent);
        if (reconnectTimeoutRef.current != null) {
          return;
        }
        const cursor = reconnectCursorRef.current;
        if (!cursor?.afterEventId && cursor?.afterSequence == null) {
          return;
        }
        setStreamState({
          message: "Reconnecting to factory events...",
          status: "reconnecting",
        });
        reconnectTimeoutRef.current = window.setTimeout(() => {
          reconnectTimeoutRef.current = null;
          openDashboardStream(cursor);
        }, 1000);
      };
    };

    reconnectCursorRef.current = undefined;
    openDashboardStream();
    return () => {
      clearReconnectTimeout();
      clearQueuedFlush(flushHandleRef);
      flushQueuedEvents();
      streamRef.current?.close();
      streamRef.current = null;
    };
  }, [
    debugOptions,
    enabled,
    flushHandleRef,
    flushQueuedEvents,
    openStream,
    queryClient,
    queuedEventsRef,
    refreshToken,
    scheduleQueuedFlush,
    sessionID,
    setStreamState,
    streamSessionID,
  ]);
}

export function useFactoryEventStream({
  enabled,
  locale: _locale,
  onEvent,
  openStream = openFactoryEventStream,
  refreshToken = 0,
  sessionID,
}: UseFactoryEventStreamOptions) {
  const queryClient = useQueryClient();
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
    for (const event of events) {
      onEvent(event);
    }
  }, [onEvent]);

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
    openStream,
    queryClient,
    queuedEventsRef,
    refreshToken,
    scheduleQueuedFlush,
    sessionID,
    setStreamState,
    streamSessionID,
  });
}
