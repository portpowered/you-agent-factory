import type { ReactFlowProps } from "@xyflow/react";

export const FACTORY_GRAPH_EDITOR_REACT_FLOW_GESTURE_PROPS = {
  deleteKeyCode: null,
  elementsSelectable: true,
  panActivationKeyCode: "Space",
  // An empty mouse-button allowlist keeps primary-button marquee selection
  // while React Flow still accepts touchstart gestures for pane panning.
  panOnDrag: [],
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

export function isTouchPanePointerDown(input: {
  pointerType: string;
  target: EventTarget | null;
}): boolean {
  return (
    input.pointerType === "touch" &&
    input.target instanceof Element &&
    input.target.classList.contains("react-flow__pane")
  );
}
