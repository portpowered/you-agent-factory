import { cx } from "../../lib/cx";
import { DASHBOARD_SUPPORTING_LABEL_CLASS } from "../../components/dashboard/typography";

export type DashboardChartSemanticRole =
  | "queued"
  | "inFlight"
  | "completed"
  | "failed"
  | "failureTrend"
  | "reworkTrend"
  | "timingTrend";

export type DashboardWorkChartSemanticRole = Extract<
  DashboardChartSemanticRole,
  "queued" | "inFlight" | "completed" | "failed"
>;

export interface DashboardChartSemanticStyle {
  color: string;
  lineClassName: string;
  pointClassName: string;
  pointRadius: number;
}

export interface DashboardWorkChartSeriesStyle {
  lineClassName: string;
  lineColor: string;
  pointClassName: string;
  pointRadius: number;
}

export interface DashboardWorkChartSeriesDefinitionInput {
  key: DashboardWorkChartSemanticRole;
  label: string;
}

const DASHBOARD_CHART_LINE_WEIGHT_CLASS = "[stroke-width:2.25]";
const DASHBOARD_CHART_POINT_WEIGHT_CLASS = "[stroke-width:1.5]";
const DASHBOARD_CHART_DEFAULT_POINT_RADIUS = 3.25;

export const DASHBOARD_CHART_AXIS_CLASS = "stroke-af-ink/18 [stroke-width:1]";
export const DASHBOARD_CHART_AXIS_LABEL_CLASS = cx(
  "fill-af-ink/58 [letter-spacing:0.16em]",
  DASHBOARD_SUPPORTING_LABEL_CLASS,
);
export const DASHBOARD_CHART_GRID_CLASS = "stroke-af-ink/8 [stroke-width:1]";
export const DASHBOARD_CHART_LINE_CLASS = cx(
  "fill-none [stroke-linecap:round] [stroke-linejoin:round]",
  DASHBOARD_CHART_LINE_WEIGHT_CLASS,
);
export const DASHBOARD_CHART_POINT_CLASS = cx(
  "stroke-af-canvas/88",
  DASHBOARD_CHART_POINT_WEIGHT_CLASS,
);
export const DASHBOARD_CHART_SURFACE_CLASS =
  "block min-h-0 w-full rounded-lg [background:linear-gradient(rgb(from_var(--color-af-overlay)_r_g_b_/_0.032)_1px,transparent_1px),linear-gradient(90deg,rgb(from_var(--color-af-overlay)_r_g_b_/_0.032)_1px,transparent_1px)] [background-size:48px_32px]";

const DASHBOARD_CHART_SEMANTIC_STYLES: Record<
  DashboardChartSemanticRole,
  DashboardChartSemanticStyle
> = {
  queued: {
    color: "var(--color-af-chart-queued)",
    lineClassName: cx(DASHBOARD_CHART_LINE_CLASS, "[stroke-dasharray:5_4]"),
    pointClassName: cx(DASHBOARD_CHART_POINT_CLASS, "fill-af-chart-queued"),
    pointRadius: DASHBOARD_CHART_DEFAULT_POINT_RADIUS,
  },
  inFlight: {
    color: "var(--color-af-chart-in-flight)",
    lineClassName: cx(DASHBOARD_CHART_LINE_CLASS, "[stroke-dasharray:7_5]"),
    pointClassName: cx(DASHBOARD_CHART_POINT_CLASS, "fill-af-chart-in-flight"),
    pointRadius: DASHBOARD_CHART_DEFAULT_POINT_RADIUS,
  },
  completed: {
    color: "var(--color-af-chart-completed)",
    lineClassName: DASHBOARD_CHART_LINE_CLASS,
    pointClassName: cx(DASHBOARD_CHART_POINT_CLASS, "fill-af-chart-completed"),
    pointRadius: DASHBOARD_CHART_DEFAULT_POINT_RADIUS,
  },
  failed: {
    color: "var(--color-af-chart-failed)",
    lineClassName: cx(DASHBOARD_CHART_LINE_CLASS, "[stroke-dasharray:2.5_4.5]"),
    pointClassName: cx(DASHBOARD_CHART_POINT_CLASS, "fill-af-chart-failed"),
    pointRadius: DASHBOARD_CHART_DEFAULT_POINT_RADIUS,
  },
  failureTrend: {
    color: "var(--color-af-chart-failure-trend)",
    lineClassName: DASHBOARD_CHART_LINE_CLASS,
    pointClassName: cx(DASHBOARD_CHART_POINT_CLASS, "fill-af-chart-failure-trend"),
    pointRadius: DASHBOARD_CHART_DEFAULT_POINT_RADIUS,
  },
  reworkTrend: {
    color: "var(--color-af-chart-rework-trend)",
    lineClassName: DASHBOARD_CHART_LINE_CLASS,
    pointClassName: cx(DASHBOARD_CHART_POINT_CLASS, "fill-af-chart-rework-trend"),
    pointRadius: DASHBOARD_CHART_DEFAULT_POINT_RADIUS,
  },
  timingTrend: {
    color: "var(--color-af-chart-timing-trend)",
    lineClassName: DASHBOARD_CHART_LINE_CLASS,
    pointClassName: cx(DASHBOARD_CHART_POINT_CLASS, "fill-af-chart-timing-trend"),
    pointRadius: DASHBOARD_CHART_DEFAULT_POINT_RADIUS,
  },
};

export function getDashboardChartSemanticStyle(
  role: DashboardChartSemanticRole,
): DashboardChartSemanticStyle {
  return DASHBOARD_CHART_SEMANTIC_STYLES[role];
}

export function getDashboardWorkChartSeriesStyle(
  role: DashboardWorkChartSemanticRole,
): DashboardWorkChartSeriesStyle {
  const semanticStyle = getDashboardChartSemanticStyle(role);

  return {
    lineClassName: semanticStyle.lineClassName,
    lineColor: semanticStyle.color,
    pointClassName: semanticStyle.pointClassName,
    pointRadius: semanticStyle.pointRadius,
  };
}

export function getDashboardWorkChartSeriesDefinitions<
  TDefinition extends DashboardWorkChartSeriesDefinitionInput,
>(
  definitions: readonly TDefinition[],
): Array<TDefinition & DashboardWorkChartSeriesStyle> {
  return definitions.map((definition) => ({
    ...definition,
    ...getDashboardWorkChartSeriesStyle(definition.key),
  }));
}

