import {
  cloneRelationsByWorkID,
  cloneTracesByWorkID,
  cloneWorkRequestsByID,
  cloneWorkstationDispatchRequestsByID,
} from "./cloneTimelineSnapshot";
import { projectWorkstationDispatchRequestsByID } from "./projectWorkstationRequests";
import { projectRuntime } from "./projectRuntime";
import { projectTopology } from "./projectTopology";
import { isSystemTimeWorkType } from "./systemTime";
import type { ReplayWorldState, WorldState } from "./types";

export function projectSnapshot(state: ReplayWorldState): WorldState {
  const runtime = projectRuntime(state);

  const tracesByWorkID = Object.fromEntries(
    Object.values(state.tracesByID).flatMap((trace) =>
      trace.work_ids.map((workID) => [workID, trace] as const),
    ),
  );
  const workRequestsByID = cloneWorkRequestsByID(state.workRequestsByID);

  for (const request of Object.values(workRequestsByID)) {
    if (!request.work_items) {
      continue;
    }
    request.work_items = request.work_items.filter(
      (item) => !isSystemTimeWorkType(item.work_type_id),
    );
  }

  const publicWorkRequestsByID = Object.fromEntries(
    Object.entries(workRequestsByID).filter(([, request]) => {
      const workItems = request.work_items ?? [];
      return workItems.length > 0;
    }),
  );

  return {
    factory_state: state.factory_state,
    runtime,
    tick_count: state.tick_count,
    topology: projectTopology(state.topology),
    uptime_seconds: state.uptime_seconds,
    relationsByWorkID: cloneRelationsByWorkID(state.relationsByWorkID),
    tracesByWorkID: cloneTracesByWorkID(tracesByWorkID),
    workstationRequestsByDispatchID: cloneWorkstationDispatchRequestsByID(
      projectWorkstationDispatchRequestsByID({
        activeDispatches: state.activeDispatches,
        completedDispatches: state.completedDispatches,
        inferenceAttemptsByDispatchID: state.inferenceAttemptsByDispatchID,
        runtimeRequestsByDispatchID: runtime.workstation_requests_by_dispatch_id ?? {},
        scriptRequestsByDispatchID: state.scriptRequestsByDispatchID,
        scriptResponsesByDispatchID: state.scriptResponsesByDispatchID,
        workRequestsByID: publicWorkRequestsByID,
      }),
    ),
    workRequestsByID: publicWorkRequestsByID,
  };
}


