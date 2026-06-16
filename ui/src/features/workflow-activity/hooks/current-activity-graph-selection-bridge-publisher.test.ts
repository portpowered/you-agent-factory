import { describe, expect, it, vi } from "vitest";

import { createEmptyFactoryGraphEditorSelection } from "../../factory-graph-editor/lib/selection/factory-graph-editor-selection";
import {
  publishCurrentActivityGraphSelectionBridgeState,
  syncCurrentActivityGraphSelectionToDashboard,
} from "./current-activity-graph-selection-bridge-publisher";
import { useFactoryGraphEditorSelectionBridge } from "../state/factory-graph-editor-selection-bridge";

describe("current-activity-graph-selection-bridge-publisher", () => {
  beforeEach(() => {
    useFactoryGraphEditorSelectionBridge.setState({ selection: null });
  });

  it("publishes bulk-selection summary and syncs single node selection to dashboard handlers", () => {
    const onSelectWorkstation = vi.fn();
    const state = {
      selectedEdgeIds: new Set<string>(),
      selectedNodeIds: new Set(["workstation:review"]),
      primaryTarget: { kind: "node" as const, id: "workstation:review" },
    };

    publishCurrentActivityGraphSelectionBridgeState(state);
    syncCurrentActivityGraphSelectionToDashboard(
      state,
      {
        onSelectDoc: vi.fn(),
        onSelectResource: vi.fn(),
        onSelectStateNode: vi.fn(),
        onSelectWorker: vi.fn(),
        onSelectWorkType: vi.fn(),
        onSelectWorkstation,
      },
      { current: null },
    );

    expect(useFactoryGraphEditorSelectionBridge.getState().selection).toEqual({
      bulkSelectionSummary: null,
      primaryTarget: { kind: "node", id: "workstation:review" },
      selectedEdgeIds: [],
      selectedNodeIds: ["workstation:review"],
    });
    expect(onSelectWorkstation).toHaveBeenCalledWith("review");
  });

  it("publishes multi-selection summary without dispatching dashboard sync actions", () => {
    const onSelectWorkstation = vi.fn();
    const state = {
      selectedEdgeIds: new Set(["edge-review-done"]),
      selectedNodeIds: new Set(["workstation:review", "worker:writer"]),
      primaryTarget: { kind: "node" as const, id: "workstation:review" },
    };

    publishCurrentActivityGraphSelectionBridgeState(state);
    syncCurrentActivityGraphSelectionToDashboard(
      state,
      {
        onSelectDoc: vi.fn(),
        onSelectResource: vi.fn(),
        onSelectStateNode: vi.fn(),
        onSelectWorker: vi.fn(),
        onSelectWorkType: vi.fn(),
        onSelectWorkstation,
      },
      { current: null },
    );

    expect(
      useFactoryGraphEditorSelectionBridge.getState().selection
        ?.bulkSelectionSummary,
    ).toEqual({
      totalCount: 3,
      kindCounts: [
        { kind: "workstation", count: 1 },
        { kind: "worker", count: 1 },
        { kind: "edge", count: 1 },
      ],
    });
    expect(onSelectWorkstation).not.toHaveBeenCalled();
  });

  it("ignores empty editor-local selection", () => {
    publishCurrentActivityGraphSelectionBridgeState(
      createEmptyFactoryGraphEditorSelection(),
    );

    expect(useFactoryGraphEditorSelectionBridge.getState().selection).toEqual({
      bulkSelectionSummary: null,
      primaryTarget: null,
      selectedEdgeIds: [],
      selectedNodeIds: [],
    });
  });
});
