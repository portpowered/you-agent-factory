import type {
  DashboardRuntime,
} from "../../api/dashboard";
import type { FactoryWorkItem } from "../../api/events";
import {
  cloneFailedWorkDetailsByWorkID,
  cloneInferenceAttemptsByDispatchID,
  cloneProviderSessionAttempts,
  cloneWorkItemRef,
} from "./cloneTimelineSnapshot";
import { projectRuntimeWorkstationRequests } from "./projectWorkstationRequests";
import { uniqueSorted } from "./shared";
import { isSystemTimePlace, isSystemTimeWorkItem } from "./systemTime";
import type { WorldDispatch, WorldState } from "./types";
import { workRef } from "./workItemRef";

export function projectRuntime(state: WorldState): DashboardRuntime {
  const activeDispatches = Object.values(state.activeDispatches);
  const customerActiveDispatches = activeDispatches.filter(dispatchHasCustomerWork);
  const customerCompletedDispatches = state.completedDispatches.filter(dispatchHasCustomerWork);
  const activeExecutions = Object.fromEntries(
    customerActiveDispatches.map((dispatch) => [
      dispatch.dispatchID,
      projectActiveExecution(dispatch),
    ]),
  );
  const activeDispatchIDs = Object.keys(activeExecutions).sort();
  return {
    active_dispatch_ids: activeDispatchIDs,
    active_executions_by_dispatch_id: activeExecutions,
    active_workstation_node_ids: uniqueSorted(
      activeDispatches.map((dispatch) => dispatch.transitionID),
    ),
    inference_attempts_by_dispatch_id: cloneInferenceAttemptsByDispatchID(
      state.inferenceAttemptsByDispatchID,
    ),
    in_flight_dispatch_count: activeDispatchIDs.length,
    place_token_counts: Object.fromEntries(
      Object.values(state.occupancyByID)
        .filter((occupancy) => !isSystemTimePlace(occupancy.placeID))
        .map((occupancy) => [occupancy.placeID, occupancy.tokenCount]),
    ),
    current_work_items_by_place_id: projectCurrentWorkItemsByPlaceID(state),
    place_occupancy_work_items_by_place_id: projectOccupancyWorkItemsByPlaceID(state),
    workstation_requests_by_dispatch_id: projectRuntimeWorkstationRequests({
      activeDispatches: customerActiveDispatches,
      attemptsByDispatchID: state.inferenceAttemptsByDispatchID,
      completedDispatches: customerCompletedDispatches,
      scriptRequestsByDispatchID: state.scriptRequestsByDispatchID,
      scriptResponsesByDispatchID: state.scriptResponsesByDispatchID,
    }),
    session: {
      completed_count: customerCompletedDispatches.filter(
        (dispatch) => dispatch.outcome === "ACCEPTED",
      ).length,
      completed_work_labels: uniqueSorted(
        Object.values(state.terminalWorkByID).map(
          (work) => work.work_item.display_name ?? work.work_item.id,
        ),
      ),
      dispatched_count: activeDispatchIDs.length + customerCompletedDispatches.length,
      failed_by_work_type: countFailedByWorkType(state.failedWorkItemsByID),
      failed_count: countFailedWorkItems(state.failedWorkItemsByID),
      failed_work_details_by_work_id: cloneFailedWorkDetailsByWorkID(
        state.failedWorkDetailsByWorkID,
      ),
      failed_work_labels: uniqueSorted(
        Object.values(state.failedWorkItemsByID).map(
          (work) => work.display_name ?? work.id,
        ),
      ),
      has_data:
        activeDispatchIDs.length > 0 ||
        customerCompletedDispatches.length > 0 ||
        Object.values(state.workItemsByID).some((item) => !isSystemTimeWorkItem(item)),
      provider_sessions: cloneProviderSessionAttempts(state.providerSessions),
    },
    workstation_activity_by_node_id: projectActivity(customerActiveDispatches),
  };
}

function dispatchHasCustomerWork(dispatch: WorldDispatch): boolean {
  return !dispatch.systemOnly;
}

function projectCurrentWorkItemsByPlaceID(state: WorldState): DashboardRuntime["current_work_items_by_place_id"] {
  const workTypeIDs = new Set((state.topology.work_types ?? []).map((workType) => workType.id));
  const entries = (state.topology.places ?? [])
    .filter((place) => workTypeIDs.has(place.type_id))
    .filter((place) => place.category !== "TERMINAL" && place.category !== "FAILED")
    .sort((left, right) => left.id.localeCompare(right.id))
    .map((place) => {
      const occupancy = state.occupancyByID[place.id];
      const workItems = uniqueSorted(occupancy?.workItemIDs ?? [])
        .map((workID) => state.workItemsByID[workID])
        .filter((item): item is FactoryWorkItem => item !== undefined)
        .filter((item) => state.failedWorkItemsByID[item.id] === undefined)
        .filter((item) => state.terminalWorkByID[item.id] === undefined)
        .map(workRef);
      return [place.id, workItems] as const;
    });
  return Object.fromEntries(entries);
}

function projectOccupancyWorkItemsByPlaceID(
  state: WorldState,
): DashboardRuntime["place_occupancy_work_items_by_place_id"] {
  const entries = (state.topology.places ?? [])
    .sort((left, right) => left.id.localeCompare(right.id))
    .map((place) => {
      const occupancy = state.occupancyByID[place.id];
      const workItems = uniqueSorted(occupancy?.workItemIDs ?? [])
        .map((workID) => state.workItemsByID[workID])
        .filter((item): item is FactoryWorkItem => item !== undefined)
        .map(workRef);
      return [place.id, workItems] as const;
    });
  return Object.fromEntries(entries);
}

function projectActiveExecution(
  dispatch: WorldDispatch,
): NonNullable<DashboardRuntime["active_executions_by_dispatch_id"]>[string] {
  return {
    consumed_tokens: dispatch.consumedTokens,
    dispatch_id: dispatch.dispatchID,
    model: dispatch.model,
    model_provider: dispatch.modelProvider,
    provider: dispatch.provider,
    started_at: dispatch.startedAt,
    trace_ids: [...dispatch.traceIDs],
    transition_id: dispatch.transitionID,
    work_items: dispatch.workItems.map(cloneWorkItemRef),
    work_type_ids: uniqueSorted(
      dispatch.workItems.map((item) => item.work_type_id ?? ""),
    ),
    workstation_name: dispatch.workstationName,
    workstation_node_id: dispatch.transitionID,
  };
}

function projectActivity(
  activeDispatches: WorldDispatch[],
): DashboardRuntime["workstation_activity_by_node_id"] {
  return Object.fromEntries(
    activeDispatches.map((dispatch) => [
      dispatch.transitionID,
      {
        active_dispatch_ids: [dispatch.dispatchID],
        active_work_items: dispatch.workItems.map(cloneWorkItemRef),
        trace_ids: [...dispatch.traceIDs],
        workstation_node_id: dispatch.transitionID,
      },
    ]),
  );
}

function countFailedByWorkType(
  values: Record<string, FactoryWorkItem>,
): Record<string, number> | undefined {
  const counts: Record<string, number> = {};
  for (const item of Object.values(values)) {
    if (isSystemTimeWorkItem(item)) {
      continue;
    }
    counts[item.work_type_id] = (counts[item.work_type_id] ?? 0) + 1;
  }
  return Object.keys(counts).length > 0 ? counts : undefined;
}

function countFailedWorkItems(
  values: Record<string, FactoryWorkItem>,
): number {
  let count = 0;
  for (const item of Object.values(values)) {
    if (isSystemTimeWorkItem(item)) {
      continue;
    }
    count += 1;
  }
  return count;
}
