import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState } from "react";

import {
  type FactoryValidationAPIError,
  type FactoryValidationResult,
  validateFactoryDefinition,
} from "../../../../api/factory-validation";
import type { CanonicalFactoryDefinition } from "../../lib/draft/factory-graph-draft-types";
import { projectFactoryValidationTargets } from "../../lib/projection/factory-validation-graph-projection";
import { serializeFactoryValidationDefinition } from "../../lib/projection/factory-validation-query-key";

export const FACTORY_VALIDATION_QUERY_KEY_PREFIX = "factory-validation";
export const FACTORY_VALIDATION_DEBOUNCE_MS = 200;

export interface FactoryValidationOptions {
  debounceMs?: number;
  validateDefinition?: typeof validateFactoryDefinition;
}

export function factoryValidationQueryKey(serializedDefinition: string | null) {
  return [FACTORY_VALIDATION_QUERY_KEY_PREFIX, serializedDefinition] as const;
}

function useDebouncedFactoryValidationDefinition(
  factoryDefinition: CanonicalFactoryDefinition | null,
  debounceMs: number,
) {
  const [debouncedDefinition, setDebouncedDefinition] =
    useState<CanonicalFactoryDefinition | null>(factoryDefinition);

  useEffect(() => {
    if (factoryDefinition == null) {
      setDebouncedDefinition(null);
      return;
    }

    const timer = window.setTimeout(() => {
      setDebouncedDefinition(factoryDefinition);
    }, debounceMs);

    return () => {
      window.clearTimeout(timer);
    };
  }, [debounceMs, factoryDefinition]);

  return debouncedDefinition;
}

export function useFactoryValidation(
  factoryDefinition: CanonicalFactoryDefinition | null,
  enabled: boolean,
  options?: FactoryValidationOptions,
) {
  const debouncedDefinition = useDebouncedFactoryValidationDefinition(
    factoryDefinition,
    options?.debounceMs ?? FACTORY_VALIDATION_DEBOUNCE_MS,
  );
  const serializedDefinition = useMemo(
    () => serializeFactoryValidationDefinition(debouncedDefinition),
    [debouncedDefinition],
  );
  const latestSerializedDefinitionRef = useRef(serializedDefinition);
  latestSerializedDefinitionRef.current = serializedDefinition;

  const query = useQuery<FactoryValidationResult, FactoryValidationAPIError>({
    queryKey: factoryValidationQueryKey(serializedDefinition),
    queryFn: async ({ signal }) => {
      if (debouncedDefinition == null || serializedDefinition == null) {
        throw new Error(
          "factory validation requires a draft-applied definition",
        );
      }

      const requestedSerializedDefinition = serializedDefinition;
      const result = await (options?.validateDefinition ??
        validateFactoryDefinition)(debouncedDefinition, {
        signal,
      });

      if (
        requestedSerializedDefinition !== latestSerializedDefinitionRef.current
      ) {
        throw new DOMException(
          "Factory validation response is stale.",
          "AbortError",
        );
      }

      return result;
    },
    enabled:
      enabled && debouncedDefinition != null && serializedDefinition != null,
    gcTime: 0,
    refetchOnWindowFocus: false,
    retry: false,
    staleTime: 0,
  });

  const isValidationCurrent =
    serializedDefinition != null &&
    serializedDefinition === latestSerializedDefinitionRef.current;
  const targets =
    isValidationCurrent && query.data?.targets ? query.data.targets : [];
  const projection = projectFactoryValidationTargets(targets);

  return {
    ...query,
    projection,
    targets,
    validationDefinitionKey: serializedDefinition,
  };
}
