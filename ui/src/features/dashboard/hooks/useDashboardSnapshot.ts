import { useQueryClient } from "@tanstack/react-query";
import { type RefObject, useCallback, useEffect, useMemo, useRef } from "react";

import type { FactoryEvent } from "../../../api/events";
import { FACTORY_EVENT_TYPES, openFactoryEventStream } from "../../../api/events";
import { normalizeFactoryDefinition } from "../../../api/factory-definition";
import {
  CURRENT_FACTORY_DEFINITION_QUERY_KEY_PREFIX,
  currentFactoryDocumentQueryKey,
  currentFactoryDefinitionQueryKey,
} from "../../current-factory-definition/public";
import { resetSelectionHistoryStore } from "../../current-selection/base/public";
import {
  compactFactoryEventForTimeline,
  installFactoryTimelineDebugGlobal,
  persistFactoryTimelineMemorySummary,
  readFactoryTimelineDebugOptions,
  summarizeFactoryTimelineMemory,
} from "../../timeline/state/factoryTimelineDebug";
import { useFactoryTimelineStore } from "../../timeline/state/factoryTimelineStore";
import {
  useDashboardStreamStore,
} from "../state/dashboardStreamStore";
import { useDashboardSessionStore } from "../state/dashboardSessionStore";

export interface UseDashboardSnapshotOptions {
  locale?: string | null;
  refreshToken?: number;
}

interface DashboardStreamConnectionOptions {
  debugOptions: ReturnType<typeof readFactoryTimelineDebugOptions>;
  flushHandleRef: RefObject<number | null>;
  flushQueuedEvents: () => void;
  queryClient: ReturnType<typeof useQueryClient>;
  queuedEventsRef: RefObject<FactoryEvent[]>;
  refreshToken: number;
  resetStreamState: (locale?: string | null) => void;
  resetTimeline: () => void;
  scheduleQueuedFlush: () => void;
  selectedSessionID: string | null;
  setStreamState: (streamState: ReturnType<typeof useDashboardStreamStore.getState>["streamState"]) => void;
}

function resetDashboardSessionScopedState(
  queryClient: ReturnType<typeof useQueryClient>,
  resetStreamState: (locale?: string | null) => void,
  resetTimeline: () => void,
  locale?: string | null,
): void {
  resetTimeline();
  resetStreamState(locale);
  resetSelectionHistoryStore();
  queryClient.removeQueries({
    queryKey: [CURRENT_FACTORY_DEFINITION_QUERY_KEY_PREFIX],
    exact: false,
  });
  queryClient.removeQueries({
    queryKey: [CURRENT_FACTORY_DEFINITION_QUERY_KEY_PREFIX],
    exact: false,
  });
}

function clearQueuedFlush(flushHandleRef: RefObject<number | null>): void {
  if (flushHandleRef.current === null) {
    return;
  }
  if (typeof window.cancelAnimationFrame === "function") {
    window.cancelAnimationFrame(flushHandleRef.current);
  } else {
    window.clearTimeout(flushHandleRef.current);
  }
  flushHandleRef.current = null;
}

function resetDashboardSessionStateForSelectionChange({
  hasOpenedStreamRef,
  previousSessionKey,
  queryClient,
  queuedEventsRef,
  refreshToken,
  resetStreamState,
  resetTimeline,
  selectedSessionID,
}: {
  hasOpenedStreamRef: RefObject<boolean>;
  previousSessionKey: string | null;
  queryClient: ReturnType<typeof useQueryClient>;
  queuedEventsRef: RefObject<FactoryEvent[]>;
  refreshToken: number;
  resetStreamState: () => void;
  resetTimeline: () => void;
  selectedSessionID: string | null;
}): boolean {
  if (selectedSessionID == null) {
    queuedEventsRef.current = [];
    hasOpenedStreamRef.current = false;
    resetDashboardSessionScopedState(queryClient, resetStreamState, resetTimeline);
    return false;
  }

  if (previousSessionKey !== null || refreshToken !== 0) {
    queuedEventsRef.current = [];
    resetDashboardSessionScopedState(queryClient, resetStreamState, resetTimeline);
  }
  hasOpenedStreamRef.current = true;

  return true;
}

function pausedDashboardStreamState() {
  return {
    status: "offline" as const,
    // hardcoded-ui-copy-exception: non-product-diagnostic
    message: "Live session updates paused. Showing last event state.",
  };
}

function dashboardSessionKey(
  selectedSessionID: string | null,
  refreshToken: number,
): string | null {
  return selectedSessionID == null ? null : `${selectedSessionID}::${refreshToken}`;
}

function useDashboardStreamConnection({
  debugOptions,
  flushHandleRef,
  flushQueuedEvents,
  queryClient,
  queuedEventsRef,
  refreshToken,
  resetStreamState,
  resetTimeline,
  scheduleQueuedFlush,
  selectedSessionID,
  setStreamState,
}: DashboardStreamConnectionOptions) {
  const isSessionStreamPaused = useDashboardSessionStore((state) =>
    selectedSessionID == null
      ? false
      : state.pausedSessionIDs.includes(selectedSessionID),
  );
  const hasOpenedStreamRef = useRef(false);
  const lastSessionKeyRef = useRef<string | null>(null);

  useEffect(() => {
    const sessionKey = dashboardSessionKey(selectedSessionID, refreshToken);
    const previousSessionKey = lastSessionKeyRef.current;
    const sessionSelectionChanged = sessionKey !== previousSessionKey;
    lastSessionKeyRef.current = sessionKey;

    if (!sessionSelectionChanged && isSessionStreamPaused) {
      setStreamState(pausedDashboardStreamState());
      return;
    }

    if (!sessionSelectionChanged && !isSessionStreamPaused && selectedSessionID == null) {
      return;
    }

    const shouldOpenStream = resetDashboardSessionStateForSelectionChange({
      hasOpenedStreamRef,
      previousSessionKey: sessionSelectionChanged ? previousSessionKey : null,
      queryClient,
      queuedEventsRef,
      refreshToken,
      resetStreamState,
      resetTimeline,
      selectedSessionID,
    });
    if (!shouldOpenStream || selectedSessionID == null) {
      return;
    }
    if (isSessionStreamPaused) {
      setStreamState(pausedDashboardStreamState());
      return;
    }

    const stream = openFactoryEventStream(
      (event) => {
        syncCurrentFactoryDefinition(queryClient, event, selectedSessionID);
        queuedEventsRef.current.push(
          compactFactoryEventForTimeline(event, debugOptions),
        );
        scheduleQueuedFlush();
      },
      (status, message) => {
        setStreamState({ status, message });
      },
      selectedSessionID,
    );
    return () => {
      clearQueuedFlush(flushHandleRef);
      flushQueuedEvents();
      stream?.close();
    };
  }, [
    debugOptions,
    flushHandleRef,
    flushQueuedEvents,
    isSessionStreamPaused,
    queryClient,
    queuedEventsRef,
    refreshToken,
    resetStreamState,
    resetTimeline,
    scheduleQueuedFlush,
    selectedSessionID,
    setStreamState,
  ]);
}

function useDashboardTimelineMemoryDebug({
  debugOptions,
  eventCount,
}: {
  debugOptions: ReturnType<typeof readFactoryTimelineDebugOptions>;
  eventCount: number;
}) {
  useEffect(() => {
    if (typeof window === "undefined" || !debugOptions.memoryDebug) {
      return;
    }

    installFactoryTimelineDebugGlobal(
      window,
      () => useFactoryTimelineStore.getState(),
      debugOptions,
    );
  }, [debugOptions]);

  useEffect(() => {
    if (typeof window === "undefined" || !debugOptions.memoryDebug || eventCount === 0) {
      return;
    }

    const state = useFactoryTimelineStore.getState();
    const summary = summarizeFactoryTimelineMemory(
      state.events,
      state.selectedTick,
      window,
    );
    persistFactoryTimelineMemorySummary(window.localStorage, summary);
  }, [debugOptions, eventCount]);
}

export function useDashboardSnapshot({
  locale,
  refreshToken = 0,
}: UseDashboardSnapshotOptions = {}) {
  const queryClient = useQueryClient();
  const appendEvents = useFactoryTimelineStore((state) => state.appendEvents);
  const eventCount = useFactoryTimelineStore((state) => state.events.length);
  const resetTimeline = useFactoryTimelineStore((state) => state.reset);
  const selectedTick = useFactoryTimelineStore((state) => state.selectedTick);
  const snapshot = useFactoryTimelineStore((state) => state.worldViewCache[state.selectedTick]);
  const streamState = useDashboardStreamStore((state) => state.streamState);
  const resetStreamState = useDashboardStreamStore((state) => state.resetStreamState);
  const setStreamState = useDashboardStreamStore((state) => state.setStreamState);
  const selectedSessionID = useDashboardSessionStore(
    (state) => state.selectedSessionID,
  );
  const queuedEventsRef = useRef<FactoryEvent[]>([]);
  const flushHandleRef = useRef<number | null>(null);
  const debugOptions = useMemo(() => readFactoryTimelineDebugOptions(), []);
  const resetLocalizedStreamState = useCallback(() => {
    resetStreamState(locale);
  }, [locale, resetStreamState]);

  const flushQueuedEvents = useCallback(() => {
    flushHandleRef.current = null;
    if (queuedEventsRef.current.length === 0) {
      return;
    }
    const events = queuedEventsRef.current;
    queuedEventsRef.current = [];
    appendEvents(events);
  }, [appendEvents]);

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
    flushHandleRef,
    flushQueuedEvents,
    queryClient,
    queuedEventsRef,
    refreshToken,
    resetStreamState: resetLocalizedStreamState,
    resetTimeline,
    scheduleQueuedFlush,
    setStreamState,
    selectedSessionID,
  });

  useDashboardTimelineMemoryDebug({
    debugOptions,
    eventCount,
  });

  const hasNoStreamedSnapshot = selectedTick === 0 && eventCount === 0;
  const isInitialLoading =
    selectedSessionID != null &&
    hasNoStreamedSnapshot &&
    streamState.status !== "offline";
  const error =
    selectedSessionID != null &&
    hasNoStreamedSnapshot &&
    streamState.status === "offline"
      ? new Error(streamState.message)
      : null;

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

function syncCurrentFactoryDefinition(
  queryClient: ReturnType<typeof useQueryClient>,
  event: FactoryEvent,
  sessionID: string,
): void {
  if (event.type !== FACTORY_EVENT_TYPES.factoryChange) {
    return;
  }
  const payloadFactory = (event.payload as { factory?: unknown }).factory;
  if (payloadFactory == null) {
    return;
  }
  try {
    queryClient.setQueryData(
      currentFactoryDefinitionQueryKey(sessionID),
      normalizeFactoryDefinition(payloadFactory),
    );
    void queryClient.invalidateQueries({
      queryKey: currentFactoryDocumentQueryKey(sessionID),
    });
  } catch {
    return;
  }
}
