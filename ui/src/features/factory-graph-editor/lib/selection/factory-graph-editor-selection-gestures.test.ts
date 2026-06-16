import type { Edge, Node } from "@xyflow/react";
import { describe, expect, it } from "vitest";

import { createEmptyFactoryGraphEditorSelection } from "./factory-graph-editor-selection";
import {
  applyFactoryGraphEditorReactFlowSelection,
  applyGraphItemClickSelection,
  collectFactoryGraphSelectionItemsFromReactFlow,
  isToggleSelectionModifier,
  resolveFactoryGraphEdgeIdFromRenderedEdge,
  toggleFactoryGraphEditorSelectionItem,
} from "./factory-graph-editor-selection-gestures";

describe("factory-graph-editor-selection-gestures", () => {
  it("detects toggle modifiers from meta or ctrl keys", () => {
    expect(
      isToggleSelectionModifier({
        ctrlKey: false,
        metaKey: true,
        shiftKey: false,
      }),
    ).toBe(true);
    expect(
      isToggleSelectionModifier({
        ctrlKey: true,
        metaKey: false,
        shiftKey: false,
      }),
    ).toBe(true);
    expect(
      isToggleSelectionModifier({
        ctrlKey: false,
        metaKey: false,
        shiftKey: true,
      }),
    ).toBe(false);
  });

  it("replaces selection on plain clicks and toggles with modifier clicks", () => {
    const empty = createEmptyFactoryGraphEditorSelection();
    const replaced = applyGraphItemClickSelection(
      empty,
      { kind: "node", id: "workstation:review" },
      { ctrlKey: false, metaKey: false, shiftKey: false },
    );

    expect(replaced.selectedNodeIds).toEqual(new Set(["workstation:review"]));

    const toggledOff = toggleFactoryGraphEditorSelectionItem(replaced, {
      kind: "node",
      id: "workstation:review",
    });
    expect(toggledOff.selectedNodeIds).toEqual(new Set());

    const toggledOn = toggleFactoryGraphEditorSelectionItem(toggledOff, {
      kind: "edge",
      id: "edge-review-done",
    });
    expect(toggledOn.selectedEdgeIds).toEqual(new Set(["edge-review-done"]));
    expect(toggledOn.primaryTarget).toEqual({
      kind: "edge",
      id: "edge-review-done",
    });
  });

  it("maps rendered React Flow ids to factory graph selection items", () => {
    const nodes = [{ id: "workstation:review" }] satisfies Node[];
    const edges = [
      {
        data: {
          factoryGraphEdgeId:
            "workstation-input:work-state:story:queued->workstation:review",
        },
        id: "workstation-resource:resource:story->workstation:review",
      },
    ] satisfies Edge[];

    expect(
      collectFactoryGraphSelectionItemsFromReactFlow(nodes, edges),
    ).toEqual({
      edgeIds: [
        "workstation-input:work-state:story:queued->workstation:review",
      ],
      nodeIds: ["workstation:review"],
      primaryTarget: {
        kind: "edge",
        id: "workstation-input:work-state:story:queued->workstation:review",
      },
    });
    expect(
      resolveFactoryGraphEdgeIdFromRenderedEdge({
        data: undefined,
        id: "edge-review-done",
      }),
    ).toBe("edge-review-done");
  });

  it("applies additive and replace marquee selection modes", () => {
    const seeded = applyFactoryGraphEditorReactFlowSelection(
      createEmptyFactoryGraphEditorSelection(),
      { nodeIds: ["workstation:review"] },
      "replace",
    );
    const additive = applyFactoryGraphEditorReactFlowSelection(
      seeded,
      { nodeIds: ["workstation:done"], edgeIds: ["edge-review-done"] },
      "add",
    );

    expect(additive.selectedNodeIds).toEqual(
      new Set(["workstation:review", "workstation:done"]),
    );
    expect(additive.selectedEdgeIds).toEqual(new Set(["edge-review-done"]));
  });
});
