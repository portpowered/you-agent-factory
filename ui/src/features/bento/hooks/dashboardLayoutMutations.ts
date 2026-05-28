import type { AgentBentoLayoutItem } from "../components/agent-bento";
import {
  canAddDashboardWidgetType,
  type DashboardWidgetPickerWidgetType,
} from "../lib/dashboard-widget-picker";
import {
  createDashboardWidgetInstanceID,
  DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID,
  getDefaultInlineAddWidgetLayout,
  getDefaultWidgetLayoutByType,
  getPrimaryInstanceIDForWidgetType,
} from "./dashboardLayoutSchema";

const DASHBOARD_GRID_COLUMNS = 12;
const DASHBOARD_WIDGET_INSTANCE_SLOT_PREFIX = "instance";

export function addDashboardWidgetToLayout(
  layout: AgentBentoLayoutItem[],
  widgetType: DashboardWidgetPickerWidgetType,
): AgentBentoLayoutItem[] {
  if (!canAddDashboardWidgetType(layout, widgetType)) {
    return layout;
  }

  const addWidgetLayout =
    layout.find(
      (item) => item.id === DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID,
    ) ?? getDefaultInlineAddWidgetLayout();
  const widgetDefaultLayout = getDefaultWidgetLayoutByType(widgetType);
  if (!widgetDefaultLayout) {
    return layout;
  }

  const retainedItems = layout.filter(
    (item) =>
      item.id !== DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID &&
      !(item.hidden && item.widgetType === widgetType),
  );
  const nextWidgetLayout = {
    ...widgetDefaultLayout,
    id: getNextDashboardWidgetInstanceID(layout, widgetType),
    widgetType,
  };
  const widgetPlacement = findNextOpenDashboardPosition(
    retainedItems,
    nextWidgetLayout,
    addWidgetLayout,
  );
  const positionedWidgetLayout = {
    ...nextWidgetLayout,
    x: widgetPlacement.x,
    y: widgetPlacement.y,
  };
  const repositionedAddWidgetLayout = {
    ...addWidgetLayout,
    ...findNextOpenDashboardPosition(
      [...retainedItems, positionedWidgetLayout],
      addWidgetLayout,
      addWidgetLayout,
    ),
  };

  return [
    ...retainedItems,
    positionedWidgetLayout,
    repositionedAddWidgetLayout,
  ];
}

export function removeDashboardWidgetFromLayout(
  layout: AgentBentoLayoutItem[],
  widgetInstanceID: string,
): AgentBentoLayoutItem[] {
  if (widgetInstanceID === DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID) {
    return layout;
  }

  const removedItem = layout.find((item) => item.id === widgetInstanceID);
  if (!removedItem) {
    return layout;
  }

  const primaryInstanceID = getPrimaryInstanceIDForWidgetType(
    removedItem.widgetType,
  );
  if (widgetInstanceID === primaryInstanceID) {
    return layout.map((item) =>
      item.id === widgetInstanceID ? { ...item, hidden: true } : item,
    );
  }

  return layout.filter((item) => item.id !== widgetInstanceID);
}

function getNextDashboardWidgetInstanceID(
  layout: readonly AgentBentoLayoutItem[],
  widgetType: string,
): string {
  const primaryInstanceID = getPrimaryInstanceIDForWidgetType(widgetType);
  if (!layout.some((item) => item.id === primaryInstanceID && !item.hidden)) {
    return primaryInstanceID;
  }

  const nextInstanceNumber =
    layout.reduce((highestNumber, item) => {
      if (item.widgetType !== widgetType) {
        return highestNumber;
      }

      const instanceNumber = parseDashboardWidgetInstanceNumber(item.id);
      return instanceNumber === null
        ? highestNumber
        : Math.max(highestNumber, instanceNumber);
    }, 0) + 1;

  return createDashboardWidgetInstanceID(
    widgetType,
    `${DASHBOARD_WIDGET_INSTANCE_SLOT_PREFIX}-${nextInstanceNumber}`,
  );
}

function parseDashboardWidgetInstanceNumber(id: string): number | null {
  const match = id.match(/::instance-(\d+)$/);
  if (!match) {
    return null;
  }

  const numericValue = Number.parseInt(match[1] ?? "", 10);
  return Number.isFinite(numericValue) ? numericValue : null;
}

function findNextOpenDashboardPosition(
  layout: readonly AgentBentoLayoutItem[],
  item: Pick<AgentBentoLayoutItem, "h" | "w">,
  preferredPosition: Pick<AgentBentoLayoutItem, "x" | "y">,
): Pick<AgentBentoLayoutItem, "x" | "y"> {
  const maxX = Math.max(0, DASHBOARD_GRID_COLUMNS - item.w);
  const preferredX = clampGridValue(preferredPosition.x, 0, maxX);
  const preferredY = Math.max(0, preferredPosition.y);
  const layoutBottom =
    layout.reduce(
      (highestY, layoutItem) => Math.max(highestY, layoutItem.y + layoutItem.h),
      0,
    ) + item.h;

  for (let y = preferredY; y <= layoutBottom; y += 1) {
    const xCandidates =
      y === preferredY
        ? [
            preferredX,
            ...getGridColumnCandidates(maxX).filter((x) => x !== preferredX),
          ]
        : getGridColumnCandidates(maxX);

    for (const x of xCandidates) {
      const candidate = { h: item.h, w: item.w, x, y };
      if (
        !layout.some((layoutItem) =>
          dashboardLayoutItemsOverlap(layoutItem, candidate),
        )
      ) {
        return { x, y };
      }
    }
  }

  return { x: 0, y: layoutBottom };
}

function getGridColumnCandidates(maxX: number): number[] {
  return Array.from({ length: maxX + 1 }, (_, index) => index);
}

function clampGridValue(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}

function dashboardLayoutItemsOverlap(
  left: Pick<AgentBentoLayoutItem, "h" | "w" | "x" | "y">,
  right: Pick<AgentBentoLayoutItem, "h" | "w" | "x" | "y">,
): boolean {
  return !(
    left.x + left.w <= right.x ||
    right.x + right.w <= left.x ||
    left.y + left.h <= right.y ||
    right.y + right.h <= left.y
  );
}
