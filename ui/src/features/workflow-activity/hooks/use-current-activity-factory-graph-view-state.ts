import { useMemo } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import {
  type CurrentActivityFactoryGraphSource,
  currentActivityCardCurrentFactoryDefinition,
  currentActivityCardDisplayFactoryDefinition,
  currentActivityCardFactoryDefinition,
  currentActivityCardPendingFactoryDefinition,
} from "./current-activity-card-factory-definition";

export interface CurrentActivityFactoryGraphViewState {
  currentFactoryDefinition: DashboardSnapshot["factory"] | null;
  displayFactoryDefinition: DashboardSnapshot["factory"] | null | undefined;
  pendingFactoryDefinition: DashboardSnapshot["factory"] | null;
  persistedFactoryDefinition: DashboardSnapshot["factory"] | null | undefined;
}

export function useCurrentActivityFactoryGraphViewState(
  source: CurrentActivityFactoryGraphSource,
): CurrentActivityFactoryGraphViewState {
  return useMemo(
    () => ({
      currentFactoryDefinition:
        currentActivityCardCurrentFactoryDefinition(source),
      displayFactoryDefinition:
        currentActivityCardDisplayFactoryDefinition(source),
      pendingFactoryDefinition:
        currentActivityCardPendingFactoryDefinition(source),
      persistedFactoryDefinition: currentActivityCardFactoryDefinition(source),
    }),
    [source],
  );
}
