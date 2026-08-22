import type { ReactFlowProps } from "@xyflow/react";

export const FACTORY_GRAPH_EDITOR_REACT_FLOW_GESTURE_PROPS = {
  deleteKeyCode: null,
  elementsSelectable: true,
  panActivationKeyCode: "Space",
  // React Flow button 1 is the middle mouse button. Keeping button 0 out of
  // this allowlist preserves primary-button marquee selection, while the
  // custom touch handler below the React Flow surface continues to own touch
  // pane panning.
  panOnDrag: [1],
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
