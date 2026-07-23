import {
  WidgetDetailCopy,
  WidgetSubtitle,
} from "@you-agent-factory/components/recipes";
import { normalizeDetailText } from "../../../base/components/detail-card/detail-card-shared";
import { SelectionDetailLayout } from "../../../base/components/layout/current-selection-detail-layout";
import { useCurrentSelectionShellMessages } from "../../../base/components/presentation/current-selection-locale";
import type { TerminalWorkSummaryCardProps } from "../../lib/detail-card-types";
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
      <WidgetSubtitle>{label}</WidgetSubtitle>
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
        <WidgetDetailCopy>
          {messages.failureDetailsUnavailable}
        </WidgetDetailCopy>
      ) : null}
      {executionDetails ? (
        <ExecutionDetailsSection details={executionDetails} now={now} />
      ) : null}
    </SelectionDetailLayout>
  );
}
