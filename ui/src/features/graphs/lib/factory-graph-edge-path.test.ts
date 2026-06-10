import { Position } from "@xyflow/react";
import { describe, expect, it } from "vitest";

import { buildFactoryGraphEdgePathThroughWaypoints } from "./factory-graph-edge-path";

describe("buildFactoryGraphEdgePathThroughWaypoints", () => {
  it("routes through authored waypoints with centripetal Catmull-Rom segments", () => {
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
    expect(routed.path).toContain("100, 20 C");
    expect(routed.path).toContain("200, 100");
  });

  it("keeps aligned waypoint segments on their shared axis", () => {
    const routed = buildFactoryGraphEdgePathThroughWaypoints({
      sourcePosition: Position.Bottom,
      sourceX: 0,
      sourceY: 0,
      targetPosition: Position.Top,
      targetX: 0,
      targetY: 200,
      waypoints: [{ x: 0, y: 100 }],
    });

    expect(routed.path).toContain("C 0 33.333 0, 66.667 0, 100");
    expect(routed.path).toContain("C 0 133.333 0, 166.667 0, 200");
  });

  it("compacts duplicate route points before spline interpolation", () => {
    const routed = buildFactoryGraphEdgePathThroughWaypoints({
      sourcePosition: Position.Right,
      sourceX: 0,
      sourceY: 0,
      targetPosition: Position.Left,
      targetX: 100,
      targetY: 0,
      waypoints: [
        { x: 0, y: 0 },
        { x: 50, y: 0 },
        { x: 50, y: 0 },
      ],
    });

    expect(routed.path).not.toContain("NaN");
    expect(routed.path).toContain("50, 0 C");
    expect(routed.path).toContain("100, 0");
  });
});
