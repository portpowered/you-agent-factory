import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";

import {
  discoverSessionNamedFactoryNames,
  getSessionFactory,
  resolveImportCreateFactoryName,
} from "../../../api/session-factory";

export interface UseFactoryImportActivationTargetOptions {
  enabled?: boolean;
  preferredFactoryName?: string | null;
  sessionID?: string | null;
}

export interface FactoryImportActivationTarget {
  createTargetFactoryName: string | null;
  currentFactoryName: string | null;
  existingNamedFactoryNames: string[];
  isLoading: boolean;
  replacesExistingCreateTarget: boolean;
}

export function useFactoryImportActivationTarget({
  enabled = true,
  preferredFactoryName,
  sessionID,
}: UseFactoryImportActivationTargetOptions = {}): FactoryImportActivationTarget {
  const currentFactoryQuery = useQuery({
    enabled,
    queryFn: async () => {
      const document = await getSessionFactory(sessionID?.trim() || "~default");
      return { name: document.name };
    },
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
