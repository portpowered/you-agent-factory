import type { Node } from "@xyflow/react";
import { resolveFactoryGraphNodeDimensions } from "@you-agent-factory/factory-graph";
import { describe, expect, it } from "vitest";

import type { FactoryGraphAddEntityDraft } from "../../factory-graph-editor/lib/editor/factory-graph-editor-additions";
import {
  factoryGraphNodeIdForAddEntityDraft,
  occupiedRectsFromRenderedNodes,
  resolveInitialPlacementTopLeft,
  resolveInitialPlacementTopLeftForViewport,
  viewportCenterFromPlacementViewport,
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
    expect(
      factoryGraphNodeIdForAddEntityDraft({
        fileName: "playbook.md",
        inlineContent: "",
        kind: "doc",
      }),
    ).toBe("doc:factory/docs/playbook.md");
  });
});

describe("resolveInitialPlacementTopLeft for docs", () => {
  const docDraft: FactoryGraphAddEntityDraft = {
    fileName: "playbook.md",
    inlineContent: "# Playbook\n",
    kind: "doc",
  };

  it("centers doc nodes at the viewport when the canvas center is free", () => {
    const docSize = resolveFactoryGraphNodeDimensions("doc", {
      content: ["factory/docs/playbook.md"],
    }).resolvedDimensions;
    const topLeft = resolveInitialPlacementTopLeft({
      draft: docDraft,
      nodes: [],
      viewportCenter: { x: 640, y: 360 },
    });

    expect(topLeft).toEqual({
      x: 640 - docSize.width / 2,
      y: 360 - docSize.height / 2,
    });
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
      viewportCenter: { x: 500, y: 300 },
    });

    expect(topLeft).toEqual({
      x: 500 - workerSize.width / 2,
      y: 300 - workerSize.height / 2,
    });
  });

  it("uses the fitted draft label size when placing a new node", () => {
    const draft: FactoryGraphAddEntityDraft = {
      kind: "worker",
      model: "gpt",
      name: "reviewer-with-a-deliberately-long-identifier",
    };
    const fittedSize = resolveFactoryGraphNodeDimensions("worker", {
      content: [draft.name],
    }).resolvedDimensions;

    const topLeft = resolveInitialPlacementTopLeft({
      draft,
      nodes: [],
      viewportCenter: { x: 500, y: 300 },
    });

    expect(topLeft).toEqual({
      x: 500 - fittedSize.width / 2,
      y: 300 - fittedSize.height / 2,
    });
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
      viewportCenter: nearCenter,
    });
    const workerFar = resolveInitialPlacementTopLeft({
      draft: workerDraft,
      nodes: [],
      viewportCenter: farCenter,
    });
    const workstationNear = resolveInitialPlacementTopLeft({
      draft: workstationDraft,
      nodes: [],
      viewportCenter: nearCenter,
    });
    const workstationFar = resolveInitialPlacementTopLeft({
      draft: workstationDraft,
      nodes: [],
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

describe("viewportCenterFromPlacementViewport", () => {
  it("converts measured viewport size and React Flow transform into flow coordinates", () => {
    expect(
      viewportCenterFromPlacementViewport({
        height: 800,
        viewport: { x: -100, y: -50, zoom: 2 },
        width: 1000,
      }),
    ).toEqual({ x: 300, y: 225 });
  });

  it("resolves top-left placement from the current viewport snapshot", () => {
    const workerSize = graphEditorNodeDimensionsForKind("worker");
    const topLeft = resolveInitialPlacementTopLeftForViewport({
      draft: {
        kind: "worker",
        model: "gpt",
        name: "reviewer",
      },
      nodes: [],
      placementViewport: {
        height: 800,
        viewport: { x: -100, y: -50, zoom: 2 },
        width: 1000,
      },
    });

    expect(topLeft).toEqual({
      x: 300 - workerSize.width / 2,
      y: 225 - workerSize.height / 2,
    });
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

  it("uses the rendered fitted size for known node families", () => {
    const rects = occupiedRectsFromRenderedNodes([
      {
        data: { kind: "worker" },
        height: 58,
        id: "worker:writer",
        position: { x: 40, y: 50 },
        width: 320,
      },
    ]);

    expect(rects).toEqual([{ height: 58, width: 320, x: 40, y: 50 }]);
  });
});
