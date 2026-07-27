import type { EdgeChange, OnSelectionChangeFunc } from "@xyflow/react";
import { useCallback, useRef } from "react";

import type { FactoryGraphEditorSelectionController } from "../../factory-graph-editor/hooks/selection/use-factory-graph-editor-selection";
import {
  resolveCurrentActivityGraphSelectionChange,
  selectGraphEdgePresentationChanges,
} from "./state/current-activity-graph-selection-gesture-state";

export function useCurrentActivityGraphEdgePresentation(
  graphSelection: FactoryGraphEditorSelectionController,
  graphSelectionGesturesEnabled: boolean,
) {
  const handleEdgesChange = useCallback(
    (changes: EdgeChange[]) => {
      const selectionChanges = selectGraphEdgePresentationChanges(
        changes,
        graphSelectionGesturesEnabled,
      );

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
      const resolution = resolveCurrentActivityGraphSelectionChange({
        additive: additiveMarqueeRef.current,
        edges: params.edges,
        enabled: graphSelectionEnabled,
        nodes: params.nodes,
        selectionState: graphSelection.state,
      });
      if (resolution) {
        graphSelection.applyReactFlowSelection(
          resolution.items,
          resolution.mode,
        );
      }
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
