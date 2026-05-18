import { useEffect, useState } from "react";
import {
  DETAIL_COPY_CLASS,
  WIDGET_SUBTITLE_CLASS,
} from "../../components/dashboard/widget-board";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../components/ui/dashboard-typography";
import {
  formatDurationFromISO,
  formatWorkItemLabel,
} from "../../components/ui/formatters";
import { cx } from "../../lib/cx";
import { SelectionDetailLayout } from "./current-selection-detail-layout";
import {
  EXECUTION_PILL_CLASS,
  HISTORY_HEADER_CLASS,
  HISTORY_TOGGLE_CLASS,
  PROVIDER_SESSION_CARD_CLASS,
  REQUEST_SELECTION_STATUS_CLASS,
  WORK_SELECTION_BUTTON_CLASS,
} from "./detail-card-shared";
import type {
  WorkstationActiveWorkListProps,
  WorkstationDetailCardProps,
} from "./detail-card-types";
import { getWorkstationDetailMessages } from "./messages";
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
  onSelectWorkID,
  onSelectWorkstationRequest,
  providerSessions,
  saveState,
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
      <EditableConfigurationSection
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
      <WorkstationSummary
        activeRunCount={activeExecutions.length}
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
      {hasProjectedRequestHistory ? (
        <CollapsibleWorkstationRequests
          key={selectedNode.node_id}
          messages={messages}
          now={now}
          onSelectWorkstationRequest={onSelectWorkstationRequest}
          requests={workstationRequests}
          resetKey={selectedNode.node_id}
        />
      ) : (
        <CollapsibleProviderSessionAttempts
          key={selectedNode.node_id}
          attempts={providerSessions}
          collapseActionLabel={messages.collapseAction}
          emptyMessage={messages.noWorkstationRuns}
          expandActionLabel={messages.expandAction}
          historyItemCountLabel={messages.historyRunCountLabel}
          messages={messages}
          onSelectWorkID={onSelectWorkID}
          onSelectWorkstationRequest={onSelectWorkstationRequest}
          renderHeading={(attempt) =>
            attempt.work_items?.map(formatWorkItemLabel).join(", ") ||
            messages.unknownWorkLabel
          }
          resetKey={selectedNode.node_id}
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
  onSelectWorkstationRequest,
  requests,
  resetKey,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  now: number;
  onSelectWorkstationRequest?: WorkstationDetailCardProps["onSelectWorkstationRequest"];
  requests: NonNullable<WorkstationDetailCardProps["workstationRequests"]>;
  resetKey: string;
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
      className="mt-4 grid gap-[0.65rem]"
    >
      <div className={HISTORY_HEADER_CLASS}>
        <div className="grid min-w-0 gap-[0.18rem]">
          <h4
            className={DASHBOARD_SECTION_HEADING_CLASS}
            id={`${historyID}-heading`}
          >
            {messages.requestHistoryHeading}
          </h4>
          <p
            className={cx(
              "m-0 text-af-ink/62",
              DASHBOARD_SUPPORTING_TEXT_CLASS,
            )}
          >
            {itemCountLabel}
          </p>
        </div>
        <button
          aria-controls={historyID}
          aria-expanded={expanded}
          className={HISTORY_TOGGLE_CLASS}
          onClick={() => setExpanded((current) => !current)}
          type="button"
        >
          {expanded ? messages.collapseAction : messages.expandAction}
        </button>
      </div>
      {expanded ? (
        <div className="grid gap-[0.8rem]" id={historyID}>
          {requests.length > 0 ? (
            requests.map((request) => {
              const requestLabel =
                request.request_id ||
                request.work_items.map(formatWorkItemLabel).join(", ") ||
                request.dispatch_id;
              const requestStatus =
                request.script_response?.outcome ??
                request.outcome ??
                (request.errored_request_count > 0
                  ? "FAILED"
                  : request.responded_request_count > 0
                    ? "RESPONDED"
                    : "PENDING");
              const requestSummary = request.script_request
                ? messages.scriptCommandSummary(
                    request.script_request.command ?? messages.unavailableValue,
                  )
                : request.provider != null
                  ? messages.providerSummary(request.provider, request.model)
                  : messages.projectedWorkstationRequestSummary;

              return (
                <article
                  className={PROVIDER_SESSION_CARD_CLASS}
                  key={request.dispatch_id}
                >
                  <div className="flex items-start justify-between gap-[0.8rem]">
                    <strong>{requestLabel}</strong>
                    <span className={EXECUTION_PILL_CLASS}>
                      {request.dispatch_id}
                    </span>
                  </div>
                  <div className="mt-[0.45rem] grid gap-[0.18rem]">
                    <p
                      className={cx(
                        "m-0 text-af-ink/70",
                        DASHBOARD_BODY_TEXT_CLASS,
                      )}
                    >
                      {requestStatus}
                    </p>
                    <p
                      className={cx(
                        "m-0 text-af-ink/62",
                        DASHBOARD_SUPPORTING_TEXT_CLASS,
                      )}
                    >
                      {requestSummary}
                    </p>
                    {request.started_at ? (
                      <p
                        className={cx(
                          "m-0 text-af-ink/62",
                          DASHBOARD_SUPPORTING_TEXT_CLASS,
                        )}
                      >
                        {messages.requestStatusStartedAgo(
                          formatDurationFromISO(request.started_at, now),
                        )}
                      </p>
                    ) : null}
                  </div>
                  {onSelectWorkstationRequest ? (
                    <button
                      aria-label={messages.selectRequestLabel(
                        requestLabel,
                        request.dispatch_id,
                      )}
                      className={WORK_SELECTION_BUTTON_CLASS}
                      onClick={() => onSelectWorkstationRequest(request)}
                      type="button"
                    >
                      {messages.openRequestAction}
                    </button>
                  ) : null}
                </article>
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
  return (
    <section className="mt-4 grid gap-[0.65rem] [&_h4]:m-0">
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>
        {messages.activeWorkHeading}
      </h4>
      {executions.length > 0 ? (
        <ul className="m-0 grid list-none gap-[0.65rem] p-0">
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

              return (
                <li
                  className={cx(
                    "grid min-w-0 gap-[0.45rem] rounded-lg border border-af-overlay/8 bg-af-overlay/4 px-3 py-2",
                    DASHBOARD_BODY_TEXT_CLASS,
                  )}
                  key={`${execution.dispatch_id}-${workIdentifier}`}
                >
                  <strong className="min-w-0 [overflow-wrap:anywhere]">
                    {workLabel}
                  </strong>
                  <dl
                    className={cx(
                      "m-0 grid gap-[0.35rem] [&_dd]:m-0 [&_div]:grid [&_div]:min-w-0 [&_div]:grid-cols-[5.5rem_minmax(0,1fr)] [&_div]:gap-2",
                      DASHBOARD_BODY_TEXT_CLASS,
                    )}
                  >
                    <div>
                      <dt>{messages.workIdLabel}</dt>
                      <dd className="[overflow-wrap:anywhere]">
                        {workIdentifier}
                      </dd>
                    </div>
                    {traceID ? (
                      <div>
                        <dt>{messages.traceIdLabel}</dt>
                        <dd className="[overflow-wrap:anywhere]">{traceID}</dd>
                      </div>
                    ) : null}
                    <div>
                      <dt>{messages.elapsedLabel}</dt>
                      <dd>
                        {formatDurationFromISO(execution.started_at, now)}
                      </dd>
                    </div>
                    <div>
                      <dt>{messages.dispatchLabel}</dt>
                      <dd className="[overflow-wrap:anywhere]">
                        {execution.dispatch_id}
                      </dd>
                    </div>
                    <div>
                      <dt>{messages.stationLabel}</dt>
                      <dd className="[overflow-wrap:anywhere]">
                        {execution.workstation_name ??
                          selectedNode.workstation_name}
                      </dd>
                    </div>
                  </dl>
                  {workItem && onSelectWorkID ? (
                    <button
                      aria-label={messages.selectWorkItemLabel(workLabel)}
                      aria-pressed={selectedWorkID === workItem.work_id}
                      className={WORK_SELECTION_BUTTON_CLASS}
                      onClick={() => onSelectWorkID(workItem.work_id)}
                      type="button"
                    >
                      {selectedWorkID === workItem.work_id
                        ? messages.workSelectedAction
                        : messages.openWorkItemAction}
                    </button>
                  ) : workItem ? null : (
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
                    request ? (
                      <button
                        aria-label={messages.selectWorkstationRequestLabel(
                          request.dispatch_id,
                        )}
                        aria-pressed={requestSelected}
                        className={WORK_SELECTION_BUTTON_CLASS}
                        onClick={() => onSelectWorkstationRequest(request)}
                        type="button"
                      >
                        {requestSelected
                          ? messages.requestSelectedAction
                          : messages.openRequestDetailsAction}
                      </button>
                    ) : (
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
