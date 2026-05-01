import { useMemo } from "react";

import { line, scaleLinear } from "d3";

import {
  DASHBOARD_CHART_AXIS_CLASS,
  DASHBOARD_CHART_AXIS_LABEL_CLASS,
  DASHBOARD_CHART_GRID_CLASS,
  DASHBOARD_CHART_LINE_CLASS,
  DASHBOARD_CHART_SURFACE_CLASS,
} from "./chart-contract";
import { cx } from "../../components/dashboard/classnames";
import {
  EMPTY_STATE_CLASS,
  EMPTY_STATE_COMPACT_CLASS,
} from "../../components/dashboard/widget-board";
import type { WorkChartModel, WorkChartSeriesKey } from "./trends";

export const WORK_CHART_WIDTH = 320;
export const WORK_CHART_HEIGHT = 120;
export const WORK_CHART_PADDING = { bottom: 28, left: 36, right: 10, top: 12 } as const;
export const WORK_CHART_AXIS_CLASS = DASHBOARD_CHART_AXIS_CLASS;
export const WORK_CHART_GRID_CLASS = DASHBOARD_CHART_GRID_CLASS;
export const WORK_CHART_CLASS = DASHBOARD_CHART_SURFACE_CLASS;
export const WORK_CHART_AXIS_LABEL_CLASS = DASHBOARD_CHART_AXIS_LABEL_CLASS;
export const WORK_CHART_LINE_CLASS = DASHBOARD_CHART_LINE_CLASS;
export const WORK_CHART_EMPTY_TITLE = "No work outcome samples";
export const WORK_CHART_EMPTY_MESSAGE =
  "Work outcome data appears after the event stream receives work history.";
export const WORK_CHART_LOADING_TITLE = "Loading work outcome samples";
export const WORK_CHART_LOADING_MESSAGE = "Waiting for dashboard timeline data.";
export const WORK_CHART_ERROR_TITLE = "Work outcome chart unavailable";
export const WORK_CHART_ERROR_MESSAGE =
  "Chart data is incomplete, so the dashboard cannot draw this work outcome view yet.";

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
  descriptionID?: string;
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
  descriptionID,
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

    return buildWorkChartData(
      model,
      series,
      WORK_CHART_WIDTH,
      WORK_CHART_HEIGHT,
      WORK_CHART_PADDING,
    );
  }, [model, series, state.status]);

  if (state.status === "loading") {
    return (
      <WorkChartStatusPanel
        ariaBusy={true}
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

  return (
    <svg
      aria-describedby={descriptionID}
      aria-label={ariaLabel}
      className={cx(WORK_CHART_CLASS, className)}
      role="img"
      viewBox={`0 0 ${WORK_CHART_WIDTH} ${WORK_CHART_HEIGHT}`}
    >
      {chartData.data.yTicks.map((tick, index) => (
        <g key={`y-${tick.label}-${index}`}>
          <line
            className={WORK_CHART_GRID_CLASS}
            data-axis-gridline="y"
            x1={WORK_CHART_PADDING.left}
            x2={WORK_CHART_WIDTH - WORK_CHART_PADDING.right}
            y1={tick.y}
            y2={tick.y}
          />
          <text
            className={WORK_CHART_AXIS_LABEL_CLASS}
            data-axis-tick="y"
            data-axis-tick-value={tick.label}
            textAnchor="end"
            x={WORK_CHART_PADDING.left - 5}
            y={tick.y + 3}
          >
            {tick.label}
          </text>
        </g>
      ))}
      {chartData.data.xTicks.map((tick, index) => (
        <g key={`x-${tick.label}-${index}`}>
          <line
            className={WORK_CHART_GRID_CLASS}
            data-axis-gridline="x"
            x1={tick.x}
            x2={tick.x}
            y1={WORK_CHART_PADDING.top}
            y2={WORK_CHART_HEIGHT - WORK_CHART_PADDING.bottom}
          />
          <text
            className={WORK_CHART_AXIS_LABEL_CLASS}
            data-axis-tick="x"
            data-axis-tick-value={tick.label}
            textAnchor="middle"
            x={tick.x}
            y={WORK_CHART_HEIGHT - WORK_CHART_PADDING.bottom + 12}
          >
            {tick.label}
          </text>
        </g>
      ))}
      <line
        className={WORK_CHART_AXIS_CLASS}
        x1={WORK_CHART_PADDING.left}
        x2={WORK_CHART_WIDTH - WORK_CHART_PADDING.right}
        y1={WORK_CHART_HEIGHT - WORK_CHART_PADDING.bottom}
        y2={WORK_CHART_HEIGHT - WORK_CHART_PADDING.bottom}
      />
      <line
        className={WORK_CHART_AXIS_CLASS}
        x1={WORK_CHART_PADDING.left}
        x2={WORK_CHART_PADDING.left}
        y1={WORK_CHART_PADDING.top}
        y2={WORK_CHART_HEIGHT - WORK_CHART_PADDING.bottom}
      />
      <text
        className={WORK_CHART_AXIS_LABEL_CLASS}
        x={WORK_CHART_WIDTH / 2}
        y={WORK_CHART_HEIGHT - 3}
        textAnchor="middle"
      >
        {xAxisLabel}
      </text>
      <text
        className={WORK_CHART_AXIS_LABEL_CLASS}
        textAnchor="start"
        x={WORK_CHART_PADDING.left}
        y={8}
      >
        {yAxisLabel}
      </text>
      {chartData.data.series.map((seriesData) => {
        if (!seriesData.hasData || !seriesData.path) {
          return null;
        }

        return (
          <g key={seriesData.key}>
            <path
              className={cx(WORK_CHART_LINE_CLASS, seriesData.lineClassName)}
              d={seriesData.path}
              data-chart-series={seriesData.key}
              data-chart-series-color={seriesData.lineColor}
              style={{ stroke: seriesData.lineColor }}
            />
          </g>
        );
      })}
    </svg>
  );
}

interface WorkChartStatusPanelProps {
  ariaBusy?: boolean;
  message: string;
  role: "alert" | "status";
  title: string;
}

function WorkChartStatusPanel({
  ariaBusy = false,
  message,
  role,
  title,
}: WorkChartStatusPanelProps) {
  return (
    <div
      aria-busy={ariaBusy || undefined}
      aria-live={role === "alert" ? "assertive" : "polite"}
      className={cx(EMPTY_STATE_CLASS, EMPTY_STATE_COMPACT_CLASS)}
      role={role}
    >
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
  hasData: boolean;
  path: string;
  points: WorkChartBuiltPoint[];
}

interface WorkChartBuiltPoint {
  label: string;
  value: number;
  x: number;
  y: number;
}

interface WorkChartData {
  maxValue: number;
  series: WorkChartBuiltSeries[];
  xTicks: WorkChartXAxisTick[];
  yTicks: WorkChartYAxisTick[];
}

interface WorkChartXAxisTick {
  label: string;
  x: number;
}

interface WorkChartYAxisTick {
  label: string;
  y: number;
}

type WorkChartDataResult =
  | { data: WorkChartData; status: "ready" }
  | { status: "empty" }
  | { status: "invalid" };

function buildWorkChartData(
  model: WorkChartModel | undefined,
  series: readonly WorkChartSeriesDefinition[],
  width: number,
  height: number,
  padding: typeof WORK_CHART_PADDING,
): WorkChartDataResult {
  if (!isWorkChartModel(model) || !isWorkChartSeriesDefinitionArray(series)) {
    return { status: "invalid" };
  }

  if (model.points.length === 0 || series.length === 0) {
    return { status: "empty" };
  }

  const tickValues = model.points.map((point) => point.tick);
  const minTick = Math.min(...tickValues);
  const maxTick = Math.max(...tickValues);
  const xScale = scaleLinear()
    .domain([minTick, maxTick === minTick ? minTick + 1 : maxTick])
    .range([padding.left, width - padding.right]);

  const seriesByKey = new Map<
    string,
    { label: string; observedAt: number; order: number; value: number }[]
  >(
    model.series.map((definition) => [definition.key, definition.points]),
  );

  const plottedPoints = model.points.map((point, index) => ({
    label: point.label,
    x: model.points.length === 1 ? width / 2 : xScale(point.tick),
  }));
  const builtSeries = series.map((definition) => {
    const seriesPoints = seriesByKey.get(definition.key) ?? [];
    const hasData = seriesPoints.length > 0;
    const values = plottedPoints.map((point, index) => {
      const value = seriesPoints.find((seriesPoint) => seriesPoint.order === index)?.value ?? 0;
      return { ...point, value };
    });

    return {
      ...definition,
      hasData,
      points: values,
    };
  });

  const maxValue = Math.max(
    1,
    ...builtSeries.flatMap((lineSeries) => lineSeries.points.map((point) => point.value)),
  );
  const yScale = scaleLinear()
    .domain([0, maxValue])
    .nice(4)
    .range([height - padding.bottom, padding.top]);
  const niceMaxValue = yScale.domain()[1] ?? maxValue;

  const scaledSeries = builtSeries.map((seriesEntry) => {
    const values = seriesEntry.points.map((point) => ({ ...point, y: yScale(point.value) }));
    const path = line<(typeof values)[number]>()
      .x((point) => point.x)
      .y((point) => point.y)(values) ?? "";

    return {
      ...seriesEntry,
      points: values,
      path,
    };
  });

  const hasRenderableSeries = scaledSeries.some(
    (entry) => entry.hasData && entry.path !== "",
  );
  if (!hasRenderableSeries) {
    return { status: "empty" };
  }

  return {
    data: {
      maxValue: niceMaxValue,
      series: scaledSeries,
      xTicks: buildXAxisTicks(model.points, xScale),
      yTicks: yScale.ticks(4).map((value) => ({
        label: formatAxisNumber(value),
        y: yScale(value),
      })),
    },
    status: "ready",
  };
}

function buildXAxisTicks(
  points: WorkChartModel["points"],
  xScale: ReturnType<typeof scaleLinear>,
): WorkChartXAxisTick[] {
  const lastIndex = points.length - 1;
  const selectedIndexes =
    points.length <= 5
      ? points.map((_, index) => index)
      : [0, Math.round(lastIndex * 0.25), Math.round(lastIndex * 0.5), Math.round(lastIndex * 0.75), lastIndex];
  const uniqueIndexes = [...new Set(selectedIndexes)];

  return uniqueIndexes.map((index) => ({
    label: formatAxisNumber(points[index]?.tick ?? index + 1),
    x: points.length === 1 ? WORK_CHART_WIDTH / 2 : Number(xScale(points[index]?.tick ?? index)),
  }));
}

function formatAxisNumber(value: number): string {
  if (Number.isInteger(value)) {
    return String(value);
  }
  return value.toFixed(1);
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
