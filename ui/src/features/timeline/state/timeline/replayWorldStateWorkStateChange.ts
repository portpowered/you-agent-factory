import type { FactoryWorkItem } from "../../../../api/events";
import { addToken, removeWorkToken, removeWorkTokenFromPlace } from "./replayGraphState";
import type { WorkStateChangeEvent } from "./replayWorldStateTypes";
import type { ReplayWorldState } from "./types";

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
