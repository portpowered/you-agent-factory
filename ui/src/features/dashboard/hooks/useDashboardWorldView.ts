import { useMemo } from "react";

import { deriveDashboardWorldViewShellState } from "../lib/dashboard-world-view";
import { useDashboardSession } from "../session/dashboard-session-provider";
import { useFactoryTimelineStore } from "../../timeline/state/factoryTimelineStore";
import { useDashboardStreamStore } from "../state/dashboardStreamStore";

export function useDashboardWorldView() {
  const eventCount = useFactoryTimelineStore((state) => state.events.length);
  const selectedTick = useFactoryTimelineStore((state) => state.selectedTick);
  const snapshot = useFactoryTimelineStore(
    (state) => state.worldViewCache[state.selectedTick],
  );
  const streamState = useDashboardStreamStore((state) => state.streamState);
  const { rawSessionID } = useDashboardSession();

  const shellState = useMemo(
    () =>
      deriveDashboardWorldViewShellState({
        eventCount,
        rawSessionID,
        selectedTick,
        streamState,
      }),
    [eventCount, rawSessionID, selectedTick, streamState],
  );

  return useMemo(
    () => ({
      error: shellState.error,
      hasEvents: shellState.hasEvents,
      isInitialLoading: shellState.isInitialLoading,
      selectedTick,
      snapshot,
      streamState,
    }),
    [
      selectedTick,
      shellState.error,
      shellState.hasEvents,
      shellState.isInitialLoading,
      snapshot,
      streamState,
    ],
  );
}
