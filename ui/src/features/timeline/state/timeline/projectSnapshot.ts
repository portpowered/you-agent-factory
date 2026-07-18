import {
  cloneRelationsByWorkID,
  cloneTracesByWorkID,
  cloneWorkRequestsByID,
} from "./cloneTimelineSnapshot";
import { projectHostedDashboard } from "./projections/projectFactoryReplay";
import { projectWorkstationDispatchRequestsByID } from "./projectWorkstationRequests";
import { isSystemTimeWorkType } from "./systemTime";
import type { ReplayWorldState, WorldState } from "./types";

export function projectSnapshot(state: ReplayWorldState): WorldState {
  const hostedDashboard = projectHostedDashboard(state);
  const { runtime } = hostedDashboard;

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
    factoryReplay: hostedDashboard.factoryReplay,
    factory_state: state.factory_state,
    factory: state.factory ? structuredClone(state.factory) : undefined,
    runtime,
    tick_count: state.tick_count,
    topology: hostedDashboard.topology,
    uptime_seconds: state.uptime_seconds,
    relationsByWorkID: cloneRelationsByWorkID(state.relationsByWorkID),
    tracesByWorkID: cloneTracesByWorkID(tracesByWorkID),
    workstationRequestsByDispatchID: projectWorkstationDispatchRequestsByID({
      activeDispatches: state.activeDispatches,
      completedDispatches: state.completedDispatches,
      inferenceAttemptsByDispatchID: state.inferenceAttemptsByDispatchID,
      runtimeRequestsByDispatchID:
        runtime.workstation_requests_by_dispatch_id ?? {},
      scriptRequestsByDispatchID: state.scriptRequestsByDispatchID,
      scriptResponsesByDispatchID: state.scriptResponsesByDispatchID,
      textBlobsByID: state.textBlobsByID,
      workRequestsByID: publicWorkRequestsByID,
    }),
    workRequestsByID: publicWorkRequestsByID,
  };
}
