import { useQuery } from "@tanstack/react-query";
import {
  type CanonicalFactoryDefinition,
  type CurrentFactoryDefinitionError,
  type CurrentFactoryDocument,
  getCurrentFactoryDocument,
} from "../../../api/current-factory-definition";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { useDashboardStreamStore } from "../../dashboard/public/runtime-cache-scope";
import { useDashboardSession } from "../../dashboard/public/session-context";
import {
  normalizeStreamDerivedCacheIdentity,
  type StreamDerivedCacheIdentity,
  streamDerivedCacheKeyPrefix,
} from "../../timeline/public/stream-identity";

export const CURRENT_FACTORY_DEFINITION_QUERY_KEY_PREFIX =
  "current-factory-definition";

function normalizeSessionQueryKey(
  sessionID: string | null | undefined,
): string {
  return sessionID ?? DEFAULT_FACTORY_SESSION_ID;
}

export function currentFactoryDefinitionQueryKey(
  sessionID: string | null | undefined,
  streamIdentity?: StreamDerivedCacheIdentity | null,
) {
  const normalizedStreamIdentity =
    normalizeStreamDerivedCacheIdentity(streamIdentity);
  if (normalizedStreamIdentity) {
    return [
      CURRENT_FACTORY_DEFINITION_QUERY_KEY_PREFIX,
      ...streamDerivedCacheKeyPrefix(normalizedStreamIdentity),
    ] as const;
  }
  return [
    CURRENT_FACTORY_DEFINITION_QUERY_KEY_PREFIX,
    normalizeSessionQueryKey(sessionID),
  ] as const;
}

export const CURRENT_FACTORY_DEFINITION_QUERY_KEY =
  currentFactoryDefinitionQueryKey(DEFAULT_FACTORY_SESSION_ID);

export function useCurrentFactoryDefinition(isEnabled = true) {
  const { sessionID } = useDashboardSession();
  const streamIdentity = useDashboardStreamStore(
    (state) => state.resolvedStreamIdentity,
  );

  return useQuery<CanonicalFactoryDefinition, CurrentFactoryDefinitionError>({
    queryKey: currentFactoryDefinitionQueryKey(sessionID, streamIdentity),
    queryFn: () => getCurrentFactoryDocument({ sessionID }),
    enabled: isEnabled,
    gcTime: 0,
    refetchOnWindowFocus: false,
    retry: false,
  });
}

export function currentFactoryDocumentQueryKey(
  sessionID: string | null | undefined,
  streamIdentity?: StreamDerivedCacheIdentity | null,
) {
  return [
    ...currentFactoryDefinitionQueryKey(sessionID, streamIdentity),
    "document",
  ] as const;
}

export const CURRENT_FACTORY_DOCUMENT_QUERY_KEY =
  currentFactoryDocumentQueryKey(DEFAULT_FACTORY_SESSION_ID);

export function useCurrentFactoryDocument(isEnabled = true) {
  const { sessionID } = useDashboardSession();
  const streamIdentity = useDashboardStreamStore(
    (state) => state.resolvedStreamIdentity,
  );

  return useQuery<CurrentFactoryDocument, CurrentFactoryDefinitionError>({
    queryKey: currentFactoryDocumentQueryKey(sessionID, streamIdentity),
    queryFn: () => getCurrentFactoryDocument({ sessionID }),
    enabled: isEnabled,
    gcTime: 0,
    refetchOnWindowFocus: false,
    retry: false,
  });
}
