import { useMemo } from "react";

import type {
  DashboardActiveExecution,
  DashboardSnapshot,
} from "../../api/dashboard/types";

export function useActiveExecutions(snapshot: DashboardSnapshot) {
  return useMemo(
    () =>
      snapshot.runtime.active_dispatch_ids
        ?.map(
          (dispatchId) =>
            snapshot.runtime.active_executions_by_dispatch_id?.[dispatchId],
        )
        .filter(
          (execution): execution is DashboardActiveExecution =>
            execution !== undefined,
        ) ?? [],
    [
      snapshot.runtime.active_dispatch_ids,
      snapshot.runtime.active_executions_by_dispatch_id,
    ],
  );
}

export function groupActiveExecutionsByWorkstationNodeID(
  activeExecutions: DashboardActiveExecution[],
) {
  return activeExecutions.reduce<Record<string, DashboardActiveExecution[]>>(
    (accumulator, execution) => {
      const executions = accumulator[execution.workstation_node_id] ?? [];
      accumulator[execution.workstation_node_id] = [...executions, execution];
      return accumulator;
    },
    {},
  );
}
