import type { AgentBentoLayoutItem } from "../components/agent-bento";
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

export const DUPLICATE_CAPABLE_DASHBOARD_WIDGET_TYPES = [
  DASHBOARD_WIDGET_IDS.terminalWork,
  DASHBOARD_WIDGET_IDS.workGraph,
  DASHBOARD_WIDGET_IDS.workOutcomeChart,
  DASHBOARD_WIDGET_IDS.workTotals,
] as const satisfies readonly DashboardWidgetPickerWidgetType[];

export interface DashboardWidgetPickerAvailability {
  duplicateCapable: boolean;
  enabled: boolean;
  widgetType: DashboardWidgetPickerWidgetType;
}

export function isDuplicateCapableDashboardWidgetType(
  widgetType: string,
): boolean {
  return (
    DUPLICATE_CAPABLE_DASHBOARD_WIDGET_TYPES as readonly string[]
  ).includes(widgetType);
}

export function canAddDashboardWidgetType(
  layout: readonly AgentBentoLayoutItem[],
  widgetType: DashboardWidgetPickerWidgetType,
): boolean {
  if (isDuplicateCapableDashboardWidgetType(widgetType)) {
    return true;
  }

  return !layout.some((item) => item.widgetType === widgetType && !item.hidden);
}

export function getDashboardWidgetPickerAvailability(
  layout: readonly AgentBentoLayoutItem[],
): DashboardWidgetPickerAvailability[] {
  return DASHBOARD_WIDGET_PICKER_WIDGET_TYPES.map((widgetType) => ({
    duplicateCapable: isDuplicateCapableDashboardWidgetType(widgetType),
    enabled: canAddDashboardWidgetType(layout, widgetType),
    widgetType,
  }));
}
