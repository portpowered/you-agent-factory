import type { AgentBentoLayoutItem } from "../../../components/ui";

const PRIMARY_WIDGET_INSTANCE_SLOT = "primary";
const INLINE_ADD_WIDGET_INSTANCE_SLOT = "inline-add";

export const DASHBOARD_LAYOUT_STORAGE_KEY = "agent-factory.dashboard.layout.v2";

export const DASHBOARD_WIDGET_IDS = {
  addWidget: "add-widget",
  currentSelection: "current-selection",
  providerSession: "provider-session",
  submitWork: "submit-work",
  terminalWork: "terminal-work",
  trace: "trace",
  workGraph: "work-graph",
  workOutcomeChart: "work-outcome-chart",
  workTotals: "work-totals",
} as const;

export const LEGACY_SELECTION_WIDGET_IDS = [
  "workstation-info",
  "work-info",
  "terminal-summary",
] as const;

export const LEGACY_WORK_OUTCOME_WIDGET_IDS = [
  "completion-trend",
  "failure-trend",
] as const;

export const DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS = {
  currentSelection: createDashboardWidgetInstanceID(
    DASHBOARD_WIDGET_IDS.currentSelection,
    PRIMARY_WIDGET_INSTANCE_SLOT,
  ),
  providerSession: createDashboardWidgetInstanceID(
    DASHBOARD_WIDGET_IDS.providerSession,
    PRIMARY_WIDGET_INSTANCE_SLOT,
  ),
  submitWork: createDashboardWidgetInstanceID(
    DASHBOARD_WIDGET_IDS.submitWork,
    PRIMARY_WIDGET_INSTANCE_SLOT,
  ),
  terminalWork: createDashboardWidgetInstanceID(
    DASHBOARD_WIDGET_IDS.terminalWork,
    PRIMARY_WIDGET_INSTANCE_SLOT,
  ),
  trace: createDashboardWidgetInstanceID(
    DASHBOARD_WIDGET_IDS.trace,
    PRIMARY_WIDGET_INSTANCE_SLOT,
  ),
  workGraph: createDashboardWidgetInstanceID(
    DASHBOARD_WIDGET_IDS.workGraph,
    PRIMARY_WIDGET_INSTANCE_SLOT,
  ),
  workOutcomeChart: createDashboardWidgetInstanceID(
    DASHBOARD_WIDGET_IDS.workOutcomeChart,
    PRIMARY_WIDGET_INSTANCE_SLOT,
  ),
  workTotals: createDashboardWidgetInstanceID(
    DASHBOARD_WIDGET_IDS.workTotals,
    PRIMARY_WIDGET_INSTANCE_SLOT,
  ),
} as const;

export const DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID = createDashboardWidgetInstanceID(
  DASHBOARD_WIDGET_IDS.addWidget,
  INLINE_ADD_WIDGET_INSTANCE_SLOT,
);

const DEFAULT_DASHBOARD_LAYOUT_ITEMS = [
  dashboardLayoutItem({
    h: 2,
    id: DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.workTotals,
    minH: 1,
    minW: 1,
    w: 12,
    widgetType: DASHBOARD_WIDGET_IDS.workTotals,
    x: 0,
    y: 0,
  }),
  dashboardLayoutItem({
    h: 8,
    id: DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.workGraph,
    minH: 1,
    minW: 1,
    w: 12,
    widgetType: DASHBOARD_WIDGET_IDS.workGraph,
    x: 0,
    y: 2,
  }),
  dashboardLayoutItem({
    h: 5,
    id: DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.currentSelection,
    minH: 1,
    minW: 1,
    w: 4,
    widgetType: DASHBOARD_WIDGET_IDS.currentSelection,
    x: 0,
    y: 10,
  }),
  dashboardLayoutItem({
    h: 5,
    id: DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.providerSession,
    minH: 1,
    minW: 1,
    w: 4,
    widgetType: DASHBOARD_WIDGET_IDS.providerSession,
    x: 4,
    y: 10,
  }),
  dashboardLayoutItem({
    h: 5,
    id: DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.terminalWork,
    minH: 1,
    minW: 1,
    w: 4,
    widgetType: DASHBOARD_WIDGET_IDS.terminalWork,
    x: 8,
    y: 10,
  }),
  dashboardLayoutItem({
    h: 6,
    id: DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.workOutcomeChart,
    minH: 1,
    minW: 1,
    w: 4,
    widgetType: DASHBOARD_WIDGET_IDS.workOutcomeChart,
    x: 8,
    y: 15,
  }),
  dashboardLayoutItem({
    h: 6,
    id: DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.submitWork,
    minH: 1,
    minW: 1,
    w: 4,
    widgetType: DASHBOARD_WIDGET_IDS.submitWork,
    x: 8,
    y: 21,
  }),
  dashboardLayoutItem({
    h: 11,
    id: DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.trace,
    minH: 1,
    minW: 1,
    w: 8,
    widgetType: DASHBOARD_WIDGET_IDS.trace,
    x: 0,
    y: 15,
  }),
  dashboardLayoutItem({
    h: 4,
    id: DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID,
    minH: 1,
    minW: 1,
    w: 4,
    widgetType: DASHBOARD_WIDGET_IDS.addWidget,
    x: 8,
    y: 27,
  }),
] as const satisfies readonly AgentBentoLayoutItem[];

export const DEFAULT_DASHBOARD_LAYOUT = DEFAULT_DASHBOARD_LAYOUT_ITEMS.map((item) => ({
  ...item,
}));

export function createDashboardWidgetInstanceID(
  widgetType: string,
  slot: string,
): string {
  return `${widgetType}::${slot}`;
}

export function getRenderableDashboardLayout(
  layout: AgentBentoLayoutItem[],
  renderableWidgetTypes: readonly string[],
): AgentBentoLayoutItem[] {
  const renderableWidgetTypeSet = new Set(renderableWidgetTypes);
  return layout.filter(
    (item) => !item.hidden && renderableWidgetTypeSet.has(item.widgetType),
  );
}

export function getDefaultInlineAddWidgetLayout(): AgentBentoLayoutItem {
  const inlineAddWidget = DEFAULT_DASHBOARD_LAYOUT.find(
    (item) => item.id === DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID,
  );
  if (!inlineAddWidget) {
    throw new Error("default inline add-widget layout is missing");
  }

  return { ...inlineAddWidget };
}

export function getDefaultWidgetLayoutByType(
  widgetType: string,
): AgentBentoLayoutItem | undefined {
  return DEFAULT_DASHBOARD_LAYOUT.find((item) => item.widgetType === widgetType);
}

export function getPrimaryInstanceIDForWidgetType(widgetType: string): string {
  switch (widgetType) {
    case DASHBOARD_WIDGET_IDS.currentSelection:
      return DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.currentSelection;
    case DASHBOARD_WIDGET_IDS.providerSession:
      return DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.providerSession;
    case DASHBOARD_WIDGET_IDS.submitWork:
      return DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.submitWork;
    case DASHBOARD_WIDGET_IDS.terminalWork:
      return DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.terminalWork;
    case DASHBOARD_WIDGET_IDS.trace:
      return DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.trace;
    case DASHBOARD_WIDGET_IDS.workGraph:
      return DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.workGraph;
    case DASHBOARD_WIDGET_IDS.workOutcomeChart:
      return DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.workOutcomeChart;
    case DASHBOARD_WIDGET_IDS.workTotals:
      return DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.workTotals;
    case DASHBOARD_WIDGET_IDS.addWidget:
      return DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID;
    default:
      return createDashboardWidgetInstanceID(widgetType, PRIMARY_WIDGET_INSTANCE_SLOT);
  }
}

export function isDashboardWidgetID(value: unknown): value is string {
  return (
    typeof value === "string" &&
    (Object.values(DASHBOARD_WIDGET_IDS) as string[]).includes(value)
  );
}

function dashboardLayoutItem(item: AgentBentoLayoutItem): AgentBentoLayoutItem {
  return { ...item };
}
