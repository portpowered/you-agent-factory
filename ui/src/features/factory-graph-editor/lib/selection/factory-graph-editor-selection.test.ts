import type { EdgeChange, NodeChange } from "@xyflow/react";
import { describe, expect, it } from "vitest";

import {
  addToFactoryGraphEditorSelection,
  applyFactoryGraphEditorEdgeSelectChanges,
  applyFactoryGraphEditorNodeSelectChanges,
  clearFactoryGraphEditorSelection,
  createEmptyFactoryGraphEditorSelection,
  removeFromFactoryGraphEditorSelection,
  replaceFactoryGraphEditorSelection,
  resolveFactoryGraphEditorPrimaryTarget,
} from "./factory-graph-editor-selection";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: selection operation contract cases stay together.
describe("factory-graph-editor-selection", () => {
  it("starts empty and clears all selected ids and primary target", () => {
    const state = replaceFactoryGraphEditorSelection(
      createEmptyFactoryGraphEditorSelection(),
      {
        nodeIds: ["workstation:review"],
        primaryTarget: { kind: "node", id: "workstation:review" },
      },
    );

    expect(clearFactoryGraphEditorSelection()).toEqual({
      selectedNodeIds: new Set(),
      selectedEdgeIds: new Set(),
      primaryTarget: null,
    });
    expect(state.selectedNodeIds).toEqual(new Set(["workstation:review"]));
  });

  it("replaces node and edge selection and resolves the latest primary target", () => {
    const next = replaceFactoryGraphEditorSelection(
      createEmptyFactoryGraphEditorSelection(),
      {
        nodeIds: ["workstation:review", "workstation:done"],
        edgeIds: ["edge-review-done"],
      },
    );

    expect(next.selectedNodeIds).toEqual(
      new Set(["workstation:review", "workstation:done"]),
    );
    expect(next.selectedEdgeIds).toEqual(new Set(["edge-review-done"]));
    expect(next.primaryTarget).toEqual({
      kind: "edge",
      id: "edge-review-done",
    });
    expect(resolveFactoryGraphEditorPrimaryTarget(next)).toEqual({
      kind: "edge",
      id: "edge-review-done",
    });
  });

  it("honors an explicit primary target on replace", () => {
    const next = replaceFactoryGraphEditorSelection(
      createEmptyFactoryGraphEditorSelection(),
      {
        edgeIds: ["edge-review-done"],
        nodeIds: ["workstation:review"],
        primaryTarget: { kind: "node", id: "workstation:review" },
      },
    );

    expect(resolveFactoryGraphEditorPrimaryTarget(next)).toEqual({
      kind: "node",
      id: "workstation:review",
    });
  });

  it("adds nodes and edges while updating the primary target to the latest addition", () => {
    const replaced = replaceFactoryGraphEditorSelection(
      createEmptyFactoryGraphEditorSelection(),
      {
        nodeIds: ["workstation:review"],
      },
    );
    const withEdge = addToFactoryGraphEditorSelection(replaced, {
      edgeIds: ["edge-review-done"],
    });

    expect(withEdge.selectedNodeIds).toEqual(new Set(["workstation:review"]));
    expect(withEdge.selectedEdgeIds).toEqual(new Set(["edge-review-done"]));
    expect(resolveFactoryGraphEditorPrimaryTarget(withEdge)).toEqual({
      kind: "edge",
      id: "edge-review-done",
    });
  });

  it("removes nodes and edges and re-resolves the primary target", () => {
    const state = replaceFactoryGraphEditorSelection(
      createEmptyFactoryGraphEditorSelection(),
      {
        edgeIds: ["edge-review-done"],
        nodeIds: ["workstation:review", "workstation:done"],
        primaryTarget: { kind: "edge", id: "edge-review-done" },
      },
    );
    const afterEdgeRemoval = removeFromFactoryGraphEditorSelection(state, {
      edgeIds: ["edge-review-done"],
    });

    expect(afterEdgeRemoval.selectedEdgeIds).toEqual(new Set());
    expect(resolveFactoryGraphEditorPrimaryTarget(afterEdgeRemoval)).toEqual({
      kind: "node",
      id: "workstation:review",
    });

    const afterNodeRemoval = removeFromFactoryGraphEditorSelection(
      afterEdgeRemoval,
      {
        nodeIds: ["workstation:review"],
      },
    );

    expect(resolveFactoryGraphEditorPrimaryTarget(afterNodeRemoval)).toEqual({
      kind: "node",
      id: "workstation:done",
    });
  });

  it("applies React Flow node select changes through editor-local selection operations", () => {
    const empty = createEmptyFactoryGraphEditorSelection();
    const selected = applyFactoryGraphEditorNodeSelectChanges(empty, [
      {
        id: "workstation:review",
        selected: true,
        type: "select",
      },
      {
        id: "workstation:done",
        selected: true,
        type: "select",
      },
    ] satisfies NodeChange[]);

    expect(selected.selectedNodeIds).toEqual(
      new Set(["workstation:review", "workstation:done"]),
    );
    expect(resolveFactoryGraphEditorPrimaryTarget(selected)).toEqual({
      kind: "node",
      id: "workstation:done",
    });

    const deselected = applyFactoryGraphEditorNodeSelectChanges(selected, [
      {
        id: "workstation:review",
        selected: false,
        type: "select",
      },
      {
        id: "workstation:missing",
        type: "remove",
      },
    ] satisfies NodeChange[]);

    expect(deselected.selectedNodeIds).toEqual(new Set(["workstation:done"]));
    expect(resolveFactoryGraphEditorPrimaryTarget(deselected)).toEqual({
      kind: "node",
      id: "workstation:done",
    });
  });

  it("applies React Flow edge select changes through editor-local selection operations", () => {
    const empty = createEmptyFactoryGraphEditorSelection();
    const selected = applyFactoryGraphEditorEdgeSelectChanges(empty, [
      {
        id: "edge-review-done",
        selected: true,
        type: "select",
      },
    ] satisfies EdgeChange[]);

    expect(selected.selectedEdgeIds).toEqual(new Set(["edge-review-done"]));
    expect(resolveFactoryGraphEditorPrimaryTarget(selected)).toEqual({
      kind: "edge",
      id: "edge-review-done",
    });
  });
});
