import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { useFactoryGraphEditorSelection } from "./use-factory-graph-editor-selection";

describe("useFactoryGraphEditorSelection", () => {
  it("exposes replace, add, remove, clear, and primary-target resolution", () => {
    const { result } = renderHook(() => useFactoryGraphEditorSelection());

    act(() => {
      result.current.replaceSelection({
        nodeIds: ["workstation:review"],
      });
    });

    expect(result.current.state.selectedNodeIds).toEqual(
      new Set(["workstation:review"]),
    );
    expect(result.current.resolvePrimaryTarget()).toEqual({
      kind: "node",
      id: "workstation:review",
    });

    act(() => {
      result.current.addToSelection({
        edgeIds: ["edge-review-done"],
      });
    });

    expect(result.current.isEdgeSelected("edge-review-done")).toBe(true);
    expect(result.current.resolvePrimaryTarget()).toEqual({
      kind: "edge",
      id: "edge-review-done",
    });

    act(() => {
      result.current.removeFromSelection({
        edgeIds: ["edge-review-done"],
      });
    });

    expect(result.current.isEdgeSelected("edge-review-done")).toBe(false);
    expect(result.current.resolvePrimaryTarget()).toEqual({
      kind: "node",
      id: "workstation:review",
    });

    act(() => {
      result.current.clearSelection();
    });

    expect(result.current.state).toEqual({
      selectedNodeIds: new Set(),
      selectedEdgeIds: new Set(),
      primaryTarget: null,
    });
    expect(result.current.resolvePrimaryTarget()).toBeNull();
  });
});
