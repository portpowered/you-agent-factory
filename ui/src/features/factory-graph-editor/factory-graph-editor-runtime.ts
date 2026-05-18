import type { DashboardSnapshot } from "../../api/dashboard/types";
import type { CanonicalFactoryDefinition } from "./factory-graph-draft-types";

export type FactoryGraphWorkerRuntimeStatus =
  | "active"
  | "errored"
  | "idle"
  | "unavailable";

const WORKER_STATUS_PRIORITY: Record<FactoryGraphWorkerRuntimeStatus, number> = {
  active: 4,
  unavailable: 3,
  errored: 2,
  idle: 1,
};

export function buildFactoryGraphWorkerStatusMap(input: {
  factoryDefinition: CanonicalFactoryDefinition | null;
  snapshot: DashboardSnapshot;
}) {
  const workerStatus = new Map<string, FactoryGraphWorkerRuntimeStatus>();
  const factoryDefinition = input.factoryDefinition;

  for (const worker of factoryDefinition?.workers ?? []) {
    workerStatus.set(worker.name, "idle");
  }

  if (!factoryDefinition) {
    return workerStatus;
  }

  const workerByWorkstationName = new Map<string, string>();
  for (const workstation of factoryDefinition.workstations ?? []) {
    if (workstation.worker.trim().length === 0) {
      continue;
    }
    workerByWorkstationName.set(workstation.name, workstation.worker);
  }

  for (const pause of input.snapshot.runtime.active_throttle_pauses ?? []) {
    for (const workerType of pause.affected_worker_types ?? []) {
      promoteWorkerStatus(workerStatus, workerType, "unavailable");
    }
  }

  for (const request of Object.values(
    input.snapshot.runtime.workstation_requests_by_dispatch_id ?? {},
  )) {
    const workerName = request.workstation_name
      ? workerByWorkstationName.get(request.workstation_name)
      : undefined;
    if (!workerName) {
      continue;
    }
    if (workstationRequestHasError(request)) {
      promoteWorkerStatus(workerStatus, workerName, "errored");
    }
  }

  for (const execution of Object.values(
    input.snapshot.runtime.active_executions_by_dispatch_id ?? {},
  )) {
    const workerName = execution.workstation_name
      ? workerByWorkstationName.get(execution.workstation_name)
      : undefined;
    if (!workerName) {
      continue;
    }
    promoteWorkerStatus(workerStatus, workerName, "active");
  }

  return workerStatus;
}

export function describeFactoryGraphWorkerStatus(
  status: FactoryGraphWorkerRuntimeStatus,
) {
  switch (status) {
    case "active":
      return "Active";
    case "errored":
      return "Errored";
    case "idle":
      return "Idle";
    case "unavailable":
      return "Unavailable";
  }
}

function promoteWorkerStatus(
  statusMap: Map<string, FactoryGraphWorkerRuntimeStatus>,
  workerName: string,
  nextStatus: FactoryGraphWorkerRuntimeStatus,
) {
  const currentStatus = statusMap.get(workerName);
  if (
    currentStatus &&
    WORKER_STATUS_PRIORITY[currentStatus] >= WORKER_STATUS_PRIORITY[nextStatus]
  ) {
    return;
  }
  statusMap.set(workerName, nextStatus);
}

function workstationRequestHasError(
  request: NonNullable<
    DashboardSnapshot["runtime"]["workstation_requests_by_dispatch_id"]
  >[string],
) {
  return Boolean(
    request.response?.failure_reason ||
      request.response?.failureReason ||
      request.response?.failure_message ||
      request.response?.failureMessage ||
      request.counts.errored_count ||
      request.counts.erroredCount ||
      (request.response?.outcome ?? "").toLowerCase() === "failed",
  );
}
