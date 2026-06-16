import type { ReactFlowProps } from "@xyflow/react";

export const FACTORY_GRAPH_EDITOR_REACT_FLOW_GESTURE_PROPS = {
  deleteKeyCode: null,
  elementsSelectable: true,
  panActivationKeyCode: "Space",
  panOnDrag: false,
  panOnScroll: true,
  selectionOnDrag: true,
  zoomOnPinch: true,
  zoomOnScroll: false,
} satisfies Pick<
  ReactFlowProps,
  | "deleteKeyCode"
  | "elementsSelectable"
  | "panActivationKeyCode"
  | "panOnDrag"
  | "panOnScroll"
  | "selectionOnDrag"
  | "zoomOnPinch"
  | "zoomOnScroll"
>;
