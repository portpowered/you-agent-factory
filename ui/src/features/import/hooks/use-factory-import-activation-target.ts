import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";

import {
  discoverSessionNamedFactoryNames,
  getCurrentFactory,
  resolveImportCreateFactoryName,
} from "../../../api/named-factory";

export interface UseFactoryImportActivationTargetOptions {
  enabled?: boolean;
  preferredFactoryName?: string | null;
  sessionID?: string | null;
}

export function useFactoryImportActivationTarget({
  enabled = true,
  preferredFactoryName,
  sessionID,
}: UseFactoryImportActivationTargetOptions = {}) {
  const currentFactoryQuery = useQuery({
    enabled,
    queryFn: () => getCurrentFactory({ sessionID }),
    queryKey: ["factory-import-current-factory", sessionID ?? "~default"],
  });
  const existingNamesQuery = useQuery({
    enabled,
    queryFn: () => discoverSessionNamedFactoryNames({ sessionID }),
    queryKey: ["factory-import-named-factories", sessionID ?? "~default"],
  });

  const createTarget = useMemo(() => {
    if (!preferredFactoryName?.trim()) {
      return null;
    }
    return resolveImportCreateFactoryName(
      preferredFactoryName,
      existingNamesQuery.data ?? [],
    );
  }, [existingNamesQuery.data, preferredFactoryName]);

  return {
    createTargetFactoryName: createTarget?.factoryName ?? null,
    currentFactoryName: currentFactoryQuery.data?.name ?? null,
    existingNamedFactoryNames: existingNamesQuery.data ?? [],
    isLoading: currentFactoryQuery.isLoading || existingNamesQuery.isLoading,
    replacesExistingCreateTarget: createTarget?.replacesExisting ?? false,
  };
}
