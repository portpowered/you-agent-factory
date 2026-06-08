import type { Node } from "@xyflow/react";
import { describe, expect, it } from "vitest";

import type { FactoryGraphAddEntityDraft } from "../../factory-graph-editor/lib/editor/factory-graph-editor-additions";
import {
  factoryGraphNodeIdForAddEntityDraft,
  occupiedRectsFromRenderedNodes,
  resolveInitialPlacementTopLeft,
} from "./graph-editor-add-node-placement";
import { graphEditorNodeDimensionsForKind } from "./graph-editor-node-placement";

function workerNode(id: string, position: { x: number; y: number }): Node {
  const dimensions = graphEditorNodeDimensionsForKind("worker");
  return {
    data: { kind: "worker" },
    height: dimensions.height,
    id,
    position,
    width: dimensions.width,
  };
}

function workstationNode(id: string, position: { x: number; y: number }): Node {
  const dimensions = graphEditorNodeDimensionsForKind("workstation");
  return {
    data: { kind: "workstation" },
    height: dimensions.height,
    id,
    position,
    width: dimensions.width,
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
    const workerSize = graphEditorNodeDimensionsForKind("worker");
    const topLeft = resolveInitialPlacementTopLeft({
      draft: workerDraft,
      nodes: [],
      storedPositions: {},
      viewportCenter: { x: 500, y: 300 },
    });

    expect(topLeft).toEqual({
      x: 500 - workerSize.width / 2,
      y: 300 - workerSize.height / 2,
    });
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
    const workerSize = graphEditorNodeDimensionsForKind("worker");
    const centeredTopLeft = {
      x: 500 - workerSize.width / 2,
      y: 300 - workerSize.height / 2,
    };
    const nodes = [workerNode("worker:writer", centeredTopLeft)];
    const topLeft = resolveInitialPlacementTopLeft({
      draft: workerDraft,
      nodes,
      storedPositions: {},
      viewportCenter: { x: 500, y: 300 },
    });

    expect(topLeft).not.toEqual(centeredTopLeft);
    expect(topLeft).not.toBeNull();
  });

  it("does not include the new node id in occupied bounds when it is already rendered", () => {
    const workerSize = graphEditorNodeDimensionsForKind("worker");
    const centeredTopLeft = {
      x: 500 - workerSize.width / 2,
      y: 300 - workerSize.height / 2,
    };
    const nodes = [workerNode("worker:reviewer", centeredTopLeft)];
    const topLeft = resolveInitialPlacementTopLeft({
      draft: workerDraft,
      nodes,
      storedPositions: {},
      viewportCenter: { x: 500, y: 300 },
    });

    expect(topLeft).toEqual(centeredTopLeft);
  });

  it("changes the computed top-left when the viewport center moves for worker and workstation kinds", () => {
    const workstationDraft: FactoryGraphAddEntityDraft = {
      behavior: "STANDARD",
      body: "",
      kind: "workstation",
      name: "review",
      workerName: "writer",
    };
    const nearCenter = { x: 500, y: 300 };
    const farCenter = { x: 1200, y: 900 };

    const workerNear = resolveInitialPlacementTopLeft({
      draft: workerDraft,
      nodes: [],
      storedPositions: {},
      viewportCenter: nearCenter,
    });
    const workerFar = resolveInitialPlacementTopLeft({
      draft: workerDraft,
      nodes: [],
      storedPositions: {},
      viewportCenter: farCenter,
    });
    const workstationNear = resolveInitialPlacementTopLeft({
      draft: workstationDraft,
      nodes: [],
      storedPositions: {},
      viewportCenter: nearCenter,
    });
    const workstationFar = resolveInitialPlacementTopLeft({
      draft: workstationDraft,
      nodes: [],
      storedPositions: {},
      viewportCenter: farCenter,
    });

    expect(workerNear).not.toEqual(workerFar);
    expect(workstationNear).not.toEqual(workstationFar);
    expect(workerNear).not.toBeNull();
    expect(workerFar).not.toBeNull();
    expect(workstationNear).not.toBeNull();
    expect(workstationFar).not.toBeNull();
  });
});

describe("resolveInitialPlacementTopLeft for workstations", () => {
  const workstationDraft: FactoryGraphAddEntityDraft = {
    behavior: "STANDARD",
    body: "",
    kind: "workstation",
    name: "review",
    workerName: "writer",
  };

  it("centers at the viewport when the canvas center is free", () => {
    const viewportCenter = { x: 500, y: 300 };
    const workstationSize = graphEditorNodeDimensionsForKind("workstation");

    const topLeft = resolveInitialPlacementTopLeft({
      draft: workstationDraft,
      nodes: [],
      storedPositions: {},
      viewportCenter,
    });

    expect(topLeft).toEqual({
      x: viewportCenter.x - workstationSize.width / 2,
      y: viewportCenter.y - workstationSize.height / 2,
    });
  });

  it("nudges away from occupied nodes at the viewport center", () => {
    const viewportCenter = { x: 500, y: 300 };
    const workstationSize = graphEditorNodeDimensionsForKind("workstation");
    const centeredTopLeft = {
      x: viewportCenter.x - workstationSize.width / 2,
      y: viewportCenter.y - workstationSize.height / 2,
    };
    const nodes = [workstationNode("workstation:writer", centeredTopLeft)];

    const topLeft = resolveInitialPlacementTopLeft({
      draft: workstationDraft,
      nodes,
      storedPositions: {},
      viewportCenter,
    });

    expect(topLeft).not.toEqual(centeredTopLeft);
    expect(topLeft).not.toBeNull();
  });
});

describe("occupiedRectsFromRenderedNodes", () => {
  it("builds axis-aligned rects from rendered node positions and kind dimensions", () => {
    const dimensions = graphEditorNodeDimensionsForKind("worker");
    const rects = occupiedRectsFromRenderedNodes([
      workerNode("worker:writer", { x: 100, y: 200 }),
    ]);

    expect(rects).toEqual([
      { height: dimensions.height, width: dimensions.width, x: 100, y: 200 },
    ]);
  });

  it("uses measured node size when kind metadata is unavailable", () => {
    const rects = occupiedRectsFromRenderedNodes([
      {
        height: 120,
        id: "unknown:node",
        position: { x: 40, y: 50 },
        width: 80,
      },
    ]);

    expect(rects).toEqual([{ height: 120, width: 80, x: 40, y: 50 }]);
  });
});
