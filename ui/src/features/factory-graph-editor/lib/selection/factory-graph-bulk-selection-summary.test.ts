import { describe, expect, it } from "vitest";

import { createEmptyFactoryGraphEditorSelection } from "./factory-graph-editor-selection";
import {
  buildFactoryGraphBulkSelectionSummary,
  resolveFactoryGraphNodeKindFromNodeId,
  resolveGraphSelectionDashboardSyncAction,
} from "./factory-graph-bulk-selection-summary";

describe("factory-graph-bulk-selection-summary", () => {
  it("resolves factory graph node kinds from canonical node ids", () => {
    expect(resolveFactoryGraphNodeKindFromNodeId("workstation:review")).toBe(
      "workstation",
    );
    expect(resolveFactoryGraphNodeKindFromNodeId("worker:writer")).toBe(
      "worker",
    );
    expect(resolveFactoryGraphNodeKindFromNodeId("work-type:story")).toBe(
      "work-type",
    );
    expect(
      resolveFactoryGraphNodeKindFromNodeId("work-state:story:queued"),
    ).toBe("work-state");
    expect(resolveFactoryGraphNodeKindFromNodeId("place:story:queued")).toBe(
      "work-state",
    );
    expect(resolveFactoryGraphNodeKindFromNodeId("resource:gpu")).toBe(
      "resource",
    );
    expect(
      resolveFactoryGraphNodeKindFromNodeId("doc:factory/docs/guide.md"),
    ).toBe("doc");
  });

  it("returns null for single graph item selections", () => {
    const summary = buildFactoryGraphBulkSelectionSummary({
      selectedEdgeIds: new Set(),
      selectedNodeIds: new Set(["workstation:review"]),
    });

    expect(summary).toBeNull();
  });

  it("builds mixed node and edge bulk-selection counts", () => {
    const summary = buildFactoryGraphBulkSelectionSummary({
      selectedEdgeIds: new Set([
        "workstation-output:workstation:review->work-state:story:done",
      ]),
      selectedNodeIds: new Set([
        "workstation:review",
        "worker:writer",
        "work-type:story",
      ]),
    });

    expect(summary).toEqual({
      totalCount: 4,
      kindCounts: [
        { kind: "workstation", count: 1 },
        { kind: "worker", count: 1 },
        { kind: "work-type", count: 1 },
        { kind: "edge", count: 1 },
      ],
    });
  });

  it("maps graph node ids to dashboard sync actions", () => {
    expect(
      resolveGraphSelectionDashboardSyncAction("workstation:review"),
    ).toEqual({
      kind: "workstation",
      nodeId: "review",
    });
    expect(resolveGraphSelectionDashboardSyncAction("worker:writer")).toEqual({
      kind: "worker",
      workerName: "writer",
    });
    expect(resolveGraphSelectionDashboardSyncAction("work-type:story")).toEqual(
      {
        kind: "work-type",
        workTypeName: "story",
      },
    );
    expect(
      resolveGraphSelectionDashboardSyncAction("work-state:story:queued"),
    ).toEqual({
      kind: "state-node",
      placeId: "story:queued",
    });
    expect(resolveGraphSelectionDashboardSyncAction("resource:gpu")).toEqual({
      kind: "resource",
      resourceName: "gpu",
    });
    expect(
      resolveGraphSelectionDashboardSyncAction("doc:factory/docs/guide.md"),
    ).toEqual({
      kind: "doc",
      targetPath: "factory/docs/guide.md",
    });
  });

  it("returns null for empty editor-local selection summaries", () => {
    const empty = createEmptyFactoryGraphEditorSelection();
    expect(buildFactoryGraphBulkSelectionSummary(empty)).toBeNull();
  });
});
