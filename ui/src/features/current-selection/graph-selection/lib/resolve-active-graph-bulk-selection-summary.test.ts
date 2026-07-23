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

  it("keeps bulk summary when the dashboard worker still matches graph selection", () => {
    const selection: DashboardSelection = {
      kind: "worker",
      workerName: "writer",
    };
    const bridgeSnapshot: FactoryGraphEditorSelectionBridgeSnapshot = {
      ...bulkBridgeSnapshot,
      selectedNodeIds: ["worker:writer", "workstation:review"],
    };

    expect(
      resolveActiveGraphBulkSelectionSummary(selection, bridgeSnapshot),
    ).toEqual(bridgeSnapshot.bulkSelectionSummary);
  });

  it("hides bulk summary for workstation-request selections", () => {
    const selection: DashboardSelection = {
      kind: "workstation-request",
      nodeId: "review",
      requestId: "request-1",
    };

    expect(
      resolveActiveGraphBulkSelectionSummary(selection, bulkBridgeSnapshot),
    ).toBeNull();
  });

  it("returns null when bridge or selection is missing", () => {
    expect(
      resolveActiveGraphBulkSelectionSummary(null, bulkBridgeSnapshot),
    ).toBeNull();
    expect(
      resolveActiveGraphBulkSelectionSummary(
        { kind: "worker", workerName: "writer" },
        null,
      ),
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
    expect(
      resolveGraphNodeIdCandidatesForDashboardSelection({
        kind: "resource",
        resourceName: "gpu",
      }),
    ).toEqual(["resource:gpu"]);
    expect(
      resolveGraphNodeIdCandidatesForDashboardSelection({
        kind: "work-type",
        workTypeName: "story",
      }),
    ).toEqual(["work-type:story"]);
    expect(
      resolveGraphNodeIdCandidatesForDashboardSelection({
        kind: "doc",
        targetPath: "factory/docs/guide.md",
      }),
    ).toEqual(["doc:factory/docs/guide.md"]);
    expect(
      resolveGraphNodeIdCandidatesForDashboardSelection({
        kind: "node",
        nodeId: "workstation:review",
      }),
    ).toEqual(["workstation:review"]);
  });
});
