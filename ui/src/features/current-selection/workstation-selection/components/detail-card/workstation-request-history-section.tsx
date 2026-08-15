import { Code } from "@you-agent-factory/components/primitives";
import { WidgetDetailCopy } from "@you-agent-factory/components/recipes";
import { useEffect, useRef, useState } from "react";
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
import { CurrentSelectionHistoryCard } from "../../../history/components/current-selection-history-card";
import type { WorkstationRequestHistorySectionProps } from "../../lib/keys/detail-card-types";
import type { getWorkstationDetailMessages } from "../../messages/workstation-detail";
import { WorkstationRequestAttempts } from "./history/workstation-request-attempts";
import { WorkstationDispatchRow } from "./workstation-dispatch-row";

// Keep the workstation history compact while making every retained request reachable.
const WORKSTATION_HISTORY_PAGE_SIZE = 10;

export function WorkstationRequestHistorySection({
  locale,
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
  const [visibleRequestCount, setVisibleRequestCount] = useState(
    WORKSTATION_HISTORY_PAGE_SIZE,
  );
  const [focusHistoryListAfterReveal, setFocusHistoryListAfterReveal] =
    useState(false);
  const historyListRef = useRef<HTMLUListElement>(null);

  useEffect(() => {
    if (!focusHistoryListAfterReveal) {
      return;
    }

    historyListRef.current?.focus();
    setFocusHistoryListAfterReveal(false);
  }, [focusHistoryListAfterReveal]);

  const visibleRequests = requests.slice(0, visibleRequestCount);
  const remainingRequestCount = requests.length - visibleRequests.length;
  const revealMoreRequests = () => {
    if (remainingRequestCount <= 0) {
      return;
    }

    if (remainingRequestCount <= WORKSTATION_HISTORY_PAGE_SIZE) {
      setFocusHistoryListAfterReveal(true);
    }
    setVisibleRequestCount((currentCount) =>
      Math.min(currentCount + WORKSTATION_HISTORY_PAGE_SIZE, requests.length),
    );
  };

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
        <>
          <ul
            className="m-0 grid list-none gap-2.5 p-0"
            id={`${historyID}-items`}
            ref={historyListRef}
            tabIndex={-1}
          >
            {visibleRequests.map((request) => (
              <WorkstationRequestHistoryRow
                key={request.dispatch_id}
                locale={locale}
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
          {remainingRequestCount > 0 ? (
            <div className="flex flex-wrap items-center justify-between gap-3">
              <CurrentSelectionSupportingText tone="status">
                {messages.historyProgressLabel(
                  visibleRequests.length,
                  requests.length,
                  remainingRequestCount,
                )}
              </CurrentSelectionSupportingText>
              <DashboardActionButton
                aria-controls={`${historyID}-items`}
                onClick={revealMoreRequests}
                type="button"
              >
                {messages.showMoreHistoryAction(remainingRequestCount)}
              </DashboardActionButton>
            </div>
          ) : null}
        </>
      ) : (
        <WidgetDetailCopy>{messages.noWorkstationRequests}</WidgetDetailCopy>
      )}
    </CurrentSelectionExpandableSection>
  );
}

function WorkstationRequestHistoryRow({
  locale,
  messages,
  now,
  onSelectWorkID,
  onSelectWorkstationRequest,
  request,
  selectedRequest,
  selectedWorkID,
}: {
  locale?: string;
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  now: number;
  onSelectWorkID?: WorkstationRequestHistorySectionProps["onSelectWorkID"];
  onSelectWorkstationRequest?: WorkstationRequestHistorySectionProps["onSelectWorkstationRequest"];
  request: DashboardWorkstationRequest;
  selectedRequest?: WorkstationRequestHistorySectionProps["selectedRequest"];
  selectedWorkID?: WorkstationRequestHistorySectionProps["selectedWorkID"];
}) {
  const requestSelected = selectedRequest?.dispatch_id === request.dispatch_id;
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
        onSelectWorkstationRequest,
        request,
        requestSelected,
      })}
      status={renderWorkstationRequestStatusPill({
        hasFailedOutcome,
        messages,
        now,
        request,
        totalDurationMillis,
      })}
      supportingContent={
        <div className="grid gap-3">
          <WorkstationRequestWorkItems
            messages={messages}
            onSelectWorkID={onSelectWorkID}
            request={request}
            selectedWorkID={selectedWorkID}
          />
          <WorkstationRequestAttempts
            locale={locale}
            messages={messages}
            request={request}
          />
          {requestSelected ? (
            <CurrentSelectionSupportingText tone="status">
              {messages.selectedRequestLabel(request.dispatch_id)}
            </CurrentSelectionSupportingText>
          ) : null}
        </div>
      }
      title={
        <>
          <span>{messages.projectedWorkstationRequestSummary}</span>{" "}
          <Code>{request.request_id ?? request.dispatch_id}</Code>
        </>
      }
    />
  );
}

function WorkstationRequestWorkItems({
  messages,
  onSelectWorkID,
  request,
  selectedWorkID,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  onSelectWorkID?: WorkstationRequestHistorySectionProps["onSelectWorkID"];
  request: DashboardWorkstationRequest;
  selectedWorkID?: WorkstationRequestHistorySectionProps["selectedWorkID"];
}) {
  if (request.work_items.length === 0) {
    return (
      <CurrentSelectionSupportingText tone="status">
        {messages.unknownWorkLabel}
      </CurrentSelectionSupportingText>
    );
  }

  return (
    <div className="grid gap-2">
      {request.work_items.map((workItem) => {
        const workLabel = formatWorkItemLabel(workItem);
        const selected = selectedWorkID === workItem.work_id;

        return (
          <CurrentSelectionHistoryCard
            className="p-3"
            key={`${request.dispatch_id}:${workItem.work_id}`}
          >
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div className="grid min-w-0 gap-1">
                <strong className="min-w-0 [overflow-wrap:anywhere]">
                  {workLabel}
                </strong>
                <Code>
                  {messages.workIdLabel}: {workItem.work_id}
                </Code>
              </div>
              {onSelectWorkID ? (
                <DashboardActionButton
                  aria-label={messages.selectWorkItemLabel(workLabel)}
                  aria-pressed={selected}
                  onClick={() => onSelectWorkID(workItem.work_id)}
                  type="button"
                >
                  {selected
                    ? messages.workSelectedAction
                    : messages.openWorkItemAction}
                </DashboardActionButton>
              ) : null}
            </div>
          </CurrentSelectionHistoryCard>
        );
      })}
    </div>
  );
}

function renderWorkstationRequestActions({
  messages,
  onSelectWorkstationRequest,
  request,
  requestSelected,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  onSelectWorkstationRequest?: WorkstationRequestHistorySectionProps["onSelectWorkstationRequest"];
  request: DashboardWorkstationRequest;
  requestSelected: boolean;
}) {
  if (!onSelectWorkstationRequest) {
    return undefined;
  }

  return (
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
