import type { MouseEvent as ReactMouseEvent } from "react";
import { useMemo, useState } from "react";
import {
  CartesianGrid,
  Line,
  LineChart,
  ReferenceArea,
  XAxis,
  YAxis,
} from "recharts";
import {
  EMPTY_STATE_CLASS,
  EMPTY_STATE_COMPACT_CLASS,
} from "../../components/dashboard/widget-board";
import { Button } from "../../components/ui/button";
import {
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
} from "../../components/ui/chart";
import { Skeleton } from "../../components/ui/skeleton";
import { cn } from "../../lib/cn";
import { DASHBOARD_CHART_AXIS_LABEL_CLASS } from "./chart-contract";
import type { WorkChartModel } from "./trends";
import {
  buildWorkChartData,
  type WorkChartBuiltSeries,
  type WorkChartData,
  type WorkChartRow,
  type WorkChartSeriesDefinition,
} from "./work-chart-data";

export type { WorkChartSeriesDefinition } from "./work-chart-data";

export const WORK_CHART_AXIS_LABEL_CLASS = DASHBOARD_CHART_AXIS_LABEL_CLASS;
export const WORK_CHART_EMPTY_TITLE = "No work outcome samples";
export const WORK_CHART_EMPTY_MESSAGE =
  "Work outcome data appears after the event stream receives work history.";
export const WORK_CHART_LOADING_TITLE = "Loading work outcome samples";
export const WORK_CHART_LOADING_MESSAGE =
  "Waiting for dashboard timeline data.";
export const WORK_CHART_ERROR_TITLE = "Work outcome chart unavailable";
export const WORK_CHART_ERROR_MESSAGE =
  "Chart data is incomplete, so the dashboard cannot draw this work outcome view yet.";
const WORK_CHART_MARGIN = { bottom: 40, left: 18, right: 28, top: 28 };
// tailwind-exception: intrinsic-sizing
const WORK_CHART_READY_CLASS =
  "h-64 min-h-56 px-5 pb-5 pt-4 sm:h-72 sm:px-6 sm:pb-6 sm:pt-5";
const WORK_CHART_OVERLAY_CLASS =
  "grid h-full grid-rows-[auto_1fr_auto] gap-2 px-5 pb-4 pt-4 sm:px-6 sm:pb-5 sm:pt-5";
const WORK_CHART_TOP_OVERLAY_CLASS = "flex items-start justify-between gap-3";
const WORK_CHART_ZOOM_CONTEXT_CLASS =
  "pointer-events-auto flex max-w-[70%] flex-wrap items-center justify-end gap-2 text-right sm:max-w-none";
const WORK_CHART_X_AXIS_OVERLAY_CLASS = "justify-self-end";
const WORK_CHART_Y_AXIS_WIDTH = 52;

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

  return (
    <ReadyWorkChart
      ariaLabel={ariaLabel}
      chartData={chartData.data}
      className={className}
      xAxisLabel={xAxisLabel}
      yAxisLabel={yAxisLabel}
    />
  );
}

interface ReadyWorkChartProps {
  ariaLabel: string;
  chartData: WorkChartData;
  className: string;
  xAxisLabel: string;
  yAxisLabel: string;
}

function ReadyWorkChart({
  ariaLabel,
  chartData,
  className,
  xAxisLabel,
  yAxisLabel,
}: ReadyWorkChartProps) {
  const [zoomRange, setZoomRange] = useState<WorkChartZoomRange | null>(null);
  const [selectionStartTick, setSelectionStartTick] = useState<number | null>(
    null,
  );
  const [selectionEndTick, setSelectionEndTick] = useState<number | null>(null);

  const visibleRows = useMemo(
    () => filterRowsForZoomRange(chartData.rows, zoomRange),
    [chartData.rows, zoomRange],
  );
  const selectionRange = buildSelectionRange(
    selectionStartTick,
    selectionEndTick,
  );
  const zoomContext =
    zoomRange === null
      ? null
      : `Zoomed to ticks ${formatAxisNumber(zoomRange.startTick)}-${formatAxisNumber(zoomRange.endTick)}`;

  const beginSelection = (event: ReactMouseEvent<HTMLDivElement>) => {
    const tick = readPointerTick(event, visibleRows);
    setSelectionStartTick(tick);
    setSelectionEndTick(tick);
  };

  const updateSelection = (event: ReactMouseEvent<HTMLDivElement>) => {
    if (selectionStartTick === null) {
      return;
    }

    const tick = readPointerTick(event, visibleRows);
    if (tick !== null) {
      setSelectionEndTick(tick);
    }
  };

  const commitSelection = (event: ReactMouseEvent<HTMLDivElement>) => {
    const endTick = readPointerTick(event, visibleRows) ?? selectionEndTick;
    const nextRange = buildSelectionRange(selectionStartTick, endTick);
    setSelectionStartTick(null);
    setSelectionEndTick(null);

    if (nextRange === null || nextRange.startTick === nextRange.endTick) {
      return;
    }

    setZoomRange(nextRange);
  };

  return (
    <ChartContainer
      className={cn(WORK_CHART_READY_CLASS, className)}
      config={chartData.config}
      interactionAttributes={{
        onMouseDown: beginSelection,
        onMouseMove: updateSelection,
        onMouseUp: commitSelection,
      }}
      overlay={
        <div
          className={WORK_CHART_OVERLAY_CLASS}
          data-work-chart-overlay="true"
        >
          <div className={WORK_CHART_TOP_OVERLAY_CLASS}>
            <p className={cn("m-0", WORK_CHART_AXIS_LABEL_CLASS)}>
              {yAxisLabel}
            </p>
            {zoomContext === null ? null : (
              <div className={WORK_CHART_ZOOM_CONTEXT_CLASS}>
                <p className={cn("m-0", WORK_CHART_AXIS_LABEL_CLASS)}>
                  {zoomContext}
                </p>
                <Button
                  aria-label="Reset work outcome chart zoom"
                  className="min-h-8 rounded-lg px-2.5 py-1.5 text-xs"
                  onClick={() => setZoomRange(null)}
                  size="sm"
                  tone="outline"
                >
                  Reset zoom
                </Button>
              </div>
            )}
          </div>
          <p
            className={cn(
              "m-0",
              WORK_CHART_AXIS_LABEL_CLASS,
              WORK_CHART_X_AXIS_OVERLAY_CLASS,
            )}
          >
            {xAxisLabel}
          </p>
        </div>
      }
      rootAttributes={{
        "data-work-chart-ready": "true",
        "data-work-chart-visible-ticks": visibleRows
          .map((row) => row.tick)
          .join(","),
      }}
      style={{ height: "16rem", minHeight: "14rem" }}
      title={ariaLabel}
    >
      <LineChart
        accessibilityLayer
        data={visibleRows}
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
        <WorkChartSelectionArea selectionRange={selectionRange} />
        <WorkChartLines series={chartData.series} />
      </LineChart>
    </ChartContainer>
  );
}

function WorkChartSelectionArea({
  selectionRange,
}: {
  selectionRange: WorkChartZoomRange | null;
}) {
  if (
    selectionRange === null ||
    selectionRange.startTick === selectionRange.endTick
  ) {
    return null;
  }

  return (
    <ReferenceArea
      fill="rgb(from var(--color-af-accent) r g b / 0.12)"
      stroke="rgb(from var(--color-af-accent) r g b / 0.42)"
      x1={selectionRange.startTick}
      x2={selectionRange.endTick}
    />
  );
}

function WorkChartLines({
  series,
}: {
  series: readonly WorkChartBuiltSeries[];
}) {
  return series.map((seriesData) => (
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
  ));
}

interface WorkChartZoomRange {
  endTick: number;
  startTick: number;
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

function filterRowsForZoomRange(
  rows: readonly WorkChartRow[],
  zoomRange: WorkChartZoomRange | null,
): WorkChartRow[] {
  if (zoomRange === null) {
    return [...rows];
  }

  const filteredRows = rows.filter(
    (row) => row.tick >= zoomRange.startTick && row.tick <= zoomRange.endTick,
  );
  return filteredRows.length >= 2 ? filteredRows : [...rows];
}

function buildSelectionRange(
  startTick: number | null,
  endTick: number | null,
): WorkChartZoomRange | null {
  if (startTick === null || endTick === null) {
    return null;
  }

  return {
    endTick: Math.max(startTick, endTick),
    startTick: Math.min(startTick, endTick),
  };
}

function readPointerTick(
  event: ReactMouseEvent<HTMLDivElement>,
  rows: readonly WorkChartRow[],
): number | null {
  if (rows.length === 0) {
    return null;
  }

  const rect = event.currentTarget.getBoundingClientRect();
  if (!Number.isFinite(rect.width) || rect.width <= 0) {
    return null;
  }

  const relativeX = Math.min(
    Math.max(event.clientX - rect.left, 0),
    rect.width,
  );
  const rowIndex = Math.round((relativeX / rect.width) * (rows.length - 1));
  return rows[rowIndex]?.tick ?? null;
}

function formatAxisNumber(value: number): string {
  if (Number.isInteger(value)) {
    return String(value);
  }
  return value.toFixed(1);
}
