import type { QueryClient } from "@tanstack/react-query";

import type { CurrentFactoryDocument } from "../../../api/current-factory-definition";
import { useDashboardStreamStore } from "../../dashboard/public";
import type { StreamDerivedCacheIdentity } from "../../timeline/public";
import {
  currentFactoryDefinitionQueryKey,
  currentFactoryDocumentQueryKey,
} from "../hooks/useCurrentFactoryDefinition";

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
