import type {
  DashboardWorkMoveOperation,
  DashboardWorkMoveOperationSource,
} from "../../../../api/dashboard";
import type { FactoryWorkItem } from "../../../../api/events";
import { addToken, removeWorkToken, removeWorkTokenFromPlace } from "./replayGraphState";
import type { WorkStateChangeEvent } from "./replayWorldStateTypes";
import type { ReplayWorldState } from "./types";

function recordWorkStateChange(
  state: ReplayWorldState,
  event: WorkStateChangeEvent,
): void {
  const payload = event.payload;
  const workID = payload.workId;
  if (!workID) {
    return;
  }

  const record: DashboardWorkMoveOperation = {
    work_id: workID,
    work_type_name: payload.workTypeName,
    from_state: payload.fromState,
    to_state: payload.toState,
    from_place_id: payload.fromPlaceId,
    to_place_id: payload.toPlaceId,
    source: payload.source as DashboardWorkMoveOperationSource,
    request_id: event.context.requestId,
    tick: event.context.tick,
    sequence: event.context.sequence,
    event_time: event.context.eventTime,
  };

  if (!state.workStateChangesByWorkID) {
    state.workStateChangesByWorkID = {};
  }
  state.workStateChangesByWorkID[workID] = [
    ...(state.workStateChangesByWorkID[workID] ?? []),
    record,
  ];
}

function placeCategory(
  state: ReplayWorldState,
  placeIDValue: string | undefined,
): string | undefined {
  return state.topology.places?.find((place) => place.id === placeIDValue)?.category;
}

export function applyWorkStateChange(
  state: ReplayWorldState,
  event: WorkStateChangeEvent,
): void {
  const payload = event.payload;
  const workID = payload.workId;
  if (!workID) {
    return;
  }

  recordWorkStateChange(state, event);

  const existing = state.workItemsByID[workID];
  let item: FactoryWorkItem = existing ?? {
    id: workID,
    work_type_id: payload.workTypeName ?? "",
  };
  if (payload.workTypeName) {
    item = {
      ...item,
      work_type_id: item.work_type_id || payload.workTypeName,
    };
  }

  const toPlaceID = payload.toPlaceId;
  const fromPlaceID = payload.fromPlaceId;
  if (fromPlaceID && fromPlaceID !== toPlaceID) {
    if (placeCategory(state, fromPlaceID) === "FAILED") {
      delete state.failedWorkItemsByID[workID];
    }
    if (placeCategory(state, fromPlaceID) === "TERMINAL") {
      delete state.terminalWorkByID[workID];
    }
    removeWorkTokenFromPlace(state, workID, fromPlaceID);
  } else if (fromPlaceID === undefined) {
    removeWorkToken(state, workID);
  }

  if (toPlaceID) {
    item = { ...item, place_id: toPlaceID };
  }

  state.workItemsByID[workID] = item;
  if (toPlaceID) {
    addToken(state, toPlaceID, workID, workID);
    if (placeCategory(state, toPlaceID) === "FAILED") {
      state.failedWorkItemsByID[workID] = item;
    } else if (placeCategory(state, toPlaceID) === "TERMINAL") {
      state.terminalWorkByID[workID] = { status: "TERMINAL", work_item: item };
    }
  }
}
