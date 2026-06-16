import type { EdgeChange, OnSelectionChangeFunc } from "@xyflow/react";
import { useCallback, useRef } from "react";

import type { FactoryGraphEditorSelectionController } from "../../factory-graph-editor/hooks/selection/use-factory-graph-editor-selection";
import { collectFactoryGraphSelectionItemsFromReactFlow } from "../../factory-graph-editor/lib/selection/factory-graph-editor-selection-gestures";

export function useCurrentActivityGraphEdgePresentation(
  graphSelection: FactoryGraphEditorSelectionController,
) {
  const handleEdgesChange = useCallback(
    (changes: EdgeChange[]) => {
      const selectionChanges = changes.filter(
        (change) =>
          change.type === "select" ||
          change.type === "remove" ||
          change.type === "add" ||
          change.type === "replace",
      );

      if (selectionChanges.length > 0) {
        graphSelection.applyEdgeSelectChanges(selectionChanges);
      }
    },
    [graphSelection],
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

      graphSelection.applyReactFlowSelection(
        collectFactoryGraphSelectionItemsFromReactFlow(
          params.nodes,
          params.edges,
        ),
        additiveMarqueeRef.current ? "add" : "replace",
      );
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
