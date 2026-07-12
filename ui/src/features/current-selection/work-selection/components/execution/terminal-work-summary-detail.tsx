import {
  WidgetDetailCopy,
  WidgetSubtitle,
} from "@you-agent-factory/components/recipes";
import { normalizeDetailText } from "../../../base/components/detail-card/detail-card-shared";
import { SelectionDetailLayout } from "../../../base/components/layout/current-selection-detail-layout";
import { useCurrentSelectionShellMessages } from "../../../base/components/presentation/current-selection-locale";
import {
  CurrentSelectionDescriptionList,
  CurrentSelectionDetailItem,
} from "../../../base/public";
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
      <CurrentSelectionDescriptionList>
        <CurrentSelectionDetailItem
          label={messages.statusLabel}
          value={
            status === "completed"
              ? messages.completedStatus
              : messages.failedStatus
          }
        />
        <CurrentSelectionDetailItem
          label={messages.sourceLabel}
          value={messages.sourceSummary}
        />
        {status === "failed" ? (
          <>
            <CurrentSelectionDetailItem
              code
              label={messages.failureReasonLabel}
              value={
                normalizedFailureReason ?? messages.failureReasonUnavailable
              }
            />
            <CurrentSelectionDetailItem
              label={messages.failureMessageLabel}
              value={
                normalizedFailureMessage ?? messages.failureMessageUnavailable
              }
            />
          </>
        ) : null}
      </CurrentSelectionDescriptionList>
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
