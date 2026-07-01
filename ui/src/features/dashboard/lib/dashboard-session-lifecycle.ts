import type { QueryClient } from "@tanstack/react-query";

import {
  CURRENT_FACTORY_DEFINITION_QUERY_KEY_PREFIX,
  currentFactoryDefinitionQueryKey,
  currentFactoryDocumentQueryKey,
} from "../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { resetSelectionHistoryStore } from "../../current-selection/base/public";
import {
  FACTORY_SESSION_DETAIL_QUERY_KEY,
  factorySessionDetailQueryKey,
} from "../../factory-session-detail/public";
import { backendRuntimeCacheScopeKey } from "./backend-runtime-cache-scope";

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
  queryClient.removeQueries({
    queryKey: FACTORY_SESSION_DETAIL_QUERY_KEY,
    exact: false,
  });
}

export function clearDashboardSessionRuntimeQueries(
  queryClient: QueryClient,
  sessionID: string,
  backendScopeID?: string | null,
): void {
  queryClient.removeQueries({
    queryKey: currentFactoryDefinitionQueryKey(sessionID, backendScopeID),
    exact: false,
  });
  queryClient.removeQueries({
    queryKey: factorySessionDetailQueryKey(sessionID, backendScopeID),
    exact: false,
  });
}

export function recoverDashboardSessionScopedState(
  queryClient: QueryClient,
  sessionID: string,
  resetTimeline: () => void,
  backendScopeID?: string | null,
): void {
  resetTimeline();
  resetSelectionHistoryStore();
  clearDashboardSessionRuntimeQueries(queryClient, sessionID, backendScopeID);
  queryClient.removeQueries({
    queryKey: currentFactoryDocumentQueryKey(sessionID, backendScopeID),
    exact: true,
  });
}

export function clearDashboardRuntimeQueriesForScope(
  queryClient: QueryClient,
  backendScopeID: string,
): void {
  const scopeKey = backendRuntimeCacheScopeKey(backendScopeID);
  queryClient.removeQueries({
    predicate: (query) =>
      Array.isArray(query.queryKey) && query.queryKey.includes(scopeKey),
  });
}
