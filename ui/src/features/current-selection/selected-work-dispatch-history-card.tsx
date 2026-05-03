import { cx } from "../../lib/cx";
import {
  DASHBOARD_SECTION_HEADING_CLASS,
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
  InferenceAttemptCard,
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
  requestInferenceAttempts,
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
  scriptAttemptNumber,
  scriptRequestID,
  scriptResponseDurationMillis,
  scriptResponseExitCode,
  scriptResponseFailureType,
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
      {view.sortedInferenceAttempts.length > 0 ? (
        <DispatchInferenceAttemptsSection view={view} />
      ) : null}
      {view.isScriptBackedRequest ? <DispatchScriptAttemptsSection request={request} view={view} /> : null}
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
  sortedInferenceAttempts: ReturnType<typeof requestInferenceAttempts>;
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
    failureType: normalizeDetailText(scriptResponseFailureType(scriptResponse)),
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
    sortedInferenceAttempts: requestInferenceAttempts(request),
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
            value: scriptRequestID(view.scriptResponse),
            code: true,
          },
          {
            label: "Script attempt",
            value:
              scriptAttemptNumber(view.scriptResponse) !== undefined
                ? String(scriptAttemptNumber(view.scriptResponse))
                : undefined,
          },
          { label: "Outcome", value: view.scriptResponse.outcome },
          {
            label: "Duration",
            value:
              scriptResponseDurationMillis(view.scriptResponse) !== undefined
                ? formatDurationMillis(scriptResponseDurationMillis(view.scriptResponse) ?? 0)
                : undefined,
          },
          {
            label: "Exit code",
            value:
              scriptResponseExitCode(view.scriptResponse) !== undefined
                ? String(scriptResponseExitCode(view.scriptResponse))
                : undefined,
          },
          { label: "Failure type", value: scriptResponseFailureType(view.scriptResponse) },
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

function DispatchScriptAttemptsSection({
  request,
  view,
}: {
  request: SelectedWorkRequestHistoryItem;
  view: DispatchHistoryView;
}) {
  return (
    <section
      aria-label="Script attempts"
      className="mt-[0.75rem] grid gap-[0.45rem] border-t border-af-overlay/8 pt-[0.75rem]"
    >
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>Script attempts</h4>
      <div className="grid gap-[0.65rem]">
        {view.scriptRequest ? (
          <ScriptRequestAttemptCard
            model={requestModel(request)}
            provider={requestProvider(request)}
            scriptRequest={view.scriptRequest}
            workingDirectory={requestWorkingDirectory(request)}
            worktree={requestWorktree(request)}
          />
        ) : null}
        {view.scriptResponse ? (
          <ScriptResponseAttemptCard
            fallbackAttemptNumber={scriptAttemptNumber(view.scriptRequest)}
            model={requestModel(request)}
            normalizedStderr={view.normalizedScriptStderr}
            normalizedStdout={view.normalizedScriptStdout}
            provider={requestProvider(request)}
            scriptResponse={view.scriptResponse}
            workingDirectory={requestWorkingDirectory(request)}
            worktree={requestWorktree(request)}
          />
        ) : (
          <p className={DETAIL_COPY_CLASS}>No script response attempt has been recorded yet.</p>
        )}
      </div>
    </section>
  );
}

function ScriptRequestAttemptCard({
  model,
  provider,
  scriptRequest,
  workingDirectory,
  worktree,
}: {
  model: string | undefined;
  provider: string | undefined;
  scriptRequest: NonNullable<DispatchHistoryView["scriptRequest"]>;
  workingDirectory: string | undefined;
  worktree: string | undefined;
}) {
  const attemptNumber = scriptAttemptNumber(scriptRequest);
  const requestID = scriptRequestID(scriptRequest);

  return (
    <article className={PROVIDER_SESSION_CARD_CLASS}>
      <div className="flex items-start justify-between gap-[0.8rem]">
        <div className="grid min-w-0 gap-[0.18rem]">
          <strong>Request attempt {attemptNumber ?? "pending"}</strong>
          <p className={cx("m-0 text-af-ink/70", DASHBOARD_BODY_TEXT_CLASS)}>PENDING</p>
        </div>
        <span className={EXECUTION_PILL_CLASS}>{requestID ?? "script-request"}</span>
      </div>
      <dl className={cx("mt-[0.65rem]", INFERENCE_ATTEMPT_DETAIL_CLASS)}>
        <InferenceAttemptDetail label="Script request ID" code value={requestID} />
        <InferenceAttemptDetail
          label="Script attempt"
          value={attemptNumber !== undefined ? String(attemptNumber) : undefined}
        />
        <InferenceAttemptDetail label="Provider" code value={provider} />
        <InferenceAttemptDetail label="Model" code value={model} />
        <InferenceAttemptDetail label="Working directory" code value={workingDirectory} />
        <InferenceAttemptDetail label="Worktree" code value={worktree} />
        <InferenceAttemptDetail label="Command" code value={scriptRequest.command} />
      </dl>
      <ScriptArgsSection args={scriptRequest.args} />
    </article>
  );
}

function ScriptResponseAttemptCard({
  fallbackAttemptNumber,
  model,
  normalizedStderr,
  normalizedStdout,
  provider,
  scriptResponse,
  workingDirectory,
  worktree,
}: {
  fallbackAttemptNumber: number | undefined;
  model: string | undefined;
  normalizedStderr: string | undefined;
  normalizedStdout: string | undefined;
  provider: string | undefined;
  scriptResponse: NonNullable<DispatchHistoryView["scriptResponse"]>;
  workingDirectory: string | undefined;
  worktree: string | undefined;
}) {
  const attemptNumber = scriptAttemptNumber(scriptResponse) ?? fallbackAttemptNumber;
  const requestID = scriptRequestID(scriptResponse);
  const durationMillis = scriptResponseDurationMillis(scriptResponse);
  const exitCode = scriptResponseExitCode(scriptResponse);
  const failureType = scriptResponseFailureType(scriptResponse);

  return (
    <article className={PROVIDER_SESSION_CARD_CLASS}>
      <div className="flex items-start justify-between gap-[0.8rem]">
        <div className="grid min-w-0 gap-[0.18rem]">
          <strong>Response attempt {attemptNumber ?? "completed"}</strong>
          <p className={cx("m-0 text-af-ink/70", DASHBOARD_BODY_TEXT_CLASS)}>
            {scriptResponse.outcome ?? "RECORDED"}
          </p>
        </div>
        <span className={EXECUTION_PILL_CLASS}>{requestID ?? "script-response"}</span>
      </div>
      <dl className={cx("mt-[0.65rem]", INFERENCE_ATTEMPT_DETAIL_CLASS)}>
        <InferenceAttemptDetail label="Script request ID" code value={requestID} />
        <InferenceAttemptDetail
          label="Script attempt"
          value={attemptNumber !== undefined ? String(attemptNumber) : undefined}
        />
        <InferenceAttemptDetail label="Provider" code value={provider} />
        <InferenceAttemptDetail label="Model" code value={model} />
        <InferenceAttemptDetail label="Working directory" code value={workingDirectory} />
        <InferenceAttemptDetail label="Worktree" code value={worktree} />
        <InferenceAttemptDetail label="Outcome" value={scriptResponse.outcome} />
        <InferenceAttemptDetail
          label="Duration"
          value={durationMillis !== undefined ? formatDurationMillis(durationMillis) : undefined}
        />
        <InferenceAttemptDetail
          label="Exit code"
          value={exitCode !== undefined ? String(exitCode) : undefined}
        />
        <InferenceAttemptDetail label="Failure type" code value={failureType} />
      </dl>
      <ScriptOutputSection
        emptyMessage="No stdout was recorded for this script response."
        label="Stdout"
        value={normalizedStdout}
      />
      <ScriptOutputSection
        emptyMessage="No stderr was recorded for this script response."
        label="Stderr"
        value={normalizedStderr}
      />
    </article>
  );
}

function DispatchInferenceAttemptsSection({
  view,
}: {
  view: DispatchHistoryView;
}) {
  return (
    <section
      aria-label="Inference attempts"
      className="mt-[0.75rem] grid gap-[0.45rem] border-t border-af-overlay/8 pt-[0.75rem]"
    >
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>Inference attempts</h4>
      <div className="grid gap-[0.65rem]">
        {view.sortedInferenceAttempts.map((attempt) => (
          <InferenceAttemptCard attempt={attempt} key={attempt.inference_request_id} />
        ))}
      </div>
    </section>
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
