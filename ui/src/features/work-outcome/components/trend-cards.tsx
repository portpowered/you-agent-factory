import { EnumSelect } from "@you-agent-factory/components/forms";
import {
  WidgetDetailCopy,
  WidgetEmptyState,
  WidgetEmptyStateText,
  WidgetEmptyStateTitle,
  WidgetSubtitle,
} from "@you-agent-factory/components/recipes";
import { Label, SurfacePanel, Text } from "../../../components/ui";
import {
  formatDurationMillis,
  formatTraceOutcome,
} from "../../../components/ui/formatters";
import { cn } from "../../../lib/cn";
import { DashboardWidgetFrame } from "../../bento/public";
import {
  dashboardChartAxisClassName,
  dashboardChartSurfaceClassName,
  getDashboardChartSemanticStyle,
} from "../lib/chart-contract";
import {
  type FailureTrendModel,
  type ReworkTrendModel,
  THROUGHPUT_RANGE_OPTIONS,
  type ThroughputRangeID,
  type TimingTrendModel,
} from "../lib/trends";
import { getWorkOutcomeMessages } from "../messages/work-outcome";
import { TrendSummaryGrid, TrendSummaryMetric } from "./trend-summary";

interface FailureTrendCardProps {
  className?: string;
  locale?: string;
  model: FailureTrendModel;
  onRangeChange: (rangeID: ThroughputRangeID) => void;
  rangeID: ThroughputRangeID;
  widgetId?: string;
}

interface ReworkTrendCardProps {
  className?: string;
  locale?: string;
  model: ReworkTrendModel;
  widgetId?: string;
}

interface TimingTrendCardProps {
  className?: string;
  locale?: string;
  model: TimingTrendModel;
  widgetId?: string;
}

const TREND_CHART_CLASS = cn(
  dashboardChartSurfaceClassName(),
  "min-h-44 border border-outline",
);
const FAILURE_TREND_CHART_STYLE =
  getDashboardChartSemanticStyle("failureTrend");
const REWORK_TREND_CHART_STYLE = getDashboardChartSemanticStyle("reworkTrend");
const TIMING_TREND_CHART_STYLE = getDashboardChartSemanticStyle("timingTrend");

export function FailureTrendCard({
  className = "",
  locale,
  model,
  onRangeChange,
  rangeID,
  widgetId = "failure-trend",
}: FailureTrendCardProps) {
  const messages = getWorkOutcomeMessages(locale).trends;
  const rangeSelectId = `${widgetId}-failure-range`;
  const changeRange = (value: string) => {
    if (isThroughputRangeID(value)) {
      onRangeChange(value);
    }
  };

  return (
    <DashboardWidgetFrame
      className={className}
      title={messages.failureTitle}
      wide
      widgetId={widgetId}
    >
      <div className="mb-4 flex flex-col items-start justify-between gap-3 md:flex-row">
        <WidgetSubtitle>{messages.failureSummary}</WidgetSubtitle>
        <div className="grid w-full gap-1 md:w-auto md:shrink-0 md:basis-36">
          <Label as="label" htmlFor={rangeSelectId}>
            {messages.rangeLabel}
          </Label>
          <EnumSelect
            aria-label={messages.rangeLabel}
            className="rounded-lg border-primary py-2"
            id={rangeSelectId}
            onValueChange={changeRange}
            options={THROUGHPUT_RANGE_OPTIONS.map((option) => ({
              label: messages.rangeOptionLabel(option.id, option.id),
              value: option.id,
            }))}
            value={rangeID}
          />
        </div>
      </div>

      <TrendSummaryGrid>
        <TrendSummaryMetric
          label={messages.failedInRangeLabel}
          value={model.failureDelta}
        />
        <TrendSummaryMetric
          label={messages.totalFailedLabel}
          value={model.currentFailed}
        />
        <TrendSummaryMetric
          label={messages.causeGroupsLabel}
          value={model.groups.length}
        />
      </TrendSummaryGrid>

      {model.points.length > 0 ? (
        <svg
          className={TREND_CHART_CLASS}
          role="img"
          aria-label={messages.failureChartAriaLabel(model.rangeLabel)}
          viewBox="0 0 320 120"
        >
          <TrendAxes />
          {model.path ? (
            <path
              className={FAILURE_TREND_CHART_STYLE.lineClassName}
              d={model.path}
              stroke={FAILURE_TREND_CHART_STYLE.color}
            />
          ) : null}
          {model.points.map((point) => (
            <circle
              key={`${point.label}-${point.x}-${point.y}`}
              className={FAILURE_TREND_CHART_STYLE.pointClassName}
              cx={point.x}
              cy={point.y}
              r={FAILURE_TREND_CHART_STYLE.pointRadius}
            >
              <title>{point.label}</title>
            </circle>
          ))}
        </svg>
      ) : (
        <WidgetEmptyState compact>
          <WidgetEmptyStateTitle>
            {messages.failureEmptyTitle}
          </WidgetEmptyStateTitle>
          <WidgetEmptyStateText>
            {messages.failureEmptyMessage}
          </WidgetEmptyStateText>
        </WidgetEmptyState>
      )}

      {model.groups.length > 0 ? (
        <ul
          className="mt-4 grid list-none gap-2 p-0"
          aria-label={messages.causeGroupsRegionLabel}
        >
          {model.groups.map((group) => (
            <SurfacePanel
              asChild
              className="flex items-center justify-between gap-3 px-3 py-2.5"
              key={group.label}
              padding="none"
              radius="lg"
              surface="low"
            >
              <li>
                <Text
                  as="span"
                  className="min-w-0 text-on-surface-variant [overflow-wrap:anywhere]"
                >
                  {group.label}
                </Text>
                <strong className="shrink-0 text-on-error-container">
                  {group.count}
                </strong>
              </li>
            </SurfacePanel>
          ))}
        </ul>
      ) : (
        <WidgetDetailCopy>{messages.causeGroupsEmpty}</WidgetDetailCopy>
      )}
    </DashboardWidgetFrame>
  );
}

export function ReworkTrendCard({
  className = "",
  locale,
  model,
  widgetId = "rework-trend",
}: ReworkTrendCardProps) {
  const messages = getWorkOutcomeMessages(locale).trends;

  return (
    <DashboardWidgetFrame
      className={className}
      title={messages.reworkTitle}
      wide
      widgetId={widgetId}
    >
      <WidgetSubtitle>{messages.reworkSummary}</WidgetSubtitle>

      <TrendSummaryGrid>
        <TrendSummaryMetric
          label={messages.traceWorkLabel}
          value={model.currentWorkLabel}
        />
        <TrendSummaryMetric
          label={messages.retryOrReworkLabel}
          value={model.retryOrReworkCount}
        />
        <TrendSummaryMetric
          label={messages.latestOutcomeLabel}
          value={formatTraceOutcome(model.terminalOutcome)}
        />
      </TrendSummaryGrid>

      {model.points.length > 0 ? (
        <svg
          className={TREND_CHART_CLASS}
          role="img"
          aria-label={messages.reworkChartAriaLabel(model.currentWorkLabel)}
          viewBox="0 0 320 120"
        >
          <TrendAxes />
          {model.path ? (
            <path
              className={REWORK_TREND_CHART_STYLE.lineClassName}
              d={model.path}
              stroke={REWORK_TREND_CHART_STYLE.color}
            />
          ) : null}
          {model.points.map((point) => (
            <circle
              key={`${point.dispatchLabel}-${point.x}-${point.y}`}
              className={REWORK_TREND_CHART_STYLE.pointClassName}
              cx={point.x}
              cy={point.y}
              r={REWORK_TREND_CHART_STYLE.pointRadius}
            >
              <title>
                {messages.reworkPointLabel(
                  point.dispatchLabel,
                  point.reworkCount,
                )}
              </title>
            </circle>
          ))}
        </svg>
      ) : (
        <WidgetEmptyState compact>
          <WidgetEmptyStateTitle>
            {messages.reworkEmptyTitle}
          </WidgetEmptyStateTitle>
          <WidgetEmptyStateText>
            {messages.reworkEmptyMessage}
          </WidgetEmptyStateText>
        </WidgetEmptyState>
      )}
    </DashboardWidgetFrame>
  );
}

export function TimingTrendCard({
  className = "",
  locale,
  model,
  widgetId = "timing-trend",
}: TimingTrendCardProps) {
  const messages = getWorkOutcomeMessages(locale).trends;

  return (
    <DashboardWidgetFrame
      className={className}
      title={messages.timingTitle}
      wide
      widgetId={widgetId}
    >
      <WidgetSubtitle>{messages.timingSummary}</WidgetSubtitle>

      <TrendSummaryGrid>
        <TrendSummaryMetric
          label={messages.traceWorkLabel}
          value={model.currentWorkLabel}
        />
        <TrendSummaryMetric
          label={messages.slowestDurationLabel}
          value={formatDurationMillis(model.slowestDurationMillis)}
        />
        <TrendSummaryMetric
          label={messages.averageDurationLabel}
          value={formatDurationMillis(model.averageDurationMillis)}
        />
      </TrendSummaryGrid>

      {model.points.length > 0 ? (
        <>
          <svg
            className={TREND_CHART_CLASS}
            role="img"
            aria-label={messages.timingChartAriaLabel(model.currentWorkLabel)}
            viewBox="0 0 320 120"
          >
            <TrendAxes />
            {model.path ? (
              <path
                className={TIMING_TREND_CHART_STYLE.lineClassName}
                d={model.path}
                stroke={TIMING_TREND_CHART_STYLE.color}
              />
            ) : null}
            {model.points.map((point) => (
              <circle
                key={`${point.dispatchLabel}-${point.x}-${point.y}`}
                className={TIMING_TREND_CHART_STYLE.pointClassName}
                cx={point.x}
                cy={point.y}
                r={TIMING_TREND_CHART_STYLE.pointRadius}
              >
                <title>
                  {point.dispatchLabel}:{" "}
                  {formatDurationMillis(point.durationMillis)}
                </title>
              </circle>
            ))}
          </svg>
          <TrendSummaryGrid
            aria-label={messages.timingRangeLabel}
            className="mt-3 md:grid-cols-2"
          >
            <TrendSummaryMetric
              label={messages.fastestDurationLabel}
              value={formatDurationMillis(model.fastestDurationMillis)}
            />
            <TrendSummaryMetric
              label={messages.latestDurationLabel}
              value={formatDurationMillis(model.latestDurationMillis)}
            />
          </TrendSummaryGrid>
        </>
      ) : (
        <WidgetEmptyState compact>
          <WidgetEmptyStateTitle>
            {messages.reworkEmptyTitle}
          </WidgetEmptyStateTitle>
          <WidgetEmptyStateText>
            {messages.timingEmptyMessage}
          </WidgetEmptyStateText>
        </WidgetEmptyState>
      )}
    </DashboardWidgetFrame>
  );
}

function TrendAxes() {
  return (
    <>
      <line
        className={dashboardChartAxisClassName()}
        x1="14"
        x2="306"
        y1="106"
        y2="106"
      />
      <line
        className={dashboardChartAxisClassName()}
        x1="14"
        x2="14"
        y1="14"
        y2="106"
      />
    </>
  );
}

function isThroughputRangeID(value: string): value is ThroughputRangeID {
  return THROUGHPUT_RANGE_OPTIONS.some((option) => option.id === value);
}
