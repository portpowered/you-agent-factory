import {
  formatDurationMillis,
  formatProviderSession,
  formatWorkItemLabel,
  getProviderSessionLogTarget,
} from "../../components/dashboard/formatters";
import { cx } from "../../components/dashboard/classnames";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../components/dashboard/typography";
import { DETAIL_COPY_CLASS } from "../../components/dashboard/widget-board";
import {
  EXECUTION_PILL_CLASS,
  INFERENCE_ATTEMPT_DETAIL_CLASS,
  InferenceAttemptDetail,
  PROVIDER_SESSION_CARD_CLASS,
  RequestAuthoredText,
  REQUEST_HISTORY_TEXT_CLASS,
  RUNTIME_DETAIL_CODE_CLASS,
  RUNTIME_DETAIL_VALUE_CLASS,
  TRACE_ACTION_LINK_CLASS,
  WORK_SELECTION_BUTTON_CLASS,
  normalizeDetailText,
} from "./detail-card-shared";
import type {
  SelectedWorkDispatchHistorySectionProps,
  SelectedWorkRequestHistoryItem,
} from "./detail-card-types";
import { ProviderSessionAttempts } from "./provider-session-attempts";

export function SelectedWorkDispatchHistorySection({
  activeTraceID,
  currentDispatchID,
  fallbackProviderSessions,
  onSelectTraceID,
  onSelectWorkID,
  requests,
  selectedWorkID,
  traceTargetId,
  workstationKind,
}: SelectedWorkDispatchHistorySectionProps) {
  if (requests.length === 0 && fallbackProviderSessions.length > 0) {
    return (
      <ProviderSessionAttempts
        attempts={fallbackProviderSessions}
        currentDispatchID={currentDispatchID}
        emptyMessage="No workstation dispatch has been recorded yet for this work item."
        onSelectWorkID={onSelectWorkID}
        renderHeading={(attempt) => attempt.workstation_name || attempt.transition_id}
        selectedWorkID={selectedWorkID}
        title="Workstation dispatches"
        workstationKind={workstationKind}
      />
    );
  }

  return (
    <section
      aria-labelledby="selected-work-dispatch-history-heading"
      className="mt-4 grid gap-[0.65rem]"
    >
      <div className="grid gap-[0.18rem]">
        <h4
          className={DASHBOARD_SECTION_HEADING_CLASS}
          id="selected-work-dispatch-history-heading"
        >
          Workstation dispatches
        </h4>
        <p className={cx("m-0 text-af-ink/62", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
          {requests.length} {requests.length === 1 ? "dispatch" : "dispatches"}
        </p>
      </div>
      {requests.length > 0 ? (
        <div className="grid gap-[0.8rem]">
          {requests.map((request) => (
            <DispatchHistoryCard
              activeTraceID={activeTraceID}
              currentDispatchID={currentDispatchID}
              key={request.dispatch_id}
              onSelectTraceID={onSelectTraceID}
              onSelectWorkID={onSelectWorkID}
              request={request}
              selectedWorkID={selectedWorkID}
              traceTargetId={traceTargetId}
            />
          ))}
        </div>
      ) : (
        <p className={DETAIL_COPY_CLASS}>
          No workstation dispatch has been recorded yet for this work item.
        </p>
      )}
    </section>
  );
}

function DispatchHistoryCard({
  activeTraceID,
  currentDispatchID,
  onSelectTraceID,
  onSelectWorkID,
  request,
  selectedWorkID,
  traceTargetId,
}: {
  activeTraceID?: string | null;
  currentDispatchID?: string | null;
  onSelectTraceID?: (traceID: string) => void;
  onSelectWorkID?: (workID: string) => void;
  request: SelectedWorkRequestHistoryItem;
  selectedWorkID: string;
  traceTargetId: string;
}) {
  const counts = requestCounts(request);
  const inputWorkItems = dedupeWorkItems(requestInputWorkItems(request));
  const outputWorkItems = dedupeWorkItems(requestOutputWorkItems(request));
  const prompt = normalizeDetailText(requestPrompt(request));
  const responseText = normalizeDetailText(requestResponseText(request));
  const failureReason = normalizeDetailText(requestFailureReason(request));
  const failureMessage = normalizeDetailText(requestFailureMessage(request));
  const errorClass = normalizeDetailText(requestErrorClass(request));
  const scriptRequest = requestScriptRequest(request);
  const scriptResponse = requestScriptResponse(request);
  const failureType = normalizeDetailText(scriptResponse?.failure_type);
  const normalizedScriptStdout = normalizeDetailText(scriptResponse?.stdout);
  const normalizedScriptStderr = normalizeDetailText(scriptResponse?.stderr);
  const isScriptBackedRequest = scriptRequest !== undefined || scriptResponse !== undefined;
  const traceIDs = requestTraceIDs(request);
  const providerSession = requestProviderSession(request);
  const providerSessionLogTarget = getProviderSessionLogTarget(
    providerSession,
    requestStartedAt(request),
  );
  const isCurrentDispatch = currentDispatchID === request.dispatch_id;
  const outcome = requestOutcome(request);
  const durationMillis = requestDurationMillis(request);
  const responseUnavailableCopy = responseText
    ? undefined
    : failureReason || failureMessage || errorClass
      ? "Response text is unavailable because this dispatch ended with an error."
      : hasResponseDetails(request)
        ? "Response text is unavailable for this dispatch."
        : "No response yet for this dispatch.";

  return (
    <article
      className={cx(
        PROVIDER_SESSION_CARD_CLASS,
        isCurrentDispatch && "border-af-accent/30 bg-af-accent/6",
      )}
    >
      <div className="flex items-start justify-between gap-[0.8rem]">
        <div className="grid min-w-0 gap-[0.18rem]">
          <strong className="min-w-0 [overflow-wrap:anywhere]">
            {request.workstation_name || request.transition_id}
          </strong>
          <div className="flex flex-wrap items-center gap-[0.45rem]">
            <p className={cx("m-0 text-af-ink/70", DASHBOARD_BODY_TEXT_CLASS)}>
              {outcome ?? "PENDING"}
            </p>
            {isCurrentDispatch ? (
              <span
                className={cx(
                  "inline-flex rounded-full border border-af-accent/35 bg-af-accent/10 px-2 py-[0.18rem] text-af-accent",
                  DASHBOARD_SUPPORTING_TEXT_CLASS,
                )}
              >
                Current dispatch
              </span>
            ) : null}
          </div>
        </div>
        <span className={EXECUTION_PILL_CLASS}>{request.dispatch_id}</span>
      </div>
      <dl className={cx("mt-[0.65rem]", INFERENCE_ATTEMPT_DETAIL_CLASS)}>
        <InferenceAttemptDetail label="Workstation" value={request.workstation_name} />
        <InferenceAttemptDetail label="Transition ID" code value={request.transition_id} />
        <InferenceAttemptDetail label="Provider" code value={requestProvider(request)} />
        <InferenceAttemptDetail label="Model" code value={requestModel(request)} />
        <InferenceAttemptDetail
          label="dispatchedCount"
          value={counts.dispatchedCount}
        />
        <InferenceAttemptDetail
          label="respondedCount"
          value={counts.respondedCount}
        />
        <InferenceAttemptDetail
          label="erroredCount"
          value={counts.erroredCount}
        />
        <InferenceAttemptDetail label="Started at" value={requestStartedAt(request)} />
        <InferenceAttemptDetail
          label="Duration"
          value={
            durationMillis !== undefined ? formatDurationMillis(durationMillis) : undefined
          }
        />
      </dl>
      <DispatchDetailSection title="Request details">
        {prompt ? (
          <RequestAuthoredText value={prompt} />
        ) : isScriptBackedRequest ? (
          <p className={DETAIL_COPY_CLASS}>
            Prompt details are not applicable to this script-backed dispatch.
          </p>
        ) : (
          <p className={DETAIL_COPY_CLASS}>
            Prompt details are not available for this dispatch yet.
          </p>
        )}
        <DispatchDetailList
          entries={[
            {
              label: "Working directory",
              value: requestWorkingDirectory(request),
              code: true,
            },
            {
              label: "Worktree",
              value: requestWorktree(request),
              code: true,
            },
            {
              label: "Script request ID",
              value: scriptRequest?.script_request_id,
              code: true,
            },
            {
              label: "Script attempt",
              value:
                scriptRequest?.attempt !== undefined
                  ? String(scriptRequest.attempt)
                  : undefined,
            },
            {
              label: "Command",
              value: scriptRequest?.command,
              code: true,
            },
          ]}
        />
        <ScriptArgsSection args={scriptRequest?.args} />
        <WorkItemActionGroup
          items={inputWorkItems}
          label="Input work"
          onSelectWorkID={onSelectWorkID}
          selectedWorkID={selectedWorkID}
        />
      </DispatchDetailSection>
      <DispatchDetailSection title="Response details">
        {isScriptBackedRequest ? (
          <>
            {scriptResponse ? (
              <>
                <DispatchDetailList
                  entries={[
                    {
                      label: "Script request ID",
                      value: scriptResponse.script_request_id,
                      code: true,
                    },
                    {
                      label: "Script attempt",
                      value:
                        scriptResponse.attempt !== undefined
                          ? String(scriptResponse.attempt)
                          : undefined,
                    },
                    {
                      label: "Outcome",
                      value: scriptResponse.outcome,
                    },
                    {
                      label: "Duration",
                      value:
                        scriptResponse.duration_millis !== undefined
                          ? formatDurationMillis(scriptResponse.duration_millis)
                          : undefined,
                    },
                    {
                      label: "Exit code",
                      value:
                        scriptResponse.exit_code !== undefined
                          ? String(scriptResponse.exit_code)
                          : undefined,
                    },
                    {
                      label: "Failure type",
                      value: scriptResponse.failure_type,
                    },
                  ]}
                />
                <ScriptOutputSection
                  emptyMessage="No stdout was recorded for this script response."
                  label="Stdout"
                  value={normalizedScriptStdout}
                />
                <ScriptOutputSection
                  emptyMessage="No stderr was recorded for this script response."
                  label="Stderr"
                  value={normalizedScriptStderr}
                />
              </>
            ) : (
              <p className={DETAIL_COPY_CLASS}>No script response yet for this dispatch.</p>
            )}
          </>
        ) : responseText ? (
          <pre className={REQUEST_HISTORY_TEXT_CLASS}>{responseText}</pre>
        ) : (
          <p className={DETAIL_COPY_CLASS}>{responseUnavailableCopy}</p>
        )}
        {isScriptBackedRequest ? null : (
          <DispatchDetailList
            entries={[
              {
                label: "Provider session",
                value: providerSession ? formatProviderSession(providerSession) : undefined,
                code: !providerSessionLogTarget,
                href: providerSessionLogTarget?.href,
                title: providerSessionLogTarget?.display,
              },
            ]}
          />
        )}
        <WorkItemActionGroup
          items={outputWorkItems}
          label="Output work"
          onSelectWorkID={onSelectWorkID}
          selectedWorkID={selectedWorkID}
        />
        <TraceActionGroup
          activeTraceID={activeTraceID}
          onSelectTraceID={onSelectTraceID}
          traceIDs={traceIDs}
          traceTargetId={traceTargetId}
        />
      </DispatchDetailSection>
      {failureReason || failureMessage || errorClass ? (
        <DispatchDetailSection title="Failure details">
        <DispatchDetailList
          entries={[
            { label: "Failure reason", value: failureReason },
            { label: "Failure message", value: failureMessage },
            { label: "Failure type", code: true, value: failureType },
            { label: "Error class", code: true, value: errorClass },
          ]}
        />
      </DispatchDetailSection>
      ) : null}
    </article>
  );
}

function ScriptArgsSection({
  args,
}: {
  args: string[] | undefined;
}) {
  if (!args || args.length === 0) {
    return null;
  }

  return (
    <div className="grid gap-[0.3rem]">
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>Resolved args</span>
      <div className="grid gap-[0.25rem]">
        {args.map((arg) => (
          <code className={RUNTIME_DETAIL_CODE_CLASS} key={arg}>
            {arg}
          </code>
        ))}
      </div>
    </div>
  );
}

function ScriptOutputSection({
  emptyMessage,
  label,
  value,
}: {
  emptyMessage: string;
  label: string;
  value: string | undefined;
}) {
  return (
    <div className="grid gap-[0.3rem]">
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{label}</span>
      {value ? (
        <pre className={REQUEST_HISTORY_TEXT_CLASS}>{value}</pre>
      ) : (
        <p className={DETAIL_COPY_CLASS}>{emptyMessage}</p>
      )}
    </div>
  );
}

function DispatchDetailSection({
  children,
  title,
}: {
  children: React.ReactNode;
  title: string;
}) {
  return (
    <section
      aria-label={title}
      className="mt-[0.75rem] grid gap-[0.45rem] border-t border-af-overlay/8 pt-[0.75rem]"
    >
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{title}</span>
      {children}
    </section>
  );
}

function DispatchDetailList({
  entries,
}: {
  entries: Array<{
    code?: boolean;
    href?: string;
    label: string;
    title?: string;
    value?: string;
  }>;
}) {
  const populatedEntries = entries.filter((entry) => entry.value);
  if (populatedEntries.length === 0) {
    return null;
  }

  return (
    <dl className={INFERENCE_ATTEMPT_DETAIL_CLASS}>
      {populatedEntries.map((entry) => (
        <InferenceAttemptDetailLink
          code={entry.code}
          href={entry.href}
          key={entry.label}
          label={entry.label}
          title={entry.title}
          value={entry.value}
        />
      ))}
    </dl>
  );
}

function InferenceAttemptDetailLink({
  code = false,
  href,
  label,
  title,
  value,
}: {
  code?: boolean;
  href?: string;
  label: string;
  title?: string;
  value?: string;
}) {
  if (!value) {
    return null;
  }

  return (
    <div>
      <dt>{label}</dt>
      <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
        {href ? (
          <a className={TRACE_ACTION_LINK_CLASS} href={href} title={title}>
            {value}
          </a>
        ) : code ? (
          <code className={RUNTIME_DETAIL_CODE_CLASS}>{value}</code>
        ) : (
          value
        )}
      </dd>
    </div>
  );
}

function WorkItemActionGroup({
  items,
  label,
  onSelectWorkID,
  selectedWorkID,
}: {
  items: ReturnType<typeof dedupeWorkItems>;
  label: string;
  onSelectWorkID?: (workID: string) => void;
  selectedWorkID: string;
}) {
  if (items.length === 0) {
    return null;
  }

  return (
    <div className="grid gap-[0.3rem]">
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{label}</span>
      <div className="flex flex-wrap gap-[0.45rem]">
        {items.map((workItem) => (
          <button
            aria-label={`Select work item ${formatWorkItemLabel(workItem)}`}
            aria-pressed={selectedWorkID === workItem.work_id}
            className={WORK_SELECTION_BUTTON_CLASS}
            key={`${label}-${workItem.work_id}`}
            onClick={() => onSelectWorkID?.(workItem.work_id)}
            type="button"
          >
            {selectedWorkID === workItem.work_id
              ? "Work selected"
              : `Open ${formatWorkItemLabel(workItem)}`}
          </button>
        ))}
      </div>
    </div>
  );
}

function TraceActionGroup({
  activeTraceID,
  onSelectTraceID,
  traceIDs,
  traceTargetId,
}: {
  activeTraceID?: string | null;
  onSelectTraceID?: (traceID: string) => void;
  traceIDs: string[];
  traceTargetId: string;
}) {
  if (traceIDs.length === 0) {
    return null;
  }

  return (
    <div className="grid gap-[0.3rem]">
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>Trace IDs</span>
      <div className="flex flex-wrap gap-[0.45rem]">
        {traceIDs.map((traceID) => (
          <a
            className={TRACE_ACTION_LINK_CLASS}
            href={`#${traceTargetId}`}
            key={traceID}
            onClick={() => onSelectTraceID?.(traceID)}
          >
            {traceID}
            {activeTraceID === traceID ? " (selected)" : ""}
          </a>
        ))}
      </div>
    </div>
  );
}

function isProjectedWorkstationRequest(
  request: SelectedWorkRequestHistoryItem,
): request is Extract<SelectedWorkRequestHistoryItem, { workstation_node_id: string }> {
  return "workstation_node_id" in request;
}

function requestCounts(request: SelectedWorkRequestHistoryItem) {
  if (isProjectedWorkstationRequest(request)) {
    return {
      dispatchedCount:
        request.counts?.dispatched_count ?? request.dispatched_request_count ?? 0,
      erroredCount: request.counts?.errored_count ?? request.errored_request_count ?? 0,
      respondedCount:
        request.counts?.responded_count ?? request.responded_request_count ?? 0,
    };
  }

  return {
    dispatchedCount: request.counts.dispatched_count,
    erroredCount: request.counts.errored_count,
    respondedCount: request.counts.responded_count,
  };
}

function requestInputWorkItems(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.request_view?.input_work_items ?? []
    : request.request.input_work_items ?? [];
}

function requestOutputWorkItems(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.response_view?.output_work_items ?? []
    : request.response?.output_work_items ?? [];
}

function requestPrompt(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.prompt ?? request.request_view?.prompt
    : request.request.prompt;
}

function requestProvider(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.provider ?? request.request_view?.provider
    : request.request.provider;
}

function requestModel(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.model ?? request.request_view?.model
    : request.request.model;
}

function requestTraceIDs(request: SelectedWorkRequestHistoryItem) {
  const traceIDs = isProjectedWorkstationRequest(request)
    ? [
        ...(request.trace_ids ?? []),
        ...(request.request_view?.trace_ids ?? []),
      ]
    : request.request.trace_ids ?? [];

  return [...new Set(traceIDs.filter(Boolean))];
}

function requestStartedAt(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.started_at ?? request.request_view?.started_at ?? request.request_view?.request_time
    : request.request.started_at ?? request.request.request_time;
}

function requestDurationMillis(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.total_duration_millis ??
        request.response_view?.duration_millis ??
        request.script_response?.duration_millis ??
        request.response_view?.script_response?.duration_millis
    : request.response?.duration_millis ?? request.response?.script_response?.duration_millis;
}

function requestWorkingDirectory(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.working_directory ?? request.request_view?.working_directory
    : request.request.working_directory;
}

function requestWorktree(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.worktree ?? request.request_view?.worktree
    : request.request.worktree;
}

function requestOutcome(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.outcome ??
        request.response_view?.outcome ??
        request.script_response?.outcome ??
        request.response_view?.script_response?.outcome
    : request.response?.outcome ?? request.response?.script_response?.outcome;
}

function requestProviderSession(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.provider_session ?? request.response_view?.provider_session
    : request.response?.provider_session;
}

function requestResponseText(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.response ?? request.response_view?.response_text
    : request.response?.response_text;
}

function requestFailureReason(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.failure_reason ?? request.response_view?.failure_reason
    : request.response?.failure_reason;
}

function requestFailureMessage(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.failure_message ?? request.response_view?.failure_message
    : request.response?.failure_message;
}

function requestErrorClass(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.response_view?.error_class
    : request.response?.error_class;
}

function requestScriptRequest(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.script_request ?? request.request_view?.script_request
    : request.request.script_request;
}

function requestScriptResponse(request: SelectedWorkRequestHistoryItem) {
  return isProjectedWorkstationRequest(request)
    ? request.script_response ?? request.response_view?.script_response
    : request.response?.script_response;
}

function hasResponseDetails(request: SelectedWorkRequestHistoryItem) {
  return Boolean(
    requestOutcome(request) ||
      requestScriptResponse(request) ||
      requestProviderSession(request)?.id ||
      requestResponseText(request) ||
      requestFailureReason(request) ||
      requestFailureMessage(request) ||
      requestErrorClass(request) ||
      requestOutputWorkItems(request).length > 0,
  );
}

function dedupeWorkItems<TWorkItem extends { work_id: string }>(workItems: TWorkItem[]) {
  return [...new Map(workItems.map((workItem) => [workItem.work_id, workItem])).values()];
}
