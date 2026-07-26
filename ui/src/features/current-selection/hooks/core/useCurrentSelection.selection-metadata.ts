import { useMemo } from "react";
import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import type { DashboardSnapshot } from "../../../../api/dashboard/types";
import type { FactoryWorker } from "../../../../api/events/types";
import {
  findFactoryWorkerInSnapshot,
  findFactoryWorkTypeInSnapshot,
  workstationNamesReferencingWorkerInSnapshot,
} from "../../state/dashboardSelection";
import type { DashboardSelection } from "../../state/selection-types";

export function resolveCurrentFactoryDocumentFromSnapshot(
  snapshot: DashboardSnapshot | null | undefined,
): CurrentFactoryDocument | null {
  const factory = snapshot?.factory;
  if (!factory?.version) {
    return null;
  }

  return factory as CurrentFactoryDocument;
}

export function useSelectedWorkerAndWorkTypeData(
  selection: DashboardSelection | null,
  snapshot: DashboardSnapshot | null | undefined,
) {
  return useMemo(
    () => selectWorkerAndWorkTypeData(selection, snapshot),
    [selection, snapshot],
  );
}

export function selectWorkerAndWorkTypeData(
  selection: DashboardSelection | null,
  snapshot: DashboardSnapshot | null | undefined,
) {
  const selectedWorkerName =
    selection?.kind === "worker" ? selection.workerName : null;
  const selectedWorkTypeName =
    selection?.kind === "work-type" ? selection.workTypeName : null;
  const selectedWorkType = (() => {
    if (!snapshot || !selectedWorkTypeName) {
      return null;
    }

    return (
      findFactoryWorkTypeInSnapshot(snapshot, selectedWorkTypeName) ?? null
    );
  })();
  const selectedWorker = ((): FactoryWorker | null => {
    if (!snapshot || !selectedWorkerName) {
      return null;
    }

    return findFactoryWorkerInSnapshot(snapshot, selectedWorkerName) ?? null;
  })();
  const selectedWorkerWorkstationNames = (() => {
    if (!snapshot || !selectedWorkerName) {
      return [];
    }

    return workstationNamesReferencingWorkerInSnapshot(
      snapshot,
      selectedWorkerName,
    );
  })();

  return {
    selectedWorker,
    selectedWorkerName,
    selectedWorkerWorkstationNames,
    selectedWorkType,
    selectedWorkTypeName,
  };
}
