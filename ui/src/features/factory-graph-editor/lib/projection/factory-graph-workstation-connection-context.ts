import type { WorkstationProgressOutcomeRouteContext } from "../../../current-factory-definition/lib/workstation-progress-outcome-routes";
import type {
  FactoryWorker,
  FactoryWorkstation,
} from "../draft/factory-graph-draft-types";

export function resolveAssignedWorkerStopToken(
  workstation: Pick<FactoryWorkstation, "worker">,
  workers: readonly FactoryWorker[] | undefined,
): string | undefined {
  const workerName = (workstation.worker ?? "").trim();
  if (!workerName) {
    return undefined;
  }

  return (workers ?? []).find((candidate) => candidate.name === workerName)
    ?.stopToken;
}

export function toWorkstationProgressOutcomeRouteContext(
  workstation: FactoryWorkstation,
  workers?: readonly FactoryWorker[] | undefined,
): WorkstationProgressOutcomeRouteContext {
  const assignedWorkerStopToken = resolveAssignedWorkerStopToken(
    workstation,
    workers,
  );
  if (assignedWorkerStopToken === undefined) {
    return {
      behavior: workstation.behavior,
      classificationRoutes: workstation.classificationRoutes,
      stopWords: workstation.stopWords,
      type: workstation.type,
    };
  }

  return {
    assignedWorkerStopToken,
    behavior: workstation.behavior,
    classificationRoutes: workstation.classificationRoutes,
    stopWords: workstation.stopWords,
    type: workstation.type,
  };
}
