import { QueryClient } from "@tanstack/react-query";

import type { CurrentFactoryDocument } from "../../../api/current-factory-definition";
import { useDashboardStreamStore } from "../../dashboard/public/runtime-cache-scope";
import {
  currentFactoryDefinitionQueryKey,
  currentFactoryDocumentQueryKey,
} from "../hooks/useCurrentFactoryDefinition";
import { syncCurrentFactoryDocumentCache } from "./sync-current-factory-document-cache";

describe("syncCurrentFactoryDocumentCache", () => {
  it("writes the same document to scoped definition and document query keys", () => {
    const queryClient = new QueryClient();
    const streamIdentity = {
      backendScopeID: "backend-scope-a",
      factorySessionID: "session-2",
      logicalSessionKeyID: "logical-session-2",
      streamGenerationID: "generation-1",
    };
    useDashboardStreamStore.setState({
      backendRuntimeCacheScope: streamIdentity.backendScopeID,
      resolvedStreamIdentity: streamIdentity,
    });
    const document: CurrentFactoryDocument = {
      name: "alpha",
      version: {
        logical: "3",
        physical: "2026-05-31T12:00:00Z",
      },
      workers: [],
      workstations: [],
      workTypes: [],
    };

    syncCurrentFactoryDocumentCache(queryClient, "session-2", document);

    expect(
      queryClient.getQueryData(
        currentFactoryDocumentQueryKey("session-2", streamIdentity),
      ),
    ).toEqual(document);
    expect(
      queryClient.getQueryData(
        currentFactoryDefinitionQueryKey("session-2", streamIdentity),
      ),
    ).toEqual(document);
  });
});
