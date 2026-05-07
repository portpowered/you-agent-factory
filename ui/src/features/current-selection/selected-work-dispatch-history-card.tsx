import { cx } from "../../lib/cx";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../components/ui/dashboard-typography";
import { DETAIL_COPY_CLASS } from "../../components/dashboard/widget-board";
import {
  formatDurationMillis,
} from "../../components/ui/formatters";
import {
  EXECUTION_PILL_CLASS,
  INFERENCE_ATTEMPT_DETAIL_CLASS,
  InferenceAttemptDetail,
  PROVIDER_SESSION_CARD_CLASS,
  normalizeDetailText,
} from "./detail-card-shared";
import type { SelectedWorkRequestHistoryItem } from "./detail-card-types";
import {
  DispatchInferenceAttemptsSection,
  DispatchScriptAttemptsSection,
} from "./selected-work-dispatch-attempt-sections";
import {
  DispatchDetailList,
  DispatchDetailSection,
  ScriptArgsSection,
  ScriptOutputSection,
  TraceActionGroup,
  WorkItemActionGroup,
} from "./selected-work-dispatch-history-card-shared";
import { useCurrentSelectionDispatchHistoryMessages } from "./current-selection-locale";
import type { CurrentSelectionDispatchHistoryMessages } from "./messages/current-selection-dispatch-history";
import {
  dedupeWorkItems,
  requestCounts,
  requestDurationMillis,
  requestErrorClass,
  requestFailureMessage,
  requestFailureReason,
  requestInferenceAttempts,
  requestInputWorkItems,
  requestOutcome,
  requestOutputWorkItems,
  requestScriptRequest,
  requestScriptResponse,
  requestStartedAt,
  requestTraceIDs,
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
  const messages = useCurrentSelectionDispatchHistoryMessages();
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
        messages={messages}
        outcome={view.outcome}
        workstationLabel={request.workstation_name || request.transition_id}
      />
      <DispatchSummaryDetails messages={messages} request={request} view={view} />
      <DispatchRequestSection
        messages={messages}
        onSelectWorkID={onSelectWorkID}
        selectedWorkID={selectedWorkID}
        view={view}
      />
      {view.isScriptBackedRequest ? (
        <>
          <DispatchResponseSection
            activeTraceID={activeTraceID}
            messages={messages}
            onSelectTraceID={onSelectTraceID}
            onSelectWorkID={onSelectWorkID}
            selectedWorkID={selectedWorkID}
            traceTargetId={traceTargetId}
            view={view}
          />
          <DispatchScriptAttemptsSection
            normalizedStderr={view.normalizedScriptStderr}
            normalizedStdout={view.normalizedScriptStdout}
            request={request}
            scriptRequest={view.scriptRequest}
            scriptResponse={view.scriptResponse}
          />
        </>
      ) : (
        <>
          <DispatchTraceSection
            activeTraceID={activeTraceID}
            messages={messages}
            onSelectTraceID={onSelectTraceID}
            onSelectWorkID={onSelectWorkID}
            selectedWorkID={selectedWorkID}
            traceTargetId={traceTargetId}
            view={view}
          />
          <DispatchInferenceAttemptsSection
            attempts={view.sortedInferenceAttempts}
            emptyCopy={
              view.hasFailureDetails
                ? messages.inferenceAttemptsEmptyEnded
                : messages.inferenceAttemptsEmptyPending
            }
          />
        </>
      )}
      {view.hasFailureDetails ? <DispatchFailureSection messages={messages} view={view} /> : null}
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
  const hasFailureDetails = Boolean(failureReason || failureMessage || errorClass);
  const isScriptBackedRequest = scriptRequest !== undefined || scriptResponse !== undefined;
  const sortedInferenceAttempts = requestInferenceAttempts(request);

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
    scriptRequest,
    scriptResponse,
    sortedInferenceAttempts,
    traceIDs: requestTraceIDs(request),
  };
}

function DispatchHistoryHeader({
  dispatchID,
  isCurrentDispatch,
  messages,
  outcome,
  workstationLabel,
}: {
  dispatchID: string | undefined;
  isCurrentDispatch: boolean;
  messages: CurrentSelectionDispatchHistoryMessages;
  outcome: string | undefined;
  workstationLabel: string | undefined;
}) {
  return (
    <div className="flex items-start justify-between gap-[0.8rem]">
      <div className="grid min-w-0 gap-[0.18rem]">
        <strong className="min-w-0 [overflow-wrap:anywhere]">
          {workstationLabel || dispatchID || messages.unknownDispatchTitle}
        </strong>
        <div className="flex flex-wrap items-center gap-[0.45rem]">
          <p className={cx("m-0 text-af-ink/70", DASHBOARD_BODY_TEXT_CLASS)}>
            {outcome ?? messages.pendingOutcome}
          </p>
          {isCurrentDispatch ? (
            <span
              className={cx(
                "inline-flex rounded-full border border-af-accent/35 bg-af-accent/10 px-2 py-[0.18rem] text-af-accent",
                DASHBOARD_SUPPORTING_TEXT_CLASS,
              )}
            >
              {messages.currentDispatchBadge}
            </span>
          ) : null}
        </div>
      </div>
      <span className={EXECUTION_PILL_CLASS}>{dispatchID || messages.unknownDispatchId}</span>
    </div>
  );
}

function DispatchSummaryDetails({
  messages,
  request,
  view,
}: {
  messages: CurrentSelectionDispatchHistoryMessages;
  request: SelectedWorkRequestHistoryItem;
  view: DispatchHistoryView;
}) {
  return (
    <dl className={cx("mt-[0.65rem]", INFERENCE_ATTEMPT_DETAIL_CLASS)}>
      <InferenceAttemptDetail label={messages.workstationLabel} value={request.workstation_name} />
      <InferenceAttemptDetail label={messages.transitionIdLabel} code value={request.transition_id} />
      <InferenceAttemptDetail label={messages.dispatchedCountLabel} value={view.counts.dispatchedCount} />
      <InferenceAttemptDetail label={messages.respondedCountLabel} value={view.counts.respondedCount} />
      <InferenceAttemptDetail label={messages.erroredCountLabel} value={view.counts.erroredCount} />
      <InferenceAttemptDetail label={messages.startedAtLabel} value={requestStartedAt(request)} />
      <InferenceAttemptDetail
        label={messages.durationLabel}
        value={view.durationMillis !== undefined ? formatDurationMillis(view.durationMillis) : undefined}
      />
    </dl>
  );
}

function DispatchRequestSection({
  messages,
  onSelectWorkID,
  selectedWorkID,
  view,
}: {
  messages: CurrentSelectionDispatchHistoryMessages;
  onSelectWorkID?: (workID: string) => void;
  selectedWorkID: string;
  view: DispatchHistoryView;
}) {
  return (
    <DispatchDetailSection title={messages.requestDetailsTitle}>
      {view.isScriptBackedRequest ? (
        <>
          <p className={DETAIL_COPY_CLASS}>
            {messages.promptDetailsNotApplicable}
          </p>
          <DispatchDetailList
            entries={[
              {
                label: messages.scriptRequestIdLabel,
                value: view.scriptRequest?.script_request_id,
                code: true,
              },
              {
                label: messages.scriptAttemptLabel,
                value:
                  view.scriptRequest?.attempt !== undefined
                    ? String(view.scriptRequest.attempt)
                    : undefined,
              },
              { label: messages.commandLabel, value: view.scriptRequest?.command, code: true },
            ]}
          />
          <ScriptArgsSection args={view.scriptRequest?.args} />
        </>
      ) : (
        <p className={DETAIL_COPY_CLASS}>
          {messages.inferenceRequestGuidance}
        </p>
      )}
      <WorkItemActionGroup
        items={view.inputWorkItems}
        label={messages.inputWorkLabel}
        onSelectWorkID={onSelectWorkID}
        selectedWorkID={selectedWorkID}
      />
    </DispatchDetailSection>
  );
}

function DispatchResponseSection({
  activeTraceID,
  messages,
  onSelectTraceID,
  onSelectWorkID,
  selectedWorkID,
  traceTargetId,
  view,
}: {
  activeTraceID?: string | null;
  messages: CurrentSelectionDispatchHistoryMessages;
  onSelectTraceID?: (traceID: string) => void;
  onSelectWorkID?: (workID: string) => void;
  selectedWorkID: string;
  traceTargetId: string;
  view: DispatchHistoryView;
}) {
  return (
    <DispatchDetailSection title={messages.responseDetailsTitle}>
      <ScriptResponseContent messages={messages} view={view} />
      <WorkItemActionGroup
        items={view.outputWorkItems}
        label={messages.outputWorkLabel}
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

function DispatchTraceSection({
  activeTraceID,
  messages,
  onSelectTraceID,
  onSelectWorkID,
  selectedWorkID,
  traceTargetId,
  view,
}: {
  activeTraceID?: string | null;
  messages: CurrentSelectionDispatchHistoryMessages;
  onSelectTraceID?: (traceID: string) => void;
  onSelectWorkID?: (workID: string) => void;
  selectedWorkID: string;
  traceTargetId: string;
  view: DispatchHistoryView;
}) {
  return (
    <DispatchDetailSection title={messages.traceDetailsTitle}>
      <WorkItemActionGroup
        items={view.outputWorkItems}
        label={messages.outputWorkLabel}
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
  messages,
  view,
}: {
  messages: CurrentSelectionDispatchHistoryMessages;
  view: DispatchHistoryView;
}) {
  if (!view.scriptResponse) {
    return <p className={DETAIL_COPY_CLASS}>{messages.noScriptResponseYet}</p>;
  }

  return (
    <>
      <DispatchDetailList
        entries={[
          {
            label: messages.scriptRequestIdLabel,
            value: scriptRequestID(view.scriptResponse),
            code: true,
          },
          {
            label: messages.scriptAttemptLabel,
            value:
              scriptAttemptNumber(view.scriptResponse) !== undefined
                ? String(scriptAttemptNumber(view.scriptResponse))
                : undefined,
          },
          { label: messages.outcomeLabel, value: view.scriptResponse.outcome },
          {
            label: messages.durationLabel,
            value:
              scriptResponseDurationMillis(view.scriptResponse) !== undefined
                ? formatDurationMillis(scriptResponseDurationMillis(view.scriptResponse) ?? 0)
                : undefined,
          },
          {
            label: messages.exitCodeLabel,
            value:
              scriptResponseExitCode(view.scriptResponse) !== undefined
                ? String(scriptResponseExitCode(view.scriptResponse))
                : undefined,
          },
          { label: messages.failureTypeLabel, value: scriptResponseFailureType(view.scriptResponse) },
        ]}
      />
      <ScriptOutputSection
        emptyMessage={messages.noStdoutRecorded}
        label={messages.stdoutLabel}
        value={view.normalizedScriptStdout}
      />
      <ScriptOutputSection
        emptyMessage={messages.noStderrRecorded}
        label={messages.stderrLabel}
        value={view.normalizedScriptStderr}
      />
    </>
  );
}

function DispatchFailureSection({
  messages,
  view,
}: {
  messages: CurrentSelectionDispatchHistoryMessages;
  view: DispatchHistoryView;
}) {
  return (
    <DispatchDetailSection title={messages.failureDetailsTitle}>
      <DispatchDetailList
        entries={[
          { label: messages.failureReasonLabel, value: view.failureReason },
          { label: messages.failureMessageLabel, value: view.failureMessage },
          { label: messages.failureTypeLabel, code: true, value: view.failureType },
        ]}
      />
    </DispatchDetailSection>
  );
}
