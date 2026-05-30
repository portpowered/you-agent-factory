import type { QueryClient } from "@tanstack/react-query";

import { CURRENT_FACTORY_DEFINITION_QUERY_KEY_PREFIX } from "../../current-factory-definition/public";
import { resetSelectionHistoryStore } from "../../current-selection/base/public";

export function dashboardSessionKey(
  sessionID: string | null,
  refreshToken: number,
): string | null {
  return sessionID == null ? null : `${sessionID}::${refreshToken}`;
}

export function shouldResetDashboardSessionScopedState({
  previousSessionKey,
  refreshToken,
  sessionID,
}: {
  previousSessionKey: string | null;
  refreshToken: number;
  sessionID: string | null;
}): boolean {
  if (sessionID == null) {
    return true;
  }
  return previousSessionKey !== null || refreshToken !== 0;
}

export function resetDashboardSessionScopedState(
  queryClient: QueryClient,
  resetStreamState: (locale?: string | null) => void,
  resetTimeline: () => void,
  locale?: string | null,
): void {
  resetTimeline();
  resetStreamState(locale);
  resetSelectionHistoryStore();
  queryClient.removeQueries({
    queryKey: [CURRENT_FACTORY_DEFINITION_QUERY_KEY_PREFIX],
    exact: false,
  });
}
