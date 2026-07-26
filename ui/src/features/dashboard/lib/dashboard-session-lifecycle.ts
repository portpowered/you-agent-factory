import type { QueryClient } from "@tanstack/react-query";

import { isDefaultFactorySessionID } from "../../../api/session-routing";
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
import type { StreamDerivedCacheIdentity } from "../../timeline/public/stream-identity";
import { useDashboardStreamStore } from "../state/dashboardStreamStore";
export {
  dashboardSessionKey,
  sessionIDFromDashboardSessionKey,
  shouldResetDashboardSessionScopedState,
  shouldResumeFromPersistedCheckpoint,
} from "./dashboard-session-key";

export type FactoryDefinitionQueryResetMode = "invalidate" | "remove";

function isDefaultFactorySessionAliasRemap(
  previousSessionID: string | null,
  sessionID: string | null,
): boolean {
  if (previousSessionID == null || sessionID == null) {
    return false;
  }
  if (previousSessionID === sessionID) {
    return false;
  }
  return (
    isDefaultFactorySessionID(previousSessionID) ||
    isDefaultFactorySessionID(sessionID)
  );
}

/** True when sync-preflight remaps the default alias to its runtime UUID identity. */
export function isDefaultToRuntimeSessionAliasRemap(
  previousSessionID: string | null,
  sessionID: string | null,
): boolean {
  return (
    isDefaultFactorySessionAliasRemap(previousSessionID, sessionID) &&
    isDefaultFactorySessionID(previousSessionID) &&
    !isDefaultFactorySessionID(sessionID)
  );
}

export function resetDashboardSessionScopedState(
  queryClient: QueryClient,
  resetStreamState: (locale?: string | null) => void,
  resetTimeline: () => void,
  locale?: string | null,
  factoryDefinitionQueryResetMode: FactoryDefinitionQueryResetMode = "remove",
): void {
  resetTimeline();
  resetStreamState(locale);
  resetSelectionHistoryStore();
  const queryFilter = {
    queryKey: [CURRENT_FACTORY_DEFINITION_QUERY_KEY_PREFIX],
    exact: false,
  } as const;
  if (factoryDefinitionQueryResetMode === "invalidate") {
    void queryClient.invalidateQueries(queryFilter);
    return;
  }
  queryClient.removeQueries(queryFilter);
  queryClient.removeQueries({
    queryKey: FACTORY_SESSION_DETAIL_QUERY_KEY,
    exact: false,
  });
}

function resolveSessionRuntimeCacheScope(
  streamIdentity?: StreamDerivedCacheIdentity | null,
): string | null {
  const resolvedStreamIdentity =
    streamIdentity ?? useDashboardStreamStore.getState().resolvedStreamIdentity;
  return (
    resolvedStreamIdentity?.backendScopeID ??
    useDashboardStreamStore.getState().backendRuntimeCacheScope
  );
}

export { factorySessionDetailQueryKey } from "../../factory-session-detail/public";

export function clearDashboardSessionRuntimeQueries(
  queryClient: QueryClient,
  sessionID: string,
  streamIdentity?: StreamDerivedCacheIdentity | null,
): void {
  const resolvedStreamIdentity =
    streamIdentity ?? useDashboardStreamStore.getState().resolvedStreamIdentity;
  const backendScopeID = resolveSessionRuntimeCacheScope(
    resolvedStreamIdentity,
  );
  queryClient.removeQueries({
    queryKey: currentFactoryDefinitionQueryKey(
      sessionID,
      resolvedStreamIdentity,
    ),
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
  streamIdentity?: StreamDerivedCacheIdentity | null,
): void {
  resetTimeline();
  resetSelectionHistoryStore();
  clearDashboardSessionRuntimeQueries(queryClient, sessionID, streamIdentity);
  const resolvedStreamIdentity =
    streamIdentity ?? useDashboardStreamStore.getState().resolvedStreamIdentity;
  queryClient.removeQueries({
    queryKey: currentFactoryDocumentQueryKey(sessionID, resolvedStreamIdentity),
    exact: true,
  });
}
