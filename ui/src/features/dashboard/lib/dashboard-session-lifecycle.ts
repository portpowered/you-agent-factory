import type { QueryClient } from "@tanstack/react-query";

import {
  CURRENT_FACTORY_DEFINITION_QUERY_KEY_PREFIX,
  currentFactoryDefinitionQueryKey,
  currentFactoryDocumentQueryKey,
} from "../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { resetSelectionHistoryStore } from "../../current-selection/base/public";
import { FACTORY_SESSION_DETAIL_QUERY_KEY } from "../../factory-session-detail/public";
import type { StreamDerivedCacheIdentity } from "../../timeline/lib/stream-derived-cache-identity";
import {
  normalizeStreamDerivedCacheIdentity,
  streamDerivedCacheKeyPrefix,
} from "../../timeline/lib/stream-derived-cache-identity";
import { useDashboardStreamStore } from "../state/dashboardStreamStore";

export type FactoryDefinitionQueryResetMode = "invalidate" | "remove";

export function dashboardSessionKey(
  sessionID: string | null,
  refreshToken: number,
): string | null {
  return sessionID == null ? null : `${sessionID}::${refreshToken}`;
}

export function sessionIDFromDashboardSessionKey(
  sessionKey: string | null,
): string | null {
  if (sessionKey == null) {
    return null;
  }
  const separatorIndex = sessionKey.lastIndexOf("::");
  return separatorIndex === -1 ? sessionKey : sessionKey.slice(0, separatorIndex);
}

export function shouldResumeFromPersistedCheckpoint({
  previousSessionKey,
  refreshToken,
  sessionID,
}: {
  previousSessionKey: string | null;
  refreshToken: number;
  sessionID: string | null;
}): boolean {
  if (refreshToken === 0) {
    return true;
  }
  if (sessionID == null || previousSessionKey == null) {
    return false;
  }
  return sessionIDFromDashboardSessionKey(previousSessionKey) !== sessionID;
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
}

export function factorySessionDetailQueryKey(
  sessionID: string,
  streamIdentity?: StreamDerivedCacheIdentity | null,
) {
  const normalizedStreamIdentity =
    normalizeStreamDerivedCacheIdentity(streamIdentity);
  if (normalizedStreamIdentity) {
    return [
      ...FACTORY_SESSION_DETAIL_QUERY_KEY,
      ...streamDerivedCacheKeyPrefix(normalizedStreamIdentity),
    ] as const;
  }
  return [...FACTORY_SESSION_DETAIL_QUERY_KEY, sessionID] as const;
}

export function clearDashboardSessionRuntimeQueries(
  queryClient: QueryClient,
  sessionID: string,
  streamIdentity?: StreamDerivedCacheIdentity | null,
): void {
  const resolvedStreamIdentity =
    streamIdentity ?? useDashboardStreamStore.getState().resolvedStreamIdentity;
  queryClient.removeQueries({
    queryKey: currentFactoryDefinitionQueryKey(sessionID, resolvedStreamIdentity),
    exact: false,
  });
  queryClient.removeQueries({
    queryKey: factorySessionDetailQueryKey(sessionID, resolvedStreamIdentity),
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
