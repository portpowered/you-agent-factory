import { useQuery } from "@tanstack/react-query";

import {
  validateFactoryDefinition,
  type FactoryValidationAPIError,
  type FactoryValidationResult,
} from "../../../api/factory-validation";
import type { CanonicalFactoryDefinition } from "../lib/factory-graph-draft-types";
import { projectFactoryValidationTargets } from "../lib/factory-validation-graph-projection";

export const FACTORY_VALIDATION_QUERY_KEY_PREFIX = "factory-validation";

export function factoryValidationQueryKey(
  factoryDefinition: CanonicalFactoryDefinition | null,
) {
  return [FACTORY_VALIDATION_QUERY_KEY_PREFIX, factoryDefinition] as const;
}

export function useFactoryValidation(
  factoryDefinition: CanonicalFactoryDefinition | null,
  enabled: boolean,
) {
  const query = useQuery<
    FactoryValidationResult,
    FactoryValidationAPIError
  >({
    queryKey: factoryValidationQueryKey(factoryDefinition),
    queryFn: () => validateFactoryDefinition(factoryDefinition!),
    enabled: enabled && factoryDefinition != null,
    gcTime: 0,
    refetchOnWindowFocus: false,
    retry: false,
    staleTime: 0,
  });

  const projection = projectFactoryValidationTargets(query.data?.targets ?? []);

  return {
    ...query,
    projection,
    targets: query.data?.targets ?? [],
  };
}
