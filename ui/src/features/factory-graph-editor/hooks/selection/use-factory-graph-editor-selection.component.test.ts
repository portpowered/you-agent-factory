import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { useFactoryGraphEditorSelection } from "./use-factory-graph-editor-selection";

async function flushMicrotasks(): Promise<void> {
  await Promise.resolve();
}

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

  it("exposes graph click, marquee, and react-flow selection helpers", () => {
    const { result } = renderHook(() => useFactoryGraphEditorSelection());

    act(() => {
      result.current.handleGraphItemClick(
        { kind: "node", id: "workstation:review" },
        { ctrlKey: true, metaKey: false, shiftKey: false },
      );
    });

    expect(result.current.isNodeSelected("workstation:review")).toBe(true);

    act(() => {
      result.current.handleGraphItemClick(
        { kind: "node", id: "workstation:review" },
        { ctrlKey: true, metaKey: false, shiftKey: false },
      );
    });

    expect(result.current.isNodeSelected("workstation:review")).toBe(false);

    act(() => {
      result.current.applyReactFlowSelection(
        { nodeIds: ["workstation:done"] },
        "replace",
      );
    });

    expect(result.current.isNodeSelected("workstation:done")).toBe(true);
  });

  it("defers onStateChange until after the selection commit and skips no-op updates", async () => {
    const onStateChange = vi.fn();
    const { result } = renderHook(() =>
      useFactoryGraphEditorSelection({ onStateChange }),
    );

    act(() => {
      result.current.replaceSelection({
        nodeIds: ["workstation:plan"],
      });
    });

    expect(onStateChange).not.toHaveBeenCalled();

    await act(async () => {
      await flushMicrotasks();
    });

    expect(onStateChange).toHaveBeenCalledTimes(1);
    expect(onStateChange.mock.calls[0]?.[0].selectedNodeIds).toEqual(
      new Set(["workstation:plan"]),
    );

    act(() => {
      result.current.replaceSelection({
        nodeIds: ["workstation:plan"],
        primaryTarget: { kind: "node", id: "workstation:plan" },
      });
    });

    await act(async () => {
      await flushMicrotasks();
    });

    expect(onStateChange).toHaveBeenCalledTimes(1);
  });
});
// Component lane: requires DOM APIs.
