import type { AgentBentoLayoutItem } from "../../../components/ui";
import {
  DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID,
  DASHBOARD_LAYOUT_STORAGE_KEY,
  DASHBOARD_WIDGET_IDS,
  DEFAULT_DASHBOARD_LAYOUT,
  getDefaultInlineAddWidgetLayout,
  getDefaultWidgetLayoutByType,
  getPrimaryInstanceIDForWidgetType,
  isDashboardWidgetID,
  LEGACY_SELECTION_WIDGET_IDS,
  LEGACY_WORK_OUTCOME_WIDGET_IDS,
} from "./dashboardLayoutSchema";

export function mergeDashboardLayout(
  layout: AgentBentoLayoutItem[],
  baseLayout = DEFAULT_DASHBOARD_LAYOUT,
): AgentBentoLayoutItem[] {
  const normalizedBaseLayout = migrateDashboardLayout(baseLayout);
  const normalizedLayout = migrateDashboardLayout(layout);
  const itemsByID = new Map(normalizedLayout.map((item) => [item.id, item]));
  const mergedBaseLayout = normalizedBaseLayout.map((baseItem) =>
    mergeDashboardLayoutItem(itemsByID.get(baseItem.id), baseItem),
  );
  const baseIDs = new Set(normalizedBaseLayout.map((item) => item.id));
  const additionalItems = normalizedLayout.filter((item) => !baseIDs.has(item.id));

  return [...mergedBaseLayout, ...additionalItems];
}

export function readStoredDashboardLayout(): AgentBentoLayoutItem[] {
  try {
    const storedLayout = window.localStorage.getItem(DASHBOARD_LAYOUT_STORAGE_KEY);
    if (!storedLayout) {
      return DEFAULT_DASHBOARD_LAYOUT;
    }

    const parsedLayout: unknown = JSON.parse(storedLayout);
    if (!Array.isArray(parsedLayout)) {
      return DEFAULT_DASHBOARD_LAYOUT;
    }

    return mergeDashboardLayout(parsedLayout as AgentBentoLayoutItem[]);
  } catch {
    return DEFAULT_DASHBOARD_LAYOUT;
  }
}

export function writeStoredDashboardLayout(layout: AgentBentoLayoutItem[]): void {
  try {
    window.localStorage.setItem(DASHBOARD_LAYOUT_STORAGE_KEY, JSON.stringify(layout));
  } catch {
    // Layout persistence is a convenience; dashboard interaction should keep working without it.
  }
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

function normalizeDashboardLayoutItem(value: unknown): AgentBentoLayoutItem | null {
  if (!value || typeof value !== "object") {
    return null;
  }

  const candidate = value as Partial<AgentBentoLayoutItem> & { widgetType?: unknown };
  const widgetType = resolveStoredWidgetType(candidate);
  if (!widgetType) {
    return null;
  }

  const defaultItem = getDefaultWidgetLayoutByType(widgetType);
  if (!defaultItem) {
    return null;
  }

  const id =
    typeof candidate.id === "string" && candidate.widgetType
      ? candidate.id
      : getPrimaryInstanceIDForWidgetType(widgetType);

  return {
    ...defaultItem,
    h: isFiniteNumber(candidate.h) ? candidate.h : defaultItem.h,
    hidden: candidate.hidden === true ? true : undefined,
    id,
    w: isFiniteNumber(candidate.w) ? candidate.w : defaultItem.w,
    widgetType,
    x: isFiniteNumber(candidate.x) ? candidate.x : defaultItem.x,
    y: isFiniteNumber(candidate.y) ? candidate.y : defaultItem.y,
  };
}

function resolveStoredWidgetType(
  item: Partial<AgentBentoLayoutItem> & { widgetType?: unknown },
): string | null {
  if (isDashboardWidgetID(item.widgetType)) {
    return item.widgetType;
  }

  if (typeof item.id !== "string") {
    return null;
  }
  const itemID = item.id;

  switch (itemID) {
    case DASHBOARD_WIDGET_IDS.currentSelection:
      return DASHBOARD_WIDGET_IDS.currentSelection;
    case DASHBOARD_WIDGET_IDS.providerSession:
      return DASHBOARD_WIDGET_IDS.providerSession;
    case DASHBOARD_WIDGET_IDS.submitWork:
      return DASHBOARD_WIDGET_IDS.submitWork;
    case DASHBOARD_WIDGET_IDS.terminalWork:
      return DASHBOARD_WIDGET_IDS.terminalWork;
    case DASHBOARD_WIDGET_IDS.trace:
      return DASHBOARD_WIDGET_IDS.trace;
    case DASHBOARD_WIDGET_IDS.workGraph:
      return DASHBOARD_WIDGET_IDS.workGraph;
    case DASHBOARD_WIDGET_IDS.workTotals:
      return DASHBOARD_WIDGET_IDS.workTotals;
    case DASHBOARD_WIDGET_IDS.addWidget:
      return DASHBOARD_WIDGET_IDS.addWidget;
    default:
      break;
  }

  if (LEGACY_SELECTION_WIDGET_IDS.some((legacyID) => legacyID === itemID)) {
    return DASHBOARD_WIDGET_IDS.currentSelection;
  }

  if (LEGACY_WORK_OUTCOME_WIDGET_IDS.some((legacyID) => legacyID === itemID)) {
    return DASHBOARD_WIDGET_IDS.workOutcomeChart;
  }

  if (itemID.startsWith(`${DASHBOARD_WIDGET_IDS.workOutcomeChart}::`)) {
    return DASHBOARD_WIDGET_IDS.workOutcomeChart;
  }

  if (itemID.startsWith(`${DASHBOARD_WIDGET_IDS.addWidget}::`)) {
    return DASHBOARD_WIDGET_IDS.addWidget;
  }

  const supportedWidgetID = Object.values(DASHBOARD_WIDGET_IDS).find((widgetID) =>
    itemID.startsWith(`${widgetID}::`),
  );
  return supportedWidgetID ?? null;
}

function mergeDashboardLayoutItem(
  item: AgentBentoLayoutItem | undefined,
  baseItem: AgentBentoLayoutItem,
): AgentBentoLayoutItem {
  if (!item) {
    return baseItem;
  }

  return {
    ...baseItem,
    h: isFiniteNumber(item.h) ? item.h : baseItem.h,
    hidden: item.hidden === true ? true : undefined,
    w: isFiniteNumber(item.w) ? item.w : baseItem.w,
    x: isFiniteNumber(item.x) ? item.x : baseItem.x,
    y: isFiniteNumber(item.y) ? item.y : baseItem.y,
  };
}

function migrateDashboardLayout(layout: AgentBentoLayoutItem[]): AgentBentoLayoutItem[] {
  const normalizedLayout = layout
    .map((item) => normalizeDashboardLayoutItem(item))
    .filter((item): item is AgentBentoLayoutItem => item !== null);
  const migratedLayout = normalizeInlineAddWidgetLayout(
    migrateProviderSessionLayout(
      migrateDashboardCompactionLayout(
        migrateTraceLayout(migrateWorkOutcomeLayout(normalizedLayout)),
      ),
    ),
  );
  return migratedLayout;
}

function migrateDashboardCompactionLayout(
  layout: AgentBentoLayoutItem[],
): AgentBentoLayoutItem[] {
  return layout.map((item) => {
    switch (item.widgetType) {
      case DASHBOARD_WIDGET_IDS.workGraph:
        if (item.x === 0 && item.y === 2 && item.w === 12 && item.h === 10) {
          return { ...item, h: 8 };
        }
        return item;
      case DASHBOARD_WIDGET_IDS.currentSelection:
        if (item.x === 0 && item.y === 12 && item.w === 4 && item.h === 5) {
          return { ...item, y: 10 };
        }
        return item;
      case DASHBOARD_WIDGET_IDS.providerSession:
        if (item.x === 4 && item.y === 12 && item.w === 4 && item.h === 5) {
          return { ...item, y: 10 };
        }
        return item;
      case DASHBOARD_WIDGET_IDS.terminalWork:
        if (item.x === 4 && item.y === 12 && item.w === 4 && item.h === 5) {
          return { ...item, x: 8, y: 10 };
        }
        if (item.x === 8 && item.y === 12 && item.w === 4 && item.h === 5) {
          return { ...item, y: 10 };
        }
        return item;
      case DASHBOARD_WIDGET_IDS.workOutcomeChart:
        if (item.x === 8 && item.y === 12 && item.w === 4 && item.h === 6) {
          return { ...item, y: 15 };
        }
        if (item.x === 8 && item.y === 17 && item.w === 4 && item.h === 6) {
          return { ...item, y: 15 };
        }
        return item;
      case DASHBOARD_WIDGET_IDS.submitWork:
        if (item.x === 8 && item.y === 18 && item.w === 4 && item.h === 6) {
          return { ...item, y: 21 };
        }
        if (item.x === 8 && item.y === 23 && item.w === 4 && item.h === 6) {
          return { ...item, y: 21 };
        }
        return item;
      case DASHBOARD_WIDGET_IDS.trace:
        if (item.x === 0 && item.y === 18 && item.w === 8 && item.h === 9) {
          return { ...item, y: 15 };
        }
        return item;
      default:
        return item;
    }
  });
}

function migrateProviderSessionLayout(
  layout: AgentBentoLayoutItem[],
): AgentBentoLayoutItem[] {
  if (
    layout.some((item) => item.widgetType === DASHBOARD_WIDGET_IDS.providerSession)
  ) {
    return layout;
  }

  const providerSessionItem = DEFAULT_DASHBOARD_LAYOUT.find(
    (item) => item.widgetType === DASHBOARD_WIDGET_IDS.providerSession,
  );
  if (!providerSessionItem) {
    throw new Error("default provider-session layout is missing");
  }

  return [...layout, { ...providerSessionItem }];
}

function migrateTraceLayout(layout: AgentBentoLayoutItem[]): AgentBentoLayoutItem[] {
  return layout.map((item) => {
    if (item.widgetType === DASHBOARD_WIDGET_IDS.trace) {
      if (item.x === 0 && item.y === 18 && item.w === 4 && item.h === 7) {
        return {
          ...item,
          h: 9,
          maxW: undefined,
          minH: 7,
          minW: 5,
          w: 8,
        };
      }

      if (item.maxW !== undefined) {
        return {
          ...item,
          maxW: undefined,
        };
      }
    }

    return item;
  });
}

function normalizeInlineAddWidgetLayout(
  layout: AgentBentoLayoutItem[],
): AgentBentoLayoutItem[] {
  const inlineAddWidgetItems = layout.filter(
    (item) => item.widgetType === DASHBOARD_WIDGET_IDS.addWidget,
  );

  if (inlineAddWidgetItems.length === 0) {
    return [...layout, getDefaultInlineAddWidgetLayout()];
  }

  const orderedInlineAddWidgetItems = [...inlineAddWidgetItems].reverse();
  const canonicalInlineAddWidget =
    orderedInlineAddWidgetItems.find(
      (item) => item.id === DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID,
    ) ?? orderedInlineAddWidgetItems[0];
  const normalizedInlineAddWidget: AgentBentoLayoutItem = {
    ...canonicalInlineAddWidget,
    hidden: undefined,
    id: DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID,
    widgetType: DASHBOARD_WIDGET_IDS.addWidget,
  };

  return [
    ...layout.filter((item) => item.widgetType !== DASHBOARD_WIDGET_IDS.addWidget),
    normalizedInlineAddWidget,
  ];
}

function migrateWorkOutcomeLayout(layout: AgentBentoLayoutItem[]): AgentBentoLayoutItem[] {
  return layout;
}
