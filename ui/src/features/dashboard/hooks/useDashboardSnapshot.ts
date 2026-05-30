import { useCallback, useEffect, useMemo, useRef } from "react";

import type { FactoryEvent } from "../../../api/events";
import {
  installFactoryTimelineDebugGlobal,
  persistFactoryTimelineMemorySummary,
  readFactoryTimelineDebugOptions,
  summarizeFactoryTimelineMemory,
} from "../../timeline/state/factoryTimelineDebug";
import { useFactoryTimelineStore } from "../../timeline/state/factoryTimelineStore";
import { useDashboardSession } from "../session/dashboard-session-provider";
import { useDashboardSessionLifecycle } from "./useDashboardSessionLifecycle";
import { useDashboardWorldView } from "./useDashboardWorldView";
import { useFactoryEventStream } from "./useFactoryEventStream";

export interface UseDashboardSnapshotOptions {
  locale?: string | null;
  refreshToken?: number;
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
  const appendEvents = useFactoryTimelineStore((state) => state.appendEvents);
  const eventCount = useFactoryTimelineStore((state) => state.events.length);
  const { error, isInitialLoading, snapshot, streamState } = useDashboardWorldView();
  const { isPaused, rawSessionID } = useDashboardSession();
  const debugOptions = useMemo(() => readFactoryTimelineDebugOptions(), []);
  const queuedAppendRef = useRef<(events: FactoryEvent[]) => void>(appendEvents);

  queuedAppendRef.current = appendEvents;

  useDashboardSessionLifecycle({
    locale,
    refreshToken,
    sessionID: rawSessionID,
  });

  const handleStreamEvent = useCallback((event: FactoryEvent) => {
    queuedAppendRef.current([event]);
  }, []);

  useFactoryEventStream({
    enabled: rawSessionID != null && !isPaused,
    locale,
    onEvent: handleStreamEvent,
    refreshToken,
    sessionID: rawSessionID,
  });

  useDashboardTimelineMemoryDebug({
    debugOptions,
    eventCount,
  });

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
