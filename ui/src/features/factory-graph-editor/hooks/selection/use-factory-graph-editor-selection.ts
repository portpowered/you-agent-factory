import type { NodeChange } from "@xyflow/react";
import { useCallback, useMemo, useState } from "react";

import {
  addToFactoryGraphEditorSelection,
  applyFactoryGraphEditorNodeSelectChanges,
  clearFactoryGraphEditorSelection,
  createEmptyFactoryGraphEditorSelection,
  type FactoryGraphEditorSelectionItems,
  type FactoryGraphEditorSelectionState,
  removeFromFactoryGraphEditorSelection,
  replaceFactoryGraphEditorSelection,
  resolveFactoryGraphEditorPrimaryTarget,
} from "../../lib/selection/factory-graph-editor-selection";

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
  isNodeSelected: (nodeId: string) => boolean;
  isEdgeSelected: (edgeId: string) => boolean;
};

export function useFactoryGraphEditorSelection(): FactoryGraphEditorSelectionController {
  const [state, setState] = useState(createEmptyFactoryGraphEditorSelection);

  const replaceSelection = useCallback(
    (items: FactoryGraphEditorSelectionItems) => {
      setState((current) => replaceFactoryGraphEditorSelection(current, items));
    },
    [],
  );

  const addToSelection = useCallback(
    (items: FactoryGraphEditorSelectionItems) => {
      setState((current) => addToFactoryGraphEditorSelection(current, items));
    },
    [],
  );

  const removeFromSelection = useCallback(
    (items: FactoryGraphEditorSelectionItems) => {
      setState((current) =>
        removeFromFactoryGraphEditorSelection(current, items),
      );
    },
    [],
  );

  const clearSelection = useCallback(() => {
    setState(clearFactoryGraphEditorSelection());
  }, []);

  const resolvePrimaryTarget = useCallback(
    () => resolveFactoryGraphEditorPrimaryTarget(state),
    [state],
  );

  const applyNodeSelectChanges = useCallback(
    (changes: readonly NodeChange[]) => {
      setState((current) =>
        applyFactoryGraphEditorNodeSelectChanges(current, changes),
      );
    },
    [],
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
      isNodeSelected,
      isEdgeSelected,
    ],
  );
}
