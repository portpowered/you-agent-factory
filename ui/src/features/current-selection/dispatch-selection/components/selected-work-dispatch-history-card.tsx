import {
  formatDurationMillis,
  formatLocalDateTime,
} from "../../../../components/ui/formatters";
import { DetailCopy } from "../../../../components/ui/widget-frame";
import type { LoadableProviderSessionRef } from "../../../provider-session-detail/lib/provider-session-ref";
import {
  useCurrentSelectionDispatchHistoryMessages,
  useCurrentSelectionLocale,
  useCurrentSelectionOperationalEnumMessages,
} from "../../base/components/current-selection-locale";
import { CurrentSelectionBadge } from "../../base/components/current-selection-pill";
import { normalizeDetailText } from "../../base/components/detail-card-shared";
import type { CurrentSelectionDispatchHistoryMessages } from "../../base/messages/current-selection-dispatch-history";
import { CurrentSelectionDescriptionList } from "../../base/public";
import {
  CurrentSelectionHistoryCard,
  CurrentSelectionHistoryCardHeader,
} from "../../history/public";
import {
  InferenceAttemptDetail,
  WorkItemPayloadList,
} from "../../work-selection/public";
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
} from "../dispatch-history/selected-work-dispatch-history-helpers";
import type { SelectedWorkRequestHistoryItem } from "../lib/detail-card-types";
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
import { WorkstationOperationKindBadge } from "./selected-work-operation-history-cards";

interface DispatchHistoryCardProps {
  activeTraceID?: string | null;
  currentDispatchID?: string | null;
  onSelectProviderSession?: (session: LoadableProviderSessionRef) => void;
  onSelectTraceID?: (traceID: string) => void;
  onSelectWorkID?: (workID: string) => void;
  request: SelectedWorkRequestHistoryItem;
  selectedProviderSessionKey?: string | null;
  selectedWorkID: string;
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
}: DispatchHistoryCardProps) {
  const messages = useCurrentSelectionDispatchHistoryMessages();
  const locale = useCurrentSelectionLocale();
  const view = buildDispatchHistoryView(request);
  const isCurrentDispatch = currentDispatchID === request.dispatch_id;
  const title = requestTitle(request, selectedWorkID);

  return (
    <CurrentSelectionHistoryCard
      aria-label={messages.workstationDispatchRowAccessibleLabel(
        title,
        request.dispatch_id,
      )}
      highlighted={isCurrentDispatch}
    >
      <DispatchHistoryHeader
        dispatchID={request.dispatch_id}
        isCurrentDispatch={isCurrentDispatch}
        messages={messages}
        outcome={view.outcome}
        title={title}
      />
      <DispatchSummaryDetails
        locale={locale}
        messages={messages}
        request={request}
        view={view}
      />
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
      {view.hasFailureDetails ? (
        <DispatchFailureSection messages={messages} view={view} />
      ) : null}
    </CurrentSelectionHistoryCard>
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

function buildDispatchHistoryView(
  request: SelectedWorkRequestHistoryItem,
): DispatchHistoryView {
  const failureReason = normalizeDetailText(requestFailureReason(request));
  const failureMessage = normalizeDetailText(requestFailureMessage(request));
  const errorClass = normalizeDetailText(requestErrorClass(request));
  const scriptRequest = requestScriptRequest(request);
  const scriptResponse = requestScriptResponse(request);
  const hasFailureDetails = Boolean(
    failureReason || failureMessage || errorClass,
  );
  const isScriptBackedRequest =
    scriptRequest !== undefined || scriptResponse !== undefined;
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
  const enumMessages = useCurrentSelectionOperationalEnumMessages();

  return (
    <CurrentSelectionHistoryCardHeader
      badges={
        <>
          <WorkstationOperationKindBadge
            label={messages.workstationOperationKindBadge}
          />
          {isCurrentDispatch ? (
            <CurrentSelectionBadge>
              {messages.currentDispatchBadge}
            </CurrentSelectionBadge>
          ) : null}
        </>
      }
      identifier={dispatchID || messages.unknownDispatchId}
      subtitle={
        outcome
          ? enumMessages.localizeOutcome(outcome)
          : enumMessages.localizeOutcome("PENDING")
      }
      title={title || dispatchID || messages.unknownDispatchTitle}
    />
  );
}

function DispatchSummaryDetails({
  locale,
  messages,
  request,
  view,
}: {
  locale?: string | null;
  messages: CurrentSelectionDispatchHistoryMessages;
  request: SelectedWorkRequestHistoryItem;
  view: DispatchHistoryView;
}) {
  const startedAt = formatLocalDateTime(
    requestStartedAt(request),
    messages.workstationUnavailableValue,
    locale,
  );

  return (
    <CurrentSelectionDescriptionList className="mt-2.5">
      <InferenceAttemptDetail
        label={messages.workstationLabel}
        value={request.workstation_name}
      />
      <InferenceAttemptDetail
        label={messages.startedAtLabel}
        value={startedAt}
      />
      <InferenceAttemptDetail
        label={messages.durationLabel}
        value={
          view.durationMillis !== undefined
            ? formatDurationMillis(view.durationMillis, locale)
            : undefined
        }
      />
    </CurrentSelectionDescriptionList>
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
        <DetailCopy>{messages.promptDetailsNotApplicable}</DetailCopy>
      ) : null}
      <WorkItemPayloadList
        messages={{
          consumedPayloadEmpty: messages.consumedPayloadEmpty,
          consumedPayloadError: messages.consumedPayloadError,
          consumedPayloadHeading: messages.consumedPayloadHeading,
          consumedPayloadLoading: messages.consumedPayloadLoading,
          consumedPayloadUnavailable: messages.consumedPayloadUnavailable,
          consumedWorkItemsLabel: messages.consumedWorkItemsLabel,
          selectWorkItemLabel: messages.selectWorkItemAccessibleLabel,
          stateLabel: messages.stateLabel,
          workTypeLabel: messages.workTypeLabel,
        }}
        onSelectWorkID={onSelectWorkID}
        selectedWorkID={selectedWorkID}
        variant="plain"
        workItems={view.inputWorkItems}
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
  view,
}: {
  activeTraceID?: string | null;
  messages: CurrentSelectionDispatchHistoryMessages;
  onSelectTraceID?: (traceID: string) => void;
  onSelectWorkID?: (workID: string) => void;
  selectedWorkID: string;
  view: DispatchHistoryView;
}) {
  return (
    <DispatchDetailSection title={messages.responseDetailsTitle}>
      <WorkItemActionGroup
        items={view.outputWorkItems}
        label={messages.outputWorkLabel}
        onSelectWorkID={onSelectWorkID}
        selectedWorkID={selectedWorkID}
        selectWorkItemAccessibleLabel={messages.selectWorkItemAccessibleLabel}
      />
      <TraceActionGroup
        activeTraceID={activeTraceID}
        label={messages.traceIdsLabel}
        onSelectTraceID={onSelectTraceID}
        selectedTraceSuffix={messages.selectedTraceSuffix}
        traceIDs={view.traceIDs}
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
  view,
}: {
  activeTraceID?: string | null;
  messages: CurrentSelectionDispatchHistoryMessages;
  onSelectTraceID?: (traceID: string) => void;
  onSelectWorkID?: (workID: string) => void;
  selectedWorkID: string;
  view: DispatchHistoryView;
}) {
  return (
    <DispatchDetailSection title={messages.traceDetailsTitle}>
      <WorkItemActionGroup
        items={view.outputWorkItems}
        label={messages.outputWorkLabel}
        onSelectWorkID={onSelectWorkID}
        selectedWorkID={selectedWorkID}
        selectWorkItemAccessibleLabel={messages.selectWorkItemAccessibleLabel}
      />
      <TraceActionGroup
        activeTraceID={activeTraceID}
        label={messages.traceIdsLabel}
        onSelectTraceID={onSelectTraceID}
        selectedTraceSuffix={messages.selectedTraceSuffix}
        traceIDs={view.traceIDs}
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
          {
            label: messages.failureTypeLabel,
            code: true,
            value: view.failureType,
          },
        ]}
      />
    </DispatchDetailSection>
  );
}
