import { describe, expect, it } from "vitest";

import { baseFactoryDefinition } from "../draft/factory-graph-draft.test-helpers";
import {
  createDefaultFactoryLayout,
  factoryLayoutFromDefinition,
  factoryLayoutNodePosition,
  factoryLayoutNodeSize,
  fitFactoryLayoutNode,
  hasFactoryLayoutChanges,
  moveFactoryLayoutNode,
  moveFactoryLayoutNodesByDelta,
  resetFactoryLayoutNodeSize,
  resizeFactoryLayoutNode,
  resolveProjectedLayoutPositions,
  updateFactoryLayoutViewport,
} from "./factory-graph-layout-operations";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: layout operation cases share one canonical graph fixture vocabulary.
describe("factory graph layout operations", () => {
  it("normalizes node sizes by family while preserving unrelated layout data", () => {
    const layout = {
      schemaVersion: 1,
      edges: [{ id: "edge-1", waypoints: [{ x: 8, y: 12 }] }],
      nodes: [
        {
          id: "workstation:draft",
          locked: true,
          position: { x: 40, y: 80 },
          size: { height: 240, width: 240 },
        },
      ],
    } satisfies ReturnType<typeof createDefaultFactoryLayout>;

    const resized = resizeFactoryLayoutNode(
      layout,
      "workstation:draft",
      "workstation",
      { height: 400, width: 9999 },
      { x: 40, y: 80 },
    );

    expect(factoryLayoutNodeSize(resized, "workstation:draft")).toEqual({
      height: 400,
      width: 520,
    });
    expect(resized.nodes?.[0]).toMatchObject({
      id: "workstation:draft",
      locked: true,
      position: { x: 40, y: 80 },
    });
    expect(resized.edges).toEqual(layout.edges);

    const reset = resetFactoryLayoutNodeSize(resized, "workstation:draft");
    expect(factoryLayoutNodeSize(reset, "workstation:draft")).toBeUndefined();
    expect(reset.nodes?.[0]).toMatchObject({
      id: "workstation:draft",
      locked: true,
      position: { x: 40, y: 80 },
    });
  });

  it("fits long content and normalizes invalid resize requests", () => {
    const layout = createDefaultFactoryLayout();
    const fitted = fitFactoryLayoutNode(
      layout,
      "worker:writer",
      "worker",
      "a-long-unbroken-worker-identifier-that-needs-safe-fitting",
      { x: 10, y: 20 },
    );

    expect(factoryLayoutNodeSize(fitted, "worker:writer")).toEqual({
      height: 58,
      width: 360,
    });

    const invalid = resizeFactoryLayoutNode(
      fitted,
      "worker:writer",
      "worker",
      { height: Number.NaN, width: Number.POSITIVE_INFINITY },
      { x: 10, y: 20 },
    );

    expect(factoryLayoutNodeSize(invalid, "worker:writer")).toEqual({
      height: 58,
      width: 156,
    });
  });

  it("moves one node into canonical layout.nodes", () => {
    const layout = createDefaultFactoryLayout();

    const nextLayout = moveFactoryLayoutNode(layout, "workstation:draft", {
      x: 120,
      y: 240,
    });

    expect(nextLayout.nodes).toEqual([
      {
        id: "workstation:draft",
        position: { x: 120, y: 240 },
      },
    ]);
    expect(factoryLayoutNodePosition(nextLayout, "workstation:draft")).toEqual({
      x: 120,
      y: 240,
    });
  });

  it("moves every selected node by the same delta", () => {
    const layout = moveFactoryLayoutNode(
      createDefaultFactoryLayout(),
      "worker:writer",
      {
        x: 40,
        y: 80,
      },
    );
    const resolvedPositions = new Map([
      ["worker:writer", { x: 40, y: 80 }],
      ["workstation:draft", { x: 200, y: 100 }],
    ]);

    const nextLayout = moveFactoryLayoutNodesByDelta(
      layout,
      ["worker:writer", "workstation:draft"],
      { x: 12, y: -6 },
      resolvedPositions,
    );

    expect(factoryLayoutNodePosition(nextLayout, "worker:writer")).toEqual({
      x: 52,
      y: 74,
    });
    expect(factoryLayoutNodePosition(nextLayout, "workstation:draft")).toEqual({
      x: 212,
      y: 94,
    });
  });

  it("falls back to automatic layout positions for nodes without saved layout", () => {
    const layout = moveFactoryLayoutNode(
      createDefaultFactoryLayout(),
      "worker:writer",
      {
        x: 10,
        y: 20,
      },
    );
    const autoLayoutPositions = new Map([
      ["worker:writer", { x: 10, y: 20 }],
      ["workstation:draft", { x: 300, y: 150 }],
    ]);

    const projected = resolveProjectedLayoutPositions({
      autoLayoutPositionsByNodeId: autoLayoutPositions,
      canonicalLayout: layout,
      nodeIds: ["worker:writer", "workstation:draft", "resource:gpu"],
    });

    expect(projected.get("worker:writer")).toEqual({ x: 10, y: 20 });
    expect(projected.get("workstation:draft")).toEqual({ x: 300, y: 150 });
    expect(projected.has("resource:gpu")).toBe(false);
  });

  it("updates canonical layout viewport metadata", () => {
    const layout = createDefaultFactoryLayout();

    const nextLayout = updateFactoryLayoutViewport(layout, {
      x: 120,
      y: 80,
      zoom: 1.25,
    });

    expect(nextLayout.viewport).toEqual({
      x: 120,
      y: 80,
      zoom: 1.25,
    });
    expect(hasFactoryLayoutChanges(layout, nextLayout)).toBe(true);
  });

  it("detects layout changes against the loaded factory document", () => {
    const baseLayout = factoryLayoutFromDefinition({
      ...baseFactoryDefinition,
      layout: {
        schemaVersion: 1,
        nodes: [
          {
            id: "workstation:draft",
            position: { x: 10, y: 20 },
          },
        ],
      },
    });
    const pendingLayout = moveFactoryLayoutNode(
      baseLayout,
      "workstation:draft",
      {
        x: 44,
        y: 88,
      },
    );

    expect(hasFactoryLayoutChanges(baseLayout, pendingLayout)).toBe(true);
    expect(hasFactoryLayoutChanges(baseLayout, baseLayout)).toBe(false);
  });

  it("loads visual groups from the factory document without dropping metadata", () => {
    const layout = factoryLayoutFromDefinition({
      ...baseFactoryDefinition,
      layout: {
        schemaVersion: 1,
        groups: [
          {
            bounds: { x: 360, y: 120, width: 520, height: 360 },
            color: "blue",
            id: "review-lane",
            label: "Review",
            locked: false,
            nodeIds: ["workstation:draft"],
            parentGroupId: null,
          },
        ],
      },
    });

    expect(layout.groups).toEqual([
      {
        bounds: { x: 360, y: 120, width: 520, height: 360 },
        color: "blue",
        id: "review-lane",
        label: "Review",
        locked: false,
        nodeIds: ["workstation:draft"],
        parentGroupId: null,
      },
    ]);
  });
});
