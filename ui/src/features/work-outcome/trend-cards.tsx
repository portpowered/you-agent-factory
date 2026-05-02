import {
  DASHBOARD_CHART_AXIS_CLASS,
  DASHBOARD_CHART_SURFACE_CLASS,
  getDashboardChartSemanticStyle,
} from "./chart-contract";
import { cx } from "../../components/dashboard/classnames";
import {
  formatDurationMillis,
  formatTraceOutcome,
} from "../../components/dashboard/formatters";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
} from "../../components/dashboard/typography";
import {
  THROUGHPUT_RANGE_OPTIONS,
  type FailureTrendModel,
  type ReworkTrendModel,
  type ThroughputRangeID,
  type TimingTrendModel,
} from "./trends";
import {
  DETAIL_CARD_WIDE_CLASS,
  DETAIL_COPY_CLASS,
  EMPTY_STATE_CLASS,
  EMPTY_STATE_COMPACT_CLASS,
  WIDGET_SUBTITLE_CLASS,
} from "../../components/dashboard/widget-board";
import { DashboardWidgetFrame } from "../../components/ui";

interface FailureTrendCardProps {
  className?: string;
  model: FailureTrendModel;
  onRangeChange: (rangeID: ThroughputRangeID) => void;
  rangeID: ThroughputRangeID;
  widgetId?: string;
}

interface ReworkTrendCardProps {
  className?: string;
  model: ReworkTrendModel;
  widgetId?: string;
}

interface TimingTrendCardProps {
  className?: string;
  model: TimingTrendModel;
  widgetId?: string;
}

const TREND_TOOLBAR_CLASS =
  "mb-4 flex items-start justify-between gap-3 max-[720px]:flex-col";
const TREND_RANGE_LABEL_CLASS =
  "grid shrink-0 basis-[8.5rem] gap-1 max-[720px]:w-full max-[720px]:basis-auto";
const TREND_RANGE_TEXT_CLASS = DASHBOARD_SUPPORTING_LABEL_CLASS;
const TREND_RANGE_SELECT_CLASS = cx(
  "rounded-lg border border-af-accent/35 bg-af-canvas/82 px-[0.55rem] py-[0.45rem] text-af-ink",
  DASHBOARD_BODY_TEXT_CLASS,
);
const TREND_SUMMARY_CLASS =
  cx(
    "mb-4 grid grid-cols-3 gap-3 max-[720px]:grid-cols-1 [&_dd]:m-0 [&_div]:rounded-lg [&_div]:border [&_div]:border-af-overlay/8 [&_div]:bg-af-overlay/4 [&_div]:p-[0.7rem] [&_dt]:mb-1",
    DASHBOARD_SUPPORTING_LABELS_CLASS,
  );
const TREND_CHART_CLASS = cx(DASHBOARD_CHART_SURFACE_CLASS, "min-h-44 border border-af-overlay/8");
const TREND_CAUSE_LIST_CLASS = "mt-4 grid list-none gap-[0.55rem] p-0";
const TREND_CAUSE_ITEM_CLASS =
  "flex items-center justify-between gap-3 rounded-lg border border-af-overlay/7 bg-af-overlay/4 px-[0.7rem] py-[0.6rem]";
const TREND_CAUSE_LABEL_CLASS = cx(
  "min-w-0 text-af-ink/78 [overflow-wrap:anywhere]",
  DASHBOARD_BODY_TEXT_CLASS,
);
const TIMING_RANGE_SUMMARY_CLASS = cx(TREND_SUMMARY_CLASS, "mt-[0.85rem] grid-cols-2");
const TREND_SUMMARY_TERM_CLASS = DASHBOARD_SUPPORTING_LABEL_CLASS;
const TREND_SUMMARY_VALUE_CLASS = WIDGET_SUBTITLE_CLASS;
const FAILURE_TREND_CHART_STYLE = getDashboardChartSemanticStyle("failureTrend");
const REWORK_TREND_CHART_STYLE = getDashboardChartSemanticStyle("reworkTrend");
const TIMING_TREND_CHART_STYLE = getDashboardChartSemanticStyle("timingTrend");

export function FailureTrendCard({
  className = "",
  model,
  onRangeChange,
  rangeID,
  widgetId = "failure-trend",
}: FailureTrendCardProps) {
  const changeRange = (value: string) => {
    if (isThroughputRangeID(value)) {
      onRangeChange(value);
    }
  };

  return (
    <DashboardWidgetFrame
      className={cx(DETAIL_CARD_WIDE_CLASS, className)}
      title="Failure trend"
      widgetId={widgetId}
    >
      <div className={TREND_TOOLBAR_CLASS}>
        <p className={WIDGET_SUBTITLE_CLASS}>
          Failed work and cause groups from the selected factory timeline.
        </p>
        <label className={TREND_RANGE_LABEL_CLASS}>
          <span className={TREND_RANGE_TEXT_CLASS}>Time range</span>
          <select
            aria-label="Time range"
            className={TREND_RANGE_SELECT_CLASS}
            value={rangeID}
            onChange={(event) => changeRange(event.target.value)}
          >
            {THROUGHPUT_RANGE_OPTIONS.map((option) => (
              <option key={option.id} value={option.id}>
                {option.label}
              </option>
            ))}
          </select>
        </label>
      </div>

      <dl className={TREND_SUMMARY_CLASS}>
        <div>
          <dt className={TREND_SUMMARY_TERM_CLASS}>Failed in range</dt>
          <dd className={TREND_SUMMARY_VALUE_CLASS}>{model.failureDelta}</dd>
        </div>
        <div>
          <dt className={TREND_SUMMARY_TERM_CLASS}>Total failed</dt>
          <dd className={TREND_SUMMARY_VALUE_CLASS}>{model.currentFailed}</dd>
        </div>
        <div>
          <dt className={TREND_SUMMARY_TERM_CLASS}>Cause groups</dt>
          <dd className={TREND_SUMMARY_VALUE_CLASS}>{model.groups.length}</dd>
        </div>
      </dl>

      {model.points.length > 0 ? (
        <svg
          className={TREND_CHART_CLASS}
          role="img"
          aria-label={`Failed work trend for ${model.rangeLabel}`}
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
        <div className={cx(EMPTY_STATE_CLASS, EMPTY_STATE_COMPACT_CLASS)}>
          <h3>No failure samples</h3>
          <p>Failure trend data appears after the event stream receives work history.</p>
        </div>
      )}

      {model.groups.length > 0 ? (
        <ul className={TREND_CAUSE_LIST_CLASS} aria-label="Failure cause groups">
          {model.groups.map((group) => (
            <li className={TREND_CAUSE_ITEM_CLASS} key={group.label}>
              <span className={TREND_CAUSE_LABEL_CLASS}>{group.label}</span>
              <strong className="shrink-0 text-af-danger-bright">{group.count}</strong>
            </li>
          ))}
        </ul>
      ) : (
        <p className={DETAIL_COPY_CLASS}>No failed work has been grouped yet.</p>
      )}
    </DashboardWidgetFrame>
  );
}

export function ReworkTrendCard({
  className = "",
  model,
  widgetId = "rework-trend",
}: ReworkTrendCardProps) {
  return (
    <DashboardWidgetFrame
      className={cx(DETAIL_CARD_WIDE_CLASS, className)}
      title="Retry and rework trend"
      widgetId={widgetId}
    >
      <p className={WIDGET_SUBTITLE_CLASS}>
        Reject, retry, or rework activity from the selected work trace.
      </p>

      <dl className={TREND_SUMMARY_CLASS}>
        <div>
          <dt className={TREND_SUMMARY_TERM_CLASS}>Trace work</dt>
          <dd className={TREND_SUMMARY_VALUE_CLASS}>{model.currentWorkLabel}</dd>
        </div>
        <div>
          <dt className={TREND_SUMMARY_TERM_CLASS}>Retry or rework</dt>
          <dd className={TREND_SUMMARY_VALUE_CLASS}>{model.retryOrReworkCount}</dd>
        </div>
        <div>
          <dt className={TREND_SUMMARY_TERM_CLASS}>Latest outcome</dt>
          <dd className={TREND_SUMMARY_VALUE_CLASS}>
            {formatTraceOutcome(model.terminalOutcome)}
          </dd>
        </div>
      </dl>

      {model.points.length > 0 ? (
        <svg
          className={TREND_CHART_CLASS}
          role="img"
          aria-label={`Retry and rework trend for ${model.currentWorkLabel}`}
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
        <div className={cx(EMPTY_STATE_CLASS, EMPTY_STATE_COMPACT_CLASS)}>
          <h3>No selected trace</h3>
          <p>Select active work with retained trace history to see retry activity.</p>
        </div>
      )}
    </DashboardWidgetFrame>
  );
}

export function TimingTrendCard({
  className = "",
  model,
  widgetId = "timing-trend",
}: TimingTrendCardProps) {
  return (
    <DashboardWidgetFrame
      className={cx(DETAIL_CARD_WIDE_CLASS, className)}
      title="Timing trend"
      widgetId={widgetId}
    >
      <p className={WIDGET_SUBTITLE_CLASS}>
        Dispatch duration trend from the selected work trace.
      </p>

      <dl className={TREND_SUMMARY_CLASS}>
        <div>
          <dt className={TREND_SUMMARY_TERM_CLASS}>Trace work</dt>
          <dd className={TREND_SUMMARY_VALUE_CLASS}>{model.currentWorkLabel}</dd>
        </div>
        <div>
          <dt className={TREND_SUMMARY_TERM_CLASS}>Slowest dispatch</dt>
          <dd className={TREND_SUMMARY_VALUE_CLASS}>
            {formatDurationMillis(model.slowestDurationMillis)}
          </dd>
        </div>
        <div>
          <dt className={TREND_SUMMARY_TERM_CLASS}>Average duration</dt>
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
            aria-label={`Timing trend for ${model.currentWorkLabel}`}
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
          <dl className={TIMING_RANGE_SUMMARY_CLASS} aria-label="Timing range">
            <div>
              <dt className={TREND_SUMMARY_TERM_CLASS}>Fastest</dt>
              <dd className={TREND_SUMMARY_VALUE_CLASS}>
                {formatDurationMillis(model.fastestDurationMillis)}
              </dd>
            </div>
            <div>
              <dt className={TREND_SUMMARY_TERM_CLASS}>Latest</dt>
              <dd className={TREND_SUMMARY_VALUE_CLASS}>
                {formatDurationMillis(model.latestDurationMillis)}
              </dd>
            </div>
          </dl>
        </>
      ) : (
        <div className={cx(EMPTY_STATE_CLASS, EMPTY_STATE_COMPACT_CLASS)}>
          <h3>No selected trace</h3>
          <p>Select active work with retained trace history to compare dispatch timing.</p>
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
