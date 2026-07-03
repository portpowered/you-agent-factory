import { Position } from "@xyflow/react";
import { describe, expect, it } from "vitest";

import { buildGraphEdgePathThroughWaypoints } from "./graph-edge-path";

describe("buildGraphEdgePathThroughWaypoints", () => {
  it("routes through authored waypoints with centripetal Catmull-Rom segments", () => {
    const routed = buildGraphEdgePathThroughWaypoints({
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
    const routed = buildGraphEdgePathThroughWaypoints({
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
    const routed = buildGraphEdgePathThroughWaypoints({
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

  it("falls back to bezier geometry when no waypoints are provided", () => {
    const routed = buildGraphEdgePathThroughWaypoints({
      sourcePosition: Position.Right,
      sourceX: 0,
      sourceY: 0,
      targetPosition: Position.Left,
      targetX: 120,
      targetY: 40,
    });

    expect(routed.path).toMatch(/^M[\d,. -]+/);
    expect(routed.labelX).toBeGreaterThan(0);
    expect(routed.labelY).toBeGreaterThanOrEqual(0);
  });

  it("returns a stable straight path for collinear source, waypoint, and target points", () => {
    const routed = buildGraphEdgePathThroughWaypoints({
      sourcePosition: Position.Right,
      sourceX: 0,
      sourceY: 50,
      targetPosition: Position.Left,
      targetX: 200,
      targetY: 50,
      waypoints: [{ x: 100, y: 50 }],
    });

    expect(routed.path).toContain("M 0 50");
    expect(routed.path).toContain("100, 50");
    expect(routed.path).toContain("200, 50");
    expect(routed.labelX).toBe(100);
    expect(routed.labelY).toBe(50);
  });

  it("returns a stable stepped path for orthogonal waypoint routing", () => {
    const routed = buildGraphEdgePathThroughWaypoints({
      sourcePosition: Position.Right,
      sourceX: 0,
      sourceY: 0,
      targetPosition: Position.Left,
      targetX: 200,
      targetY: 100,
      waypoints: [
        { x: 100, y: 0 },
        { x: 100, y: 100 },
      ],
    });

    expect(routed.path).toContain("M 0 0");
    expect(routed.path).toContain("100, 0");
    expect(routed.path).toContain("100, 100");
    expect(routed.path).toContain("200, 100");
    expect(routed.path).not.toContain("NaN");
  });

  it("returns a stable curved path for multi-waypoint routes", () => {
    const routed = buildGraphEdgePathThroughWaypoints({
      sourcePosition: Position.Bottom,
      sourceX: 20,
      sourceY: 0,
      targetPosition: Position.Top,
      targetX: 180,
      targetY: 160,
      waypoints: [
        { x: 20, y: 80 },
        { x: 180, y: 80 },
      ],
    });

    expect(routed.path).toMatch(/^M /);
    expect(routed.path).toContain("C ");
    expect(routed.path).toContain("180, 160");
    expect(routed.labelX).toBe(20);
    expect(routed.labelY).toBe(80);
  });
});
