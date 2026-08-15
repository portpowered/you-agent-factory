import { Code } from "@you-agent-factory/components/primitives";
import { WidgetDetailCopy } from "@you-agent-factory/components/recipes";
import type {
  DashboardInferenceAttempt,
  DashboardWorkstationRequest,
} from "../../../../../api/dashboard/types";
import { DashboardActionButton } from "../../../../../components/ui/dashboard-action-button";
import {
  formatDurationFromISO,
  formatDurationMillis,
  formatWorkItemLabel,
  getLocalDateTimeDisplay,
} from "../../../../../components/ui/formatters";
import { CurrentSelectionExpandableSection } from "../../../base/components/detail/current-selection-expandable-section";
import { CurrentSelectionExecutionPill } from "../../../base/components/presentation/current-selection-pill";
import { CurrentSelectionSupportingText } from "../../../base/components/presentation/current-selection-supporting-text";
import { getCurrentSelectionOperationalEnumMessages } from "../../../base/messages/operational/current-selection-operational-enums";
import { getCurrentSelectionDetailMessages } from "../../../base/messages/shell/current-selection-detail";
import {
  CurrentSelectionHistoryCard,
  CurrentSelectionHistoryCardHeader,
} from "../../../history/components/current-selection-history-card";
import type { WorkstationRequestHistorySectionProps } from "../../lib/keys/detail-card-types";
import type { getWorkstationDetailMessages } from "../../messages/workstation-detail";
import { WorkstationDispatchRow } from "./workstation-dispatch-row";

export function WorkstationRequestHistorySection({
  locale,
  messages,
  now,
  onSelectWorkID,
  onSelectWorkstationRequest,
  requests,
  resetKey,
  selectedRequest,
  selectedWorkID,
}: WorkstationRequestHistorySectionProps) {
  const historyID = `workstation-request-history-${resetKey}`;

  return (
    <CurrentSelectionExpandableSection
      contentId={historyID}
      headingId={`${historyID}-heading`}
      resetKey={resetKey}
      supportingText={messages.historyRequestCountLabel(requests.length)}
      title={messages.requestHistoryHeading}
      toggleLabel={(expanded) =>
        expanded ? messages.collapseAction : messages.expandAction
      }
    >
      {requests.length > 0 ? (
        <ul className="m-0 grid list-none gap-2.5 p-0">
          {requests.map((request) => (
            <WorkstationRequestHistoryRow
              key={request.dispatch_id}
              locale={locale}
              messages={messages}
              now={now}
              onSelectWorkID={onSelectWorkID}
              onSelectWorkstationRequest={onSelectWorkstationRequest}
              request={request}
              selectedRequest={selectedRequest}
              selectedWorkID={selectedWorkID}
            />
          ))}
        </ul>
      ) : (
        <WidgetDetailCopy>{messages.noWorkstationRequests}</WidgetDetailCopy>
      )}
    </CurrentSelectionExpandableSection>
  );
}

function WorkstationRequestHistoryRow({
  locale,
  messages,
  now,
  onSelectWorkID,
  onSelectWorkstationRequest,
  request,
  selectedRequest,
  selectedWorkID,
}: {
  locale?: string;
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  now: number;
  onSelectWorkID?: WorkstationRequestHistorySectionProps["onSelectWorkID"];
  onSelectWorkstationRequest?: WorkstationRequestHistorySectionProps["onSelectWorkstationRequest"];
  request: DashboardWorkstationRequest;
  selectedRequest?: WorkstationRequestHistorySectionProps["selectedRequest"];
  selectedWorkID?: WorkstationRequestHistorySectionProps["selectedWorkID"];
}) {
  const requestSelected = selectedRequest?.dispatch_id === request.dispatch_id;
  const totalDurationMillis =
    request.total_duration_millis ?? request.script_response?.duration_millis;
  const normalizedOutcome = (
    request.outcome ?? request.script_response?.outcome
  )
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
    <WorkstationDispatchRow
      actions={renderWorkstationRequestActions({
        messages,
        onSelectWorkstationRequest,
        request,
        requestSelected,
      })}
      status={renderWorkstationRequestStatusPill({
        hasFailedOutcome,
        messages,
        now,
        request,
        totalDurationMillis,
      })}
      supportingContent={
        <div className="grid gap-3">
          <WorkstationRequestWorkItems
            messages={messages}
            onSelectWorkID={onSelectWorkID}
            request={request}
            selectedWorkID={selectedWorkID}
          />
          <WorkstationRequestAttempts
            locale={locale}
            messages={messages}
            request={request}
          />
          {requestSelected ? (
            <CurrentSelectionSupportingText tone="status">
              {messages.selectedRequestLabel(request.dispatch_id)}
            </CurrentSelectionSupportingText>
          ) : null}
        </div>
      }
      title={
        <>
          <span>{messages.projectedWorkstationRequestSummary}</span>{" "}
          <Code>{request.request_id ?? request.dispatch_id}</Code>
        </>
      }
    />
  );
}

function WorkstationRequestWorkItems({
  messages,
  onSelectWorkID,
  request,
  selectedWorkID,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  onSelectWorkID?: WorkstationRequestHistorySectionProps["onSelectWorkID"];
  request: DashboardWorkstationRequest;
  selectedWorkID?: WorkstationRequestHistorySectionProps["selectedWorkID"];
}) {
  if (request.work_items.length === 0) {
    return (
      <CurrentSelectionSupportingText tone="status">
        {messages.unknownWorkLabel}
      </CurrentSelectionSupportingText>
    );
  }

  return (
    <div className="grid gap-2">
      {request.work_items.map((workItem) => {
        const workLabel = formatWorkItemLabel(workItem);
        const selected = selectedWorkID === workItem.work_id;

        return (
          <CurrentSelectionHistoryCard
            className="p-3"
            key={`${request.dispatch_id}:${workItem.work_id}`}
          >
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div className="grid min-w-0 gap-1">
                <strong className="min-w-0 [overflow-wrap:anywhere]">
                  {workLabel}
                </strong>
                <Code>
                  {messages.workIdLabel}: {workItem.work_id}
                </Code>
              </div>
              {onSelectWorkID ? (
                <DashboardActionButton
                  aria-label={messages.selectWorkItemLabel(workLabel)}
                  aria-pressed={selected}
                  onClick={() => onSelectWorkID(workItem.work_id)}
                  type="button"
                >
                  {selected
                    ? messages.workSelectedAction
                    : messages.openWorkItemAction}
                </DashboardActionButton>
              ) : null}
            </div>
          </CurrentSelectionHistoryCard>
        );
      })}
    </div>
  );
}

function WorkstationRequestAttempts({
  locale,
  messages,
  request,
}: {
  locale?: string;
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  request: DashboardWorkstationRequest;
}) {
  const inferenceAttempts = request.inference_attempts
    .map((attempt, index) => ({ attempt, index }))
    .sort(
      (left, right) =>
        left.attempt.attempt - right.attempt.attempt ||
        left.index - right.index,
    );
  const hasScriptAttempt = Boolean(
    request.script_request || request.script_response,
  );

  if (inferenceAttempts.length === 0 && !hasScriptAttempt) {
    return null;
  }

  return (
    <div className="grid gap-2">
      {inferenceAttempts.map(({ attempt }) => (
        <WorkstationInferenceAttemptCard
          attempt={attempt}
          key={`${request.dispatch_id}:inference:${attempt.inference_request_id}:${attempt.attempt}`}
          locale={locale}
          messages={messages}
        />
      ))}
      {hasScriptAttempt ? (
        <WorkstationScriptAttemptCard
          key={`${request.dispatch_id}:script:${getScriptRequestID(request) ?? "dispatch"}`}
          locale={locale}
          messages={messages}
          request={request}
        />
      ) : null}
    </div>
  );
}

function WorkstationInferenceAttemptCard({
  attempt,
  locale,
  messages,
}: {
  attempt: DashboardInferenceAttempt;
  locale?: string;
  messages: ReturnType<typeof getWorkstationDetailMessages>;
}) {
  const detailMessages = getCurrentSelectionDetailMessages(locale);
  const enumMessages = getCurrentSelectionOperationalEnumMessages(locale);
  const provider =
    attempt.diagnostics?.provider?.provider ??
    attempt.provider_session?.provider;
  const model = attempt.diagnostics?.provider?.model;
  const providerSummary = provider
    ? messages.providerSummary(provider, model)
    : model;
  const requestTime = getLocalDateTimeDisplay(
    attempt.request_time,
    detailMessages.timestampUnavailable,
    locale,
  );
  const responseTime = getLocalDateTimeDisplay(
    attempt.response_time,
    detailMessages.timestampUnavailable,
    locale,
  );
  const outcome = enumMessages.localizeOutcome(
    attempt.outcome ?? detailMessages.pendingOutcome,
  );

  return (
    <CurrentSelectionHistoryCard className="p-3">
      <CurrentSelectionHistoryCardHeader
        identifier={attempt.inference_request_id}
        subtitle={outcome}
        title={detailMessages.attemptTitle(attempt.attempt)}
      />
      <div className="grid gap-1">
        <WorkstationRequestAttemptDetail
          label={detailMessages.providerLabel}
          value={providerSummary}
        />
        <WorkstationRequestAttemptDetail
          label={detailMessages.providerSessionLabel}
          value={attempt.provider_session?.id}
        />
        <WorkstationRequestAttemptTimeDetail
          label={detailMessages.requestTimeLabel}
          timestamp={requestTime}
        />
        <WorkstationRequestAttemptDetail
          label={detailMessages.elapsedTimeLabel}
          value={
            attempt.duration_millis !== undefined
              ? formatDurationMillis(attempt.duration_millis, locale)
              : undefined
          }
        />
        <WorkstationRequestAttemptTimeDetail
          label={detailMessages.responseTimeLabel}
          timestamp={responseTime}
          visible={Boolean(attempt.response_time)}
        />
      </div>
    </CurrentSelectionHistoryCard>
  );
}

function WorkstationScriptAttemptCard({
  locale,
  messages,
  request,
}: {
  locale?: string;
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  request: DashboardWorkstationRequest;
}) {
  const detailMessages = getCurrentSelectionDetailMessages(locale);
  const enumMessages = getCurrentSelectionOperationalEnumMessages(locale);
  const scriptRequestID = getScriptRequestID(request);
  const scriptIdentifier = scriptRequestID ?? request.dispatch_id;
  const scriptResponse = request.script_response;
  const outcome = enumMessages.localizeOutcome(
    scriptResponse?.outcome ?? request.outcome ?? detailMessages.pendingOutcome,
  );
  const provider = request.provider ?? request.provider_session?.provider;
  const providerSummary = provider
    ? messages.providerSummary(provider, request.model)
    : request.model;
  const requestTime = getLocalDateTimeDisplay(
    request.started_at ?? request.request_view?.started_at,
    detailMessages.timestampUnavailable,
    locale,
  );
  const responseTime = getLocalDateTimeDisplay(
    request.response_view?.end_time,
    detailMessages.timestampUnavailable,
    locale,
  );
  const durationMillis =
    scriptResponse?.duration_millis ?? scriptResponse?.durationMillis;

  return (
    <CurrentSelectionHistoryCard className="p-3">
      <CurrentSelectionHistoryCardHeader
        identifier={scriptIdentifier}
        subtitle={outcome}
        title={detailMessages.scriptAttemptLabel}
      />
      <div className="grid gap-1">
        <WorkstationRequestAttemptDetail
          label={
            scriptRequestID
              ? detailMessages.scriptRequestIdLabel
              : detailMessages.dispatchIdLabel
          }
          value={scriptIdentifier}
        />
        <WorkstationRequestAttemptDetail
          label={detailMessages.commandLabel}
          value={request.script_request?.command}
        />
        <WorkstationRequestAttemptDetail
          label={detailMessages.providerLabel}
          value={providerSummary}
        />
        <WorkstationRequestAttemptTimeDetail
          label={detailMessages.requestTimeLabel}
          timestamp={requestTime}
        />
        <WorkstationRequestAttemptDetail
          label={detailMessages.elapsedTimeLabel}
          value={
            durationMillis !== undefined
              ? formatDurationMillis(durationMillis, locale)
              : undefined
          }
        />
        <WorkstationRequestAttemptTimeDetail
          label={detailMessages.responseTimeLabel}
          timestamp={responseTime}
          visible={Boolean(request.response_view?.end_time)}
        />
      </div>
    </CurrentSelectionHistoryCard>
  );
}

function WorkstationRequestAttemptDetail({
  label,
  value,
}: {
  label: string;
  value?: string;
}) {
  if (!value) {
    return null;
  }

  return (
    <CurrentSelectionSupportingText>
      {label}: <Code>{value}</Code>
    </CurrentSelectionSupportingText>
  );
}

function WorkstationRequestAttemptTimeDetail({
  label,
  timestamp,
  visible = true,
}: {
  label: string;
  timestamp: ReturnType<typeof getLocalDateTimeDisplay>;
  visible?: boolean;
}) {
  if (!visible) {
    return null;
  }

  return (
    <CurrentSelectionSupportingText>
      {label}:{" "}
      {timestamp.rawTimestamp ? (
        <time dateTime={timestamp.rawTimestamp}>{timestamp.label}</time>
      ) : (
        timestamp.label
      )}
    </CurrentSelectionSupportingText>
  );
}

function getScriptRequestID(
  request: DashboardWorkstationRequest,
): string | undefined {
  return (
    request.script_request?.script_request_id ??
    request.script_request?.scriptRequestId ??
    request.script_response?.script_request_id ??
    request.script_response?.scriptRequestId
  );
}

function renderWorkstationRequestActions({
  messages,
  onSelectWorkstationRequest,
  request,
  requestSelected,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  onSelectWorkstationRequest?: WorkstationRequestHistorySectionProps["onSelectWorkstationRequest"];
  request: DashboardWorkstationRequest;
  requestSelected: boolean;
}) {
  if (!onSelectWorkstationRequest) {
    return undefined;
  }

  return (
    <DashboardActionButton
      aria-label={messages.selectWorkstationRequestLabel(request.dispatch_id)}
      aria-pressed={requestSelected}
      onClick={() => onSelectWorkstationRequest(request)}
      type="button"
    >
      {requestSelected
        ? messages.requestSelectedAction
        : messages.openRequestDetailsAction}
    </DashboardActionButton>
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
      <CurrentSelectionExecutionPill
        tone={hasFailedOutcome ? "danger" : "success"}
      >
        {messages.totalRuntimeLabel}:{" "}
        {formatDurationMillis(totalDurationMillis)}
      </CurrentSelectionExecutionPill>
    );
  }

  if (!request.started_at) {
    return undefined;
  }

  return (
    <CurrentSelectionExecutionPill>
      {messages.elapsedLabel}: {formatDurationFromISO(request.started_at, now)}
    </CurrentSelectionExecutionPill>
  );
}
