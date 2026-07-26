import type { Edge, EdgeChange, Node } from "@xyflow/react";

import {
  collectFactoryGraphSelectionItemsFromReactFlow,
  type FactoryGraphEditorSelectionState,
  isFactoryGraphEditorReactFlowSelectionNoOp,
} from "../../../factory-graph-editor/public/selection-gestures";

export function selectGraphEdgePresentationChanges(
  changes: EdgeChange[],
  graphSelectionGesturesEnabled: boolean,
): EdgeChange[] {
  return changes.filter((change) => {
    if (
      change.type === "remove" ||
      change.type === "add" ||
      change.type === "replace"
    ) {
      return true;
    }
    return change.type === "select" && !graphSelectionGesturesEnabled;
  });
}

export function resolveCurrentActivityGraphSelectionChange({
  additive,
  edges,
  enabled,
  nodes,
  selectionState,
}: {
  additive: boolean;
  edges: Edge[];
  enabled: boolean;
  nodes: Node[];
  selectionState: FactoryGraphEditorSelectionState;
}) {
  if (!enabled) return null;
  const mode: "add" | "replace" = additive ? "add" : "replace";
  const items = collectFactoryGraphSelectionItemsFromReactFlow(nodes, edges);
  return isFactoryGraphEditorReactFlowSelectionNoOp(selectionState, items, mode)
    ? null
    : { items, mode };
}
