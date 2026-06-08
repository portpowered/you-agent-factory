import { Position } from "@xyflow/react";
import { describe, expect, it } from "vitest";

import { buildFactoryGraphEdgePathThroughWaypoints } from "./factory-graph-edge-path";

describe("buildFactoryGraphEdgePathThroughWaypoints", () => {
  it("routes through authored waypoints while keeping generated routing as fallback", () => {
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
    expect(routed.path).toContain("L 100 20");
    expect(routed.path).toContain("L 200 100");
  });
});
