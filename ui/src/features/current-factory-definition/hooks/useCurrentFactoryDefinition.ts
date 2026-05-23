import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import {
  getCurrentFactoryDefinition,
  getCurrentFactoryDocument,
  saveCurrentFactoryDocument,
  type CanonicalFactoryDefinition,
  type CurrentFactoryDefinitionError,
  type CurrentFactoryDocument,
  type SaveCurrentFactoryInput,
} from "../../../api/current-factory-definition";
import { useDashboardSessionStore } from "../../dashboard/state/dashboardSessionStore";

export const CURRENT_FACTORY_DEFINITION_QUERY_KEY_PREFIX =
  "current-factory-definition";

function normalizeSessionQueryKey(sessionID: string | null | undefined): string {
  return sessionID ?? DEFAULT_FACTORY_SESSION_ID;
}

export function currentFactoryDefinitionQueryKey(
  sessionID: string | null | undefined,
) {
  return [
    CURRENT_FACTORY_DEFINITION_QUERY_KEY_PREFIX,
    normalizeSessionQueryKey(sessionID),
  ] as const;
}

export const CURRENT_FACTORY_DEFINITION_QUERY_KEY =
  currentFactoryDefinitionQueryKey(DEFAULT_FACTORY_SESSION_ID);

export function useCurrentFactoryDefinition(isEnabled = true) {
  const sessionID = useDashboardSessionStore((state) => state.selectedSessionID);

  return useQuery<CanonicalFactoryDefinition, CurrentFactoryDefinitionError>({
    queryKey: currentFactoryDefinitionQueryKey(sessionID),
    queryFn: () => getCurrentFactoryDefinition({ sessionID }),
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
    ...currentFactoryDefinitionQueryKey(sessionID),
    "document",
  ] as const;
}

export const CURRENT_FACTORY_DOCUMENT_QUERY_KEY =
  currentFactoryDocumentQueryKey(DEFAULT_FACTORY_SESSION_ID);

export function useCurrentFactoryDocument(isEnabled = true) {
  const sessionID = useDashboardSessionStore((state) => state.selectedSessionID);

  return useQuery<
    CurrentFactoryDocument,
    CurrentFactoryDefinitionError
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
    CurrentFactoryDefinitionError,
    SaveCurrentFactoryInput
  >({
    mutationFn: (input) =>
      saveCurrentFactoryDocument(input, { sessionID }),
    onSuccess: (document) => {
      queryClient.setQueryData(
        currentFactoryDocumentQueryKey(sessionID),
        document,
      );
      queryClient.setQueryData(
        currentFactoryDefinitionQueryKey(sessionID),
        document,
      );
    },
  });
}
