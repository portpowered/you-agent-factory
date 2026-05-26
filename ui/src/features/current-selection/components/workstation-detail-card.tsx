import { useEffect, useState } from "react";
import {
  DETAIL_COPY_CLASS,
  WIDGET_SUBTITLE_CLASS,
} from "../../../components/dashboard/widget-board";
import type { DashboardWorkstationRequest } from "../../../api/dashboard/types";
import { DASHBOARD_BODY_TEXT_CLASS } from "../../../components/ui/dashboard-typography";
import {
  DashboardStatusPill,
  DashboardActionButton,
  DashboardActionRow,
} from "../../../components/ui";
import {
  formatDurationMillis,
  formatDurationFromISO,
  formatWorkItemLabel,
} from "../../../components/ui/formatters";
import { cn } from "../../../lib/cn";
import { SelectionDetailLayout } from "./current-selection-detail-layout";
import {
  CurrentSelectionSectionHeader,
  EXECUTION_PILL_CLASS,
  HISTORY_TOGGLE_CLASS,
  PROVIDER_SESSION_CARD_CLASS,
  REQUEST_SELECTION_STATUS_CLASS,
} from "./detail-card-shared";
import type {
  WorkstationActiveWorkListProps,
  WorkstationDetailCardProps,
} from "./detail-card-types";
import { getWorkstationDetailMessages } from "../messages/workstation-detail";
import { CollapsibleProviderSessionAttempts } from "./provider-session-attempts";
import {
  EditableConfigurationSection,
  WorkstationSummary,
} from "./workstation-editable-configuration-section";

export function WorkstationDetailCard({
  activeExecutions,
  editableConfigurationState,
  headerAction,
  locale,
  now,
  onSelectProviderSession,
  onSelectWorkID,
  onSelectWorkstationRequest,
  providerSessions,
  saveState,
  selectedProviderSessionKey,
  selectedRequest,
  selectedWorkID,
  selectedNode,
  workstationRequests = [],
  widgetId = "current-selection",
}: WorkstationDetailCardProps) {
  const messages = getWorkstationDetailMessages(locale);
  const hasProjectedRequestHistory = workstationRequests.length > 0;
  const workstationRequestsByDispatchID = Object.fromEntries(
    workstationRequests.map((request) => [request.dispatch_id, request]),
  );

  return (
    <SelectionDetailLayout headerAction={headerAction} widgetId={widgetId}>
      <p className={WIDGET_SUBTITLE_CLASS}>{selectedNode.workstation_name}</p>
      <WorkstationSummary
        activeRunCount={activeExecutions.length}
        editableConfigurationState={editableConfigurationState}
        historyCount={
          hasProjectedRequestHistory
            ? workstationRequests.length
            : providerSessions.length
        }
        historyLabel={
          hasProjectedRequestHistory
            ? messages.historicalRequestsLabel
            : messages.historicalRunsLabel
        }
        messages={messages}
        selectedNode={selectedNode}
      />
      <EditableConfigurationSection
        key={`editable-configuration:${selectedNode.node_id}`}
        messages={messages}
        saveState={saveState}
        state={editableConfigurationState}
      />
      <WorkstationActiveWorkList
        executions={activeExecutions}
        messages={messages}
        now={now}
        onSelectWorkID={onSelectWorkID}
        onSelectWorkstationRequest={onSelectWorkstationRequest}
        selectedNode={selectedNode}
        selectedRequest={selectedRequest}
        selectedWorkID={selectedWorkID}
        workstationRequestsByDispatchID={workstationRequestsByDispatchID}
      />
      {hasProjectedRequestHistory ? (
        <CollapsibleWorkstationRequests
          key={`workstation-request-history:${selectedNode.node_id}`}
          messages={messages}
          now={now}
          onSelectWorkID={onSelectWorkID}
          onSelectWorkstationRequest={onSelectWorkstationRequest}
          requests={workstationRequests}
          resetKey={selectedNode.node_id}
          selectedRequest={selectedRequest}
          selectedWorkID={selectedWorkID}
        />
      ) : (
        <CollapsibleProviderSessionAttempts
          key={`workstation-run-history:${selectedNode.node_id}`}
          attempts={providerSessions}
          collapseActionLabel={messages.collapseAction}
          emptyMessage={messages.noWorkstationRuns}
          expandActionLabel={messages.expandAction}
          historyItemCountLabel={messages.historyRunCountLabel}
          messages={messages}
          onSelectProviderSession={onSelectProviderSession}
          onSelectWorkID={onSelectWorkID}
          onSelectWorkstationRequest={onSelectWorkstationRequest}
          renderHeading={(attempt) =>
            attempt.work_items?.map(formatWorkItemLabel).join(", ") ||
            messages.unknownWorkLabel
          }
          resetKey={selectedNode.node_id}
          selectedProviderSessionKey={selectedProviderSessionKey}
          selectedRequestDispatchID={selectedRequest?.dispatch_id}
          selectedWorkID={selectedWorkID}
          title={messages.runHistoryHeading}
          workstationKind={selectedNode.workstation_kind}
          workstationRequestsByDispatchID={workstationRequestsByDispatchID}
        />
      )}
    </SelectionDetailLayout>
  );
}

function CollapsibleWorkstationRequests({
  messages,
  now,
  onSelectWorkID,
  onSelectWorkstationRequest,
  requests,
  resetKey,
  selectedRequest,
  selectedWorkID,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  now: number;
  onSelectWorkID?: WorkstationDetailCardProps["onSelectWorkID"];
  onSelectWorkstationRequest?: WorkstationDetailCardProps["onSelectWorkstationRequest"];
  requests: NonNullable<WorkstationDetailCardProps["workstationRequests"]>;
  resetKey: string;
  selectedRequest?: WorkstationDetailCardProps["selectedRequest"];
  selectedWorkID?: WorkstationDetailCardProps["selectedWorkID"];
}) {
  const [expanded, setExpanded] = useState(false);
  const historyID = `workstation-request-history-${resetKey}`;
  const itemCountLabel = messages.historyRequestCountLabel(requests.length);

  useEffect(() => {
    setExpanded(false);
  }, []);

  return (
    <section
      aria-labelledby={`${historyID}-heading`}
      className="mt-4 grid gap-2.5"
    >
      <CurrentSelectionSectionHeader
        action={
          <button
            aria-controls={historyID}
            aria-expanded={expanded}
            className={HISTORY_TOGGLE_CLASS}
            onClick={() => setExpanded((current) => !current)}
            type="button"
          >
            {expanded ? messages.collapseAction : messages.expandAction}
          </button>
        }
        headingId={`${historyID}-heading`}
        supportingText={itemCountLabel}
        title={messages.requestHistoryHeading}
      />
      {expanded ? (
        <div className="grid gap-3" id={historyID}>
          {requests.length > 0 ? (
            requests.map((request) => {
              return (
                <WorkstationRequestHistoryCard
                  key={request.dispatch_id}
                  messages={messages}
                  now={now}
                  onSelectWorkID={onSelectWorkID}
                  onSelectWorkstationRequest={onSelectWorkstationRequest}
                  request={request}
                  selectedRequest={selectedRequest}
                  selectedWorkID={selectedWorkID}
                />
              );
            })
          ) : (
            <p className={DETAIL_COPY_CLASS}>
              {messages.noWorkstationRequests}
            </p>
          )}
        </div>
      ) : null}
    </section>
  );
}

function WorkstationRequestHistoryCard({
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
  onSelectWorkID?: WorkstationDetailCardProps["onSelectWorkID"];
  onSelectWorkstationRequest?: WorkstationDetailCardProps["onSelectWorkstationRequest"];
  request: DashboardWorkstationRequest;
  selectedRequest?: WorkstationDetailCardProps["selectedRequest"];
  selectedWorkID?: WorkstationDetailCardProps["selectedWorkID"];
}) {
  const requestLabel =
    request.request_id ||
    request.work_items.map(formatWorkItemLabel).join(", ") ||
    request.dispatch_id;
  const primaryWorkItem = request.work_items[0];
  const requestSelected = selectedRequest?.dispatch_id === request.dispatch_id;
  const workLabel = primaryWorkItem
    ? formatWorkItemLabel(primaryWorkItem)
    : requestLabel;
  const totalDurationMillis =
    request.total_duration_millis ?? request.script_response?.duration_millis;
  const normalizedOutcome = (request.outcome ?? request.script_response?.outcome)
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
    <article className={PROVIDER_SESSION_CARD_CLASS}>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <strong className="min-w-0 flex-1 [overflow-wrap:anywhere]">
          {requestLabel}
        </strong>
        <DashboardActionRow
          statuses={renderWorkstationRequestStatusPill({
            hasFailedOutcome,
            messages,
            now,
            request,
            totalDurationMillis,
          })}
          statusesClassName="justify-end"
          actions={renderWorkstationRequestActions({
            messages,
            onSelectWorkID,
            onSelectWorkstationRequest,
            primaryWorkItem,
            request,
            requestLabel,
            requestSelected,
            selectedWorkID,
            workLabel,
          })}
          actionsClassName="justify-end"
          className="justify-end"
        />
      </div>
      {requestSelected ? (
        <p className={REQUEST_SELECTION_STATUS_CLASS}>
          {messages.selectedRequestLabel(request.dispatch_id)}
        </p>
      ) : null}
    </article>
  );
}

function renderWorkstationRequestActions({
  messages,
  onSelectWorkID,
  onSelectWorkstationRequest,
  primaryWorkItem,
  request,
  requestLabel,
  requestSelected,
  selectedWorkID,
  workLabel,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  onSelectWorkID?: WorkstationDetailCardProps["onSelectWorkID"];
  onSelectWorkstationRequest?: WorkstationDetailCardProps["onSelectWorkstationRequest"];
  primaryWorkItem: DashboardWorkstationRequest["work_items"][number] | undefined;
  request: DashboardWorkstationRequest;
  requestLabel: string;
  requestSelected: boolean;
  selectedWorkID?: WorkstationDetailCardProps["selectedWorkID"];
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
      aria-label={messages.selectRequestLabel(requestLabel, request.dispatch_id)}
      aria-pressed={requestSelected}
      onClick={() => onSelectWorkstationRequest(request)}
      type="button"
    >
      {requestSelected
        ? messages.requestSelectedAction
        : messages.openRequestAction}
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
      <DashboardStatusPill
        className={cn(
          "min-h-0",
          !hasFailedOutcome &&
            "border-af-success-border bg-af-success-surface text-af-success",
        )}
        tone={hasFailedOutcome ? "danger" : undefined}
      >
        {messages.totalRuntimeLabel}: {formatDurationMillis(totalDurationMillis)}
      </DashboardStatusPill>
    );
  }

  if (!request.started_at) {
    return undefined;
  }

  return (
    <span className={EXECUTION_PILL_CLASS}>
      {messages.elapsedLabel}: {formatDurationFromISO(request.started_at, now)}
    </span>
  );
}

function WorkstationActiveWorkList({
  executions,
  messages,
  now,
  onSelectWorkID,
  onSelectWorkstationRequest,
  selectedNode,
  selectedRequest,
  selectedWorkID,
  workstationRequestsByDispatchID,
}: WorkstationActiveWorkListProps) {
  const sectionId = `active-work-${selectedNode.node_id}`;

  return (
    <section aria-labelledby={sectionId} className="mt-4 grid gap-2.5 [&_h4]:m-0">
      <CurrentSelectionSectionHeader
        headingId={sectionId}
        title={messages.activeWorkHeading}
      />
      {executions.length > 0 ? (
        <ul className="m-0 grid list-none gap-2.5 p-0">
          {executions.flatMap((execution) => {
            const workItems =
              execution.work_items && execution.work_items.length > 0
                ? execution.work_items
                : [undefined];

            return workItems.map((workItem) => {
              const request =
                workstationRequestsByDispatchID?.[execution.dispatch_id];
              const traceID = workItem?.trace_id ?? execution.trace_ids?.[0];
              const workIdentifier =
                workItem?.work_id ?? traceID ?? messages.unavailableValue;
              const workLabel = workItem
                ? formatWorkItemLabel(workItem)
                : messages.unknownActiveWorkLabel;
              const requestSelected =
                selectedRequest?.dispatch_id === execution.dispatch_id;
              const elapsed = formatDurationFromISO(execution.started_at, now);
              const workAction =
                workItem && onSelectWorkID ? (
                  <DashboardActionButton
                    aria-label={messages.selectWorkItemLabel(workLabel)}
                    aria-pressed={selectedWorkID === workItem.work_id}
                    onClick={() => onSelectWorkID(workItem.work_id)}
                    type="button"
                  >
                    {selectedWorkID === workItem.work_id
                      ? messages.workSelectedAction
                      : messages.openWorkItemAction}
                  </DashboardActionButton>
                ) : null;
              const requestAction = onSelectWorkstationRequest
                ? request ? (
                    <DashboardActionButton
                      aria-label={messages.selectWorkstationRequestLabel(
                        request.dispatch_id,
                      )}
                      aria-pressed={requestSelected}
                      onClick={() => onSelectWorkstationRequest(request)}
                      type="button"
                    >
                      {requestSelected
                        ? messages.requestSelectedAction
                        : messages.openRequestDetailsAction}
                    </DashboardActionButton>
                  ) : null
                : null;
              const headerActions =
                workAction || requestAction ? (
                  <>
                    {workAction}
                    {requestAction}
                  </>
                ) : undefined;
              const headerStatus = (
                <span className={EXECUTION_PILL_CLASS}>
                  {messages.elapsedLabel}: {elapsed}
                </span>
              );

              return (
                <li
                  className={cn(
                    "grid min-w-0 gap-2 rounded-lg border border-af-border bg-af-surface-subtle px-3 py-2",
                    DASHBOARD_BODY_TEXT_CLASS,
                  )}
                  key={`${execution.dispatch_id}-${workIdentifier}`}
                >
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <strong className="min-w-0 flex-1 [overflow-wrap:anywhere]">
                      {workLabel}
                    </strong>
                    <DashboardActionRow
                      statuses={headerStatus}
                      statusesClassName="justify-end"
                      actions={headerActions}
                      actionsClassName="justify-end"
                      className="justify-end"
                    />
                  </div>
                  {workItem ? null : (
                    <p className={REQUEST_SELECTION_STATUS_CLASS}>
                      {messages.workDetailsUnavailable(execution.dispatch_id)}
                    </p>
                  )}
                  {requestSelected ? (
                    <p className={REQUEST_SELECTION_STATUS_CLASS}>
                      {messages.selectedRequestLabel(execution.dispatch_id)}
                    </p>
                  ) : null}
                  {onSelectWorkstationRequest ? (
                    request ? null : (
                      <p className={REQUEST_SELECTION_STATUS_CLASS}>
                        {messages.requestDetailsUnavailable(
                          execution.dispatch_id,
                        )}
                      </p>
                    )
                  ) : null}
                </li>
              );
            });
          })}
        </ul>
      ) : (
        <p className={DETAIL_COPY_CLASS}>{messages.activeWorkEmpty}</p>
      )}
    </section>
  );
}
