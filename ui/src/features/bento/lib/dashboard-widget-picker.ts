import { DASHBOARD_WIDGET_IDS } from "../hooks/dashboardLayoutSchema";

export const DASHBOARD_WIDGET_PICKER_WIDGET_TYPES = [
  DASHBOARD_WIDGET_IDS.workTotals,
  DASHBOARD_WIDGET_IDS.workGraph,
  DASHBOARD_WIDGET_IDS.currentSelection,
  DASHBOARD_WIDGET_IDS.providerSession,
  DASHBOARD_WIDGET_IDS.terminalWork,
  DASHBOARD_WIDGET_IDS.workOutcomeChart,
  DASHBOARD_WIDGET_IDS.submitWork,
  DASHBOARD_WIDGET_IDS.trace,
] as const;

export type DashboardWidgetPickerWidgetType =
  (typeof DASHBOARD_WIDGET_PICKER_WIDGET_TYPES)[number];
