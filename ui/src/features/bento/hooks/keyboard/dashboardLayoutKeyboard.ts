import type { Layout, LayoutItem } from "react-grid-layout";
import {
  cloneLayout,
  getLayoutItem,
  moveElement,
  verticalCompactor,
} from "react-grid-layout/core";

export type DashboardKeyboardLayoutMode = "move" | "resize";

export interface DashboardKeyboardLayoutOperationResult {
  changed: boolean;
  layout: Layout;
  reason: "changed" | "boundary";
}

const DEFAULT_MIN_GRID_SIZE = 1;

/**
 * Apply one keyboard grid step using the same vertical compaction and
 * collision resolution rules as the pointer grid.
 */
export function applyDashboardKeyboardLayoutOperation(
  layout: Layout,
  itemID: string,
  mode: DashboardKeyboardLayoutMode,
  key: string,
  columns: number,
): DashboardKeyboardLayoutOperationResult {
  const item = getLayoutItem(layout, itemID);
  if (!item) {
    return unchangedLayout(layout);
  }

  if (mode === "move") {
    return moveDashboardLayoutItem(layout, item, key, columns);
  }

  return resizeDashboardLayoutItem(layout, item, key, columns);
}

function moveDashboardLayoutItem(
  layout: Layout,
  item: LayoutItem,
  key: string,
  columns: number,
): DashboardKeyboardLayoutOperationResult {
  const delta = getMoveDelta(key);
  if (!delta) {
    return unchangedLayout(layout);
  }

  const maxX = Math.max(0, columns - item.w);
  const targetX = clamp(item.x + delta.x, 0, maxX);
  const targetY = Math.max(0, item.y + delta.y);
  if (targetX === item.x && targetY === item.y) {
    return unchangedLayout(layout);
  }

  const nextLayout = cloneLayout(layout);
  const nextItem = getLayoutItem(nextLayout, item.i);
  if (!nextItem) {
    return unchangedLayout(layout);
  }

  const movedLayout = moveElement(
    nextLayout,
    nextItem,
    targetX,
    targetY,
    true,
    false,
    "vertical",
    columns,
    false,
  );
  const compactedLayout = verticalCompactor.compact(movedLayout, columns);
  const movedItem = getLayoutItem(compactedLayout, item.i);
  if (!movedItem || hasSameGeometry(item, movedItem)) {
    return unchangedLayout(layout);
  }

  return {
    changed: true,
    layout: compactedLayout,
    reason: "changed",
  };
}

function resizeDashboardLayoutItem(
  layout: Layout,
  item: LayoutItem,
  key: string,
  columns: number,
): DashboardKeyboardLayoutOperationResult {
  const delta = getResizeDelta(key);
  if (!delta) {
    return unchangedLayout(layout);
  }

  const minimumWidth = Math.max(item.minW ?? DEFAULT_MIN_GRID_SIZE, 1);
  const minimumHeight = Math.max(item.minH ?? DEFAULT_MIN_GRID_SIZE, 1);
  const maximumWidth = Math.max(
    minimumWidth,
    Math.min(item.maxW ?? columns, columns),
  );
  const maximumHeight = Math.max(
    minimumHeight,
    item.maxH ?? Number.POSITIVE_INFINITY,
  );
  const targetWidth = clamp(item.w + delta.w, minimumWidth, maximumWidth);
  const targetHeight = clamp(item.h + delta.h, minimumHeight, maximumHeight);
  if (targetWidth === item.w && targetHeight === item.h) {
    return unchangedLayout(layout);
  }

  const nextLayout = cloneLayout(layout);
  const nextItem = getLayoutItem(nextLayout, item.i);
  if (!nextItem) {
    return unchangedLayout(layout);
  }

  nextItem.w = targetWidth;
  nextItem.h = targetHeight;
  const compactedLayout = verticalCompactor.compact(nextLayout, columns);
  const resizedItem = getLayoutItem(compactedLayout, item.i);
  if (!resizedItem || hasSameGeometry(item, resizedItem)) {
    return unchangedLayout(layout);
  }

  return {
    changed: true,
    layout: compactedLayout,
    reason: "changed",
  };
}

function getMoveDelta(key: string): { x: number; y: number } | undefined {
  switch (key) {
    case "ArrowLeft":
      return { x: -1, y: 0 };
    case "ArrowRight":
      return { x: 1, y: 0 };
    case "ArrowUp":
      return { x: 0, y: -1 };
    case "ArrowDown":
      return { x: 0, y: 1 };
    default:
      return undefined;
  }
}

function getResizeDelta(key: string): { h: number; w: number } | undefined {
  switch (key) {
    case "ArrowLeft":
      return { h: 0, w: -1 };
    case "ArrowRight":
      return { h: 0, w: 1 };
    case "ArrowUp":
      return { h: -1, w: 0 };
    case "ArrowDown":
      return { h: 1, w: 0 };
    default:
      return undefined;
  }
}

function hasSameGeometry(left: LayoutItem, right: LayoutItem): boolean {
  return (
    left.h === right.h &&
    left.w === right.w &&
    left.x === right.x &&
    left.y === right.y
  );
}

function clamp(value: number, minimum: number, maximum: number): number {
  return Math.min(Math.max(value, minimum), maximum);
}

function unchangedLayout(
  layout: Layout,
): DashboardKeyboardLayoutOperationResult {
  return { changed: false, layout, reason: "boundary" };
}
