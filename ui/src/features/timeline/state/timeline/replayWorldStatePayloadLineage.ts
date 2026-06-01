import type { FactoryWorkItem } from "../../../../api/events";
import { isSystemTimeWorkItem } from "./systemTime";
import type { ReplayWorldState, WorldDispatch } from "./types";
import {
  recordConsumedInputSnapshot,
  recordDispatchOutputSnapshot,
  recordWorkRequestSnapshot,
} from "./workPayloadLineage";

export function recordWorkRequestPayloadLineage(
  state: ReplayWorldState,
  tick: number,
  requestID: string,
  workItems: FactoryWorkItem[],
): void {
  for (const item of workItems) {
    if (isSystemTimeWorkItem(item)) {
      continue;
    }
    recordWorkRequestSnapshot(state.payloadLineage, tick, requestID, item);
  }
}

export function recordDispatchConsumedInputPayloadLineage(
  state: ReplayWorldState,
  dispatchID: string,
  workItems: FactoryWorkItem[],
): void {
  for (const item of workItems) {
    recordConsumedInputSnapshot(state.payloadLineage, dispatchID, item);
  }
}

export function recordDispatchOutputPayloadLineage(
  state: ReplayWorldState,
  tick: number,
  dispatchID: string,
  consumedInputWorkItems: FactoryWorkItem[],
  outputItems: FactoryWorkItem[],
): void {
  for (const [index, item] of outputItems.entries()) {
    if (item.id === "") {
      continue;
    }
    recordDispatchOutputSnapshot(
      state.payloadLineage,
      tick,
      dispatchID,
      consumedInputWorkItems,
      item,
      index,
    );
  }
}

export function dispatchInputWorkItems(
  state: ReplayWorldState,
  dispatch: WorldDispatch | undefined,
): FactoryWorkItem[] {
  if (!dispatch) {
    return [];
  }
  return dispatch.consumedTokens
    .map((token) => state.workItemsByID[token.work_id])
    .filter((item): item is FactoryWorkItem => item !== undefined);
}
