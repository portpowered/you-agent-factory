import type {
  DashboardSnapshot,
  DashboardWorkMoveOperation,
} from "../../../../api/dashboard/types";
import { WorkstationType } from "../../../../api/generated/openapi";
import {
  requestDispatchID,
  requestStartedAt,
  requestWorkstationNodeID,
  type WorkstationRequestLike,
} from "./useCurrentSelection.request-helpers";

export type SelectedWorkOperationHistoryItem =
  | {
      kind: "workstation";
      request: WorkstationRequestLike;
    }
  | {
      kind: "operator-move";
      move: DashboardWorkMoveOperation;
    }
  | {
      kind: "logical-move-dispatch";
      request: WorkstationRequestLike;
    };

interface SortableSelectedWorkOperation {
  item: SelectedWorkOperationHistoryItem;
  sortTime: string;
  tieBreaker: string;
}

export function buildSelectedWorkOperationHistory({
  moveOperations,
  snapshot,
  workID,
  workstationRequests,
}: {
  moveOperations: DashboardWorkMoveOperation[] | undefined;
  snapshot: DashboardSnapshot | null | undefined;
  workID: string;
  workstationRequests: WorkstationRequestLike[];
}): SelectedWorkOperationHistoryItem[] {
  const sortableItems: SortableSelectedWorkOperation[] = [];

  for (const request of workstationRequests) {
    const kind = classifyWorkstationOperationKind(request, snapshot);
    sortableItems.push({
      item:
        kind === "logical-move-dispatch"
          ? { kind, request }
          : { kind: "workstation", request },
      sortTime: workstationOperationSortTime(request),
      tieBreaker: workstationOperationTieBreaker(request),
    });
  }

  for (const move of moveOperations ?? []) {
    if (move.work_id !== workID) {
      continue;
    }
    sortableItems.push({
      item: { kind: "operator-move", move },
      sortTime: moveOperationSortTime(move),
      tieBreaker: moveOperationTieBreaker(move),
    });
  }

  return sortableItems
    .sort((left, right) => {
      const timeCompare = right.sortTime.localeCompare(left.sortTime);
      if (timeCompare !== 0) {
        return timeCompare;
      }
      return left.tieBreaker.localeCompare(right.tieBreaker);
    })
    .map((entry) => entry.item);
}

export function classifyWorkstationOperationKind(
  request: WorkstationRequestLike,
  snapshot: DashboardSnapshot | null | undefined,
): "workstation" | "logical-move-dispatch" {
  const workstationKind = resolveWorkstationKind(request, snapshot);
  return workstationKind?.toUpperCase() === WorkstationType.LOGICAL_MOVE
    ? "logical-move-dispatch"
    : "workstation";
}

function resolveWorkstationKind(
  request: WorkstationRequestLike,
  snapshot: DashboardSnapshot | null | undefined,
): string | undefined {
  const nodeID = requestWorkstationNodeID(request);
  return snapshot?.topology.workstation_nodes_by_id[nodeID]?.workstation_kind;
}

function workstationOperationSortTime(request: WorkstationRequestLike): string {
  return requestStartedAt(request);
}

function workstationOperationTieBreaker(
  request: WorkstationRequestLike,
): string {
  return requestDispatchID(request);
}

function moveOperationSortTime(move: DashboardWorkMoveOperation): string {
  if (move.event_time) {
    return move.event_time;
  }

  return `${String(move.tick).padStart(12, "0")}:${String(move.sequence).padStart(12, "0")}`;
}

function moveOperationTieBreaker(move: DashboardWorkMoveOperation): string {
  return move.request_id ?? `${move.tick}:${move.sequence}`;
}
