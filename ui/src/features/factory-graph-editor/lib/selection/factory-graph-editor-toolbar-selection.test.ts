import { createEmptyFactoryGraphEditorSelection } from "./factory-graph-editor-selection";
import {
  EMPTY_FACTORY_GRAPH_EDITOR_TOOLBAR_SELECTION_STATE,
  resolveFactoryGraphEditorToolbarDeleteAction,
  resolveFactoryGraphEditorToolbarSelectionState,
} from "./factory-graph-editor-toolbar-selection";

describe("resolveFactoryGraphEditorToolbarSelectionState", () => {
  it("reports no selection when the editor selection is empty", () => {
    expect(
      resolveFactoryGraphEditorToolbarSelectionState(
        createEmptyFactoryGraphEditorSelection(),
      ),
    ).toEqual(EMPTY_FACTORY_GRAPH_EDITOR_TOOLBAR_SELECTION_STATE);
  });

  it("reports single selection for one selected node or edge", () => {
    expect(
      resolveFactoryGraphEditorToolbarSelectionState({
        selectedEdgeIds: new Set(),
        selectedNodeIds: new Set(["workstation:review"]),
      }),
    ).toEqual({
      mode: "single",
      selectedItemCount: 1,
    });

    expect(
      resolveFactoryGraphEditorToolbarSelectionState({
        selectedEdgeIds: new Set([
          "workstation-output:workstation:review->work-state:story:done",
        ]),
        selectedNodeIds: new Set(),
      }),
    ).toEqual({
      mode: "single",
      selectedItemCount: 1,
    });
  });

  it("reports multi selection for mixed node and edge selections", () => {
    expect(
      resolveFactoryGraphEditorToolbarSelectionState({
        selectedEdgeIds: new Set([
          "workstation-output:workstation:review->work-state:story:done",
        ]),
        selectedNodeIds: new Set(["resource:gpu", "worker:editor"]),
      }),
    ).toEqual({
      mode: "multi",
      selectedItemCount: 3,
    });
  });
});

describe("resolveFactoryGraphEditorToolbarDeleteAction", () => {
  it("routes deletable selections to batch delete", () => {
    expect(
      resolveFactoryGraphEditorToolbarDeleteAction({
        canDeleteSelection: true,
        deleteToolActive: false,
        selectionState: {
          mode: "single",
          selectedItemCount: 1,
        },
      }),
    ).toEqual({
      kind: "batch-delete",
      selectionMode: "single",
      selectedItemCount: 1,
    });

    expect(
      resolveFactoryGraphEditorToolbarDeleteAction({
        canDeleteSelection: true,
        deleteToolActive: false,
        selectionState: {
          mode: "multi",
          selectedItemCount: 4,
        },
      }),
    ).toEqual({
      kind: "batch-delete",
      selectionMode: "multi",
      selectedItemCount: 4,
    });
  });

  it("disables delete when the current selection is not deletable", () => {
    expect(
      resolveFactoryGraphEditorToolbarDeleteAction({
        canDeleteSelection: false,
        deleteToolActive: false,
        selectionState: {
          mode: "single",
          selectedItemCount: 1,
        },
      }),
    ).toEqual({
      kind: "disabled",
      reason: "non-deletable-selection",
      selectionMode: "single",
      selectedItemCount: 1,
    });
  });

  it("falls back to delete-tool mode when nothing is selected", () => {
    expect(
      resolveFactoryGraphEditorToolbarDeleteAction({
        canDeleteSelection: false,
        deleteToolActive: true,
        selectionState: EMPTY_FACTORY_GRAPH_EDITOR_TOOLBAR_SELECTION_STATE,
      }),
    ).toEqual({
      kind: "delete-tool",
      active: true,
    });
  });
});
