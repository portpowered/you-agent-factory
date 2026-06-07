import type { QueryClient } from "@tanstack/react-query";

import type { CurrentFactoryDocument } from "../../../api/current-factory-definition";
import {
  currentFactoryDefinitionQueryKey,
  currentFactoryDocumentQueryKey,
} from "../hooks/useCurrentFactoryDefinition";

export function syncCurrentFactoryDocumentCache(
  queryClient: QueryClient,
  sessionID: string | null | undefined,
  document: CurrentFactoryDocument,
): void {
  queryClient.setQueryData(
    currentFactoryDocumentQueryKey(sessionID),
    document,
  );
  queryClient.setQueryData(
    currentFactoryDefinitionQueryKey(sessionID),
    document,
  );
}
