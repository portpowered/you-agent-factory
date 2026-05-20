import { create } from "zustand";

import type { AgentBentoLayoutItem } from "../../components/ui";

const DASHBOARD_LAYOUT_STORAGE_KEY = "agent-factory.dashboard.layout.v2";
const LEGACY_SELECTION_WIDGET_IDS = [
  "workstation-info",
  "work-info",
  "terminal-summary",
] as const;
const LEGACY_WORK_OUTCOME_WIDGET_IDS = ["completion-trend", "failure-trend"] as const;

export const DASHBOARD_WIDGET_IDS = {
  currentSelection: "current-selection",
  submitWork: "submit-work",
  terminalWork: "terminal-work",
  trace: "trace",
  workGraph: "work-graph",
  workOutcomeChart: "work-outcome-chart",
  workTotals: "work-totals",
} as const;

export const DEFAULT_DASHBOARD_LAYOUT: AgentBentoLayoutItem[] = [
  { id: DASHBOARD_WIDGET_IDS.workTotals, x: 0, y: 0, w: 12, h: 2, minH: 2, minW: 6 },
  { id: DASHBOARD_WIDGET_IDS.workGraph, x: 0, y: 2, w: 12, h: 10, minH: 8, minW: 6 },
  {
    id: DASHBOARD_WIDGET_IDS.currentSelection,
    x: 0,
    y: 12,
    w: 4,
    h: 5,
    minH: 4,
    minW: 3,
  },
  { id: DASHBOARD_WIDGET_IDS.terminalWork, x: 4, y: 12, w: 4, h: 5, minH: 3, minW: 3 },
  {
    id: DASHBOARD_WIDGET_IDS.workOutcomeChart,
    x: 8,
    y: 12,
    w: 4,
    h: 6,
    minH: 5,
    minW: 3,
  },
  {
    id: DASHBOARD_WIDGET_IDS.submitWork,
    x: 8,
    y: 18,
    w: 4,
    h: 6,
    minH: 5,
    minW: 4,
  },
  {
    id: DASHBOARD_WIDGET_IDS.trace,
    x: 0,
    y: 18,
    w: 8,
    h: 9,
    minH: 7,
    minW: 5,
  },
];

export interface UseDashboardLayoutResult {
  dashboardLayout: AgentBentoLayoutItem[];
  persistDashboardLayout: (layout: AgentBentoLayoutItem[]) => void;
}

interface DashboardLayoutStoreState {
  dashboardLayout: AgentBentoLayoutItem[];
  persistDashboardLayout: (layout: AgentBentoLayoutItem[]) => void;
}

const useDashboardLayoutStore = create<DashboardLayoutStoreState>((set) => ({
  dashboardLayout: readStoredDashboardLayout(),
  persistDashboardLayout: (layout) => {
    set((state) => {
      const nextLayout = mergeDashboardLayout(layout, state.dashboardLayout);
      writeStoredDashboardLayout(nextLayout);
      return { dashboardLayout: nextLayout };
    });
  },
}));

export function useDashboardLayout(): UseDashboardLayoutResult {
  const dashboardLayout = useDashboardLayoutStore((state) => state.dashboardLayout);
  const persistDashboardLayout = useDashboardLayoutStore((state) => state.persistDashboardLayout);

  return { dashboardLayout, persistDashboardLayout };
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

function mergeDashboardLayout(
  layout: AgentBentoLayoutItem[],
  baseLayout = DEFAULT_DASHBOARD_LAYOUT,
): AgentBentoLayoutItem[] {
  const normalizedBaseLayout = migrateDashboardLayout(baseLayout);
  const itemsById = new Map(migrateDashboardLayout(layout).map((item) => [item.id, item]));

  return normalizedBaseLayout.map((baseItem) => {
    const item = itemsById.get(baseItem.id);
    if (!item) {
      return baseItem;
    }

    return {
      ...baseItem,
      h: isFiniteNumber(item.h) ? item.h : baseItem.h,
      w: isFiniteNumber(item.w) ? item.w : baseItem.w,
      x: isFiniteNumber(item.x) ? item.x : baseItem.x,
      y: isFiniteNumber(item.y) ? item.y : baseItem.y,
    };
  });
}

export function reloadDashboardLayoutFromStorage(): void {
  useDashboardLayoutStore.setState({
    dashboardLayout: readStoredDashboardLayout(),
  });
}

function migrateDashboardLayout(layout: AgentBentoLayoutItem[]): AgentBentoLayoutItem[] {
  const normalizedLayout = migrateTraceLayout(migrateWorkOutcomeLayout(layout));

  const legacySelectionItem = LEGACY_SELECTION_WIDGET_IDS
    .map((id) => normalizedLayout.find((item) => item.id === id))
    .find((item): item is AgentBentoLayoutItem => item !== undefined);

  if (
    normalizedLayout.some((item) => item.id === DASHBOARD_WIDGET_IDS.currentSelection) ||
    !legacySelectionItem
  ) {
    return normalizedLayout.filter(
      (item) => !LEGACY_SELECTION_WIDGET_IDS.some((legacyId) => legacyId === item.id),
    );
  }

  return [
    ...normalizedLayout.filter(
      (item) => !LEGACY_SELECTION_WIDGET_IDS.some((legacyId) => legacyId === item.id),
    ),
    {
      ...legacySelectionItem,
      id: DASHBOARD_WIDGET_IDS.currentSelection,
    },
  ];
}

function migrateTraceLayout(layout: AgentBentoLayoutItem[]): AgentBentoLayoutItem[] {
  return layout.map((item) => {
    if (item.id === DASHBOARD_WIDGET_IDS.trace) {
      if (
        item.x === 0 &&
        item.y === 18 &&
        item.w === 4 &&
        item.h === 7
      ) {
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

function migrateWorkOutcomeLayout(layout: AgentBentoLayoutItem[]): AgentBentoLayoutItem[] {
  const hasWorkOutcomeItem = layout.some(
    (item) => item.id === DASHBOARD_WIDGET_IDS.workOutcomeChart,
  );
  const legacyWorkOutcomeItem = LEGACY_WORK_OUTCOME_WIDGET_IDS
    .map((id) => layout.find((item) => item.id === id))
    .find((item): item is AgentBentoLayoutItem => item !== undefined);
  const withoutLegacyItems = layout.filter(
    (item) => !LEGACY_WORK_OUTCOME_WIDGET_IDS.some((legacyId) => legacyId === item.id),
  );

  if (hasWorkOutcomeItem || !legacyWorkOutcomeItem) {
    return withoutLegacyItems;
  }

  return [
    ...withoutLegacyItems,
    {
      ...legacyWorkOutcomeItem,
      id: DASHBOARD_WIDGET_IDS.workOutcomeChart,
    },
  ];
}

function readStoredDashboardLayout(): AgentBentoLayoutItem[] {
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

function writeStoredDashboardLayout(layout: AgentBentoLayoutItem[]): void {
  try {
    window.localStorage.setItem(DASHBOARD_LAYOUT_STORAGE_KEY, JSON.stringify(layout));
  } catch {
    // Layout persistence is a convenience; dashboard interaction should keep working without it.
  }
}
