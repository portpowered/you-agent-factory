import {
  DASHBOARD_CHART_AXIS_CLASS,
  DASHBOARD_CHART_SURFACE_CLASS,
  getDashboardChartSemanticStyle,
} from "../lib/chart-contract";
import { cn } from "../../../lib/cn";
import {
  formatDurationMillis,
  formatTraceOutcome,
} from "../../../components/ui/formatters";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
} from "../../../components/ui/dashboard-typography";
import {
  THROUGHPUT_RANGE_OPTIONS,
  type FailureTrendModel,
  type ReworkTrendModel,
  type ThroughputRangeID,
  type TimingTrendModel,
} from "../lib/trends";
import { getWorkOutcomeMessages } from "../messages/work-outcome";
import {
  DETAIL_CARD_WIDE_CLASS,
  DETAIL_COPY_CLASS,
  EMPTY_STATE_CLASS,
  EMPTY_STATE_COMPACT_CLASS,
  WIDGET_SUBTITLE_CLASS,
} from "../../../components/dashboard/widget-board";
import { DashboardWidgetFrame } from "../../../components/ui/widget-frame";

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

const TREND_TOOLBAR_CLASS =
  "mb-4 flex flex-col items-start justify-between gap-3 md:flex-row";
const TREND_RANGE_LABEL_CLASS =
  "grid w-full gap-1 md:w-auto md:shrink-0 md:basis-36";
const TREND_RANGE_TEXT_CLASS = DASHBOARD_SUPPORTING_LABEL_CLASS;
const TREND_RANGE_SELECT_CLASS = cn(
  "rounded-lg border border-af-accent/35 bg-af-canvas/82 px-2 py-2 text-af-ink",
  DASHBOARD_BODY_TEXT_CLASS,
);
const TREND_SUMMARY_CLASS =
  cn(
    "mb-4 grid grid-cols-1 gap-3 [&_dd]:m-0 [&_div]:rounded-lg [&_div]:border [&_div]:border-af-overlay/8 [&_div]:bg-af-overlay/4 [&_div]:p-3 [&_dt]:mb-1 md:grid-cols-3",
    DASHBOARD_SUPPORTING_LABELS_CLASS,
  );
const TREND_CHART_CLASS = cn(DASHBOARD_CHART_SURFACE_CLASS, "min-h-44 border border-af-overlay/8");
const TREND_CAUSE_LIST_CLASS = "mt-4 grid list-none gap-2 p-0";
const TREND_CAUSE_ITEM_CLASS =
  "flex items-center justify-between gap-3 rounded-lg border border-af-overlay/7 bg-af-overlay/4 px-3 py-2.5";
const TREND_CAUSE_LABEL_CLASS = cn(
  "min-w-0 text-af-ink/78 [overflow-wrap:anywhere]",
  DASHBOARD_BODY_TEXT_CLASS,
);
const TIMING_RANGE_SUMMARY_CLASS = cn(TREND_SUMMARY_CLASS, "mt-3 md:grid-cols-2");
const TREND_SUMMARY_TERM_CLASS = DASHBOARD_SUPPORTING_LABEL_CLASS;
const TREND_SUMMARY_VALUE_CLASS = WIDGET_SUBTITLE_CLASS;
const FAILURE_TREND_CHART_STYLE = getDashboardChartSemanticStyle("failureTrend");
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
  const changeRange = (value: string) => {
    if (isThroughputRangeID(value)) {
      onRangeChange(value);
    }
  };

  return (
    <DashboardWidgetFrame
      className={cn(DETAIL_CARD_WIDE_CLASS, className)}
      title={messages.failureTitle}
      widgetId={widgetId}
    >
      <div className={TREND_TOOLBAR_CLASS}>
        <p className={WIDGET_SUBTITLE_CLASS}>
          {messages.failureSummary}
        </p>
        <label className={TREND_RANGE_LABEL_CLASS}>
          <span className={TREND_RANGE_TEXT_CLASS}>{messages.rangeLabel}</span>
          <select
            aria-label={messages.rangeLabel}
            className={TREND_RANGE_SELECT_CLASS}
            value={rangeID}
            onChange={(event) => changeRange(event.target.value)}
          >
            {THROUGHPUT_RANGE_OPTIONS.map((option) => (
              <option key={option.id} value={option.id}>
                {messages.rangeOptionLabel(option.id, option.label)}
              </option>
            ))}
          </select>
        </label>
      </div>

      <dl className={TREND_SUMMARY_CLASS}>
        <div>
          <dt className={TREND_SUMMARY_TERM_CLASS}>{messages.failedInRangeLabel}</dt>
          <dd className={TREND_SUMMARY_VALUE_CLASS}>{model.failureDelta}</dd>
        </div>
        <div>
          <dt className={TREND_SUMMARY_TERM_CLASS}>{messages.totalFailedLabel}</dt>
          <dd className={TREND_SUMMARY_VALUE_CLASS}>{model.currentFailed}</dd>
        </div>
        <div>
          <dt className={TREND_SUMMARY_TERM_CLASS}>{messages.causeGroupsLabel}</dt>
          <dd className={TREND_SUMMARY_VALUE_CLASS}>{model.groups.length}</dd>
        </div>
      </dl>

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
        <div className={cn(EMPTY_STATE_CLASS, EMPTY_STATE_COMPACT_CLASS)}>
          <h3>{messages.failureEmptyTitle}</h3>
          <p>{messages.failureEmptyMessage}</p>
        </div>
      )}

      {model.groups.length > 0 ? (
        <ul
          className={TREND_CAUSE_LIST_CLASS}
          aria-label={messages.causeGroupsRegionLabel}
        >
          {model.groups.map((group) => (
            <li className={TREND_CAUSE_ITEM_CLASS} key={group.label}>
              <span className={TREND_CAUSE_LABEL_CLASS}>{group.label}</span>
              <strong className="shrink-0 text-af-danger-bright">{group.count}</strong>
            </li>
          ))}
        </ul>
      ) : (
        <p className={DETAIL_COPY_CLASS}>{messages.causeGroupsEmpty}</p>
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
      className={cn(DETAIL_CARD_WIDE_CLASS, className)}
      title={messages.reworkTitle}
      widgetId={widgetId}
    >
      <p className={WIDGET_SUBTITLE_CLASS}>
        {messages.reworkSummary}
      </p>

      <dl className={TREND_SUMMARY_CLASS}>
        <div>
          <dt className={TREND_SUMMARY_TERM_CLASS}>{messages.traceWorkLabel}</dt>
          <dd className={TREND_SUMMARY_VALUE_CLASS}>{model.currentWorkLabel}</dd>
        </div>
        <div>
          <dt className={TREND_SUMMARY_TERM_CLASS}>{messages.retryOrReworkLabel}</dt>
          <dd className={TREND_SUMMARY_VALUE_CLASS}>{model.retryOrReworkCount}</dd>
        </div>
        <div>
          <dt className={TREND_SUMMARY_TERM_CLASS}>{messages.latestOutcomeLabel}</dt>
          <dd className={TREND_SUMMARY_VALUE_CLASS}>
            {formatTraceOutcome(model.terminalOutcome)}
          </dd>
        </div>
      </dl>

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
                {point.dispatchLabel}: {point.reworkCount} retry or rework events
              </title>
            </circle>
          ))}
        </svg>
      ) : (
        <div className={cn(EMPTY_STATE_CLASS, EMPTY_STATE_COMPACT_CLASS)}>
          <h3>{messages.reworkEmptyTitle}</h3>
          <p>{messages.reworkEmptyMessage}</p>
        </div>
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
      className={cn(DETAIL_CARD_WIDE_CLASS, className)}
      title={messages.timingTitle}
      widgetId={widgetId}
    >
      <p className={WIDGET_SUBTITLE_CLASS}>
        {messages.timingSummary}
      </p>

      <dl className={TREND_SUMMARY_CLASS}>
        <div>
          <dt className={TREND_SUMMARY_TERM_CLASS}>{messages.traceWorkLabel}</dt>
          <dd className={TREND_SUMMARY_VALUE_CLASS}>{model.currentWorkLabel}</dd>
        </div>
        <div>
          <dt className={TREND_SUMMARY_TERM_CLASS}>{messages.slowestDurationLabel}</dt>
          <dd className={TREND_SUMMARY_VALUE_CLASS}>
            {formatDurationMillis(model.slowestDurationMillis)}
          </dd>
        </div>
        <div>
          <dt className={TREND_SUMMARY_TERM_CLASS}>{messages.averageDurationLabel}</dt>
          <dd className={TREND_SUMMARY_VALUE_CLASS}>
            {formatDurationMillis(model.averageDurationMillis)}
          </dd>
        </div>
      </dl>

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
                  {point.dispatchLabel}: {formatDurationMillis(point.durationMillis)}
                </title>
              </circle>
            ))}
          </svg>
          <dl
            className={TIMING_RANGE_SUMMARY_CLASS}
            aria-label={messages.timingRangeLabel}
          >
            <div>
              <dt className={TREND_SUMMARY_TERM_CLASS}>{messages.fastestDurationLabel}</dt>
              <dd className={TREND_SUMMARY_VALUE_CLASS}>
                {formatDurationMillis(model.fastestDurationMillis)}
              </dd>
            </div>
            <div>
              <dt className={TREND_SUMMARY_TERM_CLASS}>{messages.latestDurationLabel}</dt>
              <dd className={TREND_SUMMARY_VALUE_CLASS}>
                {formatDurationMillis(model.latestDurationMillis)}
              </dd>
            </div>
          </dl>
        </>
      ) : (
        <div className={cn(EMPTY_STATE_CLASS, EMPTY_STATE_COMPACT_CLASS)}>
          <h3>{messages.reworkEmptyTitle}</h3>
          <p>{messages.timingEmptyMessage}</p>
        </div>
      )}
    </DashboardWidgetFrame>
  );
}

function TrendAxes() {
  return (
    <>
      <line className={DASHBOARD_CHART_AXIS_CLASS} x1="14" x2="306" y1="106" y2="106" />
      <line className={DASHBOARD_CHART_AXIS_CLASS} x1="14" x2="14" y1="14" y2="106" />
    </>
  );
}

function isThroughputRangeID(value: string): value is ThroughputRangeID {
  return THROUGHPUT_RANGE_OPTIONS.some((option) => option.id === value);
}
