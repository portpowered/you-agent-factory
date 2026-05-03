import { DETAIL_COPY_CLASS, WIDGET_SUBTITLE_CLASS } from "../../components/dashboard/widget-board";
import { SelectionDetailLayout } from "./current-selection-detail-layout";
import { normalizeDetailText } from "./detail-card-shared";
import type { TerminalWorkSummaryCardProps } from "./detail-card-types";
import { ExecutionDetailsSection } from "./execution-details";

export function TerminalWorkSummaryCard({
  executionDetails,
  failureMessage,
  failureReason,
  label,
  now = Date.now(),
  status,
  widgetId = "current-selection",
}: TerminalWorkSummaryCardProps) {
  const normalizedFailureReason = normalizeDetailText(failureReason);
  const normalizedFailureMessage = normalizeDetailText(failureMessage);

  return (
    <SelectionDetailLayout widgetId={widgetId}>
      <p className={WIDGET_SUBTITLE_CLASS}>{label}</p>
      <dl>
        <div>
          <dt>Status</dt>
          <dd>{status === "completed" ? "Completed" : "Failed"}</dd>
        </div>
        <div>
          <dt>Source</dt>
          <dd>Current workstation run summary</dd>
        </div>
        {status === "failed" ? (
          <>
            <div>
              <dt>Failure reason</dt>
              <dd>{normalizedFailureReason ?? "Failure reason unavailable"}</dd>
            </div>
            <div>
              <dt>Failure message</dt>
              <dd>{normalizedFailureMessage ?? "Failure message unavailable"}</dd>
            </div>
          </>
        ) : null}
      </dl>
      {status === "failed" &&
      normalizedFailureReason === undefined &&
      normalizedFailureMessage === undefined ? (
        <p className={DETAIL_COPY_CLASS}>
          Failure details are unavailable for this failed work item.
        </p>
      ) : status === "completed" ? (
        <p className={DETAIL_COPY_CLASS}>
          Completed terminal work is retained in the session summary.
        </p>
      ) : null}
      {executionDetails ? (
        <ExecutionDetailsSection details={executionDetails} now={now} traceTargetId="trace" />
      ) : null}
    </SelectionDetailLayout>
  );
}

