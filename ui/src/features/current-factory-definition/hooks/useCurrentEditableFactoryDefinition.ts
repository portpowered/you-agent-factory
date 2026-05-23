import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import {
  getCurrentEditableFactoryDefinition,
  getCurrentFactoryDocument,
  saveCurrentFactoryDocument,
  type CanonicalFactoryDefinition,
  type CurrentEditableFactoryDefinitionError,
  type CurrentFactoryDocument,
  type SaveCurrentEditableFactoryDefinitionInput,
} from "../../../api/current-factory-definition";
import { useDashboardSessionStore } from "../../dashboard/state/dashboardSessionStore";

export const CURRENT_EDITABLE_FACTORY_DEFINITION_QUERY_KEY_PREFIX =
  "current-editable-factory-definition";

function normalizeSessionQueryKey(sessionID: string | null | undefined): string {
  return sessionID ?? DEFAULT_FACTORY_SESSION_ID;
}

export function currentEditableFactoryDefinitionQueryKey(
  sessionID: string | null | undefined,
) {
  return [
    CURRENT_EDITABLE_FACTORY_DEFINITION_QUERY_KEY_PREFIX,
    normalizeSessionQueryKey(sessionID),
  ] as const;
}

export const CURRENT_EDITABLE_FACTORY_DEFINITION_QUERY_KEY =
  currentEditableFactoryDefinitionQueryKey(DEFAULT_FACTORY_SESSION_ID);

export function useCurrentEditableFactoryDefinition(isEnabled = true) {
  const sessionID = useDashboardSessionStore((state) => state.selectedSessionID);

  return useQuery<CanonicalFactoryDefinition, CurrentEditableFactoryDefinitionError>({
    queryKey: currentEditableFactoryDefinitionQueryKey(sessionID),
    queryFn: () => getCurrentEditableFactoryDefinition({ sessionID }),
    enabled: isEnabled,
    gcTime: 0,
    refetchOnWindowFocus: false,
    retry: false,
  });
}

export function currentFactoryDocumentQueryKey(
  sessionID: string | null | undefined,
) {
  return [
    ...currentEditableFactoryDefinitionQueryKey(sessionID),
    "document",
  ] as const;
}

export const CURRENT_FACTORY_DOCUMENT_QUERY_KEY =
  currentFactoryDocumentQueryKey(DEFAULT_FACTORY_SESSION_ID);

export function useCurrentFactoryDocument(isEnabled = true) {
  const sessionID = useDashboardSessionStore((state) => state.selectedSessionID);

  return useQuery<
    CurrentFactoryDocument,
    CurrentEditableFactoryDefinitionError
  >({
    queryKey: currentFactoryDocumentQueryKey(sessionID),
    queryFn: () => getCurrentFactoryDocument({ sessionID }),
    enabled: isEnabled,
    gcTime: 0,
    refetchOnWindowFocus: false,
    retry: false,
  });
}

export function useSaveCurrentFactory() {
  const queryClient = useQueryClient();
  const sessionID = useDashboardSessionStore((state) => state.selectedSessionID);

  return useMutation<
    CurrentFactoryDocument,
    CurrentEditableFactoryDefinitionError,
    SaveCurrentEditableFactoryDefinitionInput
  >({
    mutationFn: (input) =>
      saveCurrentFactoryDocument(input, { sessionID }),
    onSuccess: (document) => {
      queryClient.setQueryData(
        currentFactoryDocumentQueryKey(sessionID),
        document,
      );
      queryClient.setQueryData(
        currentEditableFactoryDefinitionQueryKey(sessionID),
        document,
      );
    },
  });
}
