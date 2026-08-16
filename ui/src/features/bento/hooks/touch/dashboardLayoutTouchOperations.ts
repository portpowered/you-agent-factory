import type { Layout, LayoutItem } from "react-grid-layout";
import {
  calcGridColWidth,
  calcGridItemWHPx,
  calcWH,
  cloneLayout,
  getLayoutItem,
  moveElement,
  verticalCompactor,
} from "react-grid-layout/core";

import type { DashboardTouchLayoutOperationOptions } from "./dashboardLayoutTouch";

/**
 * Apply a touch grid step using the same core movement, sizing, compaction,
 * and collision behavior used by the existing pointer and keyboard paths.
 */
export function applyDashboardTouchLayoutOperation({
  columns,
  deltaX,
  deltaY,
  handle,
  itemID,
  layout,
  margin,
  mode,
  rowHeight,
  width,
}: DashboardTouchLayoutOperationOptions): Layout {
  const item = getLayoutItem(layout, itemID);
  if (!item) {
    return layout;
  }

  const positionParams = {
    cols: columns,
    containerPadding: [0, 0] as const,
    containerWidth: width,
    margin,
    maxRows: Number.POSITIVE_INFINITY,
    rowHeight,
  };

  if (mode === "move") {
    return moveTouchLayoutItem(
      layout,
      item,
      columns,
      deltaX,
      deltaY,
      calcGridColWidth(positionParams),
      margin,
      rowHeight,
    );
  }

  if (!handle) {
    return layout;
  }

  const columnWidth = calcGridColWidth(positionParams);
  const initialWidth = calcGridItemWHPx(item.w, columnWidth, margin[0]);
  const initialHeight = calcGridItemWHPx(item.h, rowHeight, margin[1]);
  const adjustedDeltaX = handle.includes("e") ? deltaX : 0;
  const adjustedDeltaY = handle.includes("s") ? deltaY : 0;
  const calculatedSize = calcWH(
    positionParams,
    initialWidth + adjustedDeltaX,
    initialHeight + adjustedDeltaY,
    item.x,
    item.y,
    handle,
  );
  const maxWidth = Math.max(
    1,
    Math.min(item.maxW ?? columns, columns - item.x),
  );
  const maxHeight = item.maxH ?? Number.POSITIVE_INFINITY;
  const minWidth = Math.min(Math.max(item.minW ?? 1, 1), maxWidth);
  const minHeight = Math.min(Math.max(item.minH ?? 1, 1), maxHeight);
  const nextWidth = clamp(calculatedSize.w, minWidth, maxWidth);
  const nextHeight = clamp(calculatedSize.h, minHeight, maxHeight);

  if (nextWidth === item.w && nextHeight === item.h) {
    return layout;
  }

  const nextLayout = cloneLayout(layout);
  const nextItem = getLayoutItem(nextLayout, itemID);
  if (!nextItem) {
    return layout;
  }

  nextItem.w = nextWidth;
  nextItem.h = nextHeight;
  return verticalCompactor.compact(nextLayout, columns);
}

function moveTouchLayoutItem(
  layout: Layout,
  item: LayoutItem,
  columns: number,
  deltaX: number,
  deltaY: number,
  columnWidth: number,
  margin: readonly [number, number],
  rowHeight: number,
): Layout {
  const targetX = clamp(
    item.x + Math.round(deltaX / (columnWidth + margin[0])),
    0,
    Math.max(0, columns - item.w),
  );
  const targetY = Math.max(
    0,
    item.y + Math.round(deltaY / (rowHeight + margin[1])),
  );
  if (targetX === item.x && targetY === item.y) {
    return layout;
  }

  const nextLayout = cloneLayout(layout);
  const nextItem = getLayoutItem(nextLayout, item.i);
  if (!nextItem) {
    return layout;
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
  return verticalCompactor.compact(movedLayout, columns);
}

function clamp(value: number, minimum: number, maximum: number): number {
  return Math.min(Math.max(value, minimum), maximum);
}
