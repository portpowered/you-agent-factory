import type { EdgeChange, NodeChange } from "@xyflow/react";
import { useCallback, useMemo, useRef, useState } from "react";
import {
  addToFactoryGraphEditorSelection,
  applyFactoryGraphEditorEdgeSelectChanges,
  applyFactoryGraphEditorNodeSelectChanges,
  clearFactoryGraphEditorSelection,
  createEmptyFactoryGraphEditorSelection,
  type FactoryGraphEditorSelectionItems,
  type FactoryGraphEditorSelectionState,
  type FactoryGraphEditorSelectionTarget,
  removeFromFactoryGraphEditorSelection,
  replaceFactoryGraphEditorSelection,
  resolveFactoryGraphEditorPrimaryTarget,
} from "../../lib/selection/factory-graph-editor-selection";
import {
  applyFactoryGraphEditorReactFlowSelection,
  applyGraphItemClickSelection,
  type FactoryGraphEditorSelectionPointerModifiers,
} from "../../lib/selection/factory-graph-editor-selection-gestures";

export type FactoryGraphEditorSelectionController = {
  state: FactoryGraphEditorSelectionState;
  replaceSelection: (items: FactoryGraphEditorSelectionItems) => void;
  addToSelection: (items: FactoryGraphEditorSelectionItems) => void;
  removeFromSelection: (items: FactoryGraphEditorSelectionItems) => void;
  clearSelection: () => void;
  resolvePrimaryTarget: () => ReturnType<
    typeof resolveFactoryGraphEditorPrimaryTarget
  >;
  applyNodeSelectChanges: (changes: readonly NodeChange[]) => void;
  applyEdgeSelectChanges: (changes: readonly EdgeChange[]) => void;
  applyReactFlowSelection: (
    items: FactoryGraphEditorSelectionItems,
    mode: "add" | "replace",
  ) => void;
  handleGraphItemClick: (
    item: FactoryGraphEditorSelectionTarget,
    modifiers: FactoryGraphEditorSelectionPointerModifiers,
  ) => void;
  isNodeSelected: (nodeId: string) => boolean;
  isEdgeSelected: (edgeId: string) => boolean;
};

type UseFactoryGraphEditorSelectionOptions = {
  onStateChange?: (state: FactoryGraphEditorSelectionState) => void;
};

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: selection controller wiring stays in one hook.
export function useFactoryGraphEditorSelection(
  options: UseFactoryGraphEditorSelectionOptions = {},
): FactoryGraphEditorSelectionController {
  const [state, setState] = useState(createEmptyFactoryGraphEditorSelection);
  const onStateChangeRef = useRef(options.onStateChange);
  onStateChangeRef.current = options.onStateChange;

  const commitSelectionState = useCallback(
    (
      updater: (
        current: FactoryGraphEditorSelectionState,
      ) => FactoryGraphEditorSelectionState,
    ) => {
      setState((current) => {
        const nextState = updater(current);
        if (nextState !== current) {
          queueMicrotask(() => {
            onStateChangeRef.current?.(nextState);
          });
        }
        return nextState;
      });
    },
    [],
  );

  const replaceSelection = useCallback(
    (items: FactoryGraphEditorSelectionItems) => {
      commitSelectionState((current) =>
        replaceFactoryGraphEditorSelection(current, items),
      );
    },
    [commitSelectionState],
  );

  const addToSelection = useCallback(
    (items: FactoryGraphEditorSelectionItems) => {
      commitSelectionState((current) =>
        addToFactoryGraphEditorSelection(current, items),
      );
    },
    [commitSelectionState],
  );

  const removeFromSelection = useCallback(
    (items: FactoryGraphEditorSelectionItems) => {
      commitSelectionState((current) =>
        removeFromFactoryGraphEditorSelection(current, items),
      );
    },
    [commitSelectionState],
  );

  const clearSelection = useCallback(() => {
    commitSelectionState((current) =>
      clearFactoryGraphEditorSelection(current),
    );
  }, [commitSelectionState]);

  const resolvePrimaryTarget = useCallback(
    () => resolveFactoryGraphEditorPrimaryTarget(state),
    [state],
  );

  const applyNodeSelectChanges = useCallback(
    (changes: readonly NodeChange[]) => {
      commitSelectionState((current) =>
        applyFactoryGraphEditorNodeSelectChanges(current, changes),
      );
    },
    [commitSelectionState],
  );

  const applyEdgeSelectChanges = useCallback(
    (changes: readonly EdgeChange[]) => {
      commitSelectionState((current) =>
        applyFactoryGraphEditorEdgeSelectChanges(current, changes),
      );
    },
    [commitSelectionState],
  );

  const applyReactFlowSelection = useCallback(
    (items: FactoryGraphEditorSelectionItems, mode: "add" | "replace") => {
      commitSelectionState((current) =>
        applyFactoryGraphEditorReactFlowSelection(current, items, mode),
      );
    },
    [commitSelectionState],
  );

  const handleGraphItemClick = useCallback(
    (
      item: FactoryGraphEditorSelectionTarget,
      modifiers: FactoryGraphEditorSelectionPointerModifiers,
    ) => {
      commitSelectionState((current) =>
        applyGraphItemClickSelection(current, item, modifiers),
      );
    },
    [commitSelectionState],
  );

  const isNodeSelected = useCallback(
    (nodeId: string) => state.selectedNodeIds.has(nodeId),
    [state.selectedNodeIds],
  );

  const isEdgeSelected = useCallback(
    (edgeId: string) => state.selectedEdgeIds.has(edgeId),
    [state.selectedEdgeIds],
  );

  return useMemo(
    () => ({
      state,
      replaceSelection,
      addToSelection,
      removeFromSelection,
      clearSelection,
      resolvePrimaryTarget,
      applyNodeSelectChanges,
      applyEdgeSelectChanges,
      applyReactFlowSelection,
      handleGraphItemClick,
      isNodeSelected,
      isEdgeSelected,
    }),
    [
      state,
      replaceSelection,
      addToSelection,
      removeFromSelection,
      clearSelection,
      resolvePrimaryTarget,
      applyNodeSelectChanges,
      applyEdgeSelectChanges,
      applyReactFlowSelection,
      handleGraphItemClick,
      isNodeSelected,
      isEdgeSelected,
    ],
  );
}
