import type { FactoryGraphEditorSelectionState } from "./factory-graph-editor-selection";

export type FactoryGraphEditorToolbarSelectionMode =
  | "none"
  | "single"
  | "multi";

export type FactoryGraphEditorToolbarSelectionState = {
  mode: FactoryGraphEditorToolbarSelectionMode;
  selectedItemCount: number;
};

export type FactoryGraphEditorToolbarDeleteAction =
  | {
      kind: "batch-delete";
      selectionMode: "single" | "multi";
      selectedItemCount: number;
    }
  | {
      kind: "disabled";
      reason: "no-selection" | "non-deletable-selection";
      selectionMode?: "single" | "multi";
      selectedItemCount?: number;
    };

export const EMPTY_FACTORY_GRAPH_EDITOR_TOOLBAR_SELECTION_STATE: FactoryGraphEditorToolbarSelectionState =
  {
    mode: "none",
    selectedItemCount: 0,
  };

export function resolveFactoryGraphEditorToolbarSelectionState(
  state: Pick<
    FactoryGraphEditorSelectionState,
    "selectedEdgeIds" | "selectedNodeIds"
  >,
): FactoryGraphEditorToolbarSelectionState {
  const selectedItemCount =
    state.selectedNodeIds.size + state.selectedEdgeIds.size;

  if (selectedItemCount <= 0) {
    return EMPTY_FACTORY_GRAPH_EDITOR_TOOLBAR_SELECTION_STATE;
  }

  if (selectedItemCount === 1) {
    return {
      mode: "single",
      selectedItemCount: 1,
    };
  }

  return {
    mode: "multi",
    selectedItemCount,
  };
}

export function resolveFactoryGraphEditorToolbarDeleteAction(options: {
  canDeleteSelection: boolean;
  selectionState: FactoryGraphEditorToolbarSelectionState;
}): FactoryGraphEditorToolbarDeleteAction {
  if (options.canDeleteSelection) {
    return {
      kind: "batch-delete",
      selectionMode:
        options.selectionState.mode === "multi" ? "multi" : "single",
      selectedItemCount: options.selectionState.selectedItemCount,
    };
  }

  if (options.selectionState.mode === "none") {
    return {
      kind: "disabled",
      reason: "no-selection",
    };
  }

  return {
    kind: "disabled",
    reason: "non-deletable-selection",
    selectionMode: options.selectionState.mode === "multi" ? "multi" : "single",
    selectedItemCount: options.selectionState.selectedItemCount,
  };
}

export type FactoryGraphEditorToolbarDeleteButtonMessages = {
  toolbarDeleteDisabledNoSelectionDescription: string;
  toolbarDeleteDisabledNoSelectionLabel: string;
  toolbarDeleteDisabledNonDeletableDescription: string;
  toolbarDeleteDisabledNonDeletableLabel: string;
  toolbarDeleteMultiSelectionDescription: (count: number) => string;
  toolbarDeleteMultiSelectionLabel: (count: number) => string;
  toolbarDeleteSingleSelectionDescription: string;
  toolbarDeleteSingleSelectionLabel: string;
};

export function resolveFactoryGraphEditorToolbarDeleteButtonState({
  deleteAction,
  messages,
}: {
  deleteAction: FactoryGraphEditorToolbarDeleteAction;
  messages: FactoryGraphEditorToolbarDeleteButtonMessages;
}): {
  active: boolean;
  description: string;
  label: string;
  tone: "outline" | "secondary";
} {
  switch (deleteAction.kind) {
    case "batch-delete":
      return {
        active: false,
        description:
          deleteAction.selectionMode === "multi"
            ? messages.toolbarDeleteMultiSelectionDescription(
                deleteAction.selectedItemCount,
              )
            : messages.toolbarDeleteSingleSelectionDescription,
        label:
          deleteAction.selectionMode === "multi"
            ? messages.toolbarDeleteMultiSelectionLabel(
                deleteAction.selectedItemCount,
              )
            : messages.toolbarDeleteSingleSelectionLabel,
        tone: "secondary",
      };
    case "disabled":
      if (deleteAction.reason === "no-selection") {
        return {
          active: false,
          description: messages.toolbarDeleteDisabledNoSelectionDescription,
          label: messages.toolbarDeleteDisabledNoSelectionLabel,
          tone: "outline",
        };
      }
      return {
        active: false,
        description: messages.toolbarDeleteDisabledNonDeletableDescription,
        label: messages.toolbarDeleteDisabledNonDeletableLabel,
        tone: "outline",
      };
  }
}
