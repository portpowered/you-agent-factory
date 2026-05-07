import { DETAIL_COPY_CLASS } from "../../components/dashboard/widget-board";
import { DASHBOARD_SECTION_HEADING_CLASS } from "../../components/ui/dashboard-typography";
import {
  formatDurationFromISO,
  formatDurationMillis,
} from "../../components/ui/formatters";
import { useCurrentSelectionShellMessages } from "./current-selection-locale";
import {
  INFERENCE_ATTEMPT_DETAIL_CLASS,
  InferenceAttemptDetail,
  RUNTIME_DETAIL_CODE_CLASS,
  RUNTIME_DETAIL_VALUE_CLASS,
  RUNTIME_DETAILS_SECTION_CLASS,
  TRACE_ACTION_LINK_CLASS,
} from "./detail-card-shared";
import type {
  ExecutionDetailsSectionProps,
  InferenceAttemptsSectionProps,
} from "./detail-card-types";
import { InferenceAttemptCard } from "./inference-attempt";

export function ExecutionDetailsSection({
  activeTraceID,
  details,
  now,
  onSelectTraceID,
  showInferenceAttempts = true,
  traceTargetId,
}: ExecutionDetailsSectionProps) {
  const messages = useCurrentSelectionShellMessages();
  const hasTraceIDs = details.traceIDs.length > 0;

  return (
    <section
      aria-label={messages.executionDetailsRegionLabel}
      className={RUNTIME_DETAILS_SECTION_CLASS}
    >
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>
        {messages.executionDetailsHeading}
      </h4>
      <dl>
        <div>
          <dt>{messages.dispatchIdLabel}</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {details.dispatchID ? (
              <code className={RUNTIME_DETAIL_CODE_CLASS}>
                {details.dispatchID}
              </code>
            ) : (
              messages.dispatchIdUnavailable
            )}
          </dd>
        </div>
        <div>
          <dt>{messages.workstationLabel}</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {details.workstationName || messages.workstationUnavailable}
          </dd>
        </div>
        <div>
          <dt>{messages.elapsedLabel}</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {details.elapsedStartTimestamp
              ? formatDurationFromISO(details.elapsedStartTimestamp, now)
              : messages.elapsedUnavailable}
          </dd>
        </div>
        <div>
          <dt>{messages.traceIdsLabel}</dt>
          <dd className="grid gap-[0.35rem]">
            {hasTraceIDs ? (
              details.traceIDs.map((traceID) => (
                <a
                  className={TRACE_ACTION_LINK_CLASS}
                  href={`#${traceTargetId}`}
                  key={traceID}
                  onClick={() => onSelectTraceID?.(traceID)}
                >
                  {traceID}
                  {activeTraceID === traceID
                    ? messages.selectedTraceSuffix
                    : ""}
                </a>
              ))
            ) : (
              <span className={RUNTIME_DETAIL_VALUE_CLASS}>
                {messages.traceUnavailable}
              </span>
            )}
          </dd>
        </div>
      </dl>
      {hasTraceIDs ? (
        <div className="grid gap-[0.55rem]">
          <p className={DETAIL_COPY_CLASS}>{messages.traceGuidance}</p>
          <a
            className={TRACE_ACTION_LINK_CLASS}
            href={`#${traceTargetId}`}
            onClick={() =>
              onSelectTraceID?.(activeTraceID ?? details.traceIDs[0] ?? "")
            }
          >
            {messages.openTraceAction}
          </a>
        </div>
      ) : (
        <p className={DETAIL_COPY_CLASS}>{messages.traceUnavailable}</p>
      )}
      <WorkstationRequestProjectionSection details={details} />
      {showInferenceAttempts ? (
        <InferenceAttemptsSection attempts={details.inferenceAttempts} />
      ) : null}
    </section>
  );
}

export function InferenceAttemptsSection({
  attempts,
}: InferenceAttemptsSectionProps) {
  const messages = useCurrentSelectionShellMessages();

  return (
    <section
      aria-label={messages.inferenceAttemptsRegionLabel}
      className="mt-4 grid gap-[0.65rem] [&_h4]:m-0"
    >
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>
        {messages.inferenceAttemptsHeading}
      </h4>
      {attempts.length > 0 ? (
        <div className="grid gap-[0.8rem]">
          {attempts.map((attempt) => (
            <InferenceAttemptCard
              attempt={attempt}
              key={attempt.inference_request_id}
            />
          ))}
        </div>
      ) : (
        <p className={DETAIL_COPY_CLASS}>
          {messages.inferenceAttemptsEmptyState}
        </p>
      )}
    </section>
  );
}

function WorkstationRequestProjectionSection({
  details,
}: Pick<ExecutionDetailsSectionProps, "details">) {
  const messages = useCurrentSelectionShellMessages();
  const requestProjection = details.workstationRequest;
  if (!requestProjection) {
    return null;
  }

  const { counts, request, response } = requestProjection;

  return (
    <section
      aria-label={messages.workstationRequestRegionLabel}
      className="mt-4 grid gap-[0.65rem] [&_h4]:m-0"
    >
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>
        {messages.workstationRequestHeading}
      </h4>
      <dl className={INFERENCE_ATTEMPT_DETAIL_CLASS}>
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
        <InferenceAttemptDetail
          code
          label="startedAt"
          value={request.startedAt ?? request.started_at}
        />
        <InferenceAttemptDetail
          code
          label="outcome"
          value={response?.outcome}
        />
        <InferenceAttemptDetail
          label="duration"
          value={
            (response?.durationMillis ?? response?.duration_millis) !==
            undefined
              ? formatDurationMillis(
                  response?.durationMillis ?? response?.duration_millis ?? 0,
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
      </dl>
      <p className={DETAIL_COPY_CLASS}>{messages.workstationRequestGuidance}</p>
    </section>
  );
}
