import { describe, expect, it } from "vitest";

import { FACTORY_GRAPH_EDITOR_REACT_FLOW_GESTURE_PROPS } from "./factory-graph-editor-react-flow-interaction";

describe("factory-graph-editor-react-flow-interaction", () => {
  it("uses selection-first pan and zoom defaults", () => {
    expect(FACTORY_GRAPH_EDITOR_REACT_FLOW_GESTURE_PROPS).toEqual({
      deleteKeyCode: null,
      elementsSelectable: true,
      panActivationKeyCode: "Space",
      panOnDrag: false,
      panOnScroll: true,
      selectionOnDrag: true,
      zoomOnPinch: true,
      zoomOnScroll: false,
    });
  });
});
