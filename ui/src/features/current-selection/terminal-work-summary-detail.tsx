import {
  DETAIL_COPY_CLASS,
  WIDGET_SUBTITLE_CLASS,
} from "../../components/dashboard/widget-board";
import { SelectionDetailLayout } from "./current-selection-detail-layout";
import { useCurrentSelectionShellMessages } from "./current-selection-locale";
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
  const messages = useCurrentSelectionShellMessages();
  const normalizedFailureReason = normalizeDetailText(failureReason);
  const normalizedFailureMessage = normalizeDetailText(failureMessage);

  return (
    <SelectionDetailLayout widgetId={widgetId}>
      <p className={WIDGET_SUBTITLE_CLASS}>{label}</p>
      <dl>
        <div>
          <dt>{messages.statusLabel}</dt>
          <dd>
            {status === "completed"
              ? messages.completedStatus
              : messages.failedStatus}
          </dd>
        </div>
        <div>
          <dt>{messages.sourceLabel}</dt>
          <dd>{messages.sourceSummary}</dd>
        </div>
        {status === "failed" ? (
          <>
            <div>
              <dt>{messages.failureReasonLabel}</dt>
              <dd>
                {normalizedFailureReason ?? messages.failureReasonUnavailable}
              </dd>
            </div>
            <div>
              <dt>{messages.failureMessageLabel}</dt>
              <dd>
                {normalizedFailureMessage ?? messages.failureMessageUnavailable}
              </dd>
            </div>
          </>
        ) : null}
      </dl>
      {status === "failed" &&
      normalizedFailureReason === undefined &&
      normalizedFailureMessage === undefined ? (
        <p className={DETAIL_COPY_CLASS}>
          {messages.failureDetailsUnavailable}
        </p>
      ) : status === "completed" ? (
        <p className={DETAIL_COPY_CLASS}>
          {messages.completedTerminalWorkSummary}
        </p>
      ) : null}
      {executionDetails ? (
        <ExecutionDetailsSection
          details={executionDetails}
          now={now}
          traceTargetId="trace"
        />
      ) : null}
    </SelectionDetailLayout>
  );
}
