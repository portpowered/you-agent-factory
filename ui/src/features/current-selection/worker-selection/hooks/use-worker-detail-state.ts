import { useMemo } from "react";

import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import type { WorkerDetailState } from "../lib/detail-card-types";
import {
  findWorkerInFactoryDefinition,
  workstationNamesReferencingWorkerInFactoryDefinition,
} from "../lib/worker-detail-values";

export function useWorkerDetailState(workerName: string): WorkerDetailState {
  const factoryDocument = useCurrentFactoryDocument(true);

  return useMemo((): WorkerDetailState => {
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

    const worker = findWorkerInFactoryDefinition(
      factoryDocument.data,
      workerName,
    );
    if (!worker) {
      return { status: "empty" };
    }

    return {
      status: "ready",
      worker,
      workstationNames: workstationNamesReferencingWorkerInFactoryDefinition(
        factoryDocument.data,
        workerName,
      ),
    };
  }, [
    factoryDocument.data,
    factoryDocument.error,
    factoryDocument.isError,
    factoryDocument.isPending,
    workerName,
  ]);
}
