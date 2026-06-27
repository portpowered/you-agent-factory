import { useQueryClient } from "@tanstack/react-query";
import { type RefObject, useCallback, useEffect, useMemo, useRef } from "react";

import type { FactoryEvent } from "../../../../api/events";
import {
  type FactoryEventReconnectCursor,
  type FactoryEventReconnectValidationResult,
  openFactoryEventStream,
  validateFactoryEventReconnectCursor,
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
import { getDashboardSessionLifecycleMessages } from "../../messages/dashboard-session-lifecycle";
import { useDashboardStreamStore } from "../../state/dashboardStreamStore";

export interface UseFactoryEventStreamOptions {
  enabled: boolean;
  initialReconnectCursor?: FactoryEventReconnectCursor;
  locale?: string | null;
  onEvent: (event: FactoryEvent) => void;
  onEvents?: (events: FactoryEvent[]) => void;
  onInvalidReconnectCursor?: () => void;
  openStream?: typeof openFactoryEventStream;
  refreshToken?: number;
  sessionID: string | null;
  validateReconnectCursor?: (
    sessionID?: string | null,
    reconnect?: FactoryEventReconnectCursor,
  ) => Promise<FactoryEventReconnectValidationResult>;
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
  initialReconnectCursor?: FactoryEventReconnectCursor;
  flushQueuedEvents: () => void;
  openStream: typeof openFactoryEventStream;
  onInvalidReconnectCursor?: () => void;
  queryClient: ReturnType<typeof useQueryClient>;
  queuedEventsRef: RefObject<FactoryEvent[]>;
  locale?: string | null;
  refreshToken: number;
  scheduleQueuedFlush: () => void;
  sessionID: string | null;
  setStreamState: (
    streamState: ReturnType<
      typeof useDashboardStreamStore.getState
    >["streamState"],
  ) => void;
  streamSessionID: string;
  validateReconnectCursor: (
    sessionID?: string | null,
    reconnect?: FactoryEventReconnectCursor,
  ) => Promise<FactoryEventReconnectValidationResult>;
}

function reconnectCursorFromEvent(
  event: FactoryEvent,
): FactoryEventReconnectCursor {
  return {
    afterEventId: event.id,
    afterSequence: event.context.sessionSequence ?? event.context.sequence,
  };
}

function handleInvalidReconnectValidation({
  onInvalidReconnectCursor,
  openDashboardStream,
  setStreamState,
  staleRecoveryUsedRef,
  validation,
}: {
  onInvalidReconnectCursor?: () => void;
  openDashboardStream: (reconnect?: FactoryEventReconnectCursor) => void;
  setStreamState: DashboardStreamConnectionOptions["setStreamState"];
  staleRecoveryUsedRef: { current: boolean };
  validation: Exclude<FactoryEventReconnectValidationResult, { ok: true }>;
}): boolean {
  if (validation.reason === "stale_cursor" && !staleRecoveryUsedRef.current) {
    staleRecoveryUsedRef.current = true;
    onInvalidReconnectCursor?.();
    openDashboardStream(undefined);
    return true;
  }
  setStreamState({
    message: validation.message,
    status: "offline",
  });
  return false;
}

function attachReconnectOnError({
  locale,
  openDashboardStream,
  reconnectCursorRef,
  reconnectTimeoutRef,
  setStreamState,
  stream,
}: {
  locale?: string | null;
  openDashboardStream: (reconnect?: FactoryEventReconnectCursor) => void;
  reconnectCursorRef: { current: FactoryEventReconnectCursor | undefined };
  reconnectTimeoutRef: { current: number | null };
  setStreamState: DashboardStreamConnectionOptions["setStreamState"];
  stream: NonNullable<ReturnType<typeof openFactoryEventStream>>;
}) {
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
      message:
        getDashboardSessionLifecycleMessages(locale).reconnectingStreamLabel,
      status: "reconnecting",
    });
    reconnectTimeoutRef.current = window.setTimeout(() => {
      reconnectTimeoutRef.current = null;
      openDashboardStream(cursor);
    }, 1000);
  };
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

function clearReconnectTimeout(reconnectTimeoutRef: { current: number | null }) {
  if (reconnectTimeoutRef.current != null) {
    window.clearTimeout(reconnectTimeoutRef.current);
    reconnectTimeoutRef.current = null;
  }
}

function useDashboardStreamConnection({
  debugOptions,
  enabled,
  flushHandleRef,
  initialReconnectCursor,
  flushQueuedEvents,
  locale,
  openStream,
  onInvalidReconnectCursor,
  queryClient,
  queuedEventsRef,
  refreshToken,
  scheduleQueuedFlush,
  sessionID,
  setStreamState,
  streamSessionID,
  validateReconnectCursor,
}: DashboardStreamConnectionOptions) {
  const hasOpenedStreamRef = useRef(false);
  const lastSessionKeyRef = useRef<string | null>(null);
  const reconnectCursorRef = useRef<FactoryEventReconnectCursor | undefined>(
    undefined,
  );
  const reconnectTimeoutRef = useRef<number | null>(null);
  const reconnectValidationAttemptRef = useRef(0);
  const staleRecoveryUsedRef = useRef(false);
  const streamRef = useRef<ReturnType<typeof openFactoryEventStream>>(null);
  useEffect(() => {
    const sessionKey = dashboardSessionKey(sessionID, refreshToken);
    const previousSessionKey = lastSessionKeyRef.current;
    const sessionSelectionChanged = sessionKey !== previousSessionKey;
    lastSessionKeyRef.current = sessionKey;

    if (
      !resolveStreamOpenPermission({
        enabled,
        hasOpenedStreamRef,
        previousSessionKey: sessionSelectionChanged ? previousSessionKey : null,
        queuedEventsRef,
        refreshToken,
        sessionID,
        setStreamState,
      })
    ) {
      return;
    }

    const handleStreamEvent = (event: FactoryEvent) => {
      reconnectCursorRef.current = reconnectCursorFromEvent(event);
      staleRecoveryUsedRef.current = false;
      syncCurrentFactoryDefinition(queryClient, event, streamSessionID);
      queuedEventsRef.current.push(
        compactFactoryEventForTimeline(event, debugOptions),
      );
      scheduleQueuedFlush();
    };

    const openDashboardStream = (reconnect?: FactoryEventReconnectCursor) => {
      const validationAttempt = ++reconnectValidationAttemptRef.current;
      void (async () => {
        if (reconnect != null) {
          const validation = await validateReconnectCursor(
            streamSessionID,
            reconnect,
          );
          if (validationAttempt !== reconnectValidationAttemptRef.current) {
            return;
          }
          if (!validation.ok) {
            reconnectCursorRef.current = undefined;
            handleInvalidReconnectValidation({
              onInvalidReconnectCursor,
              openDashboardStream,
              setStreamState,
              staleRecoveryUsedRef,
              validation,
            });
            return;
          }
        }

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
        attachReconnectOnError({
          locale,
          openDashboardStream,
          reconnectCursorRef,
          reconnectTimeoutRef,
          setStreamState,
          stream,
        });
      })();
    };

    reconnectCursorRef.current = undefined;
    staleRecoveryUsedRef.current = false;
    openDashboardStream(initialReconnectCursor);
    return () => {
      reconnectValidationAttemptRef.current += 1;
      clearReconnectTimeout(reconnectTimeoutRef);
      clearQueuedFlush(flushHandleRef);
      flushQueuedEvents();
      streamRef.current?.close();
      streamRef.current = null;
    };
  }, [
    debugOptions,
    enabled,
    flushHandleRef,
    initialReconnectCursor,
    flushQueuedEvents,
    locale,
    openStream,
    onInvalidReconnectCursor,
    queryClient,
    queuedEventsRef,
    refreshToken,
    scheduleQueuedFlush,
    sessionID,
    setStreamState,
    streamSessionID,
    validateReconnectCursor,
  ]);
}

export function useFactoryEventStream({
  enabled,
  initialReconnectCursor,
  locale,
  onEvent,
  onEvents,
  onInvalidReconnectCursor,
  openStream = openFactoryEventStream,
  refreshToken = 0,
  sessionID,
  validateReconnectCursor = validateFactoryEventReconnectCursor,
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
    initialReconnectCursor,
    flushQueuedEvents,
    locale,
    openStream,
    onInvalidReconnectCursor,
    queryClient,
    queuedEventsRef,
    refreshToken,
    scheduleQueuedFlush,
    sessionID,
    setStreamState,
    streamSessionID,
    validateReconnectCursor,
  });
}
