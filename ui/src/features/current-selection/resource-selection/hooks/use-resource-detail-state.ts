import { useMemo } from "react";

import type { FactoryResource } from "../../../../api/events/types";
import type { ResourceDetailState } from "../lib/detail-card-types";

export function useResourceDetailState({
  resource,
  workerNames,
  workstationNames,
}: {
  resource?: FactoryResource | null;
  workerNames?: readonly string[] | null;
  workstationNames?: readonly string[] | null;
}): ResourceDetailState {
  return useMemo(
    () =>
      deriveResourceDetailState({ resource, workerNames, workstationNames }),
    [resource, workerNames, workstationNames],
  );
}

export function deriveResourceDetailState({
  resource,
  workerNames,
  workstationNames,
}: {
  resource?: FactoryResource | null;
  workerNames?: readonly string[] | null;
  workstationNames?: readonly string[] | null;
}): ResourceDetailState {
  if (!resource) {
    return { status: "empty" };
  }

  return {
    resource,
    status: "ready",
    workerNames: [...(workerNames ?? [])],
    workstationNames: [...(workstationNames ?? [])],
  };
}
