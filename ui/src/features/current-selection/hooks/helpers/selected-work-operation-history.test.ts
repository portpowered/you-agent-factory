import { describe, expect, it } from "vitest";

import type {
  DashboardSnapshot,
  DashboardWorkItemRef,
  DashboardWorkMoveOperation,
  DashboardWorkstationRequest,
} from "../../../../api/dashboard/types";
import { buildEmptyDashboardRuntimeFixture } from "../../../../components/dashboard/fixtures/runtime";
import {
  buildSelectedWorkOperationHistory,
  classifyWorkstationOperationKind,
} from "./selected-work-operation-history";

const selectedWorkItem: DashboardWorkItemRef = {
  display_name: "Selection Story",
  trace_id: "trace-selection-1",
  work_id: "work-selection-1",
  work_type_id: "story",
};

function buildReviewRequest(
  dispatchID: string,
  overrides: Partial<DashboardWorkstationRequest> = {},
): DashboardWorkstationRequest {
  return {
    dispatch_id: dispatchID,
    dispatched_request_count: 1,
    errored_request_count: 0,
    inference_attempts: [],
    responded_request_count: 1,
    started_at: "2026-04-08T12:00:00Z",
    transition_id: "review",
    work_items: [selectedWorkItem],
    workstation_name: "Review",
    workstation_node_id: "review",
    ...overrides,
  };
}

function buildSnapshot(workstationKind = "MODEL"): DashboardSnapshot {
  const runtime = buildEmptyDashboardRuntimeFixture();

  return {
    factory: {
      workers: [],
      workstations: [],
      workTypes: [],
    },
    factory_state: "IDLE",
    runtime,
    tick_count: 0,
    topology: {
      edges: [],
      workstation_node_ids: ["review", "logical-move"],
      workstation_nodes_by_id: {
        review: {
          node_id: "review",
          transition_id: "review",
          workstation_name: "Review",
          workstation_kind: workstationKind,
        },
        "logical-move": {
          node_id: "logical-move",
          transition_id: "logical-move",
          workstation_name: "Logical Move",
          workstation_kind: "LOGICAL_MOVE",
        },
      },
    },
    uptime_seconds: 0,
  };
}

function buildMoveOperation(
  overrides: Partial<DashboardWorkMoveOperation> = {},
): DashboardWorkMoveOperation {
  return {
    from_place_id: "story:init",
    from_state: "init",
    sequence: 1,
    source: "api",
    tick: 2,
    to_place_id: "story:review",
    to_state: "review",
    work_id: selectedWorkItem.work_id,
    ...overrides,
  };
}

describe("buildSelectedWorkOperationHistory", () => {
  const snapshot = buildSnapshot();

  it("returns workstation-only history sorted newest first when no move history exists", () => {
    const olderRequest = buildReviewRequest("dispatch-older", {
      started_at: "2026-04-08T10:00:00Z",
    });
    const newerRequest = buildReviewRequest("dispatch-newer", {
      started_at: "2026-04-08T14:00:00Z",
    });

    const history = buildSelectedWorkOperationHistory({
      moveOperations: undefined,
      snapshot,
      workID: selectedWorkItem.work_id,
      workstationRequests: [olderRequest, newerRequest],
    });

    expect(history).toEqual([
      { kind: "workstation", request: newerRequest },
      { kind: "workstation", request: olderRequest },
    ]);
  });

  it("returns move-only history when no workstation requests exist", () => {
    const olderMove = buildMoveOperation({
      event_time: "2026-04-08T10:00:00Z",
      request_id: "move-older",
      sequence: 1,
      tick: 1,
    });
    const newerMove = buildMoveOperation({
      event_time: "2026-04-08T14:00:00Z",
      request_id: "move-newer",
      sequence: 2,
      tick: 3,
    });

    const history = buildSelectedWorkOperationHistory({
      moveOperations: [olderMove, newerMove],
      snapshot,
      workID: selectedWorkItem.work_id,
      workstationRequests: [],
    });

    expect(history).toEqual([
      { kind: "operator-move", move: newerMove },
      { kind: "operator-move", move: olderMove },
    ]);
  });

  it("merges move and workstation operations in newest-first order with deterministic tie-breaking", () => {
    const workstationRequest = buildReviewRequest("dispatch-shared-time", {
      started_at: "2026-04-08T12:00:00Z",
    });
    const moveOperation = buildMoveOperation({
      event_time: "2026-04-08T12:00:00Z",
      request_id: "move-shared-time",
    });

    const history = buildSelectedWorkOperationHistory({
      moveOperations: [moveOperation],
      snapshot,
      workID: selectedWorkItem.work_id,
      workstationRequests: [workstationRequest],
    });

    expect(history).toEqual([
      { kind: "workstation", request: workstationRequest },
      { kind: "operator-move", move: moveOperation },
    ]);
  });

  it("classifies LOGICAL_MOVE workstation requests as logical-move-dispatch", () => {
    const logicalMoveRequest = buildReviewRequest("dispatch-logical-move", {
      started_at: "2026-04-08T12:00:00Z",
      transition_id: "logical-move",
      workstation_name: "Logical Move",
      workstation_node_id: "logical-move",
    });

    const history = buildSelectedWorkOperationHistory({
      moveOperations: [],
      snapshot,
      workID: selectedWorkItem.work_id,
      workstationRequests: [logicalMoveRequest],
    });

    expect(history).toEqual([
      { kind: "logical-move-dispatch", request: logicalMoveRequest },
    ]);
    expect(classifyWorkstationOperationKind(logicalMoveRequest, snapshot)).toBe(
      "logical-move-dispatch",
    );
  });
});
