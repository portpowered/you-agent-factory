import type { AgentBentoLayoutItem } from "../components/agent-bento";
import { mergeDashboardLayoutInstanceHighWaterMarks } from "./allocation/dashboardLayoutAllocation";
import {
  collectStoredLayoutDiagnostics,
  type DashboardLayoutSanitizationResult,
  isSafeDashboardWidgetInstanceID,
  repairDashboardLayout,
  resolveStoredWidgetType,
} from "./dashboardLayoutSanitization";
import {
  DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID,
  DASHBOARD_WIDGET_IDS,
  type DashboardLayoutInstanceHighWaterMarks,
  DEFAULT_DASHBOARD_LAYOUT,
  getDefaultInlineAddWidgetLayout,
  getDefaultWidgetLayoutByType,
  getPrimaryInstanceIDForWidgetType,
  WORK_OUTCOME_CHART_MIN_GRID_HEIGHT,
  WORK_OUTCOME_CHART_MIN_GRID_WIDTH,
} from "./dashboardLayoutSchema";

const DASHBOARD_GRID_COLUMNS = 12;

export function mergeDashboardLayout(
  layout: AgentBentoLayoutItem[],
  baseLayout = DEFAULT_DASHBOARD_LAYOUT,
): AgentBentoLayoutItem[] {
  return sanitizeDashboardLayout(layout, baseLayout).layout;
}

export function sanitizeDashboardLayout(
  layout: AgentBentoLayoutItem[],
  baseLayout = DEFAULT_DASHBOARD_LAYOUT,
  initialHighWaterMarks: DashboardLayoutInstanceHighWaterMarks = {},
): DashboardLayoutSanitizationResult {
  const normalizedBaseLayout = migrateDashboardLayout(baseLayout);
  const normalizedLayout = migrateDashboardLayout(layout);
  const mergedLayout = mergeNormalizedDashboardLayouts(
    normalizedLayout,
    normalizedBaseLayout,
  );
  const repaired = repairDashboardLayout(
    mergedLayout,
    collectStoredLayoutDiagnostics(layout),
  );
  return {
    ...repaired,
    instanceHighWaterMarks: mergeDashboardLayoutInstanceHighWaterMarks(
      initialHighWaterMarks,
      repaired.instanceHighWaterMarks,
    ),
  };
}

function mergeNormalizedDashboardLayouts(
  normalizedLayout: AgentBentoLayoutItem[],
  normalizedBaseLayout: AgentBentoLayoutItem[],
): AgentBentoLayoutItem[] {
  const hasVisibleGraph = normalizedLayout.some(
    (item) =>
      item.widgetType === DASHBOARD_WIDGET_IDS.workGraph && !item.hidden,
  );
  const itemsByID = new Map<string, AgentBentoLayoutItem[]>();
  for (const item of normalizedLayout) {
    const items = itemsByID.get(item.id) ?? [];
    items.push(item);
    itemsByID.set(item.id, items);
  }

  const mergedBaseLayout: AgentBentoLayoutItem[] = [];
  const visibleGraph = normalizedLayout.find(
    (item) =>
      item.widgetType === DASHBOARD_WIDGET_IDS.workGraph && !item.hidden,
  );
  for (const baseItem of normalizedBaseLayout) {
    if (
      hasVisibleGraph &&
      baseItem.widgetType === DASHBOARD_WIDGET_IDS.workGraph
    ) {
      if (visibleGraph) {
        mergedBaseLayout.push(visibleGraph);
      }
      continue;
    }

    const matchingItems = itemsByID.get(baseItem.id) ?? [];
    const [matchingItem, ...duplicateItems] = matchingItems;
    mergedBaseLayout.push(
      mergeDashboardLayoutItem(matchingItem, baseItem),
      ...duplicateItems,
    );
  }

  const baseIDs = new Set([
    ...normalizedBaseLayout.map((item) => item.id),
    ...mergedBaseLayout.map((item) => item.id),
  ]);
  const additionalItems = normalizedLayout.filter(
    (item) => !baseIDs.has(item.id),
  );

  return [...mergedBaseLayout, ...additionalItems];
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

function normalizeDashboardLayoutItem(
  value: unknown,
): AgentBentoLayoutItem | null {
  if (!value || typeof value !== "object") {
    return null;
  }

  const candidate = value as Partial<AgentBentoLayoutItem> & {
    widgetType?: unknown;
  };
  const widgetType = resolveStoredWidgetType(candidate);
  if (!widgetType) {
    return null;
  }

  const defaultItem = getDefaultWidgetLayoutByType(widgetType);
  if (!defaultItem) {
    return null;
  }

  const id = isSafeDashboardWidgetInstanceID(candidate.id, widgetType)
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

function migrateDashboardLayout(
  layout: AgentBentoLayoutItem[],
): AgentBentoLayoutItem[] {
  const normalizedLayout = layout
    .map((item) => normalizeDashboardLayoutItem(item))
    .filter((item): item is AgentBentoLayoutItem => item !== null);
  const needsSessionControlsMigration = !normalizedLayout.some(
    (item) => item.widgetType === DASHBOARD_WIDGET_IDS.sessionControls,
  );
  const layoutWithTraceMigration = migrateTraceLayout(normalizedLayout);
  const compactedLayout = needsSessionControlsMigration
    ? migrateDashboardCompactionLayout(layoutWithTraceMigration)
    : layoutWithTraceMigration;
  const migratedLayout = normalizeInlineAddWidgetLayout(
    migrateProviderSessionLayout(
      migrateSessionControlsLayout(migrateWorkOutcomeLayout(compactedLayout)),
    ),
  );
  return migratedLayout;
}

function migrateSessionControlsLayout(
  layout: AgentBentoLayoutItem[],
): AgentBentoLayoutItem[] {
  if (
    layout.some(
      (item) => item.widgetType === DASHBOARD_WIDGET_IDS.sessionControls,
    )
  ) {
    return layout;
  }

  const sessionControlsItem = DEFAULT_DASHBOARD_LAYOUT.find(
    (item) => item.widgetType === DASHBOARD_WIDGET_IDS.sessionControls,
  );
  if (!sessionControlsItem) {
    throw new Error("default session-controls layout is missing");
  }

  return [
    { ...sessionControlsItem },
    ...layout.map((item) => ({ ...item, y: item.y + sessionControlsItem.h })),
  ];
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
    layout.some(
      (item) => item.widgetType === DASHBOARD_WIDGET_IDS.providerSession,
    )
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

function migrateTraceLayout(
  layout: AgentBentoLayoutItem[],
): AgentBentoLayoutItem[] {
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
    ...layout.filter(
      (item) => item.widgetType !== DASHBOARD_WIDGET_IDS.addWidget,
    ),
    normalizedInlineAddWidget,
  ];
}

function migrateWorkOutcomeLayout(
  layout: AgentBentoLayoutItem[],
): AgentBentoLayoutItem[] {
  const legacyDefaultChart = layout.find(
    (item) =>
      item.widgetType === DASHBOARD_WIDGET_IDS.workOutcomeChart &&
      item.h === 6 &&
      item.w === 4 &&
      item.x === 8 &&
      (item.y === 15 || item.y === 17),
  );

  return layout.map((item) => {
    if (item.widgetType === DASHBOARD_WIDGET_IDS.workOutcomeChart) {
      const width = Math.min(
        Math.max(item.w, WORK_OUTCOME_CHART_MIN_GRID_WIDTH),
        DASHBOARD_GRID_COLUMNS,
      );
      return {
        ...item,
        h:
          item === legacyDefaultChart
            ? WORK_OUTCOME_CHART_MIN_GRID_HEIGHT
            : Math.max(item.h, WORK_OUTCOME_CHART_MIN_GRID_HEIGHT),
        minH: WORK_OUTCOME_CHART_MIN_GRID_HEIGHT,
        minW: WORK_OUTCOME_CHART_MIN_GRID_WIDTH,
        w: width,
        x: Math.min(Math.max(item.x, 0), DASHBOARD_GRID_COLUMNS - width),
      };
    }

    if (!legacyDefaultChart) {
      return item;
    }

    if (
      item.widgetType === DASHBOARD_WIDGET_IDS.submitWork &&
      item.h === 6 &&
      item.w === 4 &&
      item.x === 8 &&
      item.y === legacyDefaultChart.y + 6
    ) {
      return { ...item, y: legacyDefaultChart.y + 4 };
    }

    if (
      (item.widgetType === DASHBOARD_WIDGET_IDS.addWidget ||
        item.widgetType === DASHBOARD_WIDGET_IDS.factorySession) &&
      item.h === 4 &&
      item.w === 4 &&
      item.x === 8 &&
      item.y === legacyDefaultChart.y + 12
    ) {
      return { ...item, y: legacyDefaultChart.y + 10 };
    }

    return item;
  });
}
