import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { FactoryGraphEditorSelectionController } from "../../factory-graph-editor/hooks/selection/use-factory-graph-editor-selection";
import {
  useCurrentActivityGraphEdgePresentation,
  useCurrentActivityGraphSelectionGestures,
} from "./react-flow-current-activity-card-graph-selection-gestures";

function createGraphSelectionMock(): FactoryGraphEditorSelectionController {
  return {
    addToSelection: vi.fn(),
    applyEdgeSelectChanges: vi.fn(),
    applyNodeSelectChanges: vi.fn(),
    applyReactFlowSelection: vi.fn(),
    clearSelection: vi.fn(),
    handleGraphItemClick: vi.fn(),
    isEdgeSelected: vi.fn(() => false),
    isNodeSelected: vi.fn(() => false),
    removeFromSelection: vi.fn(),
    replaceSelection: vi.fn(),
    resolvePrimaryTarget: vi.fn(() => null),
    state: {
      primaryTarget: null,
      selectedEdgeIds: new Set(),
      selectedNodeIds: new Set(),
    },
  };
}

describe("useCurrentActivityGraphEdgePresentation", () => {
  it("routes edge selection changes through the graph selection controller", () => {
    const graphSelection = createGraphSelectionMock();
    const { result } = renderHook(() =>
      useCurrentActivityGraphEdgePresentation(graphSelection),
    );

    act(() => {
      result.current.handleEdgesChange([
        { type: "dimensions", id: "edge-1" },
        { type: "select", id: "edge-review-done", selected: true },
      ]);
    });

    expect(graphSelection.applyEdgeSelectChanges).toHaveBeenCalledWith([
      { type: "select", id: "edge-review-done", selected: true },
    ]);
  });
});

describe("useCurrentActivityGraphSelectionGestures", () => {
  it("replaces selection from React Flow selection changes by default", () => {
    const graphSelection = createGraphSelectionMock();
    const { result } = renderHook(() =>
      useCurrentActivityGraphSelectionGestures(graphSelection, true),
    );

    act(() => {
      result.current.handleGraphSelectionChange(
        {
          nodes: [{ id: "workstation:review" }],
          edges: [],
        },
        "react-flow-id",
      );
    });

    expect(graphSelection.applyReactFlowSelection).toHaveBeenCalledWith(
      {
        edgeIds: [],
        nodeIds: ["workstation:review"],
        primaryTarget: { kind: "node", id: "workstation:review" },
      },
      "replace",
    );
  });

  it("adds marquee matches when shift is held during selection start", () => {
    const graphSelection = createGraphSelectionMock();
    const { result } = renderHook(() =>
      useCurrentActivityGraphSelectionGestures(graphSelection, true),
    );

    act(() => {
      result.current.handleGraphSelectionStart({ shiftKey: true });
      result.current.handleGraphSelectionChange(
        {
          nodes: [{ id: "worker:writer" }],
          edges: [{ id: "edge-review-done" }],
        },
        "react-flow-id",
      );
    });

    expect(graphSelection.applyReactFlowSelection).toHaveBeenCalledWith(
      {
        edgeIds: ["edge-review-done"],
        nodeIds: ["worker:writer"],
        primaryTarget: { kind: "edge", id: "edge-review-done" },
      },
      "add",
    );
  });

  it("ignores React Flow selection changes when gestures are disabled", () => {
    const graphSelection = createGraphSelectionMock();
    const { result } = renderHook(() =>
      useCurrentActivityGraphSelectionGestures(graphSelection, false),
    );

    act(() => {
      result.current.handleGraphSelectionChange(
        {
          nodes: [{ id: "workstation:review" }],
          edges: [],
        },
        "react-flow-id",
      );
    });

    expect(graphSelection.applyReactFlowSelection).not.toHaveBeenCalled();
  });

  it("clears editor-local graph selection", () => {
    const graphSelection = createGraphSelectionMock();
    const { result } = renderHook(() =>
      useCurrentActivityGraphSelectionGestures(graphSelection, true),
    );

    act(() => {
      result.current.clearGraphSelection();
    });

    expect(graphSelection.clearSelection).toHaveBeenCalledTimes(1);
  });
});
