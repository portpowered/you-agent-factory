import {
  useEffect,
  useState,
} from "react";

import { formatDurationFromISO, formatList, formatWorkItemLabel } from "../../components/ui/formatters";
import { cx } from "../../lib/cx";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../components/dashboard/typography";
import { DETAIL_COPY_CLASS, WIDGET_SUBTITLE_CLASS } from "../../components/dashboard/widget-board";
import { SelectionDetailLayout } from "./current-selection-detail-layout";
import {
  EXECUTION_PILL_CLASS,
  HISTORY_HEADER_CLASS,
  HISTORY_TOGGLE_CLASS,
  PROVIDER_SESSION_CARD_CLASS,
  REQUEST_SELECTION_STATUS_CLASS,
  RUNTIME_DETAIL_CODE_CLASS,
  WORK_SELECTION_BUTTON_CLASS,
  WORKSTATION_SUMMARY_ITEM_CLASS,
} from "./detail-card-shared";
import type {
  WorkstationActiveWorkListProps,
  WorkstationDetailCardProps,
  WorkstationSummaryItemProps,
  WorkstationSummaryProps,
} from "./detail-card-types";
import {
  CollapsibleProviderSessionAttempts,
} from "./provider-session-attempts";

export function WorkstationDetailCard({
  activeExecutions,
  now,
  onSelectWorkID,
  onSelectWorkstationRequest,
  providerSessions,
  selectedRequest,
  selectedWorkID,
  selectedNode,
  workstationRequests = [],
  widgetId = "current-selection",
}: WorkstationDetailCardProps) {
  const hasProjectedRequestHistory = workstationRequests.length > 0;
  const workstationRequestsByDispatchID = Object.fromEntries(
    workstationRequests.map((request) => [request.dispatch_id, request]),
  );

  return (
    <SelectionDetailLayout widgetId={widgetId}>
      <p className={WIDGET_SUBTITLE_CLASS}>{selectedNode.workstation_name}</p>
      <WorkstationActiveWorkList
        executions={activeExecutions}
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
          hasProjectedRequestHistory ? workstationRequests.length : providerSessions.length
        }
        historyLabel={hasProjectedRequestHistory ? "Historical requests" : "Historical runs"}
        selectedNode={selectedNode}
      />
      {hasProjectedRequestHistory ? (
        <CollapsibleWorkstationRequests
          key={selectedNode.node_id}
          now={now}
          onSelectWorkstationRequest={onSelectWorkstationRequest}
          requests={workstationRequests}
          resetKey={selectedNode.node_id}
        />
      ) : (
        <CollapsibleProviderSessionAttempts
          key={selectedNode.node_id}
          attempts={providerSessions}
          emptyMessage="No workstation runs have been recorded for this workstation yet."
          onSelectWorkID={onSelectWorkID}
          onSelectWorkstationRequest={onSelectWorkstationRequest}
          renderHeading={(attempt) =>
            attempt.work_items?.map(formatWorkItemLabel).join(", ") || "Unknown work"
          }
          resetKey={selectedNode.node_id}
          selectedRequestDispatchID={selectedRequest?.dispatch_id}
          selectedWorkID={selectedWorkID}
          title="Run history"
          workstationKind={selectedNode.workstation_kind}
          workstationRequestsByDispatchID={workstationRequestsByDispatchID}
        />
      )}
    </SelectionDetailLayout>
  );
}

function WorkstationSummary({
  activeRunCount,
  historyCount,
  historyLabel,
  selectedNode,
}: WorkstationSummaryProps) {
  return (
    <section className="mt-4 grid gap-[0.65rem] [&_h4]:m-0">
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>Workstation summary</h4>
      <ul className="m-0 grid list-none gap-2 p-0 [grid-template-columns:repeat(auto-fit,minmax(8.75rem,1fr))]">
        <WorkstationSummaryItem label="Worker type" value={selectedNode.worker_type || "Unknown"} />
        <WorkstationSummaryItem label="Kind" value={selectedNode.workstation_kind || "standard"} />
        <WorkstationSummaryItem
          label="Input work types"
          value={formatList(selectedNode.input_work_type_ids)}
        />
        <WorkstationSummaryItem
          label="Output work types"
          value={formatList(selectedNode.output_work_type_ids)}
        />
        <WorkstationSummaryItem label="Active runs" value={activeRunCount} />
        <WorkstationSummaryItem label={historyLabel} value={historyCount} />
      </ul>
    </section>
  );
}

function WorkstationSummaryItem({ label, value }: WorkstationSummaryItemProps) {
  return (
    <li className={WORKSTATION_SUMMARY_ITEM_CLASS}>
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{label}</span>
      <strong className="min-w-0 text-sm text-af-ink [overflow-wrap:anywhere]">{value}</strong>
    </li>
  );
}

function CollapsibleWorkstationRequests({
  now,
  onSelectWorkstationRequest,
  requests,
  resetKey,
}: {
  now: number;
  onSelectWorkstationRequest?: WorkstationDetailCardProps["onSelectWorkstationRequest"];
  requests: NonNullable<WorkstationDetailCardProps["workstationRequests"]>;
  resetKey: string;
}) {
  const [expanded, setExpanded] = useState(false);
  const historyID = `workstation-request-history-${resetKey}`;
  const itemCountLabel = `${requests.length} ${requests.length === 1 ? "request" : "requests"}`;

  useEffect(() => {
    setExpanded(false);
  }, []);

  return (
    <section aria-labelledby={`${historyID}-heading`} className="mt-4 grid gap-[0.65rem]">
      <div className={HISTORY_HEADER_CLASS}>
        <div className="grid min-w-0 gap-[0.18rem]">
          <h4 className={DASHBOARD_SECTION_HEADING_CLASS} id={`${historyID}-heading`}>
            Request history
          </h4>
          <p className={cx("m-0 text-af-ink/62", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
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
          {expanded ? "Collapse" : "Expand"}
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
                ? `Script command ${request.script_request.command}`
                : request.provider
                  ? `Provider ${request.provider}${request.model ? ` / ${request.model}` : ""}`
                  : "Projected workstation request";

              return (
                <article className={PROVIDER_SESSION_CARD_CLASS} key={request.dispatch_id}>
                  <div className="flex items-start justify-between gap-[0.8rem]">
                    <strong>{requestLabel}</strong>
                    <span className={EXECUTION_PILL_CLASS}>{request.dispatch_id}</span>
                  </div>
                  <div className="mt-[0.45rem] grid gap-[0.18rem]">
                    <p className={cx("m-0 text-af-ink/70", DASHBOARD_BODY_TEXT_CLASS)}>
                      {requestStatus}
                    </p>
                    <p className={cx("m-0 text-af-ink/62", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
                      {requestSummary}
                    </p>
                    {request.started_at ? (
                      <p className={cx("m-0 text-af-ink/62", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
                        Started {formatDurationFromISO(request.started_at, now)} ago
                      </p>
                    ) : null}
                  </div>
                  {onSelectWorkstationRequest ? (
                    <button
                      aria-label={`Select request ${requestLabel} (${request.dispatch_id})`}
                      className={WORK_SELECTION_BUTTON_CLASS}
                      onClick={() => onSelectWorkstationRequest(request)}
                      type="button"
                    >
                      Open request
                    </button>
                  ) : null}
                </article>
              );
            })
          ) : (
            <p className={DETAIL_COPY_CLASS}>
              No workstation requests have been recorded for this workstation yet.
            </p>
          )}
        </div>
      ) : null}
    </section>
  );
}

function WorkstationActiveWorkList({
  executions,
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
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>Active work</h4>
      {executions.length > 0 ? (
        <ul className="m-0 grid list-none gap-[0.65rem] p-0">
          {executions.flatMap((execution) => {
            const workItems =
              execution.work_items && execution.work_items.length > 0
                ? execution.work_items
                : [undefined];

            return workItems.map((workItem) => {
              const request = workstationRequestsByDispatchID?.[execution.dispatch_id];
              const traceID = workItem?.trace_id ?? execution.trace_ids?.[0];
              const workIdentifier = workItem?.work_id ?? traceID ?? "Unavailable";
              const workLabel = workItem ? formatWorkItemLabel(workItem) : "Unknown active work";
              const requestSelected = selectedRequest?.dispatch_id === execution.dispatch_id;

              return (
                <li
                  className={cx(
                    "grid min-w-0 gap-[0.45rem] rounded-lg border border-af-info/20 bg-af-info/8 px-3 py-2",
                    DASHBOARD_BODY_TEXT_CLASS,
                  )}
                  key={`${execution.dispatch_id}-${workIdentifier}`}
                >
                  <strong className="min-w-0 [overflow-wrap:anywhere]">{workLabel}</strong>
                  <dl
                    className={cx(
                      "m-0 grid gap-[0.35rem] [&_dd]:m-0 [&_div]:grid [&_div]:min-w-0 [&_div]:grid-cols-[5.5rem_minmax(0,1fr)] [&_div]:gap-2",
                      DASHBOARD_BODY_TEXT_CLASS,
                    )}
                  >
                    <div>
                      <dt>Work ID</dt>
                      <dd className="[overflow-wrap:anywhere]">{workIdentifier}</dd>
                    </div>
                    {traceID ? (
                      <div>
                        <dt>Trace ID</dt>
                        <dd className="[overflow-wrap:anywhere]">{traceID}</dd>
                      </div>
                    ) : null}
                    <div>
                      <dt>Elapsed</dt>
                      <dd>{formatDurationFromISO(execution.started_at, now)}</dd>
                    </div>
                    <div>
                      <dt>Dispatch</dt>
                      <dd className="[overflow-wrap:anywhere]">{execution.dispatch_id}</dd>
                    </div>
                    <div>
                      <dt>Station</dt>
                      <dd className="[overflow-wrap:anywhere]">
                        {execution.workstation_name ?? selectedNode.workstation_name}
                      </dd>
                    </div>
                  </dl>
                  {workItem && onSelectWorkID ? (
                    <button
                      aria-label={`Select work item ${workLabel}`}
                      aria-pressed={selectedWorkID === workItem.work_id}
                      className={WORK_SELECTION_BUTTON_CLASS}
                      onClick={() => onSelectWorkID(workItem.work_id)}
                      type="button"
                    >
                      {selectedWorkID === workItem.work_id ? "Work selected" : "Open work item"}
                    </button>
                  ) : workItem ? null : (
                    <p className={REQUEST_SELECTION_STATUS_CLASS}>
                      Work details unavailable for dispatch{" "}
                      <code className={RUNTIME_DETAIL_CODE_CLASS}>{execution.dispatch_id}</code>.
                    </p>
                  )}
                  {requestSelected ? (
                    <p className={REQUEST_SELECTION_STATUS_CLASS}>
                      Selected request:{" "}
                      <code className={RUNTIME_DETAIL_CODE_CLASS}>{execution.dispatch_id}</code>.
                    </p>
                  ) : null}
                  {onSelectWorkstationRequest ? (
                    request ? (
                      <button
                        aria-label={`Select workstation request ${request.dispatch_id}`}
                        aria-pressed={requestSelected}
                        className={WORK_SELECTION_BUTTON_CLASS}
                        onClick={() => onSelectWorkstationRequest(request)}
                        type="button"
                      >
                        {requestSelected ? "Request selected" : "Open request details"}
                      </button>
                    ) : (
                      <p className={REQUEST_SELECTION_STATUS_CLASS}>
                        Request details unavailable for dispatch{" "}
                        <code className={RUNTIME_DETAIL_CODE_CLASS}>{execution.dispatch_id}</code>.
                      </p>
                    )
                  ) : null}
                </li>
              );
            });
          })}
        </ul>
      ) : (
        <p className={DETAIL_COPY_CLASS}>No active work is running on this workstation.</p>
      )}
    </section>
  );
}
