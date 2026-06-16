import { describe, expect, it } from "vitest";

import type { FactoryGraphEditorSelectionBridgeSnapshot } from "../../../workflow-activity/state/factory-graph-editor-selection-bridge";
import type { DashboardSelection } from "../../base/state/dashboardSelection";
import {
  resolveActiveGraphBulkSelectionSummary,
  resolveGraphNodeIdCandidatesForDashboardSelection,
} from "./resolve-active-graph-bulk-selection-summary";

const bulkBridgeSnapshot: FactoryGraphEditorSelectionBridgeSnapshot = {
  bulkSelectionSummary: {
    totalCount: 3,
    kindCounts: [
      { kind: "workstation", count: 2 },
      { kind: "edge", count: 1 },
    ],
  },
  primaryTarget: { kind: "node", id: "workstation:review" },
  selectedEdgeIds: ["edge-review-done"],
  selectedNodeIds: ["workstation:review", "workstation:done"],
};

describe("resolveActiveGraphBulkSelectionSummary", () => {
  it("returns bulk summary for stale workstation selections during graph multi-select", () => {
    const selection: DashboardSelection = {
      kind: "node",
      nodeId: "review",
    };

    expect(
      resolveActiveGraphBulkSelectionSummary(selection, bulkBridgeSnapshot),
    ).toEqual(bulkBridgeSnapshot.bulkSelectionSummary);
  });

  it("hides bulk summary when dashboard selection moves to a work item", () => {
    const selection: DashboardSelection = {
      dispatchId: "dispatch-1",
      kind: "work-item",
      nodeId: "review",
      workItem: {
        work_id: "work-alpha",
        work_type: "story",
      },
    };

    expect(
      resolveActiveGraphBulkSelectionSummary(selection, bulkBridgeSnapshot),
    ).toBeNull();
  });

  it("hides bulk summary when dashboard selection moves to an unrelated worker", () => {
    const selection: DashboardSelection = {
      kind: "worker",
      workerName: "editor",
    };

    expect(
      resolveActiveGraphBulkSelectionSummary(selection, bulkBridgeSnapshot),
    ).toBeNull();
  });

  it("maps dashboard selections to graph node id candidates", () => {
    expect(
      resolveGraphNodeIdCandidatesForDashboardSelection({
        kind: "node",
        nodeId: "review",
      }),
    ).toEqual(["workstation:review", "review"]);
    expect(
      resolveGraphNodeIdCandidatesForDashboardSelection({
        kind: "state-node",
        placeId: "story:queued",
      }),
    ).toEqual(["work-state:story:queued", "place:story:queued"]);
  });
});
