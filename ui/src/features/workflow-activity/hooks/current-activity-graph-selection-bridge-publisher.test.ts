import { describe, expect, it, vi } from "vitest";

import { createEmptyFactoryGraphEditorSelection } from "../../factory-graph-editor/lib/selection/factory-graph-editor-selection";
import { useFactoryGraphEditorSelectionBridge } from "../state/factory-graph-editor-selection-bridge";
import {
  createCurrentActivityGraphSelectionBridgePublisher,
  publishCurrentActivityGraphSelectionBridgeState,
  syncCurrentActivityGraphSelectionToDashboard,
} from "./current-activity-graph-selection-bridge-publisher";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: bridge publish and dashboard sync contract cases stay together.
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

  it("syncs resource, doc, state-node, and work-type single selections to dashboard handlers", () => {
    const handlers = {
      onSelectDoc: vi.fn(),
      onSelectResource: vi.fn(),
      onSelectStateNode: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
    };
    const signatureRef = { current: null as string | null };

    syncCurrentActivityGraphSelectionToDashboard(
      {
        selectedEdgeIds: new Set(),
        selectedNodeIds: new Set(["resource:gpu"]),
        primaryTarget: { kind: "node", id: "resource:gpu" },
      },
      handlers,
      signatureRef,
    );
    expect(handlers.onSelectResource).toHaveBeenCalledWith("gpu");

    syncCurrentActivityGraphSelectionToDashboard(
      {
        selectedEdgeIds: new Set(),
        selectedNodeIds: new Set(["doc:factory/docs/guide.md"]),
        primaryTarget: { kind: "node", id: "doc:factory/docs/guide.md" },
      },
      handlers,
      { current: null },
    );
    expect(handlers.onSelectDoc).toHaveBeenCalledWith("factory/docs/guide.md");

    syncCurrentActivityGraphSelectionToDashboard(
      {
        selectedEdgeIds: new Set(),
        selectedNodeIds: new Set(["work-state:story:queued"]),
        primaryTarget: { kind: "node", id: "work-state:story:queued" },
      },
      handlers,
      { current: null },
    );
    expect(handlers.onSelectStateNode).toHaveBeenCalledWith("story:queued");

    syncCurrentActivityGraphSelectionToDashboard(
      {
        selectedEdgeIds: new Set(),
        selectedNodeIds: new Set(["work-type:story"]),
        primaryTarget: { kind: "node", id: "work-type:story" },
      },
      handlers,
      { current: null },
    );
    expect(handlers.onSelectWorkType).toHaveBeenCalledWith("story");
  });

  it("skips duplicate bridge publishes and multi-selection dashboard sync", () => {
    const state = {
      selectedEdgeIds: new Set(["edge-review-done"]),
      selectedNodeIds: new Set(["workstation:review", "worker:writer"]),
      primaryTarget: { kind: "node" as const, id: "workstation:review" },
    };
    const setSelection = vi.spyOn(
      useFactoryGraphEditorSelectionBridge.getState(),
      "setSelection",
    );

    publishCurrentActivityGraphSelectionBridgeState(state);
    publishCurrentActivityGraphSelectionBridgeState(state);

    expect(setSelection).toHaveBeenCalledTimes(1);

    const onSelectWorkstation = vi.fn();
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
    expect(onSelectWorkstation).not.toHaveBeenCalled();
  });

  it("publishes and syncs through the bridge publisher factory", async () => {
    const onSelectWorker = vi.fn();
    const publish = createCurrentActivityGraphSelectionBridgePublisher({
      onSelectDoc: vi.fn(),
      onSelectResource: vi.fn(),
      onSelectStateNode: vi.fn(),
      onSelectWorker,
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
    });

    publish({
      selectedEdgeIds: new Set(),
      selectedNodeIds: new Set(["worker:writer"]),
      primaryTarget: { kind: "node", id: "worker:writer" },
    });

    await Promise.resolve();

    expect(onSelectWorker).toHaveBeenCalledWith("writer");
    expect(
      useFactoryGraphEditorSelectionBridge.getState().selection
        ?.selectedNodeIds,
    ).toEqual(["worker:writer"]);
  });
});
