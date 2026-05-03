import {
  cloneRelationsByWorkID,
  cloneTracesByWorkID,
  cloneWorkRequestsByID,
  cloneWorkstationDispatchRequestsByID,
} from "./cloneTimelineSnapshot";
import { projectWorkstationDispatchRequestsByID } from "./projectWorkstationRequests";
import { projectRuntime } from "./projectRuntime";
import { projectTopology } from "./projectTopology";
import type { FactoryTimelineSnapshot } from "./snapshotTypes";
import type { WorldState } from "./types";

export function projectSnapshot(state: WorldState): FactoryTimelineSnapshot {
  const runtime = projectRuntime(state);

  const tracesByWorkID = Object.fromEntries(
    Object.values(state.tracesByID).flatMap((trace) =>
      trace.work_ids.map((workID) => [workID, trace] as const),
    ),
  );

  return {
    dashboard: {
      factory_state: state.factoryState,
      runtime,
      tick_count: state.tick,
      topology: projectTopology(state.topology),
      uptime_seconds: 0,
    },
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
        workRequestsByID: state.workRequestsByID,
      }),
    ),
    workRequestsByID: cloneWorkRequestsByID(state.workRequestsByID),
  };
}


