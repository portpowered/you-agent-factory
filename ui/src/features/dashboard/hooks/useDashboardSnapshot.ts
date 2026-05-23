import { useQueryClient } from "@tanstack/react-query";
import { type RefObject, useCallback, useEffect, useMemo, useRef } from "react";

import type { FactoryEvent } from "../../../api/events";
import { FACTORY_EVENT_TYPES, openFactoryEventStream } from "../../../api/events";
import { normalizeFactoryDefinition } from "../../../api/factory-definition";
import {
  CURRENT_EDITABLE_FACTORY_DEFINITION_QUERY_KEY_PREFIX,
  currentFactoryDocumentQueryKey,
  currentEditableFactoryDefinitionQueryKey,
} from "../../current-factory-definition";
import { resetSelectionHistoryStore } from "../../current-selection/state/selectionHistoryStore";
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
  refreshToken?: number;
}

function resetDashboardSessionScopedState(
  queryClient: ReturnType<typeof useQueryClient>,
  resetStreamState: () => void,
  resetTimeline: () => void,
): void {
  resetTimeline();
  resetStreamState();
  resetSelectionHistoryStore();
  queryClient.removeQueries({
    queryKey: [CURRENT_EDITABLE_FACTORY_DEFINITION_QUERY_KEY_PREFIX],
    exact: false,
  });
  queryClient.removeQueries({
    queryKey: [CURRENT_EDITABLE_FACTORY_DEFINITION_QUERY_KEY_PREFIX],
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
  queryClient,
  queuedEventsRef,
  refreshToken,
  resetStreamState,
  resetTimeline,
  selectedSessionID,
}: {
  hasOpenedStreamRef: RefObject<boolean>;
  queryClient: ReturnType<typeof useQueryClient>;
  queuedEventsRef: RefObject<FactoryEvent[]>;
  refreshToken: number;
  resetStreamState: () => void;
  resetTimeline: () => void;
  selectedSessionID: string | null;
}): boolean {
  if (selectedSessionID == null) {
    queuedEventsRef.current = [];
    resetDashboardSessionScopedState(queryClient, resetStreamState, resetTimeline);
    return false;
  }

  if (hasOpenedStreamRef.current || refreshToken !== 0) {
    queuedEventsRef.current = [];
    resetDashboardSessionScopedState(queryClient, resetStreamState, resetTimeline);
  } else {
    hasOpenedStreamRef.current = true;
  }

  return true;
}

export function useDashboardSnapshot({
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
  const hasOpenedStreamRef = useRef(false);
  const debugOptions = useMemo(() => readFactoryTimelineDebugOptions(), []);

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
    const shouldOpenStream = resetDashboardSessionStateForSelectionChange({
      hasOpenedStreamRef,
      queryClient,
      queuedEventsRef,
      refreshToken,
      resetStreamState,
      resetTimeline,
      selectedSessionID,
    });
    if (!shouldOpenStream) {
      return;
    }
    if (selectedSessionID == null) {
      return;
    }
    const sessionID = selectedSessionID;

    const stream = openFactoryEventStream(
      (event) => {
        syncCurrentEditableFactoryDefinition(queryClient, event, sessionID);
        queuedEventsRef.current.push(
          compactFactoryEventForTimeline(event, debugOptions),
        );
        scheduleQueuedFlush();
      },
      (status, message) => {
        setStreamState({ status, message });
      },
      sessionID,
    );
    return () => {
      clearQueuedFlush(flushHandleRef);
      flushQueuedEvents();
      stream?.close();
    };
  }, [
    debugOptions,
    flushQueuedEvents,
    refreshToken,
    resetStreamState,
    resetTimeline,
    scheduleQueuedFlush,
    setStreamState,
    queryClient,
    selectedSessionID,
  ]);

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

  const isInitialLoading =
    selectedSessionID != null && selectedTick === 0 && eventCount === 0;

  return useMemo(
    () => ({
      snapshot,
      streamState,
      isInitialLoading,
      error: null as Error | null,
    }),
    [snapshot, streamState, isInitialLoading],
  );
}

function syncCurrentEditableFactoryDefinition(
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
      currentEditableFactoryDefinitionQueryKey(sessionID),
      normalizeFactoryDefinition(payloadFactory),
    );
    void queryClient.invalidateQueries({
      queryKey: currentFactoryDocumentQueryKey(sessionID),
    });
  } catch {
    return;
  }
}
