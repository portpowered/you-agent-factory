import { useMemo } from "react";

import type { FactoryWorker } from "../../../../api/events/types";
import type { WorkerDetailState } from "../lib/detail-card-types";

export function useWorkerDetailState({
  worker,
  workstationNames,
}: {
  worker?: FactoryWorker | null;
  workstationNames?: readonly string[] | null;
}): WorkerDetailState {
  return useMemo(
    () => deriveWorkerDetailState({ worker, workstationNames }),
    [worker, workstationNames],
  );
}

export function deriveWorkerDetailState({
  worker,
  workstationNames,
}: {
  worker?: FactoryWorker | null;
  workstationNames?: readonly string[] | null;
}): WorkerDetailState {
  if (!worker) {
    return { status: "empty" };
  }

  return {
    status: "ready",
    worker,
    workstationNames: [...(workstationNames ?? [])],
  };
}
