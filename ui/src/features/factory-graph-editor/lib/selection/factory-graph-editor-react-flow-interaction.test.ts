import { describe, expect, it } from "vitest";

import {
  FACTORY_GRAPH_EDITOR_REACT_FLOW_GESTURE_PROPS,
  isTouchPanePointerDown,
} from "./factory-graph-editor-react-flow-interaction";

describe("factory-graph-editor-react-flow-interaction", () => {
  it("uses selection-first pan and zoom defaults", () => {
    expect(FACTORY_GRAPH_EDITOR_REACT_FLOW_GESTURE_PROPS).toEqual({
      deleteKeyCode: null,
      elementsSelectable: true,
      panActivationKeyCode: "Space",
      panOnDrag: [],
      panOnScroll: true,
      selectionOnDrag: true,
      zoomOnPinch: true,
      zoomOnScroll: false,
    });
  });

  it("distinguishes a touch on the pane from mouse and node gestures", () => {
    const pane = document.createElement("div");
    pane.className = "react-flow__pane";
    const node = document.createElement("div");
    node.className = "react-flow__node";

    expect(isTouchPanePointerDown({ pointerType: "touch", target: pane })).toBe(
      true,
    );
    expect(isTouchPanePointerDown({ pointerType: "mouse", target: pane })).toBe(
      false,
    );
    expect(isTouchPanePointerDown({ pointerType: "touch", target: node })).toBe(
      false,
    );
  });
});
