import { useQuery } from "@tanstack/react-query";

import {
  getCurrentEditableFactoryDefinition,
  type CanonicalFactoryDefinition,
  type CurrentEditableFactoryDefinitionError,
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
