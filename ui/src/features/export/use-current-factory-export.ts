import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";

import {
  getCurrentFactory,
  type FactoryValue,
  NamedFactoryAPIError,
} from "../../api/named-factory";

const CURRENT_FACTORY_EXPORT_QUERY_KEY = ["agent-factory-current-export"] as const;
const CURRENT_FACTORY_UNAVAILABLE_MESSAGE =
  "The current factory definition is not available yet. Wait for the current-factory API to expose the authored definition before exporting.";
const CURRENT_FACTORY_LOAD_FAILED_MESSAGE =
  "The current factory definition could not be loaded from the current-factory API.";

export interface CurrentFactoryExportSuccess {
  factoryDefinition: FactoryValue;
  ok: true;
}

export interface CurrentFactoryExportFailure {
  code: "FACTORY_DEFINITION_UNAVAILABLE";
  message: string;
  ok: false;
}

export type CurrentFactoryExportResult =
  | CurrentFactoryExportFailure
  | CurrentFactoryExportSuccess;

export interface UseCurrentFactoryExportResult {
  currentFactoryExport: CurrentFactoryExportResult;
  isPreparing: boolean;
}

export function useCurrentFactoryExport(isEnabled: boolean): UseCurrentFactoryExportResult {
  const query = useQuery({
    queryKey: CURRENT_FACTORY_EXPORT_QUERY_KEY,
    queryFn: () => getCurrentFactory(),
    enabled: isEnabled,
    gcTime: 0,
    refetchOnWindowFocus: false,
    retry: false,
  });

  return useMemo<UseCurrentFactoryExportResult>(() => {
    const isRefreshingCurrentFactory = isEnabled && query.isFetching;

    if (query.data && !isRefreshingCurrentFactory) {
      return {
        currentFactoryExport: {
          factoryDefinition: query.data,
          ok: true,
        },
        isPreparing: false,
      };
    }

    if (query.isPending || isRefreshingCurrentFactory) {
      return {
        currentFactoryExport: {
          code: "FACTORY_DEFINITION_UNAVAILABLE",
          message: CURRENT_FACTORY_UNAVAILABLE_MESSAGE,
          ok: false,
        },
        isPreparing: true,
      };
    }

    return {
      currentFactoryExport: {
        code: "FACTORY_DEFINITION_UNAVAILABLE",
        message: currentFactoryExportFailureMessage(query.error),
        ok: false,
      },
      isPreparing: false,
    };
  }, [isEnabled, query.data, query.error, query.isFetching, query.isPending]);
}

function currentFactoryExportFailureMessage(error: unknown): string {
  if (error instanceof NamedFactoryAPIError && error.code === "NOT_FOUND") {
    return CURRENT_FACTORY_UNAVAILABLE_MESSAGE;
  }

  if (error instanceof Error && error.message.trim().length > 0) {
    return `${CURRENT_FACTORY_LOAD_FAILED_MESSAGE} ${error.message}`;
  }

  return CURRENT_FACTORY_LOAD_FAILED_MESSAGE;
}

