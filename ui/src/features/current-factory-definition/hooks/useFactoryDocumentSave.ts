import { useMutation, useQueryClient } from "@tanstack/react-query";

import {
  type CanonicalFactoryDefinition,
  CURRENT_FACTORY_EDITOR_SAVE_MODE,
  type CurrentFactoryDefinitionError,
  type CurrentFactoryDocument,
  type CurrentFactoryVersion,
  type SaveFactoryForSessionInput,
  saveFactoryForSessionDocument,
} from "../../../api/current-factory-definition";
import { useDashboardSession } from "../../dashboard/session/dashboard-session-provider";
import { syncCurrentFactoryDocumentCache } from "../lib/sync-current-factory-document-cache";
import { currentFactoryDocumentQueryKey } from "./useCurrentFactoryDefinition";

export type FactoryDocumentSaveInput = {
  baseVersion?: CurrentFactoryVersion;
  factory: CanonicalFactoryDefinition;
  mode?: SaveFactoryForSessionInput["mode"];
  sessionID?: string | null;
};

export function useFactoryDocumentSave() {
  const queryClient = useQueryClient();
  const { sessionID: dashboardSessionID } = useDashboardSession();

  const mutation = useMutation<
    CurrentFactoryDocument,
    CurrentFactoryDefinitionError,
    FactoryDocumentSaveInput
  >({
    mutationFn: (input) => {
      const resolvedSessionID = input.sessionID ?? dashboardSessionID;
      const factoryDefinition = structuredClone(input.factory);
      const resolvedMode = input.mode ?? CURRENT_FACTORY_EDITOR_SAVE_MODE;
      const cachedDocument = queryClient.getQueryData<CurrentFactoryDocument>(
        currentFactoryDocumentQueryKey(resolvedSessionID),
      );

      return saveFactoryForSessionDocument(
        {
          baseVersion: input.baseVersion,
          canonicalFactoryName: cachedDocument?.name,
          factoryDefinition,
          mode: resolvedMode,
        },
        { sessionID: resolvedSessionID },
      );
    },
    onSuccess: (document, input) => {
      const resolvedSessionID = input.sessionID ?? dashboardSessionID;
      syncCurrentFactoryDocumentCache(
        queryClient,
        resolvedSessionID,
        document,
      );
    },
  });

  return {
    error: mutation.error,
    isPending: mutation.isPending,
    reset: mutation.reset,
    save: mutation.mutate,
    saveAsync: mutation.mutateAsync,
  };
}
