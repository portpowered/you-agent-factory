import { useMemo } from "react";
import {
  CartesianGrid,
  Label,
  Line,
  LineChart,
  ReferenceArea,
  XAxis,
  YAxis,
} from "recharts";
import {
  EMPTY_STATE_CLASS,
  EMPTY_STATE_COMPACT_CLASS,
} from "../../../components/ui/widget-frame";
import { Button } from "../../../components/ui/button";
import {
  ChartContainer,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
} from "../../../components/ui/chart";
import type { LegendPayload } from "recharts/types/component/DefaultLegendContent";
import { Skeleton } from "../../../components/ui/skeleton";
import { cn } from "../../../lib/cn";
import { DASHBOARD_CHART_AXIS_LABEL_CLASS } from "../lib/chart-contract";
import type { WorkChartModel } from "../lib/trends";
import {
  buildWorkChartData,
  type WorkChartBuiltSeries,
  type WorkChartData,
  type WorkChartSeriesDefinition,
} from "../lib/work-chart-data";
import { getWorkOutcomeMessages } from "../messages/work-outcome";
import {
  useReadyWorkChartInteractions,
  type WorkChartZoomRange,
} from "./work-chart-interactions";

export type { WorkChartSeriesDefinition } from "../lib/work-chart-data";

export const WORK_CHART_AXIS_LABEL_CLASS = DASHBOARD_CHART_AXIS_LABEL_CLASS;
export const WORK_CHART_MARGIN = { bottom: 24, left: 18, right: 28, top: 28 };
// tailwind-exception: intrinsic-sizing
const WORK_CHART_READY_CLASS =
  "flex h-full min-h-[14rem] min-w-0 w-full flex-col px-5 pb-5 pt-4 sm:px-6 sm:pb-6 sm:pt-5";
const WORK_CHART_LEGEND_ROW_CLASS = "shrink-0 pb-1 pt-0 sm:pb-1.5";
// tailwind-exception: intrinsic-sizing
const WORK_CHART_STATUS_PANEL_CLASS =
  "flex h-full min-h-[14rem] min-w-0 w-full flex-1 flex-col justify-center";
const WORK_CHART_SHELL_CLASS =
  "flex h-full min-h-0 min-w-0 w-full flex-1 flex-col gap-3";
const WORK_CHART_TOOLBAR_CLASS =
  "flex flex-wrap items-center justify-end gap-2";
const WORK_CHART_OVERLAY_CLASS =
  "flex h-full flex-col gap-2 px-5 pb-4 pt-4 sm:px-6 sm:pb-5 sm:pt-5";
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
  locale?: string;
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
  emptyMessage,
  emptyTitle,
  locale,
  model,
  series,
  state = READY_WORK_CHART_STATE,
  xAxisLabel,
  yAxisLabel,
}: WorkChartProps) {
  const messages = getWorkOutcomeMessages(locale).chart;
  const resolvedEmptyMessage = emptyMessage ?? messages.emptyMessage;
  const resolvedEmptyTitle = emptyTitle ?? messages.emptyTitle;
  const resolvedXAxisLabel = xAxisLabel ?? messages.xAxisLabel;
  const resolvedYAxisLabel = yAxisLabel ?? messages.yAxisLabel;
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
        message={state.message ?? messages.loadingMessage}
        role="status"
        title={state.title ?? messages.loadingTitle}
      />
    );
  }

  if (state.status === "error" || chartData.status === "invalid") {
    return (
      <WorkChartStatusPanel
        message={
          state.status === "error"
            ? (state.message ?? messages.errorMessage)
            : messages.errorMessage
        }
        role="alert"
        title={
          state.status === "error"
            ? (state.title ?? messages.errorTitle)
            : messages.errorTitle
        }
      />
    );
  }

  if (chartData.status === "empty") {
    return (
      <WorkChartStatusPanel
        message={resolvedEmptyMessage}
        role="status"
        title={resolvedEmptyTitle}
      />
    );
  }

  if (chartData.status !== "ready") {
    return (
      <WorkChartStatusPanel
        message={messages.errorMessage}
        role="alert"
        title={messages.errorTitle}
      />
    );
  }

  return (
    <ReadyWorkChart
      ariaLabel={ariaLabel}
      chartData={chartData.data}
      className={className}
      locale={locale}
      xAxisLabel={resolvedXAxisLabel}
      yAxisLabel={resolvedYAxisLabel}
    />
  );
}

interface ReadyWorkChartProps {
  ariaLabel: string;
  chartData: WorkChartData;
  className: string;
  locale?: string;
  xAxisLabel: string;
  yAxisLabel: string;
}

function ReadyWorkChart({
  ariaLabel,
  chartData,
  className,
  locale,
  xAxisLabel,
  yAxisLabel,
}: ReadyWorkChartProps) {
  const chartMessages = getWorkOutcomeMessages(locale).chart;
  const {
    beginSelection,
    commitSelection,
    hiddenSeriesKeys,
    resetZoom,
    selectionRange,
    toggleSeries,
    updateSelection,
    visibleRows,
    visibleSeriesKeys,
    zoomRange,
  } = useReadyWorkChartInteractions(chartData);

  return (
    <div className={cn(WORK_CHART_SHELL_CLASS, className)}>
      {zoomRange === null ? null : (
        <div
          className={WORK_CHART_TOOLBAR_CLASS}
          data-work-chart-toolbar="true"
        >
          <Button
            aria-label={chartMessages.resetZoomLabel}
            className="min-h-8 rounded-lg px-2.5 py-1.5 text-xs"
            onClick={resetZoom}
            size="sm"
            tone="outline"
          >
            {chartMessages.resetZoomAction}
          </Button>
        </div>
      )}
      <ChartContainer
        className={WORK_CHART_READY_CLASS}
        config={chartData.config}
        footer={
          <WorkChartLegendRow
            chartMessages={chartMessages}
            hiddenSeriesKeys={hiddenSeriesKeys}
            series={chartData.series}
            toggleSeries={toggleSeries}
          />
        }
        interactionAttributes={{
          onMouseDown: beginSelection,
          onMouseMove: updateSelection,
          onMouseUp: commitSelection,
        }}
        rootAttributes={{
          "data-work-chart-legend-placement": "shell-row",
          "data-work-chart-plot-margin-bottom": String(WORK_CHART_MARGIN.bottom),
          "data-work-chart-ready": "true",
          "data-work-chart-hidden-series": [...hiddenSeriesKeys].join(","),
          "data-work-chart-visible-series": visibleSeriesKeys.join(","),
          "data-work-chart-visible-ticks": visibleRows
            .map((row) => row.tick)
            .join(","),
        }}
        overlay={<WorkChartAxisOverlay yAxisLabel={yAxisLabel} />}
        style={{ minHeight: "14rem" }}
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
            tickFormatter={(value) => formatAxisNumber(value)}
            tickLine={false}
          >
            <Label value="insideBottom" offset={-10} position="insideBottom">
              {xAxisLabel}
            </Label>
          </XAxis>
          <YAxis
            allowDecimals={false}
            axisLine={false}
            tick={{ className: WORK_CHART_AXIS_LABEL_CLASS }}
            tickCount={5}
            tickFormatter={(value) => formatAxisNumber(value)}
            tickLine={false}
            width={WORK_CHART_Y_AXIS_WIDTH}
          >
            <Label
              angle={-90}
              value={yAxisLabel}
              position="insideLeft"
              style={{ textAnchor: "middle" }}
            />
          </YAxis>

          <CartesianGrid strokeDasharray="3 3" />
          <ChartTooltip
            content={(props) => {
              const tickValue = props.payload?.[0]?.payload?.tick;
              const label =
                typeof tickValue === "number"
                  ? chartMessages.tickLabel(tickValue)
                  : (props.payload?.[0]?.payload?.label ?? props.label);
              return <ChartTooltipContent {...props} label={label} />;
            }}
            cursor={{ stroke: "var(--color-af-chart-cursor)" }}
          />
          <WorkChartSelectionArea selectionRange={selectionRange} />
          <WorkChartLines
            hiddenSeriesKeys={hiddenSeriesKeys}
            series={chartData.series}
          />
        </LineChart>
      </ChartContainer>
    </div>
  );
}

function buildWorkChartLegendPayload(
  series: readonly WorkChartBuiltSeries[],
): LegendPayload[] {
  return series.map((seriesData) => ({
    color: seriesData.lineColor,
    dataKey: seriesData.key,
    type: "line",
    value: seriesData.label,
  }));
}

interface WorkChartLegendRowProps {
  chartMessages: ReturnType<typeof getWorkOutcomeMessages>["chart"];
  hiddenSeriesKeys: ReadonlySet<string>;
  series: readonly WorkChartBuiltSeries[];
  toggleSeries: (key: string) => void;
}

function WorkChartLegendRow({
  chartMessages,
  hiddenSeriesKeys,
  series,
  toggleSeries,
}: WorkChartLegendRowProps) {
  return (
    <div
      className={WORK_CHART_LEGEND_ROW_CLASS}
      data-work-chart-legend="true"
    >
      <ChartLegendContent
        getToggleLabel={(label, hidden) =>
          hidden
            ? chartMessages.showSeriesLabel(label)
            : chartMessages.hideSeriesLabel(label)
        }
        hiddenSeries={hiddenSeriesKeys}
        onToggleSeries={toggleSeries}
        payload={buildWorkChartLegendPayload(series)}
      />
    </div>
  );
}

function WorkChartAxisOverlay({ yAxisLabel }: { yAxisLabel: string }) {
  return (
    <div className={WORK_CHART_OVERLAY_CLASS} data-work-chart-overlay="true">
      <span className={WORK_CHART_AXIS_LABEL_CLASS}>{yAxisLabel}</span>
    </div>
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
      fill="var(--color-af-chart-selection-fill)"
      stroke="var(--color-af-chart-selection-stroke)"
      x1={selectionRange.startTick}
      x2={selectionRange.endTick}
    />
  );
}

function WorkChartLines({
  hiddenSeriesKeys,
  series,
}: {
  hiddenSeriesKeys: ReadonlySet<string>;
  series: readonly WorkChartBuiltSeries[];
}) {
  return series.map((seriesData) => (
    <Line
      key={seriesData.key}
      activeDot={{
        className: seriesData.pointClassName,
        fill: seriesData.lineColor,
        r: seriesData.pointRadius,
        stroke: "var(--color-af-chart-active-dot-stroke)",
        strokeWidth: 1.5,
      }}
      className={seriesData.lineClassName}
      data-chart-series={seriesData.key}
      data-chart-series-color={seriesData.lineColor}
      data-chart-series-hidden={
        hiddenSeriesKeys.has(seriesData.key) ? "true" : "false"
      }
      dataKey={seriesData.key}
      dot={false}
      hide={hiddenSeriesKeys.has(seriesData.key)}
      isAnimationActive={false}
      name={seriesData.label}
      stroke={seriesData.lineColor}
      strokeDasharray={seriesData.strokeDasharray}
      strokeWidth={2.25}
      type="linear"
    />
  ));
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
      className={cn(
        EMPTY_STATE_CLASS,
        EMPTY_STATE_COMPACT_CLASS,
        WORK_CHART_STATUS_PANEL_CLASS,
      )}
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

function formatAxisNumber(value: number): string {
  if (Number.isInteger(value)) {
    return String(value);
  }
  return value.toFixed(1);
}
