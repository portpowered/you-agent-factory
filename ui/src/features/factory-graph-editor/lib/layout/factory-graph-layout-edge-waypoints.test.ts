import { describe, expect, it } from "vitest";

import { createDefaultFactoryLayout } from "./factory-graph-layout-operations";
import {
  addFactoryLayoutEdgeWaypoint,
  factoryLayoutEdgeWaypoints,
  moveFactoryLayoutEdgeWaypoint,
  removeFactoryLayoutEdgeWaypoint,
  setFactoryLayoutEdgeWaypoints,
} from "./factory-graph-layout-edge-waypoints";

const EDGE_ID =
  "workstation-output:workstation:review->work-state:story:done";

describe("factory-graph-layout-edge-waypoints", () => {
  it("adds and moves waypoints in canonical layout state", () => {
    const baseLayout = createDefaultFactoryLayout();
    const withWaypoint = addFactoryLayoutEdgeWaypoint(baseLayout, EDGE_ID, {
      x: 120,
      y: 80,
    });

    expect(factoryLayoutEdgeWaypoints(withWaypoint, EDGE_ID)).toEqual([
      { x: 120, y: 80 },
    ]);

    const moved = moveFactoryLayoutEdgeWaypoint(withWaypoint, EDGE_ID, 0, {
      x: 180,
      y: 140,
    });

    expect(factoryLayoutEdgeWaypoints(moved, EDGE_ID)).toEqual([
      { x: 180, y: 140 },
    ]);
  });

  it("clears edge layout entries when the last waypoint is removed", () => {
    const layout = setFactoryLayoutEdgeWaypoints(createDefaultFactoryLayout(), EDGE_ID, [
      { x: 10, y: 20 },
    ]);
    const cleared = setFactoryLayoutEdgeWaypoints(layout, EDGE_ID, null);

    expect(factoryLayoutEdgeWaypoints(cleared, EDGE_ID)).toBeUndefined();
    expect(cleared.edges).toBeUndefined();
  });

  it("removes one waypoint while preserving order for remaining waypoints", () => {
    const layout = setFactoryLayoutEdgeWaypoints(createDefaultFactoryLayout(), EDGE_ID, [
      { x: 10, y: 20 },
      { x: 30, y: 40 },
      { x: 50, y: 60 },
    ]);

    const withoutMiddle = removeFactoryLayoutEdgeWaypoint(layout, EDGE_ID, 1);

    expect(factoryLayoutEdgeWaypoints(withoutMiddle, EDGE_ID)).toEqual([
      { x: 10, y: 20 },
      { x: 50, y: 60 },
    ]);
  });

  it("removes the last authored waypoint and restores generated routing", () => {
    const layout = setFactoryLayoutEdgeWaypoints(createDefaultFactoryLayout(), EDGE_ID, [
      { x: 10, y: 20 },
    ]);

    const cleared = removeFactoryLayoutEdgeWaypoint(layout, EDGE_ID, 0);

    expect(factoryLayoutEdgeWaypoints(cleared, EDGE_ID)).toBeUndefined();
    expect(cleared.edges).toBeUndefined();
  });
});
