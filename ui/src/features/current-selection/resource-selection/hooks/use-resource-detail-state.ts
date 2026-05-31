import { useMemo } from "react";

import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import type { ResourceDetailState } from "../lib/detail-card-types";
import {
  findResourceInFactoryDefinition,
  workerNamesReferencingResourceInFactoryDefinition,
  workstationNamesReferencingResourceInFactoryDefinition,
} from "../lib/resource-detail-values";

export function useResourceDetailState(resourceName: string): ResourceDetailState {
  const factoryDocument = useCurrentFactoryDocument(true);

  return useMemo((): ResourceDetailState => {
    if (factoryDocument.isPending) {
      return { status: "loading" };
    }

    if (factoryDocument.isError) {
      return {
        errorMessage: factoryDocument.error.message,
        status: "error",
      };
    }

    if (!factoryDocument.data) {
      return { status: "empty" };
    }

    const resource = findResourceInFactoryDefinition(
      factoryDocument.data,
      resourceName,
    );
    if (!resource) {
      return { status: "empty" };
    }

    return {
      resource,
      status: "ready",
      workerNames: workerNamesReferencingResourceInFactoryDefinition(
        factoryDocument.data,
        resourceName,
      ),
      workstationNames: workstationNamesReferencingResourceInFactoryDefinition(
        factoryDocument.data,
        resourceName,
      ),
    };
  }, [
    factoryDocument.data,
    factoryDocument.error,
    factoryDocument.isError,
    factoryDocument.isPending,
    resourceName,
  ]);
}
