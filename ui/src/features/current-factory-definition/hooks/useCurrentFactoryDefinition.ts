import { useQuery } from "@tanstack/react-query";
import {
  type CanonicalFactoryDefinition,
  type CurrentFactoryDefinitionError,
  type CurrentFactoryDocument,
  getCurrentFactoryDocument,
} from "../../../api/current-factory-definition";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { useDashboardSession } from "../../dashboard/session/dashboard-session-provider";
import { useDashboardStreamStore } from "../../dashboard/state/dashboardStreamStore";
import { backendRuntimeCacheScopeKey } from "../../dashboard/lib/backend-runtime-cache-scope";

export const CURRENT_FACTORY_DEFINITION_QUERY_KEY_PREFIX =
  "current-factory-definition";

function normalizeSessionQueryKey(
  sessionID: string | null | undefined,
): string {
  return sessionID ?? DEFAULT_FACTORY_SESSION_ID;
}

export function currentFactoryDefinitionQueryKey(
  sessionID: string | null | undefined,
  backendScopeID?: string | null,
) {
  return [
    CURRENT_FACTORY_DEFINITION_QUERY_KEY_PREFIX,
    backendRuntimeCacheScopeKey(backendScopeID),
    normalizeSessionQueryKey(sessionID),
  ] as const;
}

export const CURRENT_FACTORY_DEFINITION_QUERY_KEY =
  currentFactoryDefinitionQueryKey(DEFAULT_FACTORY_SESSION_ID);

export function useCurrentFactoryDefinition(isEnabled = true) {
  const { sessionID } = useDashboardSession();
  const backendRuntimeCacheScope = useDashboardStreamStore(
    (state) => state.backendRuntimeCacheScope,
  );

  return useQuery<CanonicalFactoryDefinition, CurrentFactoryDefinitionError>({
    queryKey: currentFactoryDefinitionQueryKey(
      sessionID,
      backendRuntimeCacheScope,
    ),
    queryFn: () => getCurrentFactoryDocument({ sessionID }),
    enabled: isEnabled,
    gcTime: 0,
    refetchOnWindowFocus: false,
    retry: false,
  });
}

export function currentFactoryDocumentQueryKey(
  sessionID: string | null | undefined,
  backendScopeID?: string | null,
) {
  return [
    ...currentFactoryDefinitionQueryKey(sessionID, backendScopeID),
    "document",
  ] as const;
}

export const CURRENT_FACTORY_DOCUMENT_QUERY_KEY =
  currentFactoryDocumentQueryKey(DEFAULT_FACTORY_SESSION_ID);

export function useCurrentFactoryDocument(isEnabled = true) {
  const { sessionID } = useDashboardSession();
  const backendRuntimeCacheScope = useDashboardStreamStore(
    (state) => state.backendRuntimeCacheScope,
  );

  return useQuery<CurrentFactoryDocument, CurrentFactoryDefinitionError>({
    queryKey: currentFactoryDocumentQueryKey(
      sessionID,
      backendRuntimeCacheScope,
    ),
    queryFn: () => getCurrentFactoryDocument({ sessionID }),
    enabled: isEnabled,
    gcTime: 0,
    refetchOnWindowFocus: false,
    retry: false,
  });
}
