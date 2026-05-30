import { useEffect } from "react";

import {
  installFactoryTimelineDebugGlobal,
  persistFactoryTimelineMemorySummary,
  type FactoryTimelineDebugOptions,
  summarizeFactoryTimelineMemory,
} from "../../timeline/state/factoryTimelineDebug";
import { useFactoryTimelineStore } from "../../timeline/state/factoryTimelineStore";

export function useDashboardTimelineMemoryDebug({
  debugOptions,
  eventCount,
}: {
  debugOptions: FactoryTimelineDebugOptions;
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
