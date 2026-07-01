import type { QueryClient } from "@tanstack/react-query";

import type { CurrentFactoryDocument } from "../../../api/current-factory-definition";
import {
  currentFactoryDefinitionQueryKey,
  currentFactoryDocumentQueryKey,
} from "../hooks/useCurrentFactoryDefinition";
import { useDashboardStreamStore } from "../../dashboard/public/runtime-cache-scope";

export function syncCurrentFactoryDocumentCache(
  queryClient: QueryClient,
  sessionID: string | null | undefined,
  document: CurrentFactoryDocument,
): void {
  const backendRuntimeCacheScope =
    useDashboardStreamStore.getState().backendRuntimeCacheScope;
  queryClient.setQueryData(
    currentFactoryDocumentQueryKey(sessionID, backendRuntimeCacheScope),
    document,
  );
  queryClient.setQueryData(
    currentFactoryDefinitionQueryKey(sessionID, backendRuntimeCacheScope),
    document,
  );
}
