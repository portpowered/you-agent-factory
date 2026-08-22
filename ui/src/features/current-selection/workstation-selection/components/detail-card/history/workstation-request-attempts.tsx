import { Code } from "@you-agent-factory/components/primitives";
import type {
  DashboardInferenceAttempt,
  DashboardWorkstationRequest,
} from "../../../../../../api/dashboard/types";
import {
  formatDurationMillis,
  getLocalDateTimeDisplay,
} from "../../../../../../components/ui/formatters";
import { CurrentSelectionSupportingText } from "../../../../base/components/presentation/current-selection-supporting-text";
import { getCurrentSelectionOperationalEnumMessages } from "../../../../base/messages/operational/current-selection-operational-enums";
import { getCurrentSelectionDetailMessages } from "../../../../base/messages/shell/current-selection-detail";
import {
  CurrentSelectionHistoryCard,
  CurrentSelectionHistoryCardHeader,
} from "../../../../history/components/current-selection-history-card";
import type { getWorkstationDetailMessages } from "../../../messages/workstation-detail";

export function WorkstationRequestAttempts({
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
