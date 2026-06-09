import { Position } from "@xyflow/react";
import { describe, expect, it } from "vitest";

import { buildFactoryGraphEdgePathThroughWaypoints } from "./factory-graph-edge-path";

describe("buildFactoryGraphEdgePathThroughWaypoints", () => {
  it("routes through authored waypoints with smooth bezier segments", () => {
    const routed = buildFactoryGraphEdgePathThroughWaypoints({
      sourcePosition: Position.Right,
      sourceX: 0,
      sourceY: 0,
      targetPosition: Position.Left,
      targetX: 200,
      targetY: 100,
      waypoints: [{ x: 100, y: 20 }],
    });

    expect(routed.path).toContain("M 0 0");
    expect(routed.path).toContain("C ");
    expect(routed.path).toContain(", 100 20 C ");
    expect(routed.path).toContain(", 200 100");
  });

  it("keeps aligned waypoint segments on their local axis", () => {
    const routed = buildFactoryGraphEdgePathThroughWaypoints({
      sourcePosition: Position.Bottom,
      sourceX: 0,
      sourceY: 0,
      targetPosition: Position.Top,
      targetX: 0,
      targetY: 200,
      waypoints: [{ x: 0, y: 100 }],
    });

    expect(routed.path).toContain("C 0 50, 0 50, 0 100");
    expect(routed.path).toContain("C 0 150, 0 150, 0 200");
  });
});
