import { useMemo } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import {
  type CurrentActivityFactoryGraphSource,
  currentActivityCardBaseFactoryDocument,
  currentActivityCardCurrentFactoryDefinition,
  currentActivityCardDisplayFactoryDefinition,
  currentActivityCardFactoryDefinition,
  currentActivityCardPendingFactoryDefinition,
  currentActivityCardSavedFactoryDocument,
} from "./current-activity-card-factory-definition";

export interface CurrentActivityFactoryGraphViewState
  extends CurrentActivityFactoryGraphSource {
  baseFactoryDocument: DashboardSnapshot["factory"] | null;
  currentFactoryDefinition: DashboardSnapshot["factory"] | null;
  displayFactoryDefinition: DashboardSnapshot["factory"] | null | undefined;
  pendingFactoryDefinition: DashboardSnapshot["factory"] | null;
  persistedFactoryDefinition: DashboardSnapshot["factory"] | null | undefined;
  savedFactoryDocument: DashboardSnapshot["factory"] | null;
}

export function useCurrentActivityFactoryGraphViewState(
  source: CurrentActivityFactoryGraphSource,
  snapshot: DashboardSnapshot,
): CurrentActivityFactoryGraphViewState {
  return useMemo(
    () => ({
      ...source,
      baseFactoryDocument: currentActivityCardBaseFactoryDocument(source),
      currentFactoryDefinition:
        currentActivityCardCurrentFactoryDefinition(source),
      displayFactoryDefinition: currentActivityCardDisplayFactoryDefinition(
        source,
        snapshot,
      ),
      pendingFactoryDefinition:
        currentActivityCardPendingFactoryDefinition(source),
      persistedFactoryDefinition: currentActivityCardFactoryDefinition(
        source,
        snapshot,
      ),
      savedFactoryDocument: currentActivityCardSavedFactoryDocument(source),
    }),
    [snapshot, source],
  );
}
