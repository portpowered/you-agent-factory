import type { QueryClient } from "@tanstack/react-query";

import {
  CURRENT_FACTORY_DEFINITION_QUERY_KEY_PREFIX,
  currentFactoryDefinitionQueryKey,
  currentFactoryDocumentQueryKey,
} from "../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { resetSelectionHistoryStore } from "../../current-selection/base/public";
import { FACTORY_SESSION_DETAIL_QUERY_KEY } from "../../factory-session-detail/public";

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

export function clearDashboardSessionRuntimeQueries(
  queryClient: QueryClient,
  sessionID: string,
): void {
  queryClient.removeQueries({
    queryKey: currentFactoryDefinitionQueryKey(sessionID),
    exact: false,
  });
  queryClient.removeQueries({
    queryKey: [...FACTORY_SESSION_DETAIL_QUERY_KEY, sessionID],
    exact: false,
  });
}

export function recoverDashboardSessionScopedState(
  queryClient: QueryClient,
  sessionID: string,
  resetTimeline: () => void,
): void {
  resetTimeline();
  resetSelectionHistoryStore();
  clearDashboardSessionRuntimeQueries(queryClient, sessionID);
  queryClient.removeQueries({
    queryKey: currentFactoryDocumentQueryKey(sessionID),
    exact: true,
  });
}
