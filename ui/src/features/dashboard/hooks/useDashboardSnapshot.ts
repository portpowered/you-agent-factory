import { useCallback, useMemo, useRef } from "react";

import type { FactoryEvent } from "../../../api/events";
import { readFactoryTimelineDebugOptions } from "../../timeline/state/factoryTimelineDebug";
import { useFactoryTimelineStore } from "../../timeline/state/factoryTimelineStore";
import { useDashboardSession } from "../session/dashboard-session-provider";
import { useDashboardSessionLifecycle } from "./useDashboardSessionLifecycle";
import { useDashboardTimelineMemoryDebug } from "./useDashboardTimelineMemoryDebug";
import { useDashboardWorldView } from "./useDashboardWorldView";
import { useFactoryEventStream } from "./useFactoryEventStream";

export interface UseDashboardSnapshotOptions {
  locale?: string | null;
  refreshToken?: number;
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
