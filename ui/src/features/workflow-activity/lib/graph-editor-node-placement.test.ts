import { describe, expect, it } from "vitest";
import { FACTORY_GRAPH_EDITOR_NODE_DIMENSIONS_BY_KIND } from "../../factory-graph-editor/lib/editor/factory-graph-editor-layout";
import {
  axisAlignedRectFromTopLeft,
  GRAPH_EDITOR_NODE_PLACEMENT_MAX_ATTEMPTS,
  GRAPH_EDITOR_NODE_PLACEMENT_PADDING_GAP,
  graphEditorNodeDimensionsForKind,
  resolveViewportCenterNodePlacement,
  topLeftFromAxisAlignedRectCenter,
} from "./graph-editor-node-placement";

const workerSize = graphEditorNodeDimensionsForKind("worker");
const workstationSize = graphEditorNodeDimensionsForKind("workstation");

describe("graphEditorNodeDimensionsForKind", () => {
  it("matches factory graph editor layout dimensions", () => {
    expect(graphEditorNodeDimensionsForKind("workstation")).toEqual(
      FACTORY_GRAPH_EDITOR_NODE_DIMENSIONS_BY_KIND.workstation,
    );
    expect(graphEditorNodeDimensionsForKind("resource")).toEqual(
      FACTORY_GRAPH_EDITOR_NODE_DIMENSIONS_BY_KIND.resource,
    );
    expect(graphEditorNodeDimensionsForKind("worker")).toEqual(
      FACTORY_GRAPH_EDITOR_NODE_DIMENSIONS_BY_KIND.worker,
    );
  });
});

describe("resolveViewportCenterNodePlacement", () => {
  it("returns the viewport center on an empty canvas", () => {
    const viewportCenter = { x: 420, y: 280 };

    const result = resolveViewportCenterNodePlacement({
      candidateSize: workerSize,
      occupiedRects: [],
      viewportCenter,
    });

    expect(result).toEqual({
      attemptsUsed: 1,
      center: viewportCenter,
      collidesAtCenter: false,
      exhaustedSearch: false,
    });
  });

  it("centers a workstation on an empty canvas", () => {
    const viewportCenter = { x: 640, y: 360 };

    const result = resolveViewportCenterNodePlacement({
      candidateSize: workstationSize,
      occupiedRects: [],
      viewportCenter,
    });

    expect(result).toEqual({
      attemptsUsed: 1,
      center: viewportCenter,
      collidesAtCenter: false,
      exhaustedSearch: false,
    });
  });

  it("nudges a workstation away when the viewport center is blocked", () => {
    const viewportCenter = { x: 300, y: 240 };
    const blockerTopLeft = topLeftFromAxisAlignedRectCenter(
      viewportCenter,
      workstationSize,
    );

    const result = resolveViewportCenterNodePlacement({
      candidateSize: workstationSize,
      occupiedRects: [
        axisAlignedRectFromTopLeft(blockerTopLeft, workstationSize),
      ],
      viewportCenter,
    });

    expect(result.collidesAtCenter).toBe(true);
    expect(result.exhaustedSearch).toBe(false);
    expect(result.center).not.toEqual(viewportCenter);
    expect(result.attemptsUsed).toBeGreaterThan(1);
  });

  it("nudges away when the viewport center is blocked", () => {
    const viewportCenter = { x: 200, y: 200 };
    const blockerTopLeft = topLeftFromAxisAlignedRectCenter(
      viewportCenter,
      workerSize,
    );

    const result = resolveViewportCenterNodePlacement({
      candidateSize: workerSize,
      occupiedRects: [axisAlignedRectFromTopLeft(blockerTopLeft, workerSize)],
      viewportCenter,
    });

    expect(result.collidesAtCenter).toBe(true);
    expect(result.exhaustedSearch).toBe(false);
    expect(result.center).not.toEqual(viewportCenter);
    expect(result.attemptsUsed).toBeGreaterThan(1);
  });

  it("picks the nearest free slot when multiple blockers surround the center", () => {
    const viewportCenter = { x: 0, y: 0 };
    const blockerSize = { height: 120, width: 120 };
    const occupiedRects = [
      axisAlignedRectFromTopLeft({ x: -70, y: -70 }, blockerSize),
      axisAlignedRectFromTopLeft({ x: -70, y: 40 }, blockerSize),
      axisAlignedRectFromTopLeft({ x: 40, y: -70 }, blockerSize),
      axisAlignedRectFromTopLeft({ x: 40, y: 40 }, blockerSize),
    ];

    const result = resolveViewportCenterNodePlacement({
      candidateSize: workerSize,
      occupiedRects,
      viewportCenter,
    });

    expect(result.exhaustedSearch).toBe(false);
    expect(result.center).not.toEqual(viewportCenter);
    const distance = Math.hypot(
      result.center.x - viewportCenter.x,
      result.center.y - viewportCenter.y,
    );
    expect(distance).toBeGreaterThan(0);
    expect(distance).toBeLessThan(
      workerSize.width +
        workerSize.height +
        GRAPH_EDITOR_NODE_PLACEMENT_PADDING_GAP,
    );
  });

  it("falls back to the viewport center when search is exhausted", () => {
    const viewportCenter = { x: 100, y: 100 };
    const blockerSize = { height: 400, width: 400 };
    const occupiedRects = [
      axisAlignedRectFromTopLeft({ x: -200, y: -200 }, blockerSize),
    ];

    const result = resolveViewportCenterNodePlacement({
      candidateSize: workerSize,
      maxAttempts: 4,
      occupiedRects,
      paddingGap: GRAPH_EDITOR_NODE_PLACEMENT_PADDING_GAP,
      viewportCenter,
    });

    expect(result).toEqual({
      attemptsUsed: 4,
      center: viewportCenter,
      collidesAtCenter: true,
      exhaustedSearch: true,
    });
  });

  it("documents the default max attempt budget", () => {
    expect(GRAPH_EDITOR_NODE_PLACEMENT_MAX_ATTEMPTS).toBe(48);
  });
});

describe("resolveViewportCenterNodePlacement viewport bounds", () => {
  it("clamps an otherwise-free candidate to the visible viewport bounds", () => {
    const result = resolveViewportCenterNodePlacement({
      candidateSize: workerSize,
      occupiedRects: [],
      viewportBounds: { height: 240, width: 400, x: 0, y: 0 },
      viewportCenter: { x: 20, y: 20 },
    });

    expect(result).toEqual({
      attemptsUsed: 1,
      center: {
        x: workerSize.width / 2,
        y: workerSize.height / 2,
      },
      collidesAtCenter: false,
      exhaustedSearch: false,
    });
  });
});
