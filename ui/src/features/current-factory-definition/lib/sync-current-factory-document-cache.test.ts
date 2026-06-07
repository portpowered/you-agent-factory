import { QueryClient } from "@tanstack/react-query";

import type { CurrentFactoryDocument } from "../../../api/current-factory-definition";
import {
  currentFactoryDefinitionQueryKey,
  currentFactoryDocumentQueryKey,
} from "../hooks/useCurrentFactoryDefinition";
import { syncCurrentFactoryDocumentCache } from "./sync-current-factory-document-cache";

describe("syncCurrentFactoryDocumentCache", () => {
  it("writes the same document to definition and document query keys", () => {
    const queryClient = new QueryClient();
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
      queryClient.getQueryData(currentFactoryDocumentQueryKey("session-2")),
    ).toEqual(document);
    expect(
      queryClient.getQueryData(currentFactoryDefinitionQueryKey("session-2")),
    ).toEqual(document);
  });
});
