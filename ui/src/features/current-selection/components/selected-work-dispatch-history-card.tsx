import { cn } from "../../../lib/cn";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
import { DETAIL_COPY_CLASS } from "../../../components/dashboard/widget-board";
import {
  formatDurationMillis,
  formatLocalDateTime,
} from "../../../components/ui/formatters";
import {
  EXECUTION_PILL_CLASS,
  INFERENCE_ATTEMPT_DETAIL_CLASS,
  InferenceAttemptDetail,
  PROVIDER_SESSION_CARD_CLASS,
  normalizeDetailText,
} from "./detail-card-shared";
import type { SelectedWorkRequestHistoryItem } from "../detail-card-types";
import {
  DispatchInferenceAttemptsSection,
  DispatchScriptAttemptsSection,
} from "./selected-work-dispatch-attempt-sections";
import {
  DispatchDetailList,
  DispatchDetailSection,
  TraceActionGroup,
  WorkItemActionGroup,
} from "./selected-work-dispatch-history-card-shared";
import { useCurrentSelectionDispatchHistoryMessages } from "./current-selection-locale";
import type { CurrentSelectionDispatchHistoryMessages } from "../messages/current-selection-dispatch-history";
import {
  dedupeWorkItems,
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
  requestTitle,
  requestTraceIDs,
  scriptResponseFailureType,
} from "../selected-work-dispatch-history-helpers";
import type { LoadableProviderSessionRef } from "../provider-session-details";

interface DispatchHistoryCardProps {
  activeTraceID?: string | null;
  currentDispatchID?: string | null;
  onSelectProviderSession?: (session: LoadableProviderSessionRef) => void;
  onSelectTraceID?: (traceID: string) => void;
  onSelectWorkID?: (workID: string) => void;
  request: SelectedWorkRequestHistoryItem;
  selectedProviderSessionKey?: string | null;
  selectedWorkID: string;
  traceTargetId: string;
}

export function DispatchHistoryCard({
  activeTraceID,
  currentDispatchID,
  onSelectProviderSession,
  onSelectTraceID,
  onSelectWorkID,
  request,
  selectedProviderSessionKey,
  selectedWorkID,
  traceTargetId,
}: DispatchHistoryCardProps) {
  const messages = useCurrentSelectionDispatchHistoryMessages();
  const view = buildDispatchHistoryView(request);
  const isCurrentDispatch = currentDispatchID === request.dispatch_id;

  return (
    <article
      className={cn(
        PROVIDER_SESSION_CARD_CLASS,
        isCurrentDispatch && "border-on-foreground/30 bg-on-foreground/6",
      )}
    >
      <DispatchHistoryHeader
        dispatchID={request.dispatch_id}
        isCurrentDispatch={isCurrentDispatch}
        messages={messages}
        outcome={view.outcome}
        title={requestTitle(request, selectedWorkID)}
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
            onSelectProviderSession={onSelectProviderSession}
            selectedProviderSessionKey={selectedProviderSessionKey}
          />
        </>
      )}
      {view.hasFailureDetails ? <DispatchFailureSection messages={messages} view={view} /> : null}
    </article>
  );
}

interface DispatchHistoryView {
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
  title,
}: {
  dispatchID: string | undefined;
  isCurrentDispatch: boolean;
  messages: CurrentSelectionDispatchHistoryMessages;
  outcome: string | undefined;
  title: string | undefined;
}) {
  return (
    <div className="flex items-start justify-between gap-3">
      <div className="grid min-w-0 gap-1">
        <strong className="min-w-0 [overflow-wrap:anywhere]">
          {title || dispatchID || messages.unknownDispatchTitle}
        </strong>
        <div className="flex flex-wrap items-center gap-2">
          <p className={cn("m-0 text-af-ink/70", DASHBOARD_BODY_TEXT_CLASS)}>
            {outcome ?? messages.pendingOutcome}
          </p>
          {isCurrentDispatch ? (
            <span
              className={cn(
                "inline-flex rounded-full border border-on-foreground/35 bg-on-foreground/10 px-2 py-0.5 text-on-foreground",
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
  const startedAt = formatLocalDateTime(
    requestStartedAt(request),
    messages.workstationUnavailableValue,
  );

  return (
    <dl className={cn("mt-2.5", INFERENCE_ATTEMPT_DETAIL_CLASS)}>
      <InferenceAttemptDetail label={messages.workstationLabel} value={request.workstation_name} />
      <InferenceAttemptDetail label={messages.startedAtLabel} value={startedAt} />
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
        <p className={DETAIL_COPY_CLASS}>
          {messages.promptDetailsNotApplicable}
        </p>
      ) : null}
      <WorkItemActionGroup
        items={view.inputWorkItems}
        label={messages.inputWorkLabel}
        onSelectWorkID={onSelectWorkID}
        selectedWorkID={selectedWorkID}
        selectWorkItemAccessibleLabel={
          messages.selectWorkItemAccessibleLabel
        }
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
      <WorkItemActionGroup
        items={view.outputWorkItems}
        label={messages.outputWorkLabel}
        onSelectWorkID={onSelectWorkID}
        selectedWorkID={selectedWorkID}
        selectWorkItemAccessibleLabel={
          messages.selectWorkItemAccessibleLabel
        }
      />
      <TraceActionGroup
        activeTraceID={activeTraceID}
        label={messages.traceIdsLabel}
        onSelectTraceID={onSelectTraceID}
        selectedTraceSuffix={messages.selectedTraceSuffix}
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
        selectWorkItemAccessibleLabel={
          messages.selectWorkItemAccessibleLabel
        }
      />
      <TraceActionGroup
        activeTraceID={activeTraceID}
        label={messages.traceIdsLabel}
        onSelectTraceID={onSelectTraceID}
        selectedTraceSuffix={messages.selectedTraceSuffix}
        traceIDs={view.traceIDs}
        traceTargetId={traceTargetId}
      />
    </DispatchDetailSection>
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
