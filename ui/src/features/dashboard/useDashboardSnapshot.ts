import { useCallback, useEffect, useMemo, useRef } from "react";

import type { FactoryEvent } from "../../api/events";
import { openFactoryEventStream } from "../../api/events";
import {
  compactFactoryEventForTimeline,
  installFactoryTimelineDebugGlobal,
  persistFactoryTimelineMemorySummary,
  readFactoryTimelineDebugOptions,
  summarizeFactoryTimelineMemory,
} from "../timeline/state/factoryTimelineDebug";
import { useFactoryTimelineStore } from "../timeline/state/factoryTimelineStore";
import {
  useDashboardStreamStore,
} from "./state/dashboardStreamStore";

export interface UseDashboardSnapshotOptions {
  refreshToken?: number;
}

export function useDashboardSnapshot({
  refreshToken = 0,
}: UseDashboardSnapshotOptions = {}) {
  const appendEvents = useFactoryTimelineStore((state) => state.appendEvents);
  const eventCount = useFactoryTimelineStore((state) => state.events.length);
  const resetTimeline = useFactoryTimelineStore((state) => state.reset);
  const selectedTick = useFactoryTimelineStore((state) => state.selectedTick);
  const snapshot = useFactoryTimelineStore(
    (state) => state.worldViewCache[state.selectedTick]?.dashboard,
  );
  const streamState = useDashboardStreamStore((state) => state.streamState);
  const resetStreamState = useDashboardStreamStore((state) => state.resetStreamState);
  const setStreamState = useDashboardStreamStore((state) => state.setStreamState);
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
    if (hasOpenedStreamRef.current || refreshToken !== 0) {
      queuedEventsRef.current = [];
      resetTimeline();
      resetStreamState();
    } else {
      hasOpenedStreamRef.current = true;
    }

    const stream = openFactoryEventStream(
      (event) => {
        queuedEventsRef.current.push(
          compactFactoryEventForTimeline(event, debugOptions),
        );
        scheduleQueuedFlush();
      },
      (status, message) => {
        setStreamState({ status, message });
      },
    );
    return () => {
      if (flushHandleRef.current !== null) {
        if (typeof window.cancelAnimationFrame === "function") {
          window.cancelAnimationFrame(flushHandleRef.current);
        } else {
          window.clearTimeout(flushHandleRef.current);
        }
        flushHandleRef.current = null;
      }
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

  const isInitialLoading = selectedTick === 0 && eventCount === 0;

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
