import { describe, expect, it } from "vitest";

import { baseFactoryDefinition } from "../draft/factory-graph-draft.test-helpers";
import {
  createDefaultFactoryLayout,
  factoryLayoutFromDefinition,
  factoryLayoutNodePosition,
  hasFactoryLayoutChanges,
  moveFactoryLayoutNode,
  moveFactoryLayoutNodesByDelta,
  resolveProjectedLayoutPositions,
  updateFactoryLayoutViewport,
} from "./factory-graph-layout-operations";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: layout operation cases share one canonical graph fixture vocabulary.
describe("factory graph layout operations", () => {
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
