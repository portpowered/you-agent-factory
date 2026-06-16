import type { EdgeChange, OnSelectionChangeFunc } from "@xyflow/react";
import { useCallback, useRef } from "react";

import type { FactoryGraphEditorSelectionController } from "../../factory-graph-editor/hooks/selection/use-factory-graph-editor-selection";
import {
  collectFactoryGraphSelectionItemsFromReactFlow,
  isFactoryGraphEditorReactFlowSelectionNoOp,
} from "../../factory-graph-editor/lib/selection/factory-graph-editor-selection-gestures";

export function useCurrentActivityGraphEdgePresentation(
  graphSelection: FactoryGraphEditorSelectionController,
  graphSelectionGesturesEnabled: boolean,
) {
  const handleEdgesChange = useCallback(
    (changes: EdgeChange[]) => {
      const selectionChanges = changes.filter((change) => {
        if (
          change.type === "remove" ||
          change.type === "add" ||
          change.type === "replace"
        ) {
          return true;
        }

        return change.type === "select" && !graphSelectionGesturesEnabled;
      });

      if (selectionChanges.length > 0) {
        graphSelection.applyEdgeSelectChanges(selectionChanges);
      }
    },
    [graphSelection, graphSelectionGesturesEnabled],
  );

  return { handleEdgesChange };
}

export function useCurrentActivityGraphSelectionGestures(
  graphSelection: FactoryGraphEditorSelectionController,
  graphSelectionEnabled: boolean,
) {
  const additiveMarqueeRef = useRef(false);

  const handleGraphSelectionChange = useCallback<OnSelectionChangeFunc>(
    (params) => {
      if (!graphSelectionEnabled) {
        return;
      }

      const mode = additiveMarqueeRef.current ? "add" : "replace";
      const items = collectFactoryGraphSelectionItemsFromReactFlow(
        params.nodes,
        params.edges,
      );

      if (
        isFactoryGraphEditorReactFlowSelectionNoOp(
          graphSelection.state,
          items,
          mode,
        )
      ) {
        additiveMarqueeRef.current = false;
        return;
      }

      graphSelection.applyReactFlowSelection(items, mode);
      additiveMarqueeRef.current = false;
    },
    [graphSelection, graphSelectionEnabled],
  );

  const handleGraphSelectionStart = useCallback(
    (event: { shiftKey: boolean }) => {
      additiveMarqueeRef.current = event.shiftKey;
    },
    [],
  );

  const clearGraphSelection = useCallback(() => {
    graphSelection.clearSelection();
  }, [graphSelection]);

  return {
    clearGraphSelection,
    handleGraphSelectionChange,
    handleGraphSelectionStart,
  };
}
