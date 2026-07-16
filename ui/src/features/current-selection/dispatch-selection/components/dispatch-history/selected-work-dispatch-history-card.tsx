/* biome-ignore lint/style/noExcessiveLinesPerFile: keeps one dispatch-history renderer together while the current-selection card migration is still settling. */
import {
  formatDurationMillis,
  formatLocalDateTime,
  formatWorkItemLabel,
} from "../../../../../components/ui/formatters";
import {
  WidgetDetailCopy,
} from "@you-agent-factory/components/recipes";
import type { LoadableProviderSessionRef } from "../../../../provider-session-detail/lib/provider-session-ref";
import { CurrentSelectionExpandableSection } from "../../../base/components/detail/current-selection-expandable-section";
import { normalizeDetailText } from "../../../base/components/detail-card/detail-card-shared";
import {
  useCurrentSelectionDispatchHistoryMessages,
  useCurrentSelectionLocale,
  useCurrentSelectionOperationalEnumMessages,
} from "../../../base/components/presentation/current-selection-locale";
import { CurrentSelectionBadge } from "../../../base/components/presentation/current-selection-pill";
import { CurrentSelectionSelectableButton } from "../../../base/components/presentation/current-selection-selectable-button";
import { CurrentSelectionTraceButton } from "../../../base/components/presentation/current-selection-trace-button";
import type { CurrentSelectionDispatchHistoryMessages } from "../../../base/messages/shell/current-selection-dispatch-history";
import {
  CurrentSelectionDescriptionList,
  CurrentSelectionLabel,
} from "../../../base/public";
import {
  CurrentSelectionHistoryCard,
  CurrentSelectionHistoryCardHeader,
} from "../../../history/public";
import {
  InferenceAttemptDetail,
  WorkItemPayloadList,
} from "../../../work-selection/public";
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
} from "../../dispatch-history/selected-work-dispatch-history-helpers";
import type { SelectedWorkRequestHistoryItem } from "../../lib/detail-card-types";
import {
  DispatchInferenceAttemptsSection,
  DispatchScriptAttemptsSection,
} from "./selected-work-dispatch-attempt-sections";
import { DispatchDetailList } from "./selected-work-dispatch-history-card-shared";

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
      className={
        isCurrentDispatch
          ? "border-outline-variant bg-secondary-container text-on-surface"
          : undefined
      }
    >
      <DispatchHistoryHeader
        isCurrentDispatch={isCurrentDispatch}
        messages={messages}
        title={title}
      />
      <DispatchSummarySection
        activeTraceID={activeTraceID}
        dispatchID={request.dispatch_id}
        locale={locale}
        messages={messages}
        onSelectTraceID={onSelectTraceID}
        onSelectWorkID={onSelectWorkID}
        request={request}
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
  isCurrentDispatch,
  messages,
  title,
}: {
  isCurrentDispatch: boolean;
  messages: CurrentSelectionDispatchHistoryMessages;
  title: string | undefined;
}) {
  return (
    <CurrentSelectionHistoryCardHeader
      title={title || messages.unknownDispatchTitle}
      titleClassName="type-headline-large"
      trailingContent={
        isCurrentDispatch ? (
          <CurrentSelectionBadge
            className="border-outline-variant bg-secondary-container text-on-secondary-container"
            tone="neutral"
          >
            {messages.currentDispatchBadge}
          </CurrentSelectionBadge>
        ) : null
      }
    />
  );
}

function DispatchSummarySection({
  activeTraceID,
  dispatchID,
  locale,
  messages,
  onSelectTraceID,
  onSelectWorkID,
  request,
  selectedWorkID,
  view,
}: {
  activeTraceID?: string | null;
  dispatchID: string | undefined;
  locale?: string | null;
  messages: CurrentSelectionDispatchHistoryMessages;
  onSelectTraceID?: (traceID: string) => void;
  onSelectWorkID?: (workID: string) => void;
  request: SelectedWorkRequestHistoryItem;
  selectedWorkID: string;
  view: DispatchHistoryView;
}) {
  const startedAt = formatLocalDateTime(
    requestStartedAt(request),
    messages.workstationUnavailableValue,
    locale,
  );
  const enumMessages = useCurrentSelectionOperationalEnumMessages();

  return (
    <CurrentSelectionExpandableSection
      title={messages.summaryHeading}
      toggleLabel={(expanded) =>
        expanded ? messages.collapseAction : messages.expandAction
      }
    >
      <CurrentSelectionDescriptionList>
        <InferenceAttemptDetail
          code
          label={messages.dispatchIdLabel}
          value={dispatchID}
        />
        <InferenceAttemptDetail
          label={messages.outcomeLabel}
          value={
            view.outcome
              ? enumMessages.localizeOutcome(view.outcome)
              : enumMessages.localizeOutcome("PENDING")
          }
        />
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
      <div className="grid gap-3">
        <DispatchRequestContent
          messages={messages}
          onSelectWorkID={onSelectWorkID}
          selectedWorkID={selectedWorkID}
          view={view}
        />
        {!view.isScriptBackedRequest ? (
          <DispatchTraceContent
            activeTraceID={activeTraceID}
            messages={messages}
            onSelectTraceID={onSelectTraceID}
            onSelectWorkID={onSelectWorkID}
            selectedWorkID={selectedWorkID}
            view={view}
          />
        ) : null}
      </div>
    </CurrentSelectionExpandableSection>
  );
}

function DispatchRequestContent({
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
    <div className="grid gap-2">
      <CurrentSelectionLabel>{messages.requestDetailsTitle}</CurrentSelectionLabel>
      {view.isScriptBackedRequest ? (
        <WidgetDetailCopy>{messages.promptDetailsNotApplicable}</WidgetDetailCopy>
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
    </div>
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
    <CurrentSelectionExpandableSection
      title={messages.responseDetailsTitle}
      toggleLabel={(expanded) =>
        expanded ? messages.collapseAction : messages.expandAction
      }
    >
      <DispatchWorkItemDetailRow
        items={view.outputWorkItems}
        label={messages.outputWorkLabel}
        onSelectWorkID={onSelectWorkID}
        selectedWorkID={selectedWorkID}
        selectWorkItemAccessibleLabel={messages.selectWorkItemAccessibleLabel}
      />
      <DispatchTraceDetailRow
        activeTraceID={activeTraceID}
        label={messages.traceIdsLabel}
        onSelectTraceID={onSelectTraceID}
        selectedTraceSuffix={messages.selectedTraceSuffix}
        traceIDs={view.traceIDs}
      />
    </CurrentSelectionExpandableSection>
  );
}

function DispatchTraceContent({
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
    <div className="grid gap-2">
      <CurrentSelectionLabel>{messages.traceDetailsTitle}</CurrentSelectionLabel>
      <DispatchWorkItemDetailRow
        items={view.outputWorkItems}
        label={messages.outputWorkLabel}
        onSelectWorkID={onSelectWorkID}
        selectedWorkID={selectedWorkID}
        selectWorkItemAccessibleLabel={messages.selectWorkItemAccessibleLabel}
      />
      <DispatchTraceDetailRow
        activeTraceID={activeTraceID}
        label={messages.traceIdsLabel}
        onSelectTraceID={onSelectTraceID}
        selectedTraceSuffix={messages.selectedTraceSuffix}
        traceIDs={view.traceIDs}
      />
    </div>
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
    <CurrentSelectionExpandableSection
      title={messages.failureDetailsTitle}
      toggleLabel={(expanded) =>
        expanded ? messages.collapseAction : messages.expandAction
      }
    >
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
    </CurrentSelectionExpandableSection>
  );
}

function DispatchWorkItemDetailRow({
  items,
  label,
  onSelectWorkID,
  selectedWorkID,
  selectWorkItemAccessibleLabel,
}: {
  items: DispatchHistoryView["outputWorkItems"];
  label: string;
  onSelectWorkID?: (workID: string) => void;
  selectedWorkID: string;
  selectWorkItemAccessibleLabel: (workItemLabel: string) => string;
}) {
  if (items.length === 0) {
    return null;
  }

  return (
    <div className="grid gap-2">
      <CurrentSelectionLabel>{label}</CurrentSelectionLabel>
      <div className="flex flex-wrap gap-2">
        {items.map((workItem) => {
          const workItemLabel = formatWorkItemLabel(workItem);
          return (
            <CurrentSelectionSelectableButton
              aria-label={selectWorkItemAccessibleLabel(workItemLabel)}
              key={`${label}-${workItem.work_id}`}
              onClick={() => onSelectWorkID?.(workItem.work_id)}
              selected={selectedWorkID === workItem.work_id}
              selectedStyle="outline"
            >
              {workItemLabel}
            </CurrentSelectionSelectableButton>
          );
        })}
      </div>
    </div>
  );
}

function DispatchTraceDetailRow({
  activeTraceID,
  label,
  onSelectTraceID,
  selectedTraceSuffix,
  traceIDs,
}: {
  activeTraceID?: string | null;
  label: string;
  onSelectTraceID?: (traceID: string) => void;
  selectedTraceSuffix: string;
  traceIDs: string[];
}) {
  if (traceIDs.length === 0) {
    return null;
  }

  return (
    <div className="grid gap-2">
      <CurrentSelectionLabel>{label}</CurrentSelectionLabel>
      <div className="grid gap-1.5">
        {traceIDs.map((traceID) => (
          <CurrentSelectionTraceButton
            activeTraceID={activeTraceID}
            key={traceID}
            onSelectTraceID={onSelectTraceID}
            selectedTraceSuffix={selectedTraceSuffix}
            traceID={traceID}
          />
        ))}
      </div>
    </div>
  );
}
