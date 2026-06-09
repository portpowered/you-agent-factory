import { render, waitFor } from "@testing-library/react";
import type { ReactFlowInstance } from "@xyflow/react";
import { useEffect, useRef } from "react";
import { describe, expect, it, vi } from "vitest";

import { graphEditorNodeDimensionsForKind } from "../lib/graph-editor-node-placement";
import {
  GraphEditorPlacementProvider,
  GraphEditorPlacementRegistrar,
  useGraphEditorPlaceAddedNode,
} from "./graph-editor-placement-context";

function mockContainerRect(): DOMRect {
  return {
    bottom: 650,
    height: 600,
    left: 100,
    right: 900,
    toJSON: () => ({}),
    top: 50,
    width: 800,
    x: 100,
    y: 50,
  } as DOMRect;
}

function TriggerWorkstationPlacement() {
  const placeAddedNode = useGraphEditorPlaceAddedNode();

  useEffect(() => {
    placeAddedNode?.({
      behavior: "STANDARD",
      body: "",
      kind: "workstation",
      name: "review",
      workerName: "writer",
    });
  }, [placeAddedNode]);

  return null;
}

describe("GraphEditorPlacementRegistrar regression", () => {
  it("places added nodes in the canonical layout from the live viewport center", async () => {
    const moveLayoutNode = vi.fn();
    const workstationSize = graphEditorNodeDimensionsForKind("workstation");
    const screenCenter = { x: 500, y: 350 };
    const flowCenter = { x: 300, y: 200 };

    function Harness() {
      const flowContainerRef = useRef<HTMLDivElement>(null);
      const flowInstanceRef = useRef<ReactFlowInstance>({
        screenToFlowPosition: vi.fn(({ x, y }) => ({
          x: x - (screenCenter.x - flowCenter.x),
          y: y - (screenCenter.y - flowCenter.y),
        })),
      } as unknown as ReactFlowInstance);

      return (
        <div
          ref={(element) => {
            flowContainerRef.current = element;
            if (element) {
              element.getBoundingClientRect = () => mockContainerRect();
            }
          }}
        >
          <GraphEditorPlacementRegistrar
            flowContainerRef={flowContainerRef}
            flowInstanceRef={flowInstanceRef}
            moveLayoutNode={moveLayoutNode}
            nodes={[]}
          />
          <TriggerWorkstationPlacement />
        </div>
      );
    }

    render(
      <GraphEditorPlacementProvider>
        <Harness />
      </GraphEditorPlacementProvider>,
    );

    const expectedTopLeft = {
      x: flowCenter.x - workstationSize.width / 2,
      y: flowCenter.y - workstationSize.height / 2,
    };
    await waitFor(() => {
      expect(moveLayoutNode).toHaveBeenCalledWith(
        "workstation:review",
        expectedTopLeft,
      );
    });
  });
});
