import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  getCurrentEditableFactoryDefinition,
  getCurrentEditableFactoryDefinitionDocument,
  saveCurrentEditableFactoryDefinitionDocument,
  type CanonicalFactoryDefinition,
  type CurrentEditableFactoryDefinitionError,
  type EditableFactoryDefinitionDocument,
  type SaveCurrentEditableFactoryDefinitionInput,
} from "../../api/current-factory-definition";

export const CURRENT_EDITABLE_FACTORY_DEFINITION_QUERY_KEY = [
  "current-editable-factory-definition",
] as const;

export function useCurrentEditableFactoryDefinition(isEnabled = true) {
  return useQuery<CanonicalFactoryDefinition, CurrentEditableFactoryDefinitionError>({
    queryKey: CURRENT_EDITABLE_FACTORY_DEFINITION_QUERY_KEY,
    queryFn: () => getCurrentEditableFactoryDefinition(),
    enabled: isEnabled,
    gcTime: 0,
    refetchOnWindowFocus: false,
    retry: false,
  });
}

export const CURRENT_EDITABLE_FACTORY_DEFINITION_DOCUMENT_QUERY_KEY = [
  ...CURRENT_EDITABLE_FACTORY_DEFINITION_QUERY_KEY,
  "document",
] as const;

export function useCurrentEditableFactoryDefinitionDocument(isEnabled = true) {
  return useQuery<
    EditableFactoryDefinitionDocument,
    CurrentEditableFactoryDefinitionError
  >({
    queryKey: CURRENT_EDITABLE_FACTORY_DEFINITION_DOCUMENT_QUERY_KEY,
    queryFn: () => getCurrentEditableFactoryDefinitionDocument(),
    enabled: isEnabled,
    gcTime: 0,
    refetchOnWindowFocus: false,
    retry: false,
  });
}

export function useSaveCurrentEditableFactoryDefinition() {
  const queryClient = useQueryClient();

  return useMutation<
    EditableFactoryDefinitionDocument,
    CurrentEditableFactoryDefinitionError,
    SaveCurrentEditableFactoryDefinitionInput
  >({
    mutationFn: (input) => saveCurrentEditableFactoryDefinitionDocument(input),
    onSuccess: (document) => {
      queryClient.setQueryData(
        CURRENT_EDITABLE_FACTORY_DEFINITION_DOCUMENT_QUERY_KEY,
        document,
      );
      queryClient.setQueryData(
        CURRENT_EDITABLE_FACTORY_DEFINITION_QUERY_KEY,
        document.factoryDefinition,
      );
    },
  });
}
