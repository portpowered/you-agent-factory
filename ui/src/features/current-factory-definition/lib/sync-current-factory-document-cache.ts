import type { QueryClient } from "@tanstack/react-query";

import type { CurrentFactoryDocument } from "../../../api/current-factory-definition";
import {
  currentFactoryDefinitionQueryKey,
  currentFactoryDocumentQueryKey,
} from "../hooks/useCurrentFactoryDefinition";
import type { StreamDerivedCacheIdentity } from "../../timeline/lib/stream-derived-cache-identity";
import { useDashboardStreamStore } from "../../dashboard/state/dashboardStreamStore";

export function syncCurrentFactoryDocumentCache(
  queryClient: QueryClient,
  sessionID: string | null | undefined,
  document: CurrentFactoryDocument,
  streamIdentity?: StreamDerivedCacheIdentity | null,
): void {
  const resolvedStreamIdentity =
    streamIdentity ?? useDashboardStreamStore.getState().resolvedStreamIdentity;
  queryClient.setQueryData(
    currentFactoryDocumentQueryKey(sessionID, resolvedStreamIdentity),
    document,
  );
  queryClient.setQueryData(
    currentFactoryDefinitionQueryKey(sessionID, resolvedStreamIdentity),
    document,
  );
}
