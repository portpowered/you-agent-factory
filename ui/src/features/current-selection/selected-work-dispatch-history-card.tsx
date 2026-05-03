import { cx } from "../../lib/cx";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../components/dashboard/typography";
import { DETAIL_COPY_CLASS } from "../../components/dashboard/widget-board";
import {
  formatDurationMillis,
  formatProviderSession,
  getProviderSessionLogTarget,
} from "../../components/ui/formatters";
import {
  EXECUTION_PILL_CLASS,
  INFERENCE_ATTEMPT_DETAIL_CLASS,
  InferenceAttemptDetail,
  PROVIDER_SESSION_CARD_CLASS,
  RequestAuthoredText,
  REQUEST_HISTORY_TEXT_CLASS,
  normalizeDetailText,
} from "./detail-card-shared";
import type { SelectedWorkRequestHistoryItem } from "./detail-card-types";
import {
  DispatchDetailList,
  DispatchDetailSection,
  ScriptArgsSection,
  ScriptOutputSection,
  TraceActionGroup,
  WorkItemActionGroup,
} from "./selected-work-dispatch-history-card-shared";
import {
  dedupeWorkItems,
  hasResponseDetails,
  requestCounts,
  requestDurationMillis,
  requestErrorClass,
  requestFailureMessage,
  requestFailureReason,
  requestInputWorkItems,
  requestModel,
  requestOutcome,
  requestOutputWorkItems,
  requestPrompt,
  requestProvider,
  requestProviderSession,
  requestResponseText,
  requestScriptRequest,
  requestScriptResponse,
  requestStartedAt,
  requestTraceIDs,
  requestWorkingDirectory,
  requestWorktree,
} from "./selected-work-dispatch-history-helpers";

interface DispatchHistoryCardProps {
  activeTraceID?: string | null;
  currentDispatchID?: string | null;
  onSelectTraceID?: (traceID: string) => void;
  onSelectWorkID?: (workID: string) => void;
  request: SelectedWorkRequestHistoryItem;
  selectedWorkID: string;
  traceTargetId: string;
}

export function DispatchHistoryCard({
  activeTraceID,
  currentDispatchID,
  onSelectTraceID,
  onSelectWorkID,
  request,
  selectedWorkID,
  traceTargetId,
}: DispatchHistoryCardProps) {
  const view = buildDispatchHistoryView(request);
  const isCurrentDispatch = currentDispatchID === request.dispatch_id;

  return (
    <article
      className={cx(
        PROVIDER_SESSION_CARD_CLASS,
        isCurrentDispatch && "border-af-accent/30 bg-af-accent/6",
      )}
    >
      <DispatchHistoryHeader
        dispatchID={request.dispatch_id}
        isCurrentDispatch={isCurrentDispatch}
        outcome={view.outcome}
        workstationLabel={request.workstation_name || request.transition_id}
      />
      <DispatchSummaryDetails request={request} view={view} />
      <DispatchRequestSection
        onSelectWorkID={onSelectWorkID}
        request={request}
        selectedWorkID={selectedWorkID}
        view={view}
      />
      <DispatchResponseSection
        activeTraceID={activeTraceID}
        onSelectTraceID={onSelectTraceID}
        onSelectWorkID={onSelectWorkID}
        selectedWorkID={selectedWorkID}
        traceTargetId={traceTargetId}
        view={view}
      />
      {view.hasFailureDetails ? <DispatchFailureSection view={view} /> : null}
    </article>
  );
}

interface DispatchHistoryView {
  counts: ReturnType<typeof requestCounts>;
  durationMillis: number | undefined;
  errorClass: string | undefined;
  failureMessage: string | undefined;
  failureReason: string | undefined;
  failureType: string | undefined;
  hasFailureDetails: boolean;
  inputWorkItems: ReturnType<typeof dedupeWorkItems>;
  isScriptBackedRequest: boolean;
  normalizedScriptStderr: string | undefined;
  normalizedScriptStdout: string | undefined;
  outcome: string | undefined;
  outputWorkItems: ReturnType<typeof dedupeWorkItems>;
  prompt: string | undefined;
  providerSession: ReturnType<typeof requestProviderSession>;
  providerSessionLogTarget: ReturnType<typeof getProviderSessionLogTarget>;
  responseText: string | undefined;
  responseUnavailableCopy: string;
  scriptRequest: ReturnType<typeof requestScriptRequest>;
  scriptResponse: ReturnType<typeof requestScriptResponse>;
  traceIDs: string[];
}

function buildDispatchHistoryView(request: SelectedWorkRequestHistoryItem): DispatchHistoryView {
  const failureReason = normalizeDetailText(requestFailureReason(request));
  const failureMessage = normalizeDetailText(requestFailureMessage(request));
  const errorClass = normalizeDetailText(requestErrorClass(request));
  const scriptRequest = requestScriptRequest(request);
  const scriptResponse = requestScriptResponse(request);
  const responseText = normalizeDetailText(requestResponseText(request));
  const hasFailureDetails = Boolean(failureReason || failureMessage || errorClass);
  const isScriptBackedRequest = scriptRequest !== undefined || scriptResponse !== undefined;
  const providerSession = requestProviderSession(request);

  return {
    counts: requestCounts(request),
    durationMillis: requestDurationMillis(request),
    errorClass,
    failureMessage,
    failureReason,
    failureType: normalizeDetailText(scriptResponse?.failure_type),
    hasFailureDetails,
    inputWorkItems: dedupeWorkItems(requestInputWorkItems(request)),
    isScriptBackedRequest,
    normalizedScriptStderr: normalizeDetailText(scriptResponse?.stderr),
    normalizedScriptStdout: normalizeDetailText(scriptResponse?.stdout),
    outcome: requestOutcome(request),
    outputWorkItems: dedupeWorkItems(requestOutputWorkItems(request)),
    prompt: normalizeDetailText(requestPrompt(request)),
    providerSession,
    providerSessionLogTarget: getProviderSessionLogTarget(
      providerSession,
      requestStartedAt(request),
    ),
    responseText,
    responseUnavailableCopy: responseText
      ? ""
      : hasFailureDetails
        ? "Response text is unavailable because this dispatch ended with an error."
        : hasResponseDetails(request)
          ? "Response text is unavailable for this dispatch."
          : "No response yet for this dispatch.",
    scriptRequest,
    scriptResponse,
    traceIDs: requestTraceIDs(request),
  };
}

function DispatchHistoryHeader({
  dispatchID,
  isCurrentDispatch,
  outcome,
  workstationLabel,
}: {
  dispatchID: string | undefined;
  isCurrentDispatch: boolean;
  outcome: string | undefined;
  workstationLabel: string | undefined;
}) {
  return (
    <div className="flex items-start justify-between gap-[0.8rem]">
      <div className="grid min-w-0 gap-[0.18rem]">
        <strong className="min-w-0 [overflow-wrap:anywhere]">
          {workstationLabel || dispatchID || "Unknown dispatch"}
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
      <span className={EXECUTION_PILL_CLASS}>{dispatchID || "unknown-dispatch"}</span>
    </div>
  );
}

function DispatchSummaryDetails({
  request,
  view,
}: {
  request: SelectedWorkRequestHistoryItem;
  view: DispatchHistoryView;
}) {
  return (
    <dl className={cx("mt-[0.65rem]", INFERENCE_ATTEMPT_DETAIL_CLASS)}>
      <InferenceAttemptDetail label="Workstation" value={request.workstation_name} />
      <InferenceAttemptDetail label="Transition ID" code value={request.transition_id} />
      <InferenceAttemptDetail label="Provider" code value={requestProvider(request)} />
      <InferenceAttemptDetail label="Model" code value={requestModel(request)} />
      <InferenceAttemptDetail label="dispatchedCount" value={view.counts.dispatchedCount} />
      <InferenceAttemptDetail label="respondedCount" value={view.counts.respondedCount} />
      <InferenceAttemptDetail label="erroredCount" value={view.counts.erroredCount} />
      <InferenceAttemptDetail label="Started at" value={requestStartedAt(request)} />
      <InferenceAttemptDetail
        label="Duration"
        value={view.durationMillis !== undefined ? formatDurationMillis(view.durationMillis) : undefined}
      />
    </dl>
  );
}

function DispatchRequestSection({
  onSelectWorkID,
  request,
  selectedWorkID,
  view,
}: {
  onSelectWorkID?: (workID: string) => void;
  request: SelectedWorkRequestHistoryItem;
  selectedWorkID: string;
  view: DispatchHistoryView;
}) {
  return (
    <DispatchDetailSection title="Request details">
      {view.prompt ? (
        <RequestAuthoredText value={view.prompt} />
      ) : view.isScriptBackedRequest ? (
        <p className={DETAIL_COPY_CLASS}>
          Prompt details are not applicable to this script-backed dispatch.
        </p>
      ) : (
        <p className={DETAIL_COPY_CLASS}>Prompt details are not available for this dispatch yet.</p>
      )}
      <DispatchDetailList
        entries={[
          { label: "Working directory", value: requestWorkingDirectory(request), code: true },
          { label: "Worktree", value: requestWorktree(request), code: true },
          {
            label: "Script request ID",
            value: view.scriptRequest?.script_request_id,
            code: true,
          },
          {
            label: "Script attempt",
            value:
              view.scriptRequest?.attempt !== undefined ? String(view.scriptRequest.attempt) : undefined,
          },
          { label: "Command", value: view.scriptRequest?.command, code: true },
        ]}
      />
      <ScriptArgsSection args={view.scriptRequest?.args} />
      <WorkItemActionGroup
        items={view.inputWorkItems}
        label="Input work"
        onSelectWorkID={onSelectWorkID}
        selectedWorkID={selectedWorkID}
      />
    </DispatchDetailSection>
  );
}

function DispatchResponseSection({
  activeTraceID,
  onSelectTraceID,
  onSelectWorkID,
  selectedWorkID,
  traceTargetId,
  view,
}: {
  activeTraceID?: string | null;
  onSelectTraceID?: (traceID: string) => void;
  onSelectWorkID?: (workID: string) => void;
  selectedWorkID: string;
  traceTargetId: string;
  view: DispatchHistoryView;
}) {
  return (
    <DispatchDetailSection title="Response details">
      {view.isScriptBackedRequest ? (
        <ScriptResponseContent view={view} />
      ) : view.responseText ? (
        <pre className={REQUEST_HISTORY_TEXT_CLASS}>{view.responseText}</pre>
      ) : (
        <p className={DETAIL_COPY_CLASS}>{view.responseUnavailableCopy}</p>
      )}
      {view.isScriptBackedRequest ? null : (
        <DispatchDetailList
          entries={[
            {
              label: "Provider session",
              value: view.providerSession ? formatProviderSession(view.providerSession) : undefined,
              code: !view.providerSessionLogTarget,
              href: view.providerSessionLogTarget?.href,
              title: view.providerSessionLogTarget?.display,
            },
          ]}
        />
      )}
      <WorkItemActionGroup
        items={view.outputWorkItems}
        label="Output work"
        onSelectWorkID={onSelectWorkID}
        selectedWorkID={selectedWorkID}
      />
      <TraceActionGroup
        activeTraceID={activeTraceID}
        onSelectTraceID={onSelectTraceID}
        traceIDs={view.traceIDs}
        traceTargetId={traceTargetId}
      />
    </DispatchDetailSection>
  );
}

function ScriptResponseContent({
  view,
}: {
  view: DispatchHistoryView;
}) {
  if (!view.scriptResponse) {
    return <p className={DETAIL_COPY_CLASS}>No script response yet for this dispatch.</p>;
  }

  return (
    <>
      <DispatchDetailList
        entries={[
          {
            label: "Script request ID",
            value: view.scriptResponse.script_request_id,
            code: true,
          },
          {
            label: "Script attempt",
            value:
              view.scriptResponse.attempt !== undefined ? String(view.scriptResponse.attempt) : undefined,
          },
          { label: "Outcome", value: view.scriptResponse.outcome },
          {
            label: "Duration",
            value:
              view.scriptResponse.duration_millis !== undefined
                ? formatDurationMillis(view.scriptResponse.duration_millis)
                : undefined,
          },
          {
            label: "Exit code",
            value:
              view.scriptResponse.exit_code !== undefined
                ? String(view.scriptResponse.exit_code)
                : undefined,
          },
          { label: "Failure type", value: view.scriptResponse.failure_type },
        ]}
      />
      <ScriptOutputSection
        emptyMessage="No stdout was recorded for this script response."
        label="Stdout"
        value={view.normalizedScriptStdout}
      />
      <ScriptOutputSection
        emptyMessage="No stderr was recorded for this script response."
        label="Stderr"
        value={view.normalizedScriptStderr}
      />
    </>
  );
}

function DispatchFailureSection({
  view,
}: {
  view: DispatchHistoryView;
}) {
  return (
    <DispatchDetailSection title="Failure details">
      <DispatchDetailList
        entries={[
          { label: "Failure reason", value: view.failureReason },
          { label: "Failure message", value: view.failureMessage },
          { label: "Failure type", code: true, value: view.failureType },
          { label: "Error class", code: true, value: view.errorClass },
        ]}
      />
    </DispatchDetailSection>
  );
}
