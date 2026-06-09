import { useMemo } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { useFactoryTimelineStore } from "../../timeline/state/factoryTimelineStore";
import {
  currentActivityCardBaseFactoryDocument,
  currentActivityCardCurrentFactoryDefinition,
  currentActivityCardDisplayFactoryDefinition,
  currentActivityCardFactoryDefinition,
  currentActivityCardPendingFactoryDefinition,
  currentActivityCardSavedFactoryDocument,
  type CurrentActivityFactoryGraphSource,
} from "./current-activity-card-factory-definition";

export interface CurrentActivityFactoryGraphViewState
  extends CurrentActivityFactoryGraphSource {
  baseFactoryDocument: DashboardSnapshot["factory"] | null;
  currentFactoryDefinition: DashboardSnapshot["factory"] | null;
  displayFactoryDefinition: DashboardSnapshot["factory"] | null | undefined;
  pendingFactoryDefinition: DashboardSnapshot["factory"] | null;
  persistedFactoryDefinition: DashboardSnapshot["factory"] | null | undefined;
  savedFactoryDocument: DashboardSnapshot["factory"] | null;
  timelineMode: ReturnType<typeof useFactoryTimelineStore.getState>["mode"];
}

export function useCurrentActivityFactoryGraphViewState(
  source: CurrentActivityFactoryGraphSource,
  snapshot: DashboardSnapshot,
): CurrentActivityFactoryGraphViewState {
  const timelineMode = useFactoryTimelineStore((state) => state.mode);

  return useMemo(
    () => ({
      ...source,
      baseFactoryDocument: currentActivityCardBaseFactoryDocument(source),
      currentFactoryDefinition: currentActivityCardCurrentFactoryDefinition(
        source,
      ),
      displayFactoryDefinition: currentActivityCardDisplayFactoryDefinition(
        source,
        snapshot,
        timelineMode,
      ),
      pendingFactoryDefinition:
        currentActivityCardPendingFactoryDefinition(source),
      persistedFactoryDefinition: currentActivityCardFactoryDefinition(
        source,
        snapshot,
        timelineMode,
      ),
      savedFactoryDocument: currentActivityCardSavedFactoryDocument(source),
      timelineMode,
    }),
    [snapshot, source, timelineMode],
  );
}
