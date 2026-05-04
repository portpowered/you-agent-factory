import { useMemo } from "react";
import {
  CartesianGrid,
  Line,
  LineChart,
  XAxis,
  YAxis,
} from "recharts";

import {
  DASHBOARD_CHART_AXIS_LABEL_CLASS,
} from "./chart-contract";
import {
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
} from "../../components/ui/chart";
import { Skeleton } from "../../components/ui/skeleton";
import { cn } from "../../lib/cn";
import {
  EMPTY_STATE_CLASS,
  EMPTY_STATE_COMPACT_CLASS,
} from "../../components/dashboard/widget-board";
import type { WorkChartModel, WorkChartSeriesKey } from "./trends";

export const WORK_CHART_AXIS_LABEL_CLASS = DASHBOARD_CHART_AXIS_LABEL_CLASS;
export const WORK_CHART_EMPTY_TITLE = "No work outcome samples";
export const WORK_CHART_EMPTY_MESSAGE =
  "Work outcome data appears after the event stream receives work history.";
export const WORK_CHART_LOADING_TITLE = "Loading work outcome samples";
export const WORK_CHART_LOADING_MESSAGE = "Waiting for dashboard timeline data.";
export const WORK_CHART_ERROR_TITLE = "Work outcome chart unavailable";
export const WORK_CHART_ERROR_MESSAGE =
  "Chart data is incomplete, so the dashboard cannot draw this work outcome view yet.";
const WORK_CHART_MARGIN = { bottom: 40, left: 18, right: 28, top: 28 };
const WORK_CHART_OVERLAY_CLASS =
  "flex items-start justify-between gap-3 px-4 pb-3 pt-3 sm:px-5 sm:pb-4 sm:pt-4";
const WORK_CHART_X_AXIS_OVERLAY_CLASS = "self-end";
const WORK_CHART_Y_AXIS_WIDTH = 52;

export interface WorkChartSeriesDefinition {
  key: WorkChartSeriesKey;
  label: string;
  lineColor: string;
  lineClassName: string;
  pointClassName?: string;
  pointRadius?: number;
}

export type WorkChartState =
  | { status: "ready" }
  | { message?: string; status: "loading"; title?: string }
  | { message?: string; status: "error"; title?: string };

export interface WorkChartProps {
  ariaLabel: string;
  className?: string;
  emptyMessage?: string;
  emptyTitle?: string;
  model?: WorkChartModel;
  series: readonly WorkChartSeriesDefinition[];
  state?: WorkChartState;
  xAxisLabel?: string;
  yAxisLabel?: string;
}

const READY_WORK_CHART_STATE: WorkChartState = { status: "ready" };

export function WorkChart({
  ariaLabel,
  className = "",
  emptyMessage = WORK_CHART_EMPTY_MESSAGE,
  emptyTitle = WORK_CHART_EMPTY_TITLE,
  model,
  series,
  state = READY_WORK_CHART_STATE,
  xAxisLabel = "Ticks",
  yAxisLabel = "Work count",
}: WorkChartProps) {
  const chartData = useMemo(() => {
    if (state.status !== "ready") {
      return { status: state.status };
    }

    return buildWorkChartData(model, series);
  }, [model, series, state.status]);

  if (state.status === "loading") {
    return (
      <WorkChartStatusPanel
        ariaBusy={true}
        loading={true}
        message={state.message ?? WORK_CHART_LOADING_MESSAGE}
        role="status"
        title={state.title ?? WORK_CHART_LOADING_TITLE}
      />
    );
  }

  if (state.status === "error" || chartData.status === "invalid") {
    return (
      <WorkChartStatusPanel
        message={
          state.status === "error"
            ? (state.message ?? WORK_CHART_ERROR_MESSAGE)
            : WORK_CHART_ERROR_MESSAGE
        }
        role="alert"
        title={
          state.status === "error"
            ? (state.title ?? WORK_CHART_ERROR_TITLE)
            : WORK_CHART_ERROR_TITLE
        }
      />
    );
  }

  if (chartData.status === "empty") {
    return (
      <WorkChartStatusPanel
        message={emptyMessage}
        role="status"
        title={emptyTitle}
      />
    );
  }

  if (chartData.status !== "ready") {
    return (
      <WorkChartStatusPanel
        message={WORK_CHART_ERROR_MESSAGE}
        role="alert"
        title={WORK_CHART_ERROR_TITLE}
      />
    );
  }

  return renderReadyWorkChart({
    ariaLabel,
    chartData: chartData.data,
    className,
    xAxisLabel,
    yAxisLabel,
  });
}

interface ReadyWorkChartProps {
  ariaLabel: string;
  chartData: WorkChartData;
  className: string;
  xAxisLabel: string;
  yAxisLabel: string;
}

function renderReadyWorkChart({
  ariaLabel,
  chartData,
  className,
  xAxisLabel,
  yAxisLabel,
}: ReadyWorkChartProps) {
  return (
    <ChartContainer
      className={cn("h-[16rem] min-h-[14rem] px-4 pb-4 pt-3 sm:h-[18rem] sm:px-5 sm:pb-5 sm:pt-4", className)}
      config={chartData.config}
      overlay={
        <div className={WORK_CHART_OVERLAY_CLASS}>
          <p className={cn("m-0", WORK_CHART_AXIS_LABEL_CLASS)}>{yAxisLabel}</p>
          <p className={cn("m-0", WORK_CHART_AXIS_LABEL_CLASS, WORK_CHART_X_AXIS_OVERLAY_CLASS)}>
            {xAxisLabel}
          </p>
        </div>
      }
      title={ariaLabel}
    >
      <LineChart
        accessibilityLayer
        data={chartData.rows}
        margin={WORK_CHART_MARGIN}
      >
        <CartesianGrid vertical={false} />
        <XAxis
          axisLine={false}
          dataKey="tick"
          minTickGap={24}
          tick={{ className: WORK_CHART_AXIS_LABEL_CLASS }}
          tickFormatter={formatAxisNumber}
          tickLine={false}
        />
        <YAxis
          allowDecimals={false}
          axisLine={false}
          tick={{ className: WORK_CHART_AXIS_LABEL_CLASS }}
          tickCount={5}
          tickFormatter={formatAxisNumber}
          tickLine={false}
          width={WORK_CHART_Y_AXIS_WIDTH}
        />
        <ChartTooltip
          content={(props) => {
            const label = props.payload?.[0]?.payload?.label ?? props.label;
            return <ChartTooltipContent {...props} label={label} />;
          }}
          cursor={{ stroke: "rgb(from var(--color-af-overlay) r g b / 0.16)" }}
        />
        <ChartLegend content={<ChartLegendContent />} />
        {chartData.series.map((seriesData) => (
          <Line
            key={seriesData.key}
            activeDot={{
              className: seriesData.pointClassName,
              fill: seriesData.lineColor,
              r: seriesData.pointRadius,
              stroke: "rgb(from var(--color-af-canvas) r g b / 0.88)",
              strokeWidth: 1.5,
            }}
            className={seriesData.lineClassName}
            data-chart-series={seriesData.key}
            data-chart-series-color={seriesData.lineColor}
            dataKey={seriesData.key}
            dot={false}
            isAnimationActive={false}
            name={seriesData.label}
            stroke={seriesData.lineColor}
            strokeDasharray={seriesData.strokeDasharray}
            strokeWidth={2.25}
            type="linear"
          />
        ))}
      </LineChart>
    </ChartContainer>
  );
}

interface WorkChartStatusPanelProps {
  ariaBusy?: boolean;
  loading?: boolean;
  message: string;
  role: "alert" | "status";
  title: string;
}

function WorkChartStatusPanel({
  ariaBusy = false,
  loading = false,
  message,
  role,
  title,
}: WorkChartStatusPanelProps) {
  return (
    <div
      aria-busy={ariaBusy || undefined}
      aria-live={role === "alert" ? "assertive" : "polite"}
      className={cn(EMPTY_STATE_CLASS, EMPTY_STATE_COMPACT_CLASS)}
      role={role}
    >
      {loading ? (
        <div aria-hidden="true" className="grid w-full gap-3">
          <Skeleton className="h-4 w-32" />
          <Skeleton className="h-28 w-full" />
        </div>
      ) : null}
      <h3>{title}</h3>
      <p>{message}</p>
    </div>
  );
}

interface WorkChartBuiltSeries {
  key: string;
  label: string;
  lineColor: string;
  lineClassName: string;
  pointClassName?: string;
  pointRadius?: number;
  strokeDasharray?: string;
}

interface WorkChartData {
  config: Record<string, { color: string; label: string }>;
  rows: WorkChartRow[];
  series: WorkChartBuiltSeries[];
}

interface WorkChartRow {
  label: string;
  tick: number;
  [seriesKey: string]: number | string | undefined;
}

type WorkChartDataResult =
  | { data: WorkChartData; status: "ready" }
  | { status: "empty" }
  | { status: "invalid" };

function buildWorkChartData(
  model: WorkChartModel | undefined,
  series: readonly WorkChartSeriesDefinition[],
): WorkChartDataResult {
  if (!isWorkChartModel(model) || !isWorkChartSeriesDefinitionArray(series)) {
    return { status: "invalid" };
  }

  if (model.points.length === 0 || series.length === 0) {
    return { status: "empty" };
  }

  const seriesByKey = new Map(
    model.series.map((definition) => [definition.key, definition.points]),
  );
  const rows = model.points.map((point, index) => {
    const row: WorkChartRow = {
      label: point.label,
      tick: point.tick,
    };

    for (const definition of series) {
      const value = seriesByKey
        .get(definition.key)
        ?.find((seriesPoint) => seriesPoint.order === index)?.value;
      if (value !== undefined) {
        row[definition.key] = value;
      }
    }

    return row;
  });

  const builtSeries = series
    .filter((definition) => rows.some((row) => hasSeriesValue(row, definition.key)))
    .map((definition) => ({
      key: definition.key,
      label: definition.label,
      lineClassName: definition.lineClassName,
      lineColor: definition.lineColor,
      pointClassName: definition.pointClassName,
      pointRadius: definition.pointRadius,
      strokeDasharray: extractStrokeDasharray(definition.lineClassName),
    }));

  return {
    data: {
      config: Object.fromEntries(
        builtSeries.map((seriesEntry) => [
          seriesEntry.key,
          { color: seriesEntry.lineColor, label: seriesEntry.label },
        ]),
      ),
      rows,
      series: builtSeries,
    },
    status: "ready",
  };
}

function hasSeriesValue(row: WorkChartRow, key: string): boolean {
  return  Object.hasOwn(row, key) && typeof row[key] === "number";
}

function formatAxisNumber(value: number): string {
  if (Number.isInteger(value)) {
    return String(value);
  }
  return value.toFixed(1);
}

function extractStrokeDasharray(className: string): string | undefined {
  const dashArrayMatch = className.match(/\[stroke-dasharray:([^\]]+)\]/);
  return dashArrayMatch?.[1]?.replaceAll("_", " ");
}

function isWorkChartSeriesDefinitionArray(
  value: unknown,
): value is readonly WorkChartSeriesDefinition[] {
  return Array.isArray(value) && value.every(isWorkChartSeriesDefinition);
}

function isWorkChartSeriesDefinition(value: unknown): value is WorkChartSeriesDefinition {
  return (
    isRecord(value) &&
    typeof value.key === "string" &&
    typeof value.label === "string" &&
    typeof value.lineColor === "string" &&
    typeof value.lineClassName === "string" &&
    (value.pointClassName === undefined || typeof value.pointClassName === "string") &&
    (value.pointRadius === undefined || isFiniteNumber(value.pointRadius))
  );
}

function isWorkChartModel(value: unknown): value is WorkChartModel {
  return (
    isRecord(value) &&
    Array.isArray(value.points) &&
    Array.isArray(value.series) &&
    value.points.every(isWorkChartSample) &&
    value.series.every(isWorkChartSeries)
  );
}

function isWorkChartSample(value: unknown): value is WorkChartModel["points"][number] {
  return (
    isRecord(value) &&
    typeof value.label === "string" &&
    isFiniteNumber(value.observedAt) &&
    isFiniteNumber(value.order) &&
    isFiniteNumber(value.tick)
  );
}

function isWorkChartSeries(value: unknown): value is WorkChartModel["series"][number] {
  return (
    isRecord(value) &&
    typeof value.key === "string" &&
    typeof value.label === "string" &&
    Array.isArray(value.points) &&
    value.points.every(isWorkChartSeriesPoint)
  );
}

function isWorkChartSeriesPoint(
  value: unknown,
): value is WorkChartModel["series"][number]["points"][number] {
  return (
    isRecord(value) &&
    typeof value.label === "string" &&
    isFiniteNumber(value.observedAt) &&
    isFiniteNumber(value.order) &&
    isFiniteNumber(value.value)
  );
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}
