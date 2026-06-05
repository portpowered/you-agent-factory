import { Button, DashboardHeading } from "../../../../components/ui";
import {
  formatDurationFromISO,
  formatDurationMillis,
  formatLocalDateTime,
} from "../../../../components/ui/formatters";
import { DetailCopy } from "../../../../components/ui/widget-frame";
import {
  useCurrentSelectionLocale,
  useCurrentSelectionOperationalEnumMessages,
  useCurrentSelectionShellMessages,
} from "../../base/components/current-selection-locale";
import {
  CurrentSelectionDescriptionList,
  CurrentSelectionDetailItem,
  CurrentSelectionDetailSection,
} from "../../base/public";
import type {
  ExecutionDetailsSectionProps,
  InferenceAttemptsSectionProps,
} from "../lib/detail-card-types";
import { InferenceAttemptCard } from "./inference-attempt";
import { InferenceAttemptDetail } from "./inference-attempt-detail";

export function ExecutionDetailsSection({
  activeTraceID,
  details,
  now,
  onSelectTraceID,
  showInferenceAttempts = true,
  traceTargetId,
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
                <Button
                  asChild
                  className="w-fit rounded-lg"
                  key={traceID}
                  size="sm"
                  tone="outline"
                >
                  <a
                    href={`#${traceTargetId}`}
                    onClick={() => onSelectTraceID?.(traceID)}
                  >
                    {traceID}
                    {activeTraceID === traceID
                      ? messages.selectedTraceSuffix
                      : ""}
                  </a>
                </Button>
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
          <Button asChild className="w-fit rounded-lg" size="sm" tone="outline">
            <a
              href={`#${traceTargetId}`}
              onClick={() =>
                onSelectTraceID?.(activeTraceID ?? details.traceIDs[0] ?? "")
              }
            >
              {messages.openTraceAction}
            </a>
          </Button>
        </div>
      ) : (
        <DetailCopy>{messages.traceUnavailable}</DetailCopy>
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
}: InferenceAttemptsSectionProps) {
  const messages = useCurrentSelectionShellMessages();

  return (
    <section
      aria-label={messages.inferenceAttemptsRegionLabel}
      className="mt-4 grid gap-2.5 [&_h4]:m-0"
    >
      <DashboardHeading as="h4" className="m-0">
        {messages.inferenceAttemptsHeading}
      </DashboardHeading>
      {attempts.length > 0 ? (
        <div className="grid gap-3">
          {attempts.map((attempt) => (
            <InferenceAttemptCard
              attempt={attempt}
              key={attempt.inference_request_id}
            />
          ))}
        </div>
      ) : (
        <DetailCopy>{messages.inferenceAttemptsEmptyState}</DetailCopy>
      )}
    </section>
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
    <section
      aria-label={messages.workstationRequestRegionLabel}
      className="mt-4 grid gap-2.5 [&_h4]:m-0"
    >
      <DashboardHeading as="h4" className="m-0">
        {messages.workstationRequestHeading}
      </DashboardHeading>
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
          value={response?.failureReason ?? response?.failure_reason}
        />
        <InferenceAttemptDetail
          code
          label="failureMessage"
          value={response?.failureMessage ?? response?.failure_message}
        />
      </CurrentSelectionDescriptionList>
    </section>
  );
}
