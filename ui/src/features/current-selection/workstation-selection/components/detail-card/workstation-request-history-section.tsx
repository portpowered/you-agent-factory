import { WidgetDetailCopy } from "@you-agent-factory/components/recipes";
import type { DashboardWorkstationRequest } from "../../../../../api/dashboard/types";
import { DashboardActionButton } from "../../../../../components/ui/dashboard-action-button";
import {
  formatDurationFromISO,
  formatDurationMillis,
  formatWorkItemLabel,
} from "../../../../../components/ui/formatters";
import { CurrentSelectionExpandableSection } from "../../../base/components/detail/current-selection-expandable-section";
import { CurrentSelectionExecutionPill } from "../../../base/components/presentation/current-selection-pill";
import { CurrentSelectionSupportingText } from "../../../base/components/presentation/current-selection-supporting-text";
import type { WorkstationRequestHistorySectionProps } from "../../lib/keys/detail-card-types";
import type { getWorkstationDetailMessages } from "../../messages/workstation-detail";
import { WorkstationDispatchRow } from "./workstation-dispatch-row";

export function WorkstationRequestHistorySection({
  messages,
  now,
  onSelectWorkID,
  onSelectWorkstationRequest,
  requests,
  resetKey,
  selectedRequest,
  selectedWorkID,
}: WorkstationRequestHistorySectionProps) {
  const historyID = `workstation-request-history-${resetKey}`;

  return (
    <CurrentSelectionExpandableSection
      contentId={historyID}
      headingId={`${historyID}-heading`}
      resetKey={resetKey}
      supportingText={messages.historyRequestCountLabel(requests.length)}
      title={messages.requestHistoryHeading}
      toggleLabel={(expanded) =>
        expanded ? messages.collapseAction : messages.expandAction
      }
    >
      {requests.length > 0 ? (
        <ul className="m-0 grid list-none gap-2.5 p-0">
          {requests.map((request) => (
            <WorkstationRequestHistoryRow
              key={request.dispatch_id}
              messages={messages}
              now={now}
              onSelectWorkID={onSelectWorkID}
              onSelectWorkstationRequest={onSelectWorkstationRequest}
              request={request}
              selectedRequest={selectedRequest}
              selectedWorkID={selectedWorkID}
            />
          ))}
        </ul>
      ) : (
        <WidgetDetailCopy>{messages.noWorkstationRequests}</WidgetDetailCopy>
      )}
    </CurrentSelectionExpandableSection>
  );
}

function WorkstationRequestHistoryRow({
  messages,
  now,
  onSelectWorkID,
  onSelectWorkstationRequest,
  request,
  selectedRequest,
  selectedWorkID,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  now: number;
  onSelectWorkID?: WorkstationRequestHistorySectionProps["onSelectWorkID"];
  onSelectWorkstationRequest?: WorkstationRequestHistorySectionProps["onSelectWorkstationRequest"];
  request: DashboardWorkstationRequest;
  selectedRequest?: WorkstationRequestHistorySectionProps["selectedRequest"];
  selectedWorkID?: WorkstationRequestHistorySectionProps["selectedWorkID"];
}) {
  const primaryWorkItem = request.work_items[0];
  const requestSelected = selectedRequest?.dispatch_id === request.dispatch_id;
  const workLabel = primaryWorkItem
    ? formatWorkItemLabel(primaryWorkItem)
    : messages.unknownActiveWorkLabel;
  const totalDurationMillis =
    request.total_duration_millis ?? request.script_response?.duration_millis;
  const normalizedOutcome = (
    request.outcome ?? request.script_response?.outcome
  )
    ?.trim()
    .toUpperCase();
  const hasFailedOutcome =
    Boolean(request.failure_reason?.trim()) ||
    Boolean(request.failure_message?.trim()) ||
    normalizedOutcome === "FAILED" ||
    normalizedOutcome === "FAILED_EXIT_CODE" ||
    normalizedOutcome === "TIMED_OUT" ||
    normalizedOutcome === "REJECTED";

  return (
    <WorkstationDispatchRow
      actions={renderWorkstationRequestActions({
        messages,
        onSelectWorkID,
        onSelectWorkstationRequest,
        primaryWorkItem,
        request,
        requestSelected,
        selectedWorkID,
        workLabel,
      })}
      status={renderWorkstationRequestStatusPill({
        hasFailedOutcome,
        messages,
        now,
        request,
        totalDurationMillis,
      })}
      supportingContent={
        requestSelected ? (
          <CurrentSelectionSupportingText tone="status">
            {messages.selectedRequestLabel(request.dispatch_id)}
          </CurrentSelectionSupportingText>
        ) : null
      }
      title={workLabel}
    />
  );
}

function renderWorkstationRequestActions({
  messages,
  onSelectWorkID,
  onSelectWorkstationRequest,
  primaryWorkItem,
  request,
  requestSelected,
  selectedWorkID,
  workLabel,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  onSelectWorkID?: WorkstationRequestHistorySectionProps["onSelectWorkID"];
  onSelectWorkstationRequest?: WorkstationRequestHistorySectionProps["onSelectWorkstationRequest"];
  primaryWorkItem:
    | DashboardWorkstationRequest["work_items"][number]
    | undefined;
  request: DashboardWorkstationRequest;
  requestSelected: boolean;
  selectedWorkID?: WorkstationRequestHistorySectionProps["selectedWorkID"];
  workLabel: string;
}) {
  const workAction =
    primaryWorkItem && onSelectWorkID ? (
      <DashboardActionButton
        aria-label={messages.selectWorkItemLabel(workLabel)}
        aria-pressed={selectedWorkID === primaryWorkItem.work_id}
        onClick={() => onSelectWorkID(primaryWorkItem.work_id)}
        type="button"
      >
        {selectedWorkID === primaryWorkItem.work_id
          ? messages.workSelectedAction
          : messages.openWorkItemAction}
      </DashboardActionButton>
    ) : null;
  const requestAction = onSelectWorkstationRequest ? (
    <DashboardActionButton
      aria-label={messages.selectWorkstationRequestLabel(request.dispatch_id)}
      aria-pressed={requestSelected}
      onClick={() => onSelectWorkstationRequest(request)}
      type="button"
    >
      {requestSelected
        ? messages.requestSelectedAction
        : messages.openRequestDetailsAction}
    </DashboardActionButton>
  ) : null;

  if (!workAction && !requestAction) {
    return undefined;
  }

  return (
    <>
      {workAction}
      {requestAction}
    </>
  );
}

function renderWorkstationRequestStatusPill({
  hasFailedOutcome,
  messages,
  now,
  request,
  totalDurationMillis,
}: {
  hasFailedOutcome: boolean;
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  now: number;
  request: DashboardWorkstationRequest;
  totalDurationMillis: number | undefined;
}) {
  if (totalDurationMillis !== undefined) {
    return (
      <CurrentSelectionExecutionPill
        tone={hasFailedOutcome ? "danger" : "success"}
      >
        {messages.totalRuntimeLabel}:{" "}
        {formatDurationMillis(totalDurationMillis)}
      </CurrentSelectionExecutionPill>
    );
  }

  if (!request.started_at) {
    return undefined;
  }

  return (
    <CurrentSelectionExecutionPill>
      {messages.elapsedLabel}: {formatDurationFromISO(request.started_at, now)}
    </CurrentSelectionExecutionPill>
  );
}
