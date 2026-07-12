import { useFactoryTimelineStore } from "../../timeline/state/factoryTimelineStore";
import { useDashboardStreamStore } from "../state/dashboardStreamStore";

export function useDashboardWorldView() {
  const selectedTick = useFactoryTimelineStore((state) => state.selectedTick);
  const snapshot = useFactoryTimelineStore(
    (state) => state.worldViewCache[state.selectedTick],
  );
  const streamState = useDashboardStreamStore((state) => state.streamState);
  return { selectedTick, snapshot, streamState };
}
