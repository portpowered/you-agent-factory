import type {
  DashboardRuntime,
  DashboardWorkMoveOperation,
} from "../../../../api/dashboard";

export function projectWorkMoveOperationsByWorkID(
  workStateChangesByWorkID:
    | Record<string, DashboardWorkMoveOperation[]>
    | undefined,
): DashboardRuntime["work_move_operations_by_work_id"] {
  if (!workStateChangesByWorkID) {
    return undefined;
  }

  const workIDs = Object.keys(workStateChangesByWorkID).sort();
  const operationsByWorkID: Record<string, DashboardWorkMoveOperation[]> = {};
  for (const workID of workIDs) {
    const records = workStateChangesByWorkID[workID];
    if (!records?.length) {
      continue;
    }
    operationsByWorkID[workID] = records.map((record) => ({ ...record }));
  }

  return Object.keys(operationsByWorkID).length > 0
    ? operationsByWorkID
    : undefined;
}
