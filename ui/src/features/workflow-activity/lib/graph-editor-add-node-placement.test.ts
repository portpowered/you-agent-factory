import type { Node } from "@xyflow/react";
import { describe, expect, it } from "vitest";

import type { FactoryGraphAddEntityDraft } from "../../factory-graph-editor/lib/factory-graph-editor-additions";
import {
  factoryGraphNodeIdForAddEntityDraft,
  occupiedRectsFromRenderedNodes,
  resolveInitialPlacementTopLeft,
} from "./graph-editor-add-node-placement";

function workerNode(
  id: string,
  position: { x: number; y: number },
): Node {
  return {
    data: { kind: "worker" },
    height: 86,
    id,
    position,
    width: 164,
  };
}

describe("factoryGraphNodeIdForAddEntityDraft", () => {
  it("maps each add draft kind to the canonical factory graph node id", () => {
    expect(
      factoryGraphNodeIdForAddEntityDraft({
        kind: "worker",
        model: "gpt",
        name: "reviewer",
      }),
    ).toBe("worker:reviewer");
    expect(
      factoryGraphNodeIdForAddEntityDraft({
        behavior: "STANDARD",
        body: "",
        kind: "workstation",
        name: "draft",
        workerName: "writer",
      }),
    ).toBe("workstation:draft");
    expect(
      factoryGraphNodeIdForAddEntityDraft({
        kind: "work-state",
        name: "queued",
        stateType: "INITIAL",
        workTypeName: "story",
      }),
    ).toBe("work-state:story:queued");
  });
});

describe("resolveInitialPlacementTopLeft", () => {
  const workerDraft: FactoryGraphAddEntityDraft = {
    kind: "worker",
    model: "gpt",
    name: "reviewer",
  };

  it("returns a viewport-centered top-left position when the canvas center is free", () => {
    const topLeft = resolveInitialPlacementTopLeft({
      draft: workerDraft,
      nodes: [],
      storedPositions: {},
      viewportCenter: { x: 500, y: 300 },
    });

    expect(topLeft).toEqual({ x: 418, y: 257 });
  });

  it("skips placement when the new node already has a stored position", () => {
    const topLeft = resolveInitialPlacementTopLeft({
      draft: workerDraft,
      nodes: [],
      storedPositions: {
        "worker:reviewer": { x: 12, y: 34 },
      },
      viewportCenter: { x: 500, y: 300 },
    });

    expect(topLeft).toBeNull();
  });

  it("nudges away from occupied nodes at the viewport center", () => {
    const nodes = [workerNode("worker:writer", { x: 418, y: 257 })];
    const topLeft = resolveInitialPlacementTopLeft({
      draft: workerDraft,
      nodes,
      storedPositions: {},
      viewportCenter: { x: 500, y: 300 },
    });

    expect(topLeft).not.toEqual({ x: 418, y: 257 });
    expect(topLeft).not.toBeNull();
  });

  it("does not include the new node id in occupied bounds when it is already rendered", () => {
    const nodes = [workerNode("worker:reviewer", { x: 418, y: 257 })];
    const topLeft = resolveInitialPlacementTopLeft({
      draft: workerDraft,
      nodes,
      storedPositions: {},
      viewportCenter: { x: 500, y: 300 },
    });

    expect(topLeft).toEqual({ x: 418, y: 257 });
  });
});

describe("occupiedRectsFromRenderedNodes", () => {
  it("builds axis-aligned rects from rendered node positions and kind dimensions", () => {
    const rects = occupiedRectsFromRenderedNodes([
      workerNode("worker:writer", { x: 100, y: 200 }),
    ]);

    expect(rects).toEqual([{ height: 86, width: 164, x: 100, y: 200 }]);
  });
});
