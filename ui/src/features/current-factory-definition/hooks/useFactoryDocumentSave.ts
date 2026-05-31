import { useMutation, useQueryClient } from "@tanstack/react-query";

import {
  CURRENT_FACTORY_EDITOR_SAVE_MODE,
  saveFactoryForSessionDocument,
  type CanonicalFactoryDefinition,
  type CurrentFactoryDefinitionError,
  type CurrentFactoryDocument,
  type CurrentFactoryVersion,
  type SaveFactoryForSessionInput,
} from "../../../api/current-factory-definition";
import { useDashboardSession } from "../../dashboard/session/dashboard-session-provider";
import {
  currentFactoryDefinitionQueryKey,
  currentFactoryDocumentQueryKey,
} from "./useCurrentFactoryDefinition";

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

      return saveFactoryForSessionDocument(
        {
          baseVersion: input.baseVersion,
          factoryDefinition,
          mode: input.mode ?? CURRENT_FACTORY_EDITOR_SAVE_MODE,
        },
        { sessionID: resolvedSessionID },
      );
    },
    onSuccess: (document, input) => {
      const resolvedSessionID = input.sessionID ?? dashboardSessionID;
      queryClient.setQueryData(
        currentFactoryDocumentQueryKey(resolvedSessionID),
        document,
      );
      queryClient.setQueryData(
        currentFactoryDefinitionQueryKey(resolvedSessionID),
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
