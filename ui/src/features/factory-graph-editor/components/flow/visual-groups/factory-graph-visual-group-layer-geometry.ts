import type { CSSProperties } from "react";
import type { FactoryLayoutPoint } from "../../../lib/layout/factory-graph-layout-operations";
import {
  FACTORY_LAYOUT_GROUP_MIN_SIZE,
  type FactoryLayoutGroup,
} from "../../../lib/layout/visual-groups/factory-graph-layout-groups";

export const DRAG_CLICK_THRESHOLD_PX = 4;
export const GROUP_KEYBOARD_MOVE_STEP = 16;
export const GROUP_KEYBOARD_RESIZE_STEP = 16;

export type ResizeCorner = "ne" | "nw" | "se" | "sw";
export type GroupOutlineEdge = "bottom" | "left" | "right" | "top";

export const RESIZE_CORNERS: readonly ResizeCorner[] = ["nw", "ne", "sw", "se"];
export const GROUP_OUTLINE_EDGES: readonly GroupOutlineEdge[] = [
  "top",
  "right",
  "bottom",
  "left",
];

export function isDragBeyondClickThreshold(
  startScreenPosition: FactoryLayoutPoint,
  currentScreenPosition: FactoryLayoutPoint,
): boolean {
  return (
    Math.hypot(
      currentScreenPosition.x - startScreenPosition.x,
      currentScreenPosition.y - startScreenPosition.y,
    ) >= DRAG_CLICK_THRESHOLD_PX
  );
}

export function resizeHandleStyle(corner: ResizeCorner): CSSProperties {
  switch (corner) {
    case "nw":
      return { cursor: "nwse-resize", left: -8, top: -8 };
    case "ne":
      return { cursor: "nesw-resize", right: -8, top: -8 };
    case "sw":
      return { bottom: -8, cursor: "nesw-resize", left: -8 };
    case "se":
      return { bottom: -8, cursor: "nwse-resize", right: -8 };
  }
}

export function groupOutlineAffordanceStyle(
  edge: GroupOutlineEdge,
): CSSProperties {
  switch (edge) {
    case "top":
      return { height: 8, left: 8, right: 8, top: -4 };
    case "right":
      return { bottom: 8, right: -4, top: 8, width: 8 };
    case "bottom":
      return { bottom: -4, height: 8, left: 8, right: 8 };
    case "left":
      return { bottom: 8, left: -4, top: 8, width: 8 };
  }
}

export function keyboardDelta(
  key: string,
  step: number,
): FactoryLayoutPoint | null {
  switch (key) {
    case "ArrowDown":
      return { x: 0, y: step };
    case "ArrowLeft":
      return { x: -step, y: 0 };
    case "ArrowRight":
      return { x: step, y: 0 };
    case "ArrowUp":
      return { x: 0, y: -step };
    default:
      return null;
  }
}

export function resizeBoundsFromCorner(
  startBounds: FactoryLayoutGroup["bounds"],
  corner: ResizeCorner,
  delta: FactoryLayoutPoint,
): FactoryLayoutGroup["bounds"] {
  const minWidth = FACTORY_LAYOUT_GROUP_MIN_SIZE.width;
  const minHeight = FACTORY_LAYOUT_GROUP_MIN_SIZE.height;

  switch (corner) {
    case "se":
      return {
        height: Math.max(minHeight, startBounds.height + delta.y),
        width: Math.max(minWidth, startBounds.width + delta.x),
        x: startBounds.x,
        y: startBounds.y,
      };
    case "sw": {
      const width = Math.max(minWidth, startBounds.width - delta.x);
      const widthDelta = width - startBounds.width;
      return {
        height: Math.max(minHeight, startBounds.height + delta.y),
        width,
        x: startBounds.x - widthDelta,
        y: startBounds.y,
      };
    }
    case "ne": {
      const height = Math.max(minHeight, startBounds.height - delta.y);
      const heightDelta = height - startBounds.height;
      return {
        height,
        width: Math.max(minWidth, startBounds.width + delta.x),
        x: startBounds.x,
        y: startBounds.y - heightDelta,
      };
    }
    case "nw": {
      const width = Math.max(minWidth, startBounds.width - delta.x);
      const height = Math.max(minHeight, startBounds.height - delta.y);
      const widthDelta = width - startBounds.width;
      const heightDelta = height - startBounds.height;
      return {
        height,
        width,
        x: startBounds.x - widthDelta,
        y: startBounds.y - heightDelta,
      };
    }
  }
}
