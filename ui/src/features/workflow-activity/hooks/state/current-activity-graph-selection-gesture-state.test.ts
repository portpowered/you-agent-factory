import { describe, expect, it } from "vitest";

import type { FactoryGraphEditorSelectionState } from "../../../factory-graph-editor/public/selection-gestures";
import {
  resolveCurrentActivityGraphSelectionChange,
  selectGraphEdgePresentationChanges,
} from "./current-activity-graph-selection-gesture-state";

const emptySelection = (): FactoryGraphEditorSelectionState => ({
  primaryTarget: null,
  selectedEdgeIds: new Set(),
  selectedNodeIds: new Set(),
});

describe("selectGraphEdgePresentationChanges", () => {
  it("keeps select changes only when selection gestures are disabled", () => {
    const changes = [
      { type: "dimensions", id: "edge-1" },
      { type: "select", id: "edge-review-done", selected: true },
      { type: "remove", id: "edge-obsolete" },
    ] as const;
    expect(selectGraphEdgePresentationChanges([...changes], false)).toEqual([
      changes[1],
      changes[2],
    ]);
    expect(selectGraphEdgePresentationChanges([...changes], true)).toEqual([
      changes[2],
    ]);
  });
});

describe("resolveCurrentActivityGraphSelectionChange", () => {
  it("replaces selection by default and adds when marquee is additive", () => {
    const nodes = [{ id: "worker:writer" }] as never[];
    const edges = [{ id: "edge-review-done" }] as never[];
    expect(
      resolveCurrentActivityGraphSelectionChange({
        additive: false,
        edges,
        enabled: true,
        nodes,
        selectionState: emptySelection(),
      }),
    ).toEqual({
      items: {
        edgeIds: ["edge-review-done"],
        nodeIds: ["worker:writer"],
        primaryTarget: { kind: "edge", id: "edge-review-done" },
      },
      mode: "replace",
    });
    expect(
      resolveCurrentActivityGraphSelectionChange({
        additive: true,
        edges,
        enabled: true,
        nodes,
        selectionState: emptySelection(),
      })?.mode,
    ).toBe("add");
  });

  it("ignores disabled and duplicate callbacks", () => {
    const nodes = [{ id: "workstation:plan" }] as never[];
    expect(
      resolveCurrentActivityGraphSelectionChange({
        additive: false,
        edges: [],
        enabled: false,
        nodes,
        selectionState: emptySelection(),
      }),
    ).toBeNull();
    expect(
      resolveCurrentActivityGraphSelectionChange({
        additive: false,
        edges: [],
        enabled: true,
        nodes,
        selectionState: {
          primaryTarget: { kind: "node", id: "workstation:plan" },
          selectedEdgeIds: new Set(),
          selectedNodeIds: new Set(["workstation:plan"]),
        },
      }),
    ).toBeNull();
  });
});
