import { useMemo } from "react";

import {
  CurrentFactoryDefinitionError,
  type CurrentFactoryDocument,
} from "../../../api/current-factory-definition";
import type { FactoryValue } from "../../../api/named-factory";
import {
  useCurrentFactoryDocument,
} from "../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { useDashboardSession } from "../../dashboard/session/dashboard-session-provider";

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
  const { rawSessionID } = useDashboardSession();
  const isQueryEnabled = isEnabled && rawSessionID != null;
  const documentQuery = useCurrentFactoryDocument(isQueryEnabled);

  return useMemo<UseCurrentFactoryExportResult>(() => {
    if (!isEnabled) {
      return {
        currentFactoryExport: {
          code: "FACTORY_DEFINITION_UNAVAILABLE",
          message: CURRENT_FACTORY_UNAVAILABLE_MESSAGE,
          ok: false,
        },
        isPreparing: true,
      };
    }

    const isRefreshingCurrentFactory = isQueryEnabled && documentQuery.isFetching;

    if (rawSessionID == null) {
      return {
        currentFactoryExport: {
          code: "FACTORY_DEFINITION_UNAVAILABLE",
          message: CURRENT_FACTORY_UNAVAILABLE_MESSAGE,
          ok: false,
        },
        isPreparing: false,
      };
    }

    if (documentQuery.data && !isRefreshingCurrentFactory) {
      return {
        currentFactoryExport: {
          factoryDefinition: currentFactoryDocumentToExportValue(documentQuery.data),
          ok: true,
        },
        isPreparing: false,
      };
    }

    if (isQueryEnabled && (documentQuery.isPending || isRefreshingCurrentFactory)) {
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
        message: currentFactoryExportFailureMessage(documentQuery.error),
        ok: false,
      },
      isPreparing: false,
    };
  }, [
    documentQuery.data,
    documentQuery.error,
    documentQuery.isFetching,
    documentQuery.isPending,
    isEnabled,
    isQueryEnabled,
    rawSessionID,
  ]);
}

function currentFactoryDocumentToExportValue(
  document: CurrentFactoryDocument,
): FactoryValue {
  const { version: _version, ...factoryValue } = document;
  return factoryValue;
}

function currentFactoryExportFailureMessage(error: unknown): string {
  if (error instanceof CurrentFactoryDefinitionError && error.code === "NOT_FOUND") {
    return CURRENT_FACTORY_UNAVAILABLE_MESSAGE;
  }

  if (error instanceof Error && error.message.trim().length > 0) {
    return `${CURRENT_FACTORY_LOAD_FAILED_MESSAGE} ${error.message}`;
  }

  return CURRENT_FACTORY_LOAD_FAILED_MESSAGE;
}
