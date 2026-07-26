import { WidgetDetailCopy } from "@you-agent-factory/components/recipes";
import {
  formatDurationFromISO,
  formatDurationMillis,
  formatLocalDateTime,
} from "../../../../../components/ui/formatters";
import {
  useCurrentSelectionLocale,
  useCurrentSelectionOperationalEnumMessages,
  useCurrentSelectionShellMessages,
} from "../../../base/components/presentation/current-selection-locale";
import { CurrentSelectionDescriptionList } from "../../../base/components/detail/current-selection-description-list";
import { CurrentSelectionDetailItem } from "../../../base/components/detail/current-selection-detail-item";
import { CurrentSelectionDetailSection } from "../../../base/components/detail/current-selection-detail-section";
import { CurrentSelectionContentSection } from "../../../base/components/layout/current-selection-content-section";
import { CurrentSelectionTraceButton } from "../../../base/components/presentation/current-selection-trace-button";
import type {
  ExecutionDetailsSectionProps,
  InferenceAttemptsSectionProps,
} from "../../lib/detail-card-types";
import { InferenceAttemptCard } from "../inference-attempt/inference-attempt";
import { InferenceAttemptDetail } from "../inference-attempt/inference-attempt-detail";

export function ExecutionDetailsSection({
  activeTraceID,
  details,
  now,
  onSelectTraceID,
  showInferenceAttempts = true,
}: ExecutionDetailsSectionProps) {
  const messages = useCurrentSelectionShellMessages();
  const locale = useCurrentSelectionLocale();
  const hasTraceIDs = details.traceIDs.length > 0;

  return (
    <CurrentSelectionDetailSection
      ariaLabel={messages.executionDetailsRegionLabel}
      title={messages.executionDetailsHeading}
    >
      <CurrentSelectionDescriptionList>
        <CurrentSelectionDetailItem
          code={Boolean(details.dispatchID)}
          label={messages.dispatchIdLabel}
          value={details.dispatchID || messages.dispatchIdUnavailable}
        />
        <CurrentSelectionDetailItem
          label={messages.workstationLabel}
          value={details.workstationName || messages.workstationUnavailable}
        />
        <CurrentSelectionDetailItem
          label={messages.elapsedLabel}
          value={
            details.elapsedStartTimestamp
              ? formatDurationFromISO(
                  details.elapsedStartTimestamp,
                  now,
                  locale,
                  messages.elapsedUnavailable,
                )
              : messages.elapsedUnavailable
          }
        />
        <div>
          <dt>{messages.traceIdsLabel}</dt>
          <dd className="grid gap-1.5">
            {hasTraceIDs ? (
              details.traceIDs.map((traceID) => (
                <CurrentSelectionTraceButton
                  activeTraceID={activeTraceID}
                  key={traceID}
                  onSelectTraceID={onSelectTraceID}
                  selectedTraceSuffix={messages.selectedTraceSuffix}
                  traceID={traceID}
                />
              ))
            ) : (
              <span className="min-w-0 [overflow-wrap:anywhere]">
                {messages.traceUnavailable}
              </span>
            )}
          </dd>
        </div>
      </CurrentSelectionDescriptionList>
      {hasTraceIDs ? (
        <div className="grid gap-2">
          <CurrentSelectionTraceButton
            activeTraceID={activeTraceID}
            onSelectTraceID={onSelectTraceID}
            traceID={activeTraceID ?? details.traceIDs[0] ?? ""}
          >
            {messages.openTraceAction}
          </CurrentSelectionTraceButton>
        </div>
      ) : (
        <WidgetDetailCopy>{messages.traceUnavailable}</WidgetDetailCopy>
      )}
      <WorkstationRequestProjectionSection details={details} />
      {showInferenceAttempts ? (
        <InferenceAttemptsSection attempts={details.inferenceAttempts} />
      ) : null}
    </CurrentSelectionDetailSection>
  );
}

export function InferenceAttemptsSection({
  attempts,
  showHeading = true,
}: InferenceAttemptsSectionProps) {
  const messages = useCurrentSelectionShellMessages();
  const body =
    attempts.length > 0 ? (
      <div className="grid gap-3">
        {attempts.map((attempt) => (
          <InferenceAttemptCard
            attempt={attempt}
            key={attempt.inference_request_id}
          />
        ))}
      </div>
    ) : (
      <WidgetDetailCopy>
        {messages.inferenceAttemptsEmptyState}
      </WidgetDetailCopy>
    );

  if (!showHeading) {
    return body;
  }

  return (
    <CurrentSelectionContentSection
      aria-label={messages.inferenceAttemptsRegionLabel}
      title={messages.inferenceAttemptsHeading}
    >
      {body}
    </CurrentSelectionContentSection>
  );
}

function WorkstationRequestProjectionSection({
  details,
}: Pick<ExecutionDetailsSectionProps, "details">) {
  const messages = useCurrentSelectionShellMessages();
  const enumMessages = useCurrentSelectionOperationalEnumMessages();
  const locale = useCurrentSelectionLocale();
  const requestProjection = details.workstationRequest;
  if (!requestProjection) {
    return null;
  }

  const { counts, request, response } = requestProjection;
  const startedAt = formatLocalDateTime(
    request.startedAt ?? request.started_at,
    messages.elapsedUnavailable,
    locale,
  );

  return (
    <CurrentSelectionContentSection
      aria-label={messages.workstationRequestRegionLabel}
      title={messages.workstationRequestHeading}
    >
      <CurrentSelectionDescriptionList>
        <InferenceAttemptDetail
          label="dispatchedCount"
          value={counts.dispatchedCount ?? counts.dispatched_count}
        />
        <InferenceAttemptDetail
          label="respondedCount"
          value={counts.respondedCount ?? counts.responded_count}
        />
        <InferenceAttemptDetail
          label="erroredCount"
          value={counts.erroredCount ?? counts.errored_count}
        />
        <InferenceAttemptDetail label="startedAt" value={startedAt} />
        <InferenceAttemptDetail
          code
          label="outcome"
          value={
            response?.outcome
              ? enumMessages.localizeOutcome(response.outcome)
              : undefined
          }
        />
        <InferenceAttemptDetail
          label="duration"
          value={
            (response?.durationMillis ?? response?.duration_millis) !==
            undefined
              ? formatDurationMillis(
                  response?.durationMillis ?? response?.duration_millis ?? 0,
                  locale,
                )
              : undefined
          }
        />
        <InferenceAttemptDetail
          code
          label="failureReason"
          value={response?.failureDetail?.reason}
        />
        <InferenceAttemptDetail
          code
          label="failureMessage"
          value={response?.failureDetail?.message}
        />
      </CurrentSelectionDescriptionList>
    </CurrentSelectionContentSection>
  );
}
